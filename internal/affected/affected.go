// Package affected selects the specs a git change can hit. It is the shared
// engine behind `glyph affected-specs` (internal/cli) and the
// `glyph_affected_specs` MCP tool (internal/mcp): load specs from a path,
// ask codemap (via `codemap review --json`) which symbols a diff changed and
// the blast radius it reaches, then intersect the spec↔symbol link (the
// spec's coversSymbol field) against that set.
//
// The package owns the codemap CLI wrapping, the spec-file discovery, and the
// pure selection — no cobra, no CLI formatting. cli and mcp thin-wrap it.
package affected

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

// Spec is the slice of a parsed spec the selection needs. It is the
// package-local counterpart of cli.listRow (Name/Path/CoversSymbol/ParseError)
// so the pure selection does not depend on the cli package.
type Spec struct {
	Name         string
	Path         string
	CoversSymbol string
	ParseError   string
}

// ReviewSymbol captures the symbol-identity fields glyphrun needs from a
// codemap review entry. Both changed_symbols (SymbolRef) and blast_radius
// (ImpactNode) carry `symbol` + `fqn`; the extra fields are ignored on
// unmarshal, so one type serves both arrays.
type ReviewSymbol struct {
	Symbol string `json:"symbol"`
	FQN    string `json:"fqn,omitempty"`
}

// Review is the subset of `codemap review --json` glyphrun consumes. Only the
// schema version, symbol arrays, and resolution are read; changed_files,
// covering_tests, untested, hotspots, and stale/staleness are ignored.
type Review struct {
	SchemaVersion  int            `json:"schema_version"`
	ChangedSymbols []ReviewSymbol `json:"changed_symbols"`
	BlastRadius    []ReviewSymbol `json:"blast_radius"`
	Resolution     string         `json:"resolution,omitempty"`
}

type reviewV1Envelope struct {
	SchemaVersion  *int                   `json:"schema_version"`
	Project        *string                `json:"project"`
	Mode           *string                `json:"mode"`
	Depth          *int                   `json:"depth"`
	IsRepo         *bool                  `json:"is_repo"`
	Indexed        *bool                  `json:"indexed"`
	ChangedFiles   *[]reviewV1ChangedFile `json:"changed_files"`
	ChangedSymbols *[]reviewV1Symbol      `json:"changed_symbols"`
	BlastRadius    *[]reviewV1Impact      `json:"blast_radius"`
	CoveringTests  *[]reviewV1Impact      `json:"covering_tests"`
	Untested       *[]reviewV1Symbol      `json:"untested_symbols"`
	Stale          *bool                  `json:"stale"`
}

type reviewV1ChangedFile struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Symbols *int   `json:"symbols"`
}

type reviewV1Symbol struct {
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind"`
	File      string `json:"file"`
	StartLine *int   `json:"start_line"`
	EndLine   *int   `json:"end_line"`
}

type reviewV1Impact struct {
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind"`
	File      string `json:"file"`
	StartLine *int   `json:"start_line"`
	Depth     *int   `json:"depth"`
}

func validateReviewV1(out []byte) error {
	var envelope reviewV1Envelope
	if err := json.Unmarshal(out, &envelope); err != nil {
		return err
	}
	if envelope.SchemaVersion == nil || *envelope.SchemaVersion != 1 {
		version := 0
		if envelope.SchemaVersion != nil {
			version = *envelope.SchemaVersion
		}
		return fmt.Errorf("unsupported codemap review schema_version %d (supported: 1)", version)
	}
	if envelope.Project == nil || envelope.Mode == nil || envelope.Depth == nil ||
		envelope.IsRepo == nil || envelope.Indexed == nil || envelope.ChangedFiles == nil ||
		envelope.ChangedSymbols == nil || envelope.BlastRadius == nil ||
		envelope.CoveringTests == nil || envelope.Untested == nil || envelope.Stale == nil {
		return fmt.Errorf("codemap review schema_version 1 is missing required fields")
	}
	if (*envelope.Mode != "working" && *envelope.Mode != "staged" && *envelope.Mode != "since") || *envelope.Depth < 1 {
		return fmt.Errorf("codemap review schema_version 1 has invalid mode or depth")
	}
	for _, file := range *envelope.ChangedFiles {
		if strings.TrimSpace(file.Path) == "" || !validReviewFileStatus(file.Status) || file.Symbols == nil || *file.Symbols < 0 {
			return fmt.Errorf("codemap review schema_version 1 has malformed changed_files")
		}
	}
	for _, symbol := range append(append([]reviewV1Symbol(nil), (*envelope.ChangedSymbols)...), (*envelope.Untested)...) {
		if !validReviewSymbol(symbol.Symbol, symbol.Kind, symbol.File, symbol.StartLine) || symbol.EndLine == nil || *symbol.EndLine < 1 {
			return fmt.Errorf("codemap review schema_version 1 has malformed symbol entries")
		}
	}
	for _, node := range append(append([]reviewV1Impact(nil), (*envelope.BlastRadius)...), (*envelope.CoveringTests)...) {
		if !validReviewSymbol(node.Symbol, node.Kind, node.File, node.StartLine) || node.Depth == nil || *node.Depth < 0 {
			return fmt.Errorf("codemap review schema_version 1 has malformed impact entries")
		}
	}
	return nil
}

func validReviewSymbol(symbol, kind, file string, startLine *int) bool {
	return strings.TrimSpace(symbol) != "" && strings.TrimSpace(kind) != "" &&
		strings.TrimSpace(file) != "" && startLine != nil && *startLine >= 1
}

