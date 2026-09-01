// Package stories builds a catalog of terminal stories — isolated TUI states
// — and their captured screens so authors can review layout without guessing
// cell positions. It owns the stories manifest (`stories.yml`) and its
// expansion into ordinary specs, the retention-proof stories index, and the
// HTML inspect page.
//
// It does not spawn processes or import Bubble Tea. Running stories is the
// job of internal/storyrun, which drives the regular runner.
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

// ErrNoSpecs is returned when Collect finds neither manifests nor spec files
// in the given paths.
var ErrNoSpecs = errors.New("no stories manifests or spec files found in the given paths")

// Golden status values reported per snapshot and per story.
const (
	GoldenMatch   = "match"   // committed golden exists and equals the capture
	GoldenChanged = "changed" // committed golden differs from the capture
	GoldenMissing = "missing" // the story asks for a golden but none is committed
	GoldenNone    = "none"    // no golden applies to this screen
)

// Source values for Story.Source.
const (
	SourceManifest = "manifest"
	SourceSpec     = "spec"
)

// CollectOptions controls which stories appear in a catalog.
type CollectOptions struct {
	Paths        []string
	ArtifactRoot string
	SnapshotRoot string
	StoriesRoot  string
	ConfigPath   string
	Environment  string
	Feature      string
	Tag          string
	Owner        string
	All          bool
	// DefaultTerminal fills manifest stories that omit cols/rows/profile.
	DefaultTerminal spec.Terminal
}

// Catalog is the machine-readable `glyph stories` payload. Cell grids, diffs,
// and regions stay off the JSON envelope; they feed the HTML and TUI views.
type Catalog struct {
	SchemaVersion int     `json:"schemaVersion"`
	Stories       []Story `json:"stories"`
}

// Story is one catalog row: a manifest story (times variant) or a spec file
// tagged `story`, joined to its newest result.
type Story struct {
	Key        string     `json:"key"`
	ID         string     `json:"id,omitempty"`
	Variant    string     `json:"variant,omitempty"`
	Name       string     `json:"name"`
	Source     string     `json:"source"`
	Path       string     `json:"path"`
	Feature    string     `json:"feature,omitempty"`
	Owner      string     `json:"owner,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	Intent     string     `json:"intent,omitempty"`
	RunID      string     `json:"runId,omitempty"`
	Status     string     `json:"status"`
	Diagnostic string     `json:"diagnostic,omitempty"`
	Golden     string     `json:"golden"`
	Passed     int        `json:"outcomesPassed"`
	Failed     int        `json:"outcomesFailed"`
	ParseError string     `json:"parseError,omitempty"`
	Snapshots  []Snapshot `json:"snapshots"`
	// Cols/Rows is the terminal the story is declared at (before a run).
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`
}

