package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/ptyrunner"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal/adapters/gote"
	"github.com/spf13/cobra"
)

func newRecordCommand(opts *globalOptions) *cobra.Command {
	var timeoutMS int
	var cwd string
	cmd := &cobra.Command{
		Use:   "record -- <command...>",
		Short: "Record a terminal command into a Glyphrun artifact pack",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			result, err := recordCommand(context.Background(), opts, args, cwd, timeoutMS, format == formatMD)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			output, err := emit(format, result, func() string { return artifacts.RenderRunMarkdown(result) })
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			if result.ExitCode != 0 {
				return exitError{code: result.ExitCode}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&timeoutMS, "timeout-ms", 0, "stop recording after this timeout")
	cmd.Flags().StringVar(&cwd, "cwd", ".", "working directory for the recorded command")
	return cmd
}

func recordCommand(ctx context.Context, opts *globalOptions, argv []string, cwd string, timeoutMS int, echoOutput bool) (artifacts.RunResult, error) {
	rt, err := config.LoadRuntime(".", opts.configPath, opts.environment)
	if err != nil {
		return artifacts.RunResult{}, err
	}
	started := time.Now().UTC()
	runID := "record-" + started.Format("2006-01-02T15-04-05Z") + "-" + strconv.FormatInt(started.UnixNano()%1e9, 36)
	artifactRoot := opts.artifactRoot
	if artifactRoot == "" {
		artifactRoot = rt.Config.ArtifactRoot
	}
	if !filepath.IsAbs(artifactRoot) {
		artifactRoot = filepath.Join(rt.ProjectRoot, artifactRoot)
	}
	writer := artifacts.NewWriter(filepath.Join(artifactRoot, runID), artifacts.NewRedactor(rt.Config.Redaction))
	if err := writer.EnsureDirs(); err != nil {
		return artifacts.RunResult{}, err
	}
	_ = writer.AppendEvent(artifacts.Event{TS: started.Format(time.RFC3339Nano), Type: "record.started", Name: strings.Join(argv, " ")})
	emulator := gote.New(rt.Config.Terminal.Cols, rt.Config.Terminal.Rows)
	var mu sync.Mutex
	var raw []byte
	var frames []terminal.Frame
	session, err := ptyrunner.Start(ptyrunner.Options{
		Cmd:  argv,
		Cwd:  cwd,
		Env:  rt.Env,
		Cols: rt.Config.Terminal.Cols,
		Rows: rt.Config.Terminal.Rows,
		OnOutput: func(data []byte) {
			if echoOutput {
				_, _ = os.Stdout.Write(data)
			}
			mu.Lock()
			raw = append(raw, data...)
			mu.Unlock()
			newFrames, _ := emulator.Feed(data)
			mu.Lock()
			frames = append(frames, newFrames...)
			mu.Unlock()
		},
	})
	if err != nil {
		return artifacts.RunResult{}, err
	}
	go func() {
		_, _ = io.Copy(sessionWriter{session: session}, os.Stdin)
	}()
	var exit ptyrunner.ExitState
	if timeoutMS > 0 {
		select {
		case exit = <-session.WaitCh():
		case <-time.After(time.Duration(timeoutMS) * time.Millisecond):
			exit = session.Cleanup(runnerCleanupTimeout())
		case <-ctx.Done():
			exit = session.Cleanup(runnerCleanupTimeout())
		}
	} else {
		select {
		case exit = <-session.WaitCh():
		case <-ctx.Done():
			exit = session.Cleanup(runnerCleanupTimeout())
		}
	}
	ended := time.Now().UTC()
	status := artifacts.StatusPassed
	if exit.ExitCode != 0 {
		status = artifacts.StatusFailed
	}
	result := artifacts.RunResult{
		SchemaVersion: 1,
		RunID:         runID,
		SpecName:      "record",
		Status:        status,
		StartedAt:     started.Format(time.RFC3339Nano),
		EndedAt:       ended.Format(time.RFC3339Nano),
		DurationMS:    ended.Sub(started).Milliseconds(),
		Target:        spec.Target{Cmd: argv, Cwd: cwd},
		Terminal:      spec.Terminal{Cols: rt.Config.Terminal.Cols, Rows: rt.Config.Terminal.Rows, Profile: rt.Config.Terminal.Profile},
		Outcomes:      []artifacts.OutcomeResult{},
		RunDir:        writer.RunDir,
		ExitCode:      exit.ExitCode,
		Artifacts: map[string]string{
			"finalScreenText": "screens/final.txt",
			"finalScreenJSON": "screens/final.json",
			"frames":          "frames/frames.ndjson",
			"rawPtyLog":       "raw/pty.raw.log",
			"events":          "events.ndjson",
		},
	}
	mu.Lock()
	rawCopy := append([]byte(nil), raw...)
	framesCopy := append([]terminal.Frame(nil), frames...)
	mu.Unlock()
	_ = writer.WriteRawPTY(rawCopy)
	_ = writer.WriteFrames(framesCopy)
	_ = writer.WriteFinalScreen(emulator.Screen().Snapshot())
	_ = writer.WriteDiagnostic("record", "## Recorded Command\n\n`"+strings.Join(argv, " ")+"`\n")
	_ = writer.WriteRun(result)
	_ = writer.AppendEvent(artifacts.Event{TS: ended.Format(time.RFC3339Nano), Type: "record.finished", Name: strings.Join(argv, " "), Info: strconv.Itoa(exit.ExitCode)})
	return result, nil
}

type sessionWriter struct {
	session *ptyrunner.Session
}

func (w sessionWriter) Write(data []byte) (int, error) {
	if err := w.session.Write(data); err != nil {
		return 0, err
	}
	return len(data), nil
}

func runnerCleanupTimeout() time.Duration {
	return 2 * time.Second
}
