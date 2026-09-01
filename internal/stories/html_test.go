package stories

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

func TestRenderHTMLIncludesStoryAndNotRun(t *testing.T) {
	screen := &terminal.ScreenSnapshot{Cols: 2, Rows: 1, Cells: []terminal.Cell{{X: 0, Y: 0, Char: "h", Width: 1}, {X: 1, Y: 0, Char: "i", Width: 1}}}
	golden := &terminal.ScreenSnapshot{Cols: 2, Rows: 1, Cells: []terminal.Cell{{X: 0, Y: 0, Char: "h", Width: 1}, {X: 1, Y: 0, Char: "o", Width: 1}}}
	cat := Catalog{
		SchemaVersion: 1,
		Stories: []Story{
			{
				Key: "list/empty", ID: "list/empty", Name: "story_list_empty", Source: SourceManifest,
				Status: "passed", RunID: "run-1", Golden: GoldenChanged,
				Snapshots: []Snapshot{
					{
						Name: "empty", Status: "ok", Cols: 2, Rows: 1, Golden: GoldenChanged, Changed: 1,
						Screen: screen, GoldenScreen: golden,
						Diff: terminal.DiffSnapshots(*golden, *screen).Changed,
					},
				},
			},
			{Key: "list/rows", ID: "list/rows", Name: "story_list_rows", Source: SourceManifest, Status: "not_run", Golden: GoldenMissing},
		},
	}
	html := RenderHTML(cat)
	for _, want := range []string{
		"list/empty",
		"list/rows",
		"not_run",
		`"golden":"changed"`,
		`"goldenGrid":[["h","o"]]`,
		`"grid":[["h","i"]]`,
		`"diff":[{"x":1,"y":0`,
		`"live":false`,
		"glyph stories",
		"x-data=\"catalog\"",
		"window.__GLYPH_STORIES__",
		"alpine:init",
		"__glyphRenderSVG",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
	if strings.Contains(html, `"svgPlain"`) || strings.Contains(html, `"svgGrid"`) {
		t.Errorf("the page should paint screens client-side, not inline pre-rendered SVGs")
	}
	if !strings.Contains(html, "bg-ink") {
		t.Errorf("expected Tailwind-shaped utility classes on the page")
	}
	if !strings.Contains(html, ".bg-ink") {
		t.Errorf("expected hand-written .bg-ink utility in the inlined CSS")
	}
	live := RenderHTMLPayload(BuildPayload(cat, true, "2026-01-01T00:00:00Z"))
	if !strings.Contains(live, `"live":true`) || !strings.Contains(live, "/events") {
		t.Errorf("live page should declare live mode and the SSE endpoint")
	}
}
