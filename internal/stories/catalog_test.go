package stories

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

func writeSpec(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func sampleSpec(name, feature string, tags string) string {
	return `version: 1
name: ` + name + `
metadata:
  feature: ` + feature + `
  tags: ` + tags + `
intent: review layout
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: ok
    description: smoke
    verify:
      command:
        run: "true"
  - id: title
    description: title cell
    verify:
      cell:
        x: 0
        y: 0
        char: "h"
`
}

func writeRun(t *testing.T, root, specName string, snap terminal.ScreenSnapshot, snapshotName string) string {
	t.Helper()
	dir := filepath.Join(root, "run-"+specName)
	if err := os.MkdirAll(filepath.Join(dir, "screens"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := map[string]any{
		"schemaVersion": 1,
		"runId":         "run-" + specName,
		"specName":      specName,
		"status":        "passed",
		"startedAt":     "2026-01-01T00:00:00Z",
		"endedAt":       "2026-01-01T00:00:01Z",
		"durationMs":    1,
		"target":        map[string]any{"cmd": []string{"/bin/echo"}},
		"terminal":      map[string]any{"cols": snap.Cols, "rows": snap.Rows},
		"outcomes":      []any{map[string]any{"id": "ok", "status": "passed"}},
		"artifacts":     map[string]string{},
		"runDir":        dir,
		"exitCode":      0,
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	screen, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "screens", "final.json"), screen, 0o644); err != nil {
		t.Fatal(err)
	}
	if snapshotName != "" {
		if err := os.WriteFile(filepath.Join(dir, "snapshots", snapshotName+".json"), screen, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func twoCells(a, b string) terminal.ScreenSnapshot {
	return terminal.ScreenSnapshot{
		Cols: 2, Rows: 1,
		Cells: []terminal.Cell{{X: 0, Y: 0, Char: a, Width: 1}, {X: 1, Y: 0, Char: b, Width: 1}},
		Text:  a + b,
	}
}

func TestCollectJoinsLatestRunAndPrefersStoryTag(t *testing.T) {
	dir := t.TempDir()
	writeSpec(t, dir, "alpha.yml", sampleSpec("alpha", "list", "[story]"))
	writeSpec(t, dir, "beta.yml", sampleSpec("beta", "other", "[smoke]"))
	runs := filepath.Join(dir, "runs")
	writeRun(t, runs, "alpha", twoCells("h", "i"), "")

	cat, err := Collect(CollectOptions{Paths: []string{dir}, ArtifactRoot: runs})
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Stories) != 1 {
		t.Fatalf("prefer story tag: got %d stories, want 1", len(cat.Stories))
	}
	st := cat.Stories[0]
	if st.Name != "alpha" || st.Status != "passed" || st.RunID != "run-alpha" || st.Source != SourceSpec || st.Key != "alpha" {
		t.Fatalf("story = %+v", st)
	}
	if st.Passed != 1 || st.Golden != GoldenNone {
		t.Fatalf("outcome/golden summary = %+v", st)
	}
	if len(st.Snapshots) != 1 || st.Snapshots[0].Name != "final" || st.Snapshots[0].Status != "ok" {
		t.Fatalf("snapshots = %+v", st.Snapshots)
	}
	if st.Snapshots[0].Screen == nil || st.Snapshots[0].Screen.Cols != 2 {
		t.Fatalf("expected Screen snapshot for previews")
	}
	if len(st.Snapshots[0].Regions) != 1 {
		t.Fatalf("expected the cell outcome to become a region highlight, got %+v", st.Snapshots[0].Regions)
	}
	if !strings.Contains(st.Snapshots[0].SVG(true, true, false, false), `id="grid"`) {
		t.Fatalf("grid SVG missing overlay")
	}

	all, err := Collect(CollectOptions{Paths: []string{dir}, ArtifactRoot: runs, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Stories) != 2 {
		t.Fatalf("--all: got %d stories, want 2", len(all.Stories))
	}
	var beta *Story
	for i := range all.Stories {
		if all.Stories[i].Name == "beta" {
			beta = &all.Stories[i]
		}
	}
	if beta == nil || beta.Status != "not_run" {
		t.Fatalf("beta should be not_run, got %+v", beta)
	}
}

func TestCollectRejectsCrossManifestSpecNameCollision(t *testing.T) {
	dir := t.TempDir()
	manifest := "version: 1\nkind: stories\nharness:\n  cmd: [\"/bin/echo\"]\nstories:\n  - id: list/rows\n"
	writeSpec(t, dir, "first.stories.yml", manifest)
	writeSpec(t, dir, "second.stories.yml", manifest)
	_, err := Collect(CollectOptions{
		Paths:        []string{dir},
		ArtifactRoot: filepath.Join(dir, "runs"),
		SnapshotRoot: filepath.Join(dir, "goldens"),
		StoriesRoot:  filepath.Join(dir, "index"),
	})
	if err == nil || !strings.Contains(err.Error(), "story_list_rows") || !strings.Contains(err.Error(), "first.stories.yml") || !strings.Contains(err.Error(), "second.stories.yml") {
		t.Fatalf("expected global story identity collision, got %v", err)
	}
}

func TestCollectNoSpecs(t *testing.T) {
	dir := t.TempDir()
	_, err := Collect(CollectOptions{Paths: []string{dir}})
	if err != ErrNoSpecs {
		t.Fatalf("err = %v, want ErrNoSpecs", err)
	}
}

func TestCollectManifestJoinsIndexAndGoldenDiff(t *testing.T) {
	dir := t.TempDir()
	manifest := `version: 1
kind: stories
harness:
  cmd: ["/bin/echo"]
stories:
  - id: list/rows
    ready: { contains: "hi" }
    variants:
      - name: wide
        terminal: { cols: 120, rows: 40 }
  - id: list/empty
`
	writeSpec(t, dir, "stories.yml", manifest)
	runs := filepath.Join(dir, "runs")
	storiesRoot := filepath.Join(dir, "stories-index")
	snapshotRoot := filepath.Join(dir, "goldens")

	// list/rows ran and was indexed; its golden differs in one cell.
	runDir := writeRun(t, runs, "story_list_rows", twoCells("h", "i"), "rows")
	result, err := artifacts.LoadRunResult(runDir)
	if err != nil {
		t.Fatal(err)
	}
	result.RunDir = runDir
	entry := EntryFromRun("list/rows", "list/rows", "", "list", SourceManifest, filepath.Join(dir, "stories.yml"), "rows", result)
	if err := WriteIndexEntry(storiesRoot, entry, runDir); err != nil {
		t.Fatal(err)
	}
	// Prune the run dir: the catalog must still resolve through the index.
	if err := os.RemoveAll(runDir); err != nil {
		t.Fatal(err)
	}
	goldenDir := filepath.Join(snapshotRoot, "story_list_rows")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	golden, _ := json.Marshal(twoCells("h", "o"))
	if err := os.WriteFile(filepath.Join(goldenDir, "rows.json"), golden, 0o644); err != nil {
		t.Fatal(err)
	}

	cat, err := Collect(CollectOptions{
		Paths:           []string{dir},
		ArtifactRoot:    runs,
		SnapshotRoot:    snapshotRoot,
		StoriesRoot:     storiesRoot,
		DefaultTerminal: spec.Terminal{Cols: 100, Rows: 30, Profile: "xterm-256color"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Stories) != 3 {
		t.Fatalf("expected 3 rows (empty, rows, rows@wide), got %d: %+v", len(cat.Stories), cat.Stories)
	}
	byKey := map[string]Story{}
	for _, s := range cat.Stories {
		byKey[s.Key] = s
	}
	rows := byKey["list/rows"]
	if rows.Source != SourceManifest || rows.Name != "story_list_rows" || rows.Status != "passed" || rows.RunID != "run-story_list_rows" {
		t.Fatalf("rows = %+v", rows)
	}
	if rows.Golden != GoldenChanged {
		t.Fatalf("golden = %q, want changed", rows.Golden)
	}
	var snap *Snapshot
	for i := range rows.Snapshots {
		if rows.Snapshots[i].Name == "rows" {
			snap = &rows.Snapshots[i]
		}
	}
	if snap == nil || snap.Golden != GoldenChanged || snap.Changed != 1 || len(snap.Diff) != 1 || snap.GoldenScreen == nil {
		t.Fatalf("rows snapshot = %+v", snap)
	}
	if snap.Diff[0].X != 1 || snap.Diff[0].Before.Char != "o" || snap.Diff[0].After.Char != "i" {
		t.Fatalf("diff = %+v", snap.Diff)
	}
	if !strings.Contains(snap.SVG(false, false, false, true), `id="diff"`) {
		t.Fatal("diff SVG should carry the diff overlay")
	}
	wide := byKey["list/rows@wide"]
	if wide.Variant != "wide" || wide.Status != "not_run" || wide.Cols != 120 || wide.Golden != GoldenMissing {
		t.Fatalf("wide = %+v", wide)
	}
	empty := byKey["list/empty"]
	if empty.Status != "not_run" || empty.Cols != 100 || empty.Feature != "list" {
		t.Fatalf("empty = %+v", empty)
	}
	sum := cat.Summarize()
	if sum.Stories != 3 || sum.Passed != 1 || sum.NotRun != 2 || sum.Changed != 1 || sum.Missing != 2 {
		t.Fatalf("summary = %+v", sum)
	}

	// The JSON envelope carries golden state but no cell grids.
	data, err := json.Marshal(cat)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"golden":"changed"`) || strings.Contains(string(data), `"cells"`) {
		t.Fatalf("json envelope = %s", data)
	}
}

func TestReadIndexSkipsBrokenEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "broken"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken", IndexFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteIndexEntry(root, IndexEntry{Key: "a", SpecName: "story_a", Status: "passed"}, ""); err != nil {
		t.Fatal(err)
	}
	idx := ReadIndex(root)
	if len(idx) != 1 || idx["story_a"].Key != "a" || idx["story_a"].SchemaVersion != 1 {
		t.Fatalf("index = %+v", idx)
	}
}

func TestGoldenTextFallbackWhenJSONMissing(t *testing.T) {
	dir := t.TempDir()
	writeSpec(t, dir, "stories.yml", "version: 1\nkind: stories\nharness:\n  cmd: [\"/bin/echo\"]\nstories:\n  - id: list/rows\n")
	runs := filepath.Join(dir, "runs")
	writeRun(t, runs, "story_list_rows", twoCells("h", "i"), "rows")
	goldenDir := filepath.Join(dir, "goldens", "story_list_rows")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goldenDir, "rows.txt"), []byte("hx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := Collect(CollectOptions{Paths: []string{dir}, ArtifactRoot: runs, SnapshotRoot: filepath.Join(dir, "goldens"), DefaultTerminal: spec.Terminal{Cols: 80, Rows: 24}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Stories) != 1 || cat.Stories[0].Golden != GoldenChanged {
		t.Fatalf("text-only golden should still diff: %+v", cat.Stories)
	}
	var changed int
	for _, s := range cat.Stories[0].Snapshots {
		if s.Name == "rows" {
			changed = s.Changed
		}
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
}

func TestGoldenModeDecidesWhetherStylesCount(t *testing.T) {
	for _, tc := range []struct {
		mode      string
		wantState string
		styleOnly int
	}{
		{"", GoldenMatch, 1},
		{"cell", GoldenChanged, 0},
	} {
		dir := t.TempDir()
		manifest := "version: 1\nkind: stories\nharness:\n  cmd: [\"/bin/echo\"]\n"
		if tc.mode != "" {
			manifest += "defaults:\n  goldenMode: " + tc.mode + "\n"
		}
		manifest += "stories:\n  - id: list/rows\n"
		writeSpec(t, dir, "stories.yml", manifest)
		runs := filepath.Join(dir, "runs")
		current := twoCells("h", "i")
		current.Cells[1].Style.Bold = true // same text as the golden, bold differs
		writeRun(t, runs, "story_list_rows", current, "rows")
		goldenDir := filepath.Join(dir, "goldens", "story_list_rows")
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatal(err)
		}
		golden, _ := json.Marshal(twoCells("h", "i"))
		if err := os.WriteFile(filepath.Join(goldenDir, "rows.json"), golden, 0o644); err != nil {
			t.Fatal(err)
		}
		cat, err := Collect(CollectOptions{Paths: []string{dir}, ArtifactRoot: runs, SnapshotRoot: filepath.Join(dir, "goldens"), DefaultTerminal: spec.Terminal{Cols: 80, Rows: 24}})
		if err != nil {
			t.Fatal(err)
		}
		if len(cat.Stories) != 1 || cat.Stories[0].Golden != tc.wantState {
			t.Fatalf("mode %q: golden = %+v, want %s", tc.mode, cat.Stories, tc.wantState)
		}
		for _, s := range cat.Stories[0].Snapshots {
			if s.Name == "rows" && (s.StyleOnly != tc.styleOnly || s.Changed != 1) {
				t.Fatalf("mode %q: snapshot = %+v", tc.mode, s)
			}
		}
	}
}

func TestGoldenJSONModeCountsCursorChanges(t *testing.T) {
	dir := t.TempDir()
	manifest := "version: 1\nkind: stories\nharness:\n  cmd: [\"/bin/echo\"]\ndefaults:\n  goldenMode: json\nstories:\n  - id: list/rows\n"
	writeSpec(t, dir, "stories.yml", manifest)
	runs := filepath.Join(dir, "runs")
	current := twoCells("h", "i")
	current.Cursor = terminal.Cursor{X: 1, Visible: true}
	writeRun(t, runs, "story_list_rows", current, "rows")
	goldenDir := filepath.Join(dir, "goldens", "story_list_rows")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	golden := twoCells("h", "i")
	golden.Cursor = terminal.Cursor{X: 0, Visible: true}
	data, _ := json.Marshal(golden)
	if err := os.WriteFile(filepath.Join(goldenDir, "rows.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := Collect(CollectOptions{Paths: []string{dir}, ArtifactRoot: runs, SnapshotRoot: filepath.Join(dir, "goldens")})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot Snapshot
	for _, candidate := range cat.Stories[0].Snapshots {
		if candidate.Name == "rows" {
			snapshot = candidate
		}
	}
	if snapshot.Golden != GoldenChanged || !snapshot.CursorChanged || snapshot.Changed != 0 {
		t.Fatalf("cursor-only JSON golden diff = %+v", snapshot)
	}
	payload := BuildPayload(cat, false, "")
	if !payload.Stories[0].Snapshots[1].CursorChanged || payload.Stories[0].Snapshots[1].GoldenCursor == nil {
		t.Fatalf("cursor diff missing from page payload: %+v", payload.Stories[0].Snapshots)
	}
}
