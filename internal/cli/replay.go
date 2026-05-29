package cli

import (
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/spf13/cobra"
)

func newReplayCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "replay <run>",
		Short: "Replay a run's raw PTY log",
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
			output, err := emit(format, value, func() string { return string(raw) })
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
}
