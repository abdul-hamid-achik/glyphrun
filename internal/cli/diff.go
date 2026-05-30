package cli

import (
	"path/filepath"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/spf13/cobra"
)

func newDiffCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "diff <runA> <runB>",
		Short: "Compare two Glyphrun artifact packs",
		Args:  cobra.ExactArgs(2),
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
			runA, err := resolveRunDir(root, args[0])
			if err != nil {
				return exitError{code: 2, err: err}
			}
			runB, err := resolveRunDir(root, args[1])
			if err != nil {
				return exitError{code: 2, err: err}
			}
			diff, err := artifacts.DiffRunDirs(runA, runB)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			output, err := emitForCLI(cmd, opts, format, diff, func() string { return artifacts.RenderRunDiffMarkdown(diff) })
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			if diff.Changed {
				return exitError{code: 1}
			}
			return nil
		},
	}
}
