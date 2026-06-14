package cli

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
	"github.com/abdul-hamid-achik/glyphrun/internal/tui"
	"github.com/spf13/cobra"
)

func newReplayCommand(opts *globalOptions) *cobra.Command {
	var useTUI bool
	cmd := &cobra.Command{
		Use:   "replay <run>",
		Short: "Replay a run's raw PTY log (or --tui to scrub frames interactively)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			rt, err := config.LoadRuntime(".", opts.configPath, opts.environment)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			root := opts.artifactRoot
			if root == "" {
				root = rt.Config.ArtifactRoot
			}
			if !filepath.IsAbs(root) {
				root = filepath.Join(rt.ProjectRoot, root)
			}
			runDir, err := resolveRunDir(root, args[0])
			if err != nil {
				return exitError{code: 2, err: err}
			}
			result, err := artifacts.LoadRunResult(runDir)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			// --tui launches the interactive frame scrubber. It needs a real
			// terminal and ignores --format (it is not machine output).
			if useTUI {
				if !isTerminalWriter(cmd.OutOrStdout()) {
					return exitError{code: 2, err: errNotATTY}
				}
				frames, err := loadFrames(filepath.Join(runDir, "frames/frames.ndjson"))
				if err != nil {
					return exitError{code: 2, err: err}
				}
				if err := tui.Run(frames, tui.Meta{RunID: result.RunID, Spec: result.SpecName}); err != nil {
					return exitError{code: 2, err: err}
				}
				return nil
			}
			rawPath := filepath.Join(runDir, "raw/pty.raw.log")
			raw, err := os.ReadFile(rawPath)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			if format == formatMD {
				_, _ = cmd.OutOrStdout().Write(raw)
				return nil
			}
			value := map[string]any{"schemaVersion": 1, "run": result.RunID, "rawPtyLog": rawPath, "bytes": len(raw)}
			output, err := emitForCLI(cmd, opts, format, value, func() string { return string(raw) })
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
	cmd.Flags().BoolVar(&useTUI, "tui", false, "scrub the recorded frames interactively (requires a terminal)")
	return cmd
}

// errNotATTY is returned when an interactive command is run without a terminal.
var errNotATTY = errTUINeedsTerminal{}

type errTUINeedsTerminal struct{}

func (errTUINeedsTerminal) Error() string {
	return "--tui requires an interactive terminal (stdout is not a TTY)"
}

// loadFrames reads a run's frames/frames.ndjson into terminal frames for the
// scrubber. Unparseable lines are skipped so a truncated log still replays.
func loadFrames(path string) ([]terminal.Frame, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var frames []terminal.Frame
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var frame terminal.Frame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			continue
		}
		frames = append(frames, frame)
	}
	return frames, scanner.Err()
}
