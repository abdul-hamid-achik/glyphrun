package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"

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
// spec↔symbol link (the coversSymbol field) and the intersection. codemap is
// invoked as a subprocess; pass --codemap <path> when it is not on $PATH.
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
			rows, err := listSpecs(targets, opts, listFilters{})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			review, err := runCodemapReview(codemapBin, mode, since, depth)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			report := selectAffectedSpecs(rows, review)
			report.SchemaVersion = 1
			report.Mode = mode
			report.Since = since
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

// reviewSymbol captures the symbol-identity fields glyphrun needs from a
// codemap review entry. Both changed_symbols (SymbolRef) and blast_radius
// (ImpactNode) carry `symbol` + `fqn`; the extra fields are ignored on
// unmarshal, so one type serves both arrays.
type reviewSymbol struct {
	Symbol string `json:"symbol"`
	FQN    string `json:"fqn,omitempty"`
}

// codemapReview is the subset of `codemap review --json` glyphrun consumes.
// Only the symbol arrays + resolution are read; changed_files,
// covering_tests, untested, hotspots, and stale/staleness are ignored.
type codemapReview struct {
	ChangedSymbols []reviewSymbol `json:"changed_symbols"`
	BlastRadius    []reviewSymbol `json:"blast_radius"`
	Resolution     string         `json:"resolution,omitempty"`
}

// affectedSpecEntry is one selected spec in the structured report.
type affectedSpecEntry struct {
	Name         string `json:"name" yaml:"name"`
	Path         string `json:"path" yaml:"path"`
	CoversSymbol string `json:"coversSymbol,omitempty" yaml:"coversSymbol,omitempty"`
	MatchedBy    string `json:"matchedBy" yaml:"matchedBy"` // "changed", "blast", or "both"
}

// affectedSpecsReport is the --format json/yaml payload.
type affectedSpecsReport struct {
	SchemaVersion int                 `json:"schemaVersion" yaml:"schemaVersion"`
	Mode          string              `json:"mode" yaml:"mode"`
	Since         string              `json:"since,omitempty" yaml:"since,omitempty"`
	Total         int                 `json:"total" yaml:"total"`
	Matched       int                 `json:"matched" yaml:"matched"`
	Unmatched     int                 `json:"unmatched" yaml:"unmatched"` // specs with coversSymbol that no change reached
	NoCover       int                 `json:"noCover" yaml:"noCover"`     // specs without coversSymbol (never selectable by symbol)
	Specs         []affectedSpecEntry `json:"specs" yaml:"specs"`
	Resolution    string              `json:"resolution,omitempty" yaml:"resolution,omitempty"`
	Note          string              `json:"note,omitempty" yaml:"note,omitempty"`
}

// runCodemapReview shells out to `codemap review --json` for the given diff
// scope and parses the symbol arrays. mode is "working" | "staged" | "since".
func runCodemapReview(bin, mode, since string, depth int) (codemapReview, error) {
	args := []string{"review", "--json", "--depth", fmt.Sprintf("%d", depth)}
	switch mode {
	case "staged":
		args = append(args, "--staged")
	case "since":
		args = append(args, "--since", since)
	}
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return codemapReview{}, fmt.Errorf("codemap review failed: %s: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return codemapReview{}, fmt.Errorf("codemap review: %w (is codemap installed and on $PATH, or pass --codemap <path>?)", err)
	}
	var raw codemapReview
	if err := json.Unmarshal(out, &raw); err != nil {
		return codemapReview{}, fmt.Errorf("parse codemap review output: %w", err)
	}
	return raw, nil
}

// selectAffectedSpecs intersects the parsed spec rows against the codemap
// review report. It is pure (no I/O) so the selection logic is unit-tested
// without codemap installed.
//
// A spec is selected when its coversSymbol equals a changed symbol's
// `symbol` or `fqn` (directly changed) OR a blast-radius node's `symbol` or
// `fqn` (transitively reached). Rows with a parse error are skipped (not
// counted — they could not be evaluated). Rows without a coversSymbol
// cannot be selected by symbol and count as NoCover.
func selectAffectedSpecs(rows []listRow, review codemapReview) affectedSpecsReport {
	changed := indexSymbols(review.ChangedSymbols)
	blast := indexSymbols(review.BlastRadius)
	report := affectedSpecsReport{Specs: []affectedSpecEntry{}, Resolution: review.Resolution}
	for _, row := range rows {
		if row.ParseError != "" {
			continue
		}
		report.Total++
		cover := strings.TrimSpace(row.CoversSymbol)
		if cover == "" {
			report.NoCover++
			continue
		}
		inChanged := changed[cover]
		inBlast := blast[cover]
		switch {
		case inChanged && inBlast:
			report.Specs = append(report.Specs, makeAffectedEntry(row, "both"))
		case inChanged:
			report.Specs = append(report.Specs, makeAffectedEntry(row, "changed"))
		case inBlast:
			report.Specs = append(report.Specs, makeAffectedEntry(row, "blast"))
		default:
			report.Unmatched++
		}
	}
	report.Matched = len(report.Specs)
	sort.SliceStable(report.Specs, func(i, j int) bool {
		return report.Specs[i].Path < report.Specs[j].Path
	})
	return report
}

func makeAffectedEntry(row listRow, reason string) affectedSpecEntry {
	return affectedSpecEntry{
		Name:         row.Name,
		Path:         row.Path,
		CoversSymbol: row.CoversSymbol,
		MatchedBy:    reason,
	}
}

// indexSymbols builds a set keyed by every name form codemap emits for a
// node — both the short `symbol` and the qualified `fqn` — so a
// coversSymbol written either way matches.
func indexSymbols(syms []reviewSymbol) map[string]bool {
	set := make(map[string]bool, len(syms)*2)
	for _, s := range syms {
		if s.Symbol != "" {
			set[s.Symbol] = true
		}
		if s.FQN != "" {
			set[s.FQN] = true
		}
	}
	return set
}
