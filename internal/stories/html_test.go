package stories

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

func TestRenderHTMLIncludesStoryAndNotRun(t *testing.T) {
	cat := Catalog{
		SchemaVersion: 1,
		Stories: []Story{
			{
				Name:   "list_empty",
				Status: "run",
				RunID:  "run-1",
				Snapshots: []Snapshot{
					{
						Name:     "final",
						Status:   "ok",
						Cols:     2,
						Rows:     1,
						SVGPlain: "<svg xmlns='http://www.w3.org/2000/svg'></svg>",
						SVGGrid:  `<svg xmlns='http://www.w3.org/2000/svg'><g id="grid"></g></svg>`,
						Cells:    []terminal.Cell{{X: 0, Y: 0, Char: "h", Width: 1}},
					},
				},
			},
			{Name: "list_rows", Status: "not_run"},
		},
	}
	html := RenderHTML(cat)
	for _, want := range []string{
		"list_empty",
		"list_rows",
		"not_run",
		`"svgGrid"`,
		"glyph stories",
		"x-data=\"catalog\"",
		"window.__GLYPH_STORIES__",
		"alpine:init",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
	if !strings.Contains(html, "bg-ink") {
		t.Errorf("expected Tailwind-shaped utility classes on the page")
	}
	if !strings.Contains(html, ".bg-ink") {
		t.Errorf("expected hand-written .bg-ink utility in the inlined CSS")
	}
}
