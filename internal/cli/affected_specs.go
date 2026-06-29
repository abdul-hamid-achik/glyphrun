package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/affected"
	"github.com/spf13/cobra"
)

// newAffectedSpecsCommand implements `glyph affected-specs`: given a git
// diff scope, run `codemap review --json` to get the changed symbols and
// their transitive blast radius, then select only the specs whose
// coversSymbol a change can hit. This closes the structure→behavior loop
// opened by the coversSymbol field: CI runs
//
//	glyph run $(glyph affected-specs --since HEAD^)
//
// instead of the whole suite. The diff→symbol work is delegated to codemap
// ("no diff parsing on the glyphrun side"); glyphrun only owns the
// spec↔symbol link (the coversSymbol field) and the intersection. The
// shared engine lives in internal/affected (also used by the
// glyph_affected_specs MCP tool); this command is a thin CLI wrapper.
// codemap is invoked as a subprocess; pass --codemap <path> when it is not
// on $PATH.
func newAffectedSpecsCommand(opts *globalOptions) *cobra.Command {
	var since string
	var staged bool
	var codemapBin string
	var depth int
	cmd := &cobra.Command{
		Use:   "affected-specs [path...]",
		Short: "Select specs whose coversSymbol a git change can hit (via codemap review)",
		Long: `Walk one or more spec paths (files or directories; defaults to "."),
parse every spec, and intersect each spec's coversSymbol against the
changed symbols + blast radius reported by ` + "`codemap review`" + ` for the
given diff scope. The matching specs are the minimal set a change can
hit — run only those instead of the whole suite:

  glyph run $(glyph affected-specs --since HEAD^)

One diff scope selects the review window (mirrors ` + "`codemap review`" + `):
  --since <ref>   everything changed since this git ref (committed + uncommitted)
  --staged        only staged changes (the git index)
  (none)          the whole working tree (default)

The default (md) output is one spec path per line so it drops straight into
command substitution. Use --format json for a structured report (matched/
unmatched counts, the codemap resolution note, and per-spec match reason).
codemap must be installed and the project indexed; pass --codemap <path>
when the binary is not on $PATH.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			if since != "" && staged {
				return exitError{code: 2, err: errors.New("affected-specs: pass at most one of --since/--staged")}
			}
			mode := "working"
			if staged {
				mode = "staged"
			} else if since != "" {
				mode = "since"
			}
			targets := args
			if len(targets) == 0 {
				targets = []string{"."}
			}
			rows, err := affected.LoadSpecs(targets, opts.configPath, opts.environment)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			review, err := affected.RunReview(codemapBin, mode, since, depth)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			report := affected.Select(rows, review)
			report.SchemaVersion = 1
			report.Mode = mode
			report.Since = since
			// md is shell-friendly bare paths (one per line) so
			// `glyph run $(glyph affected-specs ...)` works directly; the
			// counts/note go to stderr so stdout stays a clean path list.
			output, err := emitForCLI(cmd, opts, format, report, func() string {
				paths := make([]string, 0, len(report.Specs))
				for _, m := range report.Specs {
					paths = append(paths, m.Path)
				}
				sort.Strings(paths)
				if len(paths) == 0 {
					return ""
				}
				return strings.Join(paths, "\n") + "\n"
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			if !opts.quiet && format == formatMD {
				fmt.Fprintf(cmd.ErrOrStderr(), "affected-specs: %d matched, %d unmatched, %d without coversSymbol",
					report.Matched, report.Unmatched, report.NoCover)
				if report.Note != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), " [%s]", report.Note)
				}
				if report.Resolution != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), " resolution=%q", report.Resolution)
				}
				fmt.Fprintln(cmd.ErrOrStderr())
			}
			cmd.Print(output)
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "review changes since this git ref (committed + uncommitted)")
	cmd.Flags().BoolVar(&staged, "staged", false, "review only staged changes (the git index)")
	cmd.Flags().StringVar(&codemapBin, "codemap", "codemap", "path to the codemap binary (default: $PATH)")
	cmd.Flags().IntVar(&depth, "depth", 3, "max blast-radius hops passed to codemap review")
	return cmd
}
