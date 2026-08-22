// Package stories builds a catalog of terminal specs and their captured
// snapshots so authors can review TUI layout without guessing cell positions.
//
// It does not spawn processes or import Bubble Tea. A story is a regular spec
// (typically tagged "story") whose last run's cell grid is re-rendered here.
package stories

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/affected"
	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/render"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// ErrNoSpecs is returned when Collect finds no spec files in the given paths.
var ErrNoSpecs = errors.New("no spec files found in the given paths")

// CollectOptions controls which specs appear in a catalog.
type CollectOptions struct {
	Paths        []string
	ArtifactRoot string
	ConfigPath   string
	Environment  string
	Feature      string
	Tag          string
	Owner        string
	All          bool
}

// Catalog is the machine-readable glyph stories payload (SVGs stay off the
// JSON envelope and are used only when rendering HTML).
type Catalog struct {
	SchemaVersion int     `json:"schemaVersion"`
	Stories       []Story `json:"stories"`
}

// Story is one spec in the catalog, optionally joined to its newest run.
type Story struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	Feature    string     `json:"feature,omitempty"`
	Owner      string     `json:"owner,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	Intent     string     `json:"intent,omitempty"`
	RunID      string     `json:"runId,omitempty"`
	Status     string     `json:"status"`
	ParseError string     `json:"parseError,omitempty"`
	Snapshots  []Snapshot `json:"snapshots"`
}

// Snapshot is a named screen from a run. SVG fields are omitted from JSON.
type Snapshot struct {
	Name      string                   `json:"name"`
	Status    string                   `json:"status"`
	Cols      int                      `json:"cols,omitempty"`
	Rows      int                      `json:"rows,omitempty"`
	Error     string                   `json:"error,omitempty"`
	SVGPlain  string                   `json:"-"`
	SVGGrid   string                   `json:"-"`
	SVGSpaces string                   `json:"-"`
	Cells     []terminal.Cell          `json:"-"`
	Screen    *terminal.ScreenSnapshot `json:"-"`
}

// Collect walks spec paths, optionally prefers tag "story", and joins each
// spec to the newest matching run under ArtifactRoot.
func Collect(opts CollectOptions) (Catalog, error) {
	paths := opts.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}
	files, err := affected.CollectSpecFiles(paths)
	if err != nil {
		return Catalog{}, err
	}
	if len(files) == 0 {
		return Catalog{}, ErrNoSpecs
	}

	runs := indexRuns(opts.ArtifactRoot)
	stories := make([]Story, 0, len(files))
	for _, path := range files {
		if skipSpecPath(path) {
			continue
		}
		stories = append(stories, loadStory(path, opts, runs))
	}
	filtered := filterStories(stories, opts)
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Feature != filtered[j].Feature {
			return filtered[i].Feature < filtered[j].Feature
		}
		return filtered[i].Name < filtered[j].Name
	})
	return Catalog{SchemaVersion: 1, Stories: filtered}, nil
}

func skipSpecPath(path string) bool {
	p := filepath.ToSlash(path)
	if strings.Contains(p, "/.glyphrun/") {
		return true
	}
	base := filepath.Base(path)
	if base == "run.json" || base == "final.json" {
		return true
	}
	parent := filepath.Base(filepath.Dir(path))
	return parent == "screens" || parent == "snapshots"
}

func loadStory(path string, opts CollectOptions, runs map[string]artifacts.RunResult) Story {
	row := Story{Path: path, Name: filepath.Base(path), Status: "parse_error", Snapshots: []Snapshot{}}
	rt, err := config.LoadRuntime(path, opts.ConfigPath, opts.Environment)
	if err != nil {
		row.ParseError = err.Error()
		return row
	}
	parseOpts := rt.SpecParseOptions()
	parseOpts.AllowHashMismatch = true
	parsed, err := spec.ParseFile(path, parseOpts)
	if err != nil {
		row.ParseError = err.Error()
		return row
	}
	row.Name = parsed.Spec.Name
	row.Intent = strings.TrimSpace(parsed.Spec.Intent)
	if parsed.Spec.Metadata != nil {
		row.Feature = parsed.Spec.Metadata.Feature
		row.Owner = parsed.Spec.Metadata.Owner
		row.Tags = append([]string(nil), parsed.Spec.Metadata.Tags...)
	}
	regions := regionsFromSpec(parsed.Resolved)
	if run, ok := runs[parsed.Spec.Name]; ok {
		row.RunID = run.RunID
		row.Status = string(run.Status)
		if row.Status == "" {
			row.Status = "run"
		}
		row.Snapshots = loadSnapshots(run.RunDir, regions)
		return row
	}
	row.Status = "not_run"
	return row
}

func filterStories(stories []Story, opts CollectOptions) []Story {
	tag := opts.Tag
	if !opts.All && tag == "" && opts.Feature == "" && opts.Owner == "" {
		if anyHasTag(stories, "story") {
			tag = "story"
		}
	}
	out := make([]Story, 0, len(stories))
	for _, s := range stories {
		if opts.Feature != "" && s.Feature != opts.Feature {
			continue
		}
		if opts.Owner != "" && s.Owner != opts.Owner {
			continue
		}
		if tag != "" && !hasTag(s.Tags, tag) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func anyHasTag(stories []Story, tag string) bool {
	for _, s := range stories {
		if hasTag(s.Tags, tag) {
			return true
		}
	}
	return false
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func indexRuns(root string) map[string]artifacts.RunResult {
	out := map[string]artifacts.RunResult{}
	if strings.TrimSpace(root) == "" {
		return out
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		result, err := artifacts.LoadRunResult(dir)
		if err != nil {
			continue
		}
		result.RunDir = dir
		prev, ok := out[result.SpecName]
		if !ok || result.EndedAt > prev.EndedAt {
			out[result.SpecName] = result
		}
	}
	return out
}

func loadSnapshots(runDir string, regions []render.RegionHighlight) []Snapshot {
	var snaps []Snapshot
	finalPath := filepath.Join(runDir, "screens", "final.json")
	if snap, ok := snapshotFromFile("final", finalPath, regions); ok {
		snaps = append(snaps, snap)
	}
	dir := filepath.Join(runDir, "snapshots")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return snaps
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		id := strings.TrimSuffix(name, ".json")
		if snap, ok := snapshotFromFile(id, filepath.Join(dir, name), regions); ok {
			snaps = append(snaps, snap)
		}
	}
	return snaps
}

func snapshotFromFile(name, path string, regions []render.RegionHighlight) (Snapshot, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, false
	}
	var screen terminal.ScreenSnapshot
	if err := json.Unmarshal(data, &screen); err != nil {
		return Snapshot{Name: name, Status: "unreadable", Error: err.Error()}, true
	}
	plain := render.DefaultOptions()
	grid := render.DefaultOptions()
	grid.ShowGrid = true
	grid.ShowRulers = true
	grid.Regions = regions
	spaces := grid
	spaces.ShowSpaces = true
	copy := screen
	return Snapshot{
		Name:      name,
		Status:    "ok",
		Cols:      screen.Cols,
		Rows:      screen.Rows,
		SVGPlain:  render.SnapshotSVG(screen, plain),
		SVGGrid:   render.SnapshotSVG(screen, grid),
		SVGSpaces: render.SnapshotSVG(screen, spaces),
		Cells:     screen.Cells,
		Screen:    &copy,
	}, true
}

func regionsFromSpec(s spec.Spec) []render.RegionHighlight {
	var out []render.RegionHighlight
	for _, o := range s.Outcomes {
		if o.Verify.Region != nil {
			r := o.Verify.Region
			out = append(out, render.RegionHighlight{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height})
		}
		if o.Verify.Cell != nil {
			c := o.Verify.Cell
			out = append(out, render.RegionHighlight{X: c.X, Y: c.Y, Width: 1, Height: 1})
		}
	}
	return out
}