func validReviewFileStatus(status string) bool {
	return status == "A" || status == "M" || status == "D" || status == "?"
}

// Entry is one selected spec in the structured report.
type Entry struct {
	Name         string `json:"name" yaml:"name"`
	Path         string `json:"path" yaml:"path"`
	CoversSymbol string `json:"coversSymbol,omitempty" yaml:"coversSymbol,omitempty"`
	MatchedBy    string `json:"matchedBy" yaml:"matchedBy"` // "changed", "blast", or "both"
}

// Report is the --format json/yaml payload and the MCP tool result.
type Report struct {
	SchemaVersion int     `json:"schemaVersion" yaml:"schemaVersion"`
	Mode          string  `json:"mode" yaml:"mode"`
	Since         string  `json:"since,omitempty" yaml:"since,omitempty"`
	Total         int     `json:"total" yaml:"total"`
	Matched       int     `json:"matched" yaml:"matched"`
	Unmatched     int     `json:"unmatched" yaml:"unmatched"` // specs with coversSymbol that no change reached
	NoCover       int     `json:"noCover" yaml:"noCover"`     // specs without coversSymbol (never selectable by symbol)
	Specs         []Entry `json:"specs" yaml:"specs"`
	Resolution    string  `json:"resolution,omitempty" yaml:"resolution,omitempty"`
	Note          string  `json:"note,omitempty" yaml:"note,omitempty"`
}

// RunReview shells out to `codemap review --json` for the given diff scope and
// parses the symbol arrays. mode is "working" | "staged" | "since".
func RunReview(bin, mode, since string, depth int) (Review, error) {
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
			return Review{}, fmt.Errorf("codemap review failed: %s: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return Review{}, fmt.Errorf("codemap review: %w (is codemap installed and on $PATH, or pass --codemap <path>?)", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(out, &fields); err != nil {
		return Review{}, fmt.Errorf("parse codemap review output: %w", err)
	}
	if version, present := fields["schema_version"]; present {
		if bytes.Equal(bytes.TrimSpace(version), []byte("null")) {
			return Review{}, fmt.Errorf("unsupported codemap review schema_version null (supported: 1)")
		}
		if err := validateReviewV1(out); err != nil {
			return Review{}, err
		}
	}
	var raw Review
	if err := json.Unmarshal(out, &raw); err != nil {
		return Review{}, fmt.Errorf("parse codemap review output: %w", err)
	}
	return raw, nil
}

// Select intersects the parsed spec rows against the codemap review. It is
// pure (no I/O) so the selection logic is unit-tested without codemap.
//
// A spec is selected when its coversSymbol equals a changed symbol's `symbol`
// or `fqn` (directly changed) OR a blast-radius node's `symbol` or `fqn`
// (transitively reached). Rows with a parse error are skipped (not counted —
// they could not be evaluated). Rows without a coversSymbol cannot be
// selected by symbol and count as NoCover.
func Select(rows []Spec, review Review) Report {
	changed := indexSymbols(review.ChangedSymbols)
	blast := indexSymbols(review.BlastRadius)
	report := Report{Specs: []Entry{}, Resolution: review.Resolution}
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
		inChanged, inBlast := changed[cover], blast[cover]
		switch {
		case inChanged && inBlast:
			report.Specs = append(report.Specs, Entry{Name: row.Name, Path: row.Path, CoversSymbol: row.CoversSymbol, MatchedBy: "both"})
		case inChanged:
			report.Specs = append(report.Specs, Entry{Name: row.Name, Path: row.Path, CoversSymbol: row.CoversSymbol, MatchedBy: "changed"})
		case inBlast:
			report.Specs = append(report.Specs, Entry{Name: row.Name, Path: row.Path, CoversSymbol: row.CoversSymbol, MatchedBy: "blast"})
		default:
			report.Unmatched++
		}
	}
	report.Matched = len(report.Specs)
	sort.SliceStable(report.Specs, func(i, j int) bool { return report.Specs[i].Path < report.Specs[j].Path })
	return report
}

func indexSymbols(syms []ReviewSymbol) map[string]bool {
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

// LoadSpecs walks the given paths (files or directories), parses every
// .yml/.yaml/.json spec it finds, and returns one Spec row per parseable spec
// (specs that fail to parse are still returned with a populated ParseError so
// the caller can count/skip them). The discovery rules match `glyph run` /
// `glyph list`: directories named actions/, node_modules, .git are skipped,
// and files starting with `_` or ending in `.draft.yml`/`.draft.yaml` are
// excluded.
func LoadSpecs(paths []string, configPath, environment string) ([]Spec, error) {
	files, err := CollectSpecFiles(paths)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("no spec files found in the given paths")
	}
	rows := make([]Spec, 0, len(files))
	for _, path := range files {
		row := Spec{Path: path, Name: filepath.Base(path)}
		rt, err := config.LoadRuntime(path, configPath, environment)
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
		row.CoversSymbol = parsed.Spec.CoversSymbol
		rows = append(rows, row)
	}
	return rows, nil
}

// CollectSpecFiles expands the given paths (files or directories) into a
// deduplicated, sorted list of spec files. Skips action libraries and draft
// files. The rules match what `glyph run` and `glyph list` use for directory
// inputs. It is the shared spec-discovery helper (moved out of internal/cli
// so the MCP server can reach it without importing cli).
func CollectSpecFiles(paths []string) ([]string, error) {
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
			if IsSpecFile(abs) {
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
			if !IsSpecFile(path) {
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

// IsSpecFile reports whether path has a glyphrun spec extension.
func IsSpecFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yml", ".yaml", ".json":
		return true
	}
	return false
}
