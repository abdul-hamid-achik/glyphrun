package cli

import (
	"context"
	"fmt"
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
	"github.com/abdul-hamid-achik/glyphrun/internal/render"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal/adapters/gote"
	"github.com/spf13/cobra"
)

func newRecordCommand(opts *globalOptions) *cobra.Command {
	var timeoutMS int
	var cwd string
	var scaffoldPath string
	cmd := &cobra.Command{
		Use:   "record -- <command...>",
		Short: "Record a terminal command into a Glyphrun artifact pack",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			result, scaffold, err := recordCommand(context.Background(), opts, args, cwd, timeoutMS, format == formatMD, scaffoldPath)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			var value any = result
			markdown := func() string { return artifacts.RenderRunMarkdown(result) }
			if scaffold != nil {
				// Surface the scaffold alongside the run result so both the
				// JSON and Markdown reports mention the generated spec.
				value = map[string]any{"schemaVersion": 1, "run": result, "scaffold": scaffold}
				markdown = func() string {
					return artifacts.RenderRunMarkdown(result) + renderScaffoldMarkdown(scaffold)
				}
			}
			output, err := emitForCLI(cmd, opts, format, value, markdown)
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
	cmd.Flags().StringVar(&scaffoldPath, "scaffold", "", "write a draft spec inferred from the recorded session to this path")
	return cmd
}

func renderScaffoldMarkdown(s *scaffoldResult) string {
	var b strings.Builder
	b.WriteString("\n## Scaffolded Spec\n\n")
	fmt.Fprintf(&b, "- path: `%s`\n", s.Path)
	fmt.Fprintf(&b, "- name: `%s`\n", s.Name)
	if s.Ready != "" {
		fmt.Fprintf(&b, "- inferred ready string: %q\n", s.Ready)
	}
	fmt.Fprintf(&b, "- contract stamped: %v\n", s.Stamped)
	if s.NeedsEdit {
		b.WriteString("- NOTE: no assertion could be inferred; edit the `REPLACE_ME` outcome before running.\n")
	}
	b.WriteString("- next: add interaction steps (keystrokes), then `glyph spec verify --stamp` after editing intent/outcomes.\n")
	return b.String()
}

func recordCommand(ctx context.Context, opts *globalOptions, argv []string, cwd string, timeoutMS int, echoOutput bool, scaffoldPath string) (artifacts.RunResult, *scaffoldResult, error) {
	rt, err := config.LoadRuntime(".", opts.configPath, opts.environment)
	if err != nil {
		return artifacts.RunResult{}, nil, err
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
		return artifacts.RunResult{}, nil, err
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
		return artifacts.RunResult{}, nil, err
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
			"finalScreenSVG":  "screens/final.svg",
			"frames":          "frames/frames.ndjson",
			"rawPtyLog":       "raw/pty.raw.log",
			"events":          "events.ndjson",
		},
	}
	mu.Lock()
	rawCopy := append([]byte(nil), raw...)
	framesCopy := append([]terminal.Frame(nil), frames...)
	mu.Unlock()
	finalSnapshot := emulator.Screen().Snapshot()
	_ = writer.WriteRawPTY(rawCopy)
	_ = writer.WriteFrames(framesCopy)
	_ = writer.WriteFinalScreen(finalSnapshot)
	_ = writer.WriteScreenSVG("screens/final.svg", render.SnapshotSVG(finalSnapshot, render.DefaultOptions()))
	_ = writer.WriteDiagnostic("record", "## Recorded Command\n\n`"+strings.Join(argv, " ")+"`\n")
	_ = writer.WriteRun(result)
	_ = writer.AppendEvent(artifacts.Event{TS: ended.Format(time.RFC3339Nano), Type: "record.finished", Name: strings.Join(argv, " "), Info: strconv.Itoa(exit.ExitCode)})

	var scaffold *scaffoldResult
	if scaffoldPath != "" {
		scaffold, err = writeRecordScaffold(opts, scaffoldPath, argv, cwd, result.Terminal, finalSnapshot.Text, exit)
		if err != nil {
			return result, nil, fmt.Errorf("scaffold: %w", err)
		}
	}
	return result, scaffold, nil
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
