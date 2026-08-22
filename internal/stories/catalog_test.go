package stories

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func writeRun(t *testing.T, root, specName string, snap terminal.ScreenSnapshot) {
	t.Helper()
	dir := filepath.Join(root, "run-"+specName)
	if err := os.MkdirAll(filepath.Join(dir, "screens"), 0o755); err != nil {
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
		"outcomes":      []any{},
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
}

func TestCollectJoinsLatestRunAndPrefersStoryTag(t *testing.T) {
	dir := t.TempDir()
	writeSpec(t, dir, "alpha.yml", sampleSpec("alpha", "list", "[story]"))
	writeSpec(t, dir, "beta.yml", sampleSpec("beta", "other", "[smoke]"))
	runs := filepath.Join(dir, "runs")
	snap := terminal.ScreenSnapshot{
		Cols: 2, Rows: 1,
		Cells: []terminal.Cell{{X: 0, Y: 0, Char: "h", Width: 1}, {X: 1, Y: 0, Char: "i", Width: 1}},
		Text:  "hi",
	}
	writeRun(t, runs, "alpha", snap)

	cat, err := Collect(CollectOptions{Paths: []string{dir}, ArtifactRoot: runs})
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Stories) != 1 {
		t.Fatalf("prefer story tag: got %d stories, want 1", len(cat.Stories))
	}
	st := cat.Stories[0]
	if st.Name != "alpha" || st.Status != "passed" || st.RunID != "run-alpha" {
		t.Fatalf("story = %+v", st)
	}
	if len(st.Snapshots) != 1 || st.Snapshots[0].Name != "final" || st.Snapshots[0].Status != "ok" {
		t.Fatalf("snapshots = %+v", st.Snapshots)
	}
	if st.Snapshots[0].SVGPlain == "" || st.Snapshots[0].SVGGrid == "" || st.Snapshots[0].SVGSpaces == "" {
		t.Fatalf("expected inspect SVGs to be populated")
	}
	if st.Snapshots[0].Screen == nil || st.Snapshots[0].Screen.Cols != 2 {
		t.Fatalf("expected Screen snapshot for TUI preview")
	}
	if !strings.Contains(st.Snapshots[0].SVGGrid, `id="grid"`) {
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

func TestCollectNoSpecs(t *testing.T) {
	dir := t.TempDir()
	_, err := Collect(CollectOptions{Paths: []string{dir}})
	if err != ErrNoSpecs {
		t.Fatalf("err = %v, want ErrNoSpecs", err)
	}
}
