package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/render"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
	"github.com/spf13/cobra"
)

func newRenderCommand(opts *globalOptions) *cobra.Command {
	var screen string
	var out string
	cmd := &cobra.Command{
		Use:   "render <run|latest>",
		Short: "Render a run's terminal screen to a deterministic SVG",
		Long: "Render the final screen (default) or a named snapshot of a run to a " +
			"deterministic SVG. The output is a pure function of the captured cell " +
			"grid, so it is reproducible and safe to regenerate in CI. Use --out - " +
			"to write the raw SVG to stdout.",
		Args: cobra.ExactArgs(1),
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

			// Resolve which captured screen JSON to render. "final" reads the
			// run's final screen; any other value is treated as a snapshot
			// name captured during the run.
			var srcPath, defaultOut string
			if screen == "" || screen == "final" {
				srcPath = filepath.Join(runDir, "screens", "final.json")
				defaultOut = filepath.Join(runDir, "screens", "final.svg")
			} else {
				safe := artifacts.SafeName(screen)
				srcPath = filepath.Join(runDir, "snapshots", safe+".json")
				defaultOut = filepath.Join(runDir, "snapshots", safe+".svg")
			}
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return exitError{code: 2, err: fmt.Errorf("read screen %q: %w", screen, err)}
			}
			var snapshot terminal.ScreenSnapshot
			if err := json.Unmarshal(data, &snapshot); err != nil {
				return exitError{code: 2, err: fmt.Errorf("parse screen %q: %w", srcPath, err)}
			}

			svg := render.SnapshotSVG(snapshot, render.DefaultOptions())

			// --out - streams the raw SVG to stdout, bypassing the report so
			// it can be piped into a file or a converter.
			if out == "-" {
				_, _ = cmd.OutOrStdout().Write([]byte(svg))
				return nil
			}
			outPath := out
			if outPath == "" {
				outPath = defaultOut
			}
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				return exitError{code: 2, err: err}
			}
			if err := os.WriteFile(outPath, []byte(svg), 0o644); err != nil {
				return exitError{code: 2, err: err}
			}

			value := map[string]any{
				"schemaVersion": 1,
				"run":           filepath.Base(runDir),
				"screen":        screen,
				"path":          outPath,
				"bytes":         len(svg),
				"cols":          snapshot.Cols,
				"rows":          snapshot.Rows,
			}
			output, err := emitForCLI(cmd, opts, format, value, func() string {
				return fmt.Sprintf("# Glyphrun Render\n\n- screen: `%s`\n- size: %dx%d\n- svg: `%s` (%d bytes)\n",
					screen, snapshot.Cols, snapshot.Rows, outPath, len(svg))
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
	cmd.Flags().StringVar(&screen, "screen", "final", "screen to render: final or a snapshot name")
	cmd.Flags().StringVar(&out, "out", "", "output SVG path; '-' writes raw SVG to stdout (default: alongside the screen in the run dir)")
	return cmd
}
