package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/repair"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/spf13/cobra"
)

func newRepairCommand(opts *globalOptions) *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "repair <spec> [run|latest]",
		Short: "Propose step repairs for a failed run (never touches the contract)",
		Long: "Analyze a failed run and propose fixes to a spec's `steps` — for " +
			"example, a `wait` that timed out because the on-screen text changed. " +
			"Only `steps` are touched; `intent` and `outcomes` are the contract and " +
			"are left alone, so applying a repair keeps the contract hash valid. " +
			"By default repairs are printed; pass --write to apply them.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			specPath := args[0]
			rt, err := config.LoadRuntime(specPath, opts.configPath, opts.environment)
			if err != nil {
				return exitError{code: 4, err: err}
			}
			parseOpts := rt.SpecParseOptions()
			parseOpts.AllowHashMismatch = true
			parsed, err := spec.ParseFile(specPath, parseOpts)
			if err != nil {
				return exitError{code: 4, err: err}
			}

			root := opts.artifactRoot
			if root == "" {
				root = rt.Config.ArtifactRoot
			}
			if !filepath.IsAbs(root) {
				root = filepath.Join(rt.ProjectRoot, root)
			}
			var runDir string
			if len(args) == 2 && args[1] != "latest" {
				runDir, err = resolveRunDir(root, args[1])
			} else {
				runDir, err = repair.LatestRunDirForSpec(root, parsed.Resolved.Name)
			}
			if err != nil {
				return exitError{code: 2, err: fmt.Errorf("locate run: %w", err)}
			}

			proposals := repair.Analyze(runDir, parsed.Resolved.Steps)
			if write {
				for i := range proposals {
					if proposals[i].Proposed == "" || proposals[i].Current == "" {
						continue
					}
					if err := repair.Apply(parsed.Path, proposals[i]); err != nil {
						return exitError{code: 2, err: fmt.Errorf("apply repair to step %d: %w", proposals[i].StepIndex, err)}
					}
					proposals[i].Applied = true
				}
			}

			value := map[string]any{
				"schemaVersion": 1,
				"spec":          parsed.Resolved.Name,
				"run":           filepath.Base(runDir),
				"proposals":     proposals,
				"applied":       write,
			}
			output, err := emitForCLI(cmd, opts, format, value, func() string {
				return renderRepairMarkdown(parsed.Resolved.Name, filepath.Base(runDir), proposals, write)
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "apply the proposed step repairs to the spec in place")
	return cmd
}

func renderRepairMarkdown(specName, run string, proposals []repair.Proposal, applied bool) string {
	var b strings.Builder
	b.WriteString("# Glyphrun Repair\n\n")
	fmt.Fprintf(&b, "- spec: `%s`\n", specName)
	fmt.Fprintf(&b, "- run: `%s`\n\n", run)
	if len(proposals) == 0 {
		b.WriteString("No step repairs proposed. If the run failed, inspect `glyph context latest --format md`.\n")
		return b.String()
	}
	for _, p := range proposals {
		fmt.Fprintf(&b, "## step %d (%s)\n\n", p.StepIndex, p.Kind)
		if p.Current != "" {
			fmt.Fprintf(&b, "- current: %q\n", p.Current)
		}
		if p.Proposed != "" {
			fmt.Fprintf(&b, "- proposed: %q\n", p.Proposed)
		}
		fmt.Fprintf(&b, "- rationale: %s\n", p.Rationale)
		if p.Proposed != "" && applied {
			if p.Applied {
				b.WriteString("- applied: yes\n")
			} else {
				b.WriteString("- applied: no\n")
			}
		}
		b.WriteByte('\n')
	}
	if !applied {
		b.WriteString("Re-run with `--write` to apply these step edits (the contract hash is unaffected).\n")
	}
	return b.String()
}
