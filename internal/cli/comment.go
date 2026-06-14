package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/ghreport"
	"github.com/spf13/cobra"
)

func newCommentCommand(opts *globalOptions) *cobra.Command {
	var out string
	var last int
	cmd := &cobra.Command{
		Use:   "comment [run|latest ...]",
		Short: "Render a PR-comment-ready Markdown summary of one or more runs",
		Long: "Produce GitHub-flavored Markdown summarizing run results — status, " +
			"outcome counts, failure focus, the final screen, and artifact pointers " +
			"(including the deterministic SVG screenshot). Writes to stdout by " +
			"default so it can be piped to `gh pr comment -F -`, or to a file with " +
			"--out. Use --last N to summarize the N most recent runs.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			explicit := false
			for _, arg := range args {
				if arg != "latest" {
					explicit = true
				}
			}
			var runDirs []string
			switch {
			case explicit:
				for _, arg := range args {
					dir, err := resolveRunDir(root, arg)
					if err != nil {
						return exitError{code: 2, err: err}
					}
					runDirs = append(runDirs, dir)
				}
			default:
				// No run given (or just "latest"): take the most recent runs by
				// modification time.
				if last < 1 {
					last = 1
				}
				runDirs, err = ghreport.RecentRunDirs(root, last)
				if err != nil {
					return exitError{code: 2, err: fmt.Errorf("list runs: %w", err)}
				}
			}

			results := make([]artifacts.RunResult, 0, len(runDirs))
			for _, dir := range runDirs {
				result, err := artifacts.LoadRunResult(dir)
				if err != nil {
					continue
				}
				results = append(results, result)
			}
			markdown := ghreport.Render(results)

			// Write to stdout (cmd.OutOrStdout) so the comment can be piped
			// directly into `gh pr comment -F -`.
			if out == "" {
				_, _ = cmd.OutOrStdout().Write([]byte(markdown))
				return nil
			}
			if dir := filepath.Dir(out); dir != "" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return exitError{code: 2, err: err}
				}
			}
			if err := os.WriteFile(out, []byte(markdown), 0o644); err != nil {
				return exitError{code: 2, err: err}
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "wrote PR comment to %s\n", out)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "write the comment Markdown to this file (default: stdout)")
	cmd.Flags().IntVar(&last, "last", 1, "summarize the N most recent runs when no run is given")
	return cmd
}