// Summary counts catalog rows by status and golden state.
type Summary struct {
	Stories int `json:"stories"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	NotRun  int `json:"notRun"`
	Changed int `json:"changed"`
	Missing int `json:"missing"`
}

// Summarize tallies the catalog.
func (c Catalog) Summarize() Summary {
	s := Summary{Stories: len(c.Stories)}
	for _, st := range c.Stories {
		switch st.Status {
		case "passed":
			s.Passed++
		case "not_run":
			s.NotRun++
		default:
			s.Failed++
		}
		switch st.Golden {
		case GoldenChanged:
			s.Changed++
		case GoldenMissing:
			s.Missing++
		}
	}
	return s
}

// Snapshot is a named screen from a story's newest run, compared to its
// committed golden when one applies.
type Snapshot struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
	Error   string `json:"error,omitempty"`
	Golden  string `json:"golden"`
	Changed int    `json:"changed"`
	// Off the JSON envelope: the grids and overlays the HTML/TUI views paint.
	Screen       *terminal.ScreenSnapshot `json:"-"`
	GoldenScreen *terminal.ScreenSnapshot `json:"-"`
	Diff         []terminal.CellDiff      `json:"-"`
	Regions      []render.RegionHighlight `json:"-"`
}

// Collect walks the paths for manifests and spec files, expands manifests,
// and joins every story to its newest result (stories index first, then the
// run directories). Spec files tagged `story` are preferred over untagged
// ones unless a filter or All is set.
func Collect(opts CollectOptions) (Catalog, error) {
	paths := opts.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}
	manifests, err := FindManifests(paths)
	if err != nil {
		return Catalog{}, err
	}
	files, err := affected.CollectSpecFiles(paths)
	if err != nil {
		return Catalog{}, err
	}
	specFiles := make([]string, 0, len(files))
	for _, f := range files {
		if skipSpecPath(f) || IsManifestPath(f) || UnderRoots(f, opts.ArtifactRoot, opts.SnapshotRoot, opts.StoriesRoot) {
			continue
		}
		specFiles = append(specFiles, f)
	}
	if len(manifests) == 0 && len(specFiles) == 0 {
		return Catalog{}, ErrNoSpecs
	}

	index := ReadIndex(opts.StoriesRoot)
	runs := indexRuns(opts.ArtifactRoot)

	var stories []Story
	for _, path := range manifests {
		stories = append(stories, loadManifestStories(path, opts, index, runs)...)
	}
	for _, path := range specFiles {
		stories = append(stories, loadSpecStory(path, opts, index, runs))
	}
	filtered := filterStories(stories, opts, len(manifests) > 0)
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Feature != filtered[j].Feature {
			return filtered[i].Feature < filtered[j].Feature
		}
		if filtered[i].ID != filtered[j].ID {
			return filtered[i].ID < filtered[j].ID
		}
		if filtered[i].Variant != filtered[j].Variant {
			return filtered[i].Variant < filtered[j].Variant
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
	if base == "run.json" || base == "final.json" || base == "spec.resolved.yml" || base == IndexFile {
		return true
	}
	parent := filepath.Base(filepath.Dir(path))
	return parent == "screens" || parent == "snapshots"
}

// UnderRoots reports whether path lives inside any of the given directories
// (artifact, golden, or stories roots), whose JSON/YAML files are run output,
// not authored specs.
func UnderRoots(path string, roots ...string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(absRoot, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func loadManifestStories(path string, opts CollectOptions, index map[string]IndexEntry, runs map[string]artifacts.RunResult) []Story {
	m, err := LoadManifest(path)
	if err != nil {
		return []Story{{Key: path, Name: filepath.Base(path), Source: SourceManifest, Path: path, Status: "parse_error", Golden: GoldenNone, ParseError: err.Error(), Snapshots: []Snapshot{}}}
	}
	expanded, err := Expand(m, path, opts.DefaultTerminal)
	if err != nil {
		return []Story{{Key: path, Name: filepath.Base(path), Source: SourceManifest, Path: path, Status: "parse_error", Golden: GoldenNone, ParseError: err.Error(), Snapshots: []Snapshot{}}}
	}
	out := make([]Story, 0, len(expanded))
	for _, ex := range expanded {
		row := Story{
			Key:       ex.Key(),
			ID:        ex.ID,
			Variant:   ex.Variant,
			Name:      ex.Spec.Name,
			Source:    SourceManifest,
			Path:      path,
			Feature:   ex.Feature,
			Intent:    strings.TrimSpace(ex.Spec.Intent),
			Golden:    GoldenNone,
			Cols:      ex.Spec.Terminal.Cols,
			Rows:      ex.Spec.Terminal.Rows,
			Snapshots: []Snapshot{},
		}
		if ex.Spec.Metadata != nil {
			row.Owner = ex.Spec.Metadata.Owner
			row.Tags = append([]string(nil), ex.Spec.Metadata.Tags...)
		}
		goldenName := ""
		if ex.Golden {
			goldenName = ex.SnapshotName
		}
		joinResult(&row, ex.Spec, goldenName, opts, index, runs)
		out = append(out, row)
	}
	return out
}

func loadSpecStory(path string, opts CollectOptions, index map[string]IndexEntry, runs map[string]artifacts.RunResult) Story {
	row := Story{Key: path, Path: path, Name: filepath.Base(path), Source: SourceSpec, Status: "parse_error", Golden: GoldenNone, Snapshots: []Snapshot{}}
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
	row.Key = parsed.Spec.Name
	row.Name = parsed.Spec.Name
	row.Intent = strings.TrimSpace(parsed.Spec.Intent)
	row.Cols = parsed.Resolved.Terminal.Cols
	row.Rows = parsed.Resolved.Terminal.Rows
	if parsed.Spec.Metadata != nil {
		row.Feature = parsed.Spec.Metadata.Feature
		row.Owner = parsed.Spec.Metadata.Owner
		row.Tags = append([]string(nil), parsed.Spec.Metadata.Tags...)
	}
	goldenName := ""
	for _, o := range parsed.Resolved.Outcomes {
		if o.Verify.Snapshot != nil {
			goldenName = o.Verify.Snapshot.Name
			break
		}
	}
	joinResult(&row, parsed.Resolved, goldenName, opts, index, runs)
	return row
}

// joinResult fills the run-side fields of a row from the stories index or,
// failing that, the newest matching run directory.
func joinResult(row *Story, s spec.Spec, goldenName string, opts CollectOptions, index map[string]IndexEntry, runs map[string]artifacts.RunResult) {
	regions := regionsFromSpec(s)
	if entry, ok := index[s.Name]; ok {
		row.RunID = entry.RunID
		row.Status = entry.Status
		row.Diagnostic = entry.Diagnostic
		row.Passed, row.Failed = countOutcomes(entry.Outcomes)
		dir := IndexDir(opts.StoriesRoot, s.Name)
		row.Snapshots = loadSnapshots(dir, s.Name, goldenName, regions, opts.SnapshotRoot)
		row.Golden = aggregateGolden(row.Snapshots, goldenName)
		reconcileGoldenWithOutcomes(row, goldenName, entry.Outcomes)
		return
	}
	if run, ok := runs[s.Name]; ok {
		row.RunID = run.RunID
		row.Status = string(run.Status)
		if row.Status == "" {
			row.Status = "run"
		}
		row.Diagnostic = run.Diagnostic
		row.Passed, row.Failed = countOutcomes(run.Outcomes)
		row.Snapshots = loadSnapshots(run.RunDir, s.Name, goldenName, regions, opts.SnapshotRoot)
		row.Golden = aggregateGolden(row.Snapshots, goldenName)
		reconcileGoldenWithOutcomes(row, goldenName, run.Outcomes)
		return
	}
	row.Status = "not_run"
	if goldenName != "" {
		if _, err := os.Stat(goldenPath(opts.SnapshotRoot, s.Name, goldenName)); err != nil {
			row.Golden = GoldenMissing
		} else {
			row.Golden = GoldenNone
		}
	}
}

func countOutcomes(outcomes []artifacts.OutcomeResult) (passed, failed int) {
	for _, o := range outcomes {
		if o.Status == artifacts.OutcomePassed {
			passed++
		} else {
			failed++
		}
	}
	return passed, failed
}

func aggregateGolden(snaps []Snapshot, goldenName string) string {
	if goldenName == "" {
		return GoldenNone
	}
	for _, s := range snaps {
		if s.Name == goldenName {
			return s.Golden
		}
	}
	return GoldenMissing
}

// reconcileGoldenWithOutcomes trusts the runner over the cell diff: the
// `snapshot` verifier compares normalized text (and a hand-edited golden
// .txt may differ while the .json cells still match), so a failed golden
// outcome marks the story changed even when the cell diff is empty.
func reconcileGoldenWithOutcomes(row *Story, goldenName string, outcomes []artifacts.OutcomeResult) {
	if goldenName == "" {
		return
	}
	for _, o := range outcomes {
		if o.Status == artifacts.OutcomePassed {
			continue
		}
		if o.ID != "golden" && !strings.Contains(o.Message, "snapshot") {
			continue
		}
		if row.Golden == GoldenMatch {
			row.Golden = GoldenChanged
		}
		for i := range row.Snapshots {
			if row.Snapshots[i].Name == goldenName && row.Snapshots[i].Golden == GoldenMatch {
				row.Snapshots[i].Golden = GoldenChanged
			}
		}
		return
	}
}

func filterStories(stories []Story, opts CollectOptions, haveManifests bool) []Story {
	tag := opts.Tag
	if !opts.All && tag == "" && opts.Feature == "" && opts.Owner == "" {
		if haveManifests || anyHasTag(stories, "story") {
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
		// A spec file that failed to parse is not a story (it is usually an
		// unrelated YAML file); a manifest that failed to parse is, and must
		// stay visible so the author sees the error.
		if tag != "" && !hasTag(s.Tags, tag) && !(s.Source == SourceManifest && s.ParseError != "") {
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

// loadSnapshots reads screens/final.json and snapshots/*.json under dir (a
// run directory or a stories index entry) and compares the golden-named one
// against the committed golden.
func loadSnapshots(dir, specName, goldenName string, regions []render.RegionHighlight, snapshotRoot string) []Snapshot {
	var snaps []Snapshot
	finalPath := filepath.Join(dir, "screens", "final.json")
	if snap, ok := snapshotFromFile("final", finalPath, regions); ok {
		snaps = append(snaps, snap)
	}
	snapDir := filepath.Join(dir, "snapshots")
	entries, err := os.ReadDir(snapDir)
	if err == nil {
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
			if snap, ok := snapshotFromFile(id, filepath.Join(snapDir, name), regions); ok {
				snaps = append(snaps, snap)
			}
		}
	}
	if goldenName == "" {
		return snaps
	}
	for i := range snaps {
		if snaps[i].Name != goldenName || snaps[i].Screen == nil {
			continue
		}
		golden, err := readGolden(snapshotRoot, specName, goldenName, *snaps[i].Screen)
		if err != nil {
			snaps[i].Golden = GoldenMissing
			continue
		}
		diff := terminal.DiffSnapshots(golden, *snaps[i].Screen)
		snaps[i].GoldenScreen = &golden
		snaps[i].Diff = diff.Changed
		snaps[i].Changed = len(diff.Changed)
		if diff.Equal() {
			snaps[i].Golden = GoldenMatch
		} else {
			snaps[i].Golden = GoldenChanged
		}
	}
	return snaps
}

// goldenPath mirrors the runner's committed snapshot layout:
// <snapshotRoot>/<spec>/<name>.json (the JSON sibling of the .txt golden).
func goldenPath(snapshotRoot, specName, name string) string {
	root := snapshotRoot
	if root == "" {
		root = config.DefaultSnapshotRoot
	}
	return filepath.Join(root, sanitizeName(specName), sanitizeName(name)+".json")
}

// readGolden loads the committed golden as a cell grid. The JSON sibling
// (written by the runner alongside the .txt) carries styles; a repository
// that committed only the .txt still gets a character-level comparison built
// from its lines, sized like the current screen.
func readGolden(snapshotRoot, specName, name string, current terminal.ScreenSnapshot) (terminal.ScreenSnapshot, error) {
	jsonPath := goldenPath(snapshotRoot, specName, name)
	if screen, err := readScreen(jsonPath); err == nil {
		return screen, nil
	}
	data, err := os.ReadFile(strings.TrimSuffix(jsonPath, ".json") + ".txt")
	if err != nil {
		return terminal.ScreenSnapshot{}, err
	}
	return screenFromText(string(data), current.Cols, current.Rows), nil
}

// screenFromText builds a default-styled cell grid from golden text.
func screenFromText(text string, cols, rows int) terminal.ScreenSnapshot {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if rows < len(lines) {
		rows = len(lines)
	}
	for _, line := range lines {
		if n := len([]rune(line)); n > cols {
			cols = n
		}
	}
	screen := terminal.ScreenSnapshot{Cols: cols, Rows: rows, Text: text}
	for y := 0; y < rows; y++ {
		var runes []rune
		if y < len(lines) {
			runes = []rune(lines[y])
		}
		for x := 0; x < cols; x++ {
			ch := " "
			if x < len(runes) {
				ch = string(runes[x])
			}
			screen.Cells = append(screen.Cells, terminal.Cell{X: x, Y: y, Char: ch, Width: 1})
		}
	}
	return screen
}

func readScreen(path string) (terminal.ScreenSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return terminal.ScreenSnapshot{}, err
	}
	var screen terminal.ScreenSnapshot
	if err := json.Unmarshal(data, &screen); err != nil {
		return terminal.ScreenSnapshot{}, err
	}
	return screen, nil
}

func snapshotFromFile(name, path string, regions []render.RegionHighlight) (Snapshot, bool) {
	screen, err := readScreen(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{}, false
		}
		return Snapshot{Name: name, Status: "unreadable", Error: err.Error(), Golden: GoldenNone}, true
	}
	return Snapshot{
		Name:    name,
		Status:  "ok",
		Cols:    screen.Cols,
		Rows:    screen.Rows,
		Golden:  GoldenNone,
		Screen:  &screen,
		Regions: regions,
	}, true
}

// SVG renders one catalog snapshot with the requested overlays. The HTML page
// paints the grid client-side; this is for `glyph render`-style exports.
func (s Snapshot) SVG(grid, rulers, spaces, diff bool) string {
	if s.Screen == nil {
		return ""
	}
	opts := render.DefaultOptions()
	opts.ShowGrid = grid
	opts.ShowRulers = rulers
	opts.ShowSpaces = spaces
	if grid {
		opts.Regions = s.Regions
	}
	if diff {
		opts.Changed = s.Diff
	}
	return render.SnapshotSVG(*s.Screen, opts)
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
