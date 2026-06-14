package cli

import (
	"fmt"
	"path/filepath"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/spf13/cobra"
)

func newCleanCommand(opts *globalOptions) *cobra.Command {
	var (
		keep int
		all  bool
	)
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Prune old run directories from the artifact root",
		Long: `Prune run directories under the artifact root.

By default, the runner keeps the newest N runs per the project
config (retention.keepRuns) and prunes older ones. Use --keep to
override the count for this invocation, or --all to wipe every
run directory under the artifact root.

This command is safe to run while a parallel ` + "`glyph run`" + ` is
executing; the only risk is that the wipe races a concurrent
write, in which case the next run's directory will be missing the
artifact the wipe targeted.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			// Resolve the artifact root. We don't have a spec to anchor
			// the config lookup, so we walk up from the cwd to find
			// glyphrun.config.yml (matching FindConfig's behavior).
			configPath := opts.configPath
			if configPath == "" {
				cp, _ := config.FindConfig(".")
				configPath = cp
			}
			rt, err := config.LoadRuntime(".", configPath, opts.environment)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			artifactRoot := opts.artifactRoot
			if artifactRoot == "" {
				artifactRoot = rt.Config.ArtifactRoot
			}
			if !filepath.IsAbs(artifactRoot) {
				artifactRoot = filepath.Join(rt.ProjectRoot, artifactRoot)
			}
			var report artifacts.CleanReport
			switch {
			case all:
				report, err = artifacts.CleanAll(artifactRoot)
			default:
				// An explicit --keep must win over config retention, even
				// when it is 0. Comparing against 0 alone would make an
				// explicit `--keep 0` silently fall back to config instead
				// of disabling pruning for this invocation (0 means
				// "no prune / keep all", matching retention.keepRuns).
				effectiveKeep := keep
				if !cmd.Flags().Changed("keep") {
					effectiveKeep = rt.Config.Retention.KeepRuns
				}
				if effectiveKeep < 0 {
					return exitError{code: 2, err: fmt.Errorf("--keep must be >= 0 (got %d)", effectiveKeep)}
				}
				report, err = artifacts.PruneRuns(artifactRoot, effectiveKeep)
			}
			if err != nil {
				return exitError{code: 2, err: fmt.Errorf("clean: %w", err)}
			}
			output, err := emitForCLI(cmd, opts, format, report, func() string { return renderCleanMarkdown(artifactRoot, report, all) })
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
	cmd.Flags().IntVar(&keep, "keep", 0, "keep the N newest runs (overrides config retention.keepRuns for this invocation; --keep 0 disables pruning — use --all to wipe every run)")
	cmd.Flags().BoolVar(&all, "all", false, "remove every run directory under the artifact root")
	return cmd
}

func renderCleanMarkdown(artifactRoot string, report artifacts.CleanReport, all bool) string {
	action := "pruned"
	if all {
		action = "wiped"
	}
	return fmt.Sprintf("# Glyphrun Clean\n\n- artifact root: `%s`\n- mode: %s\n- pruned: %d\n- kept: %d\n",
		artifactRoot, action, report.Pruned, report.Kept)
}
