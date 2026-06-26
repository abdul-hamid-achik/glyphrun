package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/spf13/cobra"
)

func newListCommand(opts *globalOptions) *cobra.Command {
	var (
		feature string
		tag     string
		owner   string
	)
	cmd := &cobra.Command{
		Use:   "list [path...]",
		Short: "List specs with their metadata, contract hash, and last run status",
		Long: `Walk one or more spec paths (files or directories) and print
a compact table of every parseable spec, including any metadata block
the spec declared.

Use --feature, --tag, or --owner to filter the list. The path argument
defaults to the current directory.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			targets := args
			if len(targets) == 0 {
				targets = []string{"."}
			}
			rows, err := listSpecs(targets, opts, listFilters{
				Feature: feature,
				Tag:     tag,
				Owner:   owner,
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			// Sort: priority desc, then name asc.
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].Priority != rows[j].Priority {
					return priorityRank(rows[i].Priority) > priorityRank(rows[j].Priority)
				}
				return rows[i].Name < rows[j].Name
			})
			output, err := emitForCLI(cmd, opts, format, rows, func() string { return renderListMarkdown(rows) })
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
	cmd.Flags().StringVar(&feature, "feature", "", "filter to specs whose metadata.feature matches")
	cmd.Flags().StringVar(&tag, "tag", "", "filter to specs whose metadata.tags includes the value")
	cmd.Flags().StringVar(&owner, "owner", "", "filter to specs whose metadata.owner matches")
	return cmd
}

type listFilters struct {
	Feature string
	Tag     string
	Owner   string
}

type listRow struct {
	Name         string   `json:"name" yaml:"name"`
	Path         string   `json:"path" yaml:"path"`
	Intent       string   `json:"intent,omitempty" yaml:"intent,omitempty"`
	Feature      string   `json:"feature,omitempty" yaml:"feature,omitempty"`
	Owner        string   `json:"owner,omitempty" yaml:"owner,omitempty"`
	Priority     string   `json:"priority,omitempty" yaml:"priority,omitempty"`
	Tags         []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	ContractHash string   `json:"contractHash,omitempty" yaml:"contractHash,omitempty"`
	CoversSymbol string   `json:"coversSymbol,omitempty" yaml:"coversSymbol,omitempty"`
	StepCount    int      `json:"stepCount" yaml:"stepCount"`
	OutcomeCount int      `json:"outcomeCount" yaml:"outcomeCount"`
	ParseError   string   `json:"parseError,omitempty" yaml:"parseError,omitempty"`
}

// listSpecs walks the given paths (files or directories), parses every
// .yml/.yaml/.json spec it finds, and returns one row per parseable
// spec. Specs that fail to parse are still returned (with a populated
// `parseError` field) so `glyph list` always reflects the full surface
// of the input set. The same spec dir layout the runner uses is
// honored: directories named `actions/`, files starting with `_` or
// ending in `.draft.yml`, are skipped, matching `glyph run`'s
// recursive spec discovery.
func listSpecs(paths []string, opts *globalOptions, filters listFilters) ([]listRow, error) {
	files, err := collectSpecFiles(paths)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("no spec files found in the given paths")
	}
	rows := make([]listRow, 0, len(files))
	for _, path := range files {
		row := listRow{Path: path, Name: filepath.Base(path)}
		rt, err := config.LoadRuntime(path, opts.configPath, opts.environment)
		if err != nil {
			row.ParseError = err.Error()
			rows = append(rows, row)
			continue
		}
		parsed, err := spec.ParseFile(path, rt.SpecParseOptions())
		if err != nil {
			row.ParseError = err.Error()
			rows = append(rows, row)
			continue
		}
		row.Name = parsed.Spec.Name
		row.Intent = strings.TrimSpace(parsed.Spec.Intent)
		if parsed.Spec.Metadata != nil {
			row.Feature = parsed.Spec.Metadata.Feature
			row.Owner = parsed.Spec.Metadata.Owner
			row.Priority = parsed.Spec.Metadata.Priority
			row.Tags = append([]string(nil), parsed.Spec.Metadata.Tags...)
		}
		row.ContractHash = parsed.ContractHash
		row.CoversSymbol = parsed.Spec.CoversSymbol
		row.StepCount = len(parsed.Resolved.Steps)
		row.OutcomeCount = len(parsed.Resolved.Outcomes)
		// Apply filters.
		if filters.Feature != "" && row.Feature != filters.Feature {
			continue
		}
		if filters.Owner != "" && row.Owner != filters.Owner {
			continue
		}
		if filters.Tag != "" {
			found := false
			for _, t := range row.Tags {
				if t == filters.Tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// collectSpecFiles expands the given paths (files or directories) into
// a deduplicated, sorted list of spec files. Skips action libraries
// and draft files. The rules match what `cairn run` and `glyph run`
// use for directory inputs.
func collectSpecFiles(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if !info.IsDir() {
			if isSpecFile(abs) {
				if !seen[abs] {
					seen[abs] = true
					out = append(out, abs)
				}
			}
			continue
		}
		err = filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				base := filepath.Base(path)
				if base == "actions" || base == "node_modules" || base == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if !isSpecFile(path) {
				return nil
			}
			base := filepath.Base(path)
			if strings.HasPrefix(base, "_") || strings.HasSuffix(base, ".draft.yml") || strings.HasSuffix(base, ".draft.yaml") {
				return nil
			}
			if !seen[path] {
				seen[path] = true
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func isSpecFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yml", ".yaml", ".json":
		return true
	}
	return false
}

// priorityRank translates the metadata priority string into a numeric
// rank for sorting (higher = more urgent).
func priorityRank(p string) int {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "normal":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

// renderListMarkdown produces a compact markdown table. The table is
// intentionally wide — `glyph list` is meant for humans skimming a
// project's spec surface, not for CI parsing (use --format json for
// the machine-readable form).
func renderListMarkdown(rows []listRow) string {
	if len(rows) == 0 {
		return "# Glyphrun List\n\n(no specs matched)\n"
	}
	var b strings.Builder
	b.WriteString("# Glyphrun List\n\n")
	b.WriteString("| name | feature | owner | priority | tags | steps | outcomes | contract | path |\n")
	b.WriteString("| --- | --- | --- | --- | --- | ---: | ---: | --- | --- |\n")
	for _, row := range rows {
		tags := strings.Join(row.Tags, ", ")
		if tags == "" {
			tags = "—"
		}
		hash := row.ContractHash
		if len(hash) > 12 {
			hash = hash[:12] + "…"
		}
		if hash == "" {
			hash = "—"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %d | %d | `%s` | `%s` |\n",
			row.Name,
			emptyDash(row.Feature),
			emptyDash(row.Owner),
			emptyDash(row.Priority),
			tags,
			row.StepCount,
			row.OutcomeCount,
			hash,
			relativePath(row.Path),
		)
	}
	// Surface parse errors separately so they don't get lost in a row.
	var errs []listRow
	for _, row := range rows {
		if row.ParseError != "" {
			errs = append(errs, row)
		}
	}
	if len(errs) > 0 {
		b.WriteString("\n## Parse errors\n\n")
		for _, row := range errs {
			fmt.Fprintf(&b, "- `%s`: %s\n", relativePath(row.Path), row.ParseError)
		}
	}
	return b.String()
}

func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func relativePath(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil {
			return rel
		}
	}
	return path
}

// Ensure the list command can write to stdout when invoked programmatically.
var _ = io.Discard
