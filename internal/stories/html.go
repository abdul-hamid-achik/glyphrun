package stories

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html/template"

	"github.com/abdul-hamid-achik/glyphrun/internal/render"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// Static UI assets. stories.css is hand-written Tailwind-shaped utilities
// (same class names as page.html) so a future Vue + Tailwind app can reuse
// the markup. Alpine is vendored. Nothing here requires a JS/CSS toolchain
// at `task verify` or at `glyph stories --html` time.
//
// The page paints every screen client-side from the cell grid (app.js ports
// render.SnapshotSVG), so the payload carries one grid per snapshot instead
// of three pre-rendered SVGs. That is what keeps a 100-story catalog small.

//go:embed page.html
var pageHTML string

//go:embed assets/stories.css
var storiesCSS string

//go:embed assets/app.js
var appJS string

//go:embed assets/alpine.min.js
var alpineJS string

var pageTmpl = template.Must(template.New("page.html").Parse(pageHTML))

type pageData struct {
	CSS     template.CSS
	Payload template.JS
	App     template.JS
	Alpine  template.JS
}

// PagePayload is the JSON the page boots from. Live is set by `glyph stories
// serve`, which then streams catalog updates over /events.
type PagePayload struct {
	SchemaVersion int            `json:"schemaVersion"`
	Live          bool           `json:"live"`
	GeneratedAt   string         `json:"generatedAt,omitempty"`
	Summary       Summary        `json:"summary"`
	Palette       map[string]any `json:"palette"`
	Stories       []StoryPayload `json:"stories"`
}

// StoryPayload is one story as the page sees it.
type StoryPayload struct {
	Key        string            `json:"key"`
	ID         string            `json:"id,omitempty"`
	Variant    string            `json:"variant,omitempty"`
	Name       string            `json:"name"`
	Source     string            `json:"source"`
	Path       string            `json:"path"`
	Feature    string            `json:"feature"`
	Intent     string            `json:"intent,omitempty"`
	Status     string            `json:"status"`
	Diagnostic string            `json:"diagnostic,omitempty"`
	RunID      string            `json:"runId"`
	Golden     string            `json:"golden"`
	Passed     int               `json:"passed"`
	Failed     int               `json:"failed"`
	ParseError string            `json:"parseError,omitempty"`
	Cols       int               `json:"cols"`
	Rows       int               `json:"rows"`
	Snapshots  []SnapshotPayload `json:"snapshots"`
}

// SnapshotPayload is one screen: a compact grid, cursor, regions, and the
// golden diff. Grid is rows×cols of characters; Styles and Links are runs
// over cells that carry a non-default style or an OSC 8 link, so a mostly
// plain 80×24 screen costs a few kilobytes instead of a hundred.
type SnapshotPayload struct {
	Name       string                   `json:"name"`
	Status     string                   `json:"status"`
	Cols       int                      `json:"cols"`
	Rows       int                      `json:"rows"`
	Error      string                   `json:"error,omitempty"`
	Golden     string                   `json:"golden"`
	Changed    int                      `json:"changed"`
	StyleOnly  int                      `json:"styleOnly,omitempty"`
	Cursor     terminal.Cursor          `json:"cursor"`
	Grid       [][]string               `json:"grid"`
	Styles     []StyleRun               `json:"styles,omitempty"`
	Links      []LinkRun                `json:"links,omitempty"`
	GoldenGrid [][]string               `json:"goldenGrid,omitempty"`
	GoldenSt   []StyleRun               `json:"goldenStyles,omitempty"`
	Regions    []render.RegionHighlight `json:"regions,omitempty"`
	Diff       []terminal.CellDiff      `json:"diff,omitempty"`
}

// StyleRun is a horizontal run of cells sharing one non-default style.
type StyleRun struct {
	X     int            `json:"x"`
	Y     int            `json:"y"`
	Len   int            `json:"len"`
	Style terminal.Style `json:"style"`
}

// LinkRun is a horizontal run of cells carrying one hyperlink.
type LinkRun struct {
	X   int    `json:"x"`
	Y   int    `json:"y"`
	Len int    `json:"len"`
	URL string `json:"url"`
}

// CompactScreen encodes a snapshot as grid + style/link runs.
func CompactScreen(screen terminal.ScreenSnapshot) (grid [][]string, styles []StyleRun, links []LinkRun) {
	cols, rows := screen.Cols, screen.Rows
	if cols <= 0 || rows <= 0 {
		return [][]string{}, nil, nil
	}
	at := func(x, y int) terminal.Cell {
		idx := y*cols + x
		if idx < 0 || idx >= len(screen.Cells) {
			return terminal.Cell{X: x, Y: y, Char: " ", Width: 1}
		}
		return screen.Cells[idx]
	}
	grid = make([][]string, rows)
	for y := 0; y < rows; y++ {
		row := make([]string, cols)
		for x := 0; x < cols; x++ {
			c := at(x, y)
			ch := c.Char
			if ch == "" {
				ch = " "
			}
			row[x] = ch
		}
		grid[y] = row
		x := 0
		for x < cols {
			c := at(x, y)
			if c.Style == (terminal.Style{}) {
				x++
				continue
			}
			start := x
			for x < cols && at(x, y).Style == c.Style {
				x++
			}
			styles = append(styles, StyleRun{X: start, Y: y, Len: x - start, Style: c.Style})
		}
		x = 0
		for x < cols {
			c := at(x, y)
			if c.Link == "" {
				x++
				continue
			}
			start := x
			for x < cols && at(x, y).Link == c.Link {
				x++
			}
			links = append(links, LinkRun{X: start, Y: y, Len: x - start, URL: c.Link})
		}
	}
	return grid, styles, links
}

// BuildPayload flattens a catalog into the page payload.
func BuildPayload(cat Catalog, live bool, generatedAt string) PagePayload {
	theme := render.DefaultTheme()
	palette := map[string]any{
		"background": theme.Background,
		"foreground": theme.Foreground,
		"cursor":     theme.Cursor,
		"colors":     theme.Palette,
	}
	out := PagePayload{SchemaVersion: 1, Live: live, GeneratedAt: generatedAt, Summary: cat.Summarize(), Palette: palette}
	out.Stories = make([]StoryPayload, 0, len(cat.Stories))
	for _, s := range cat.Stories {
		sp := StoryPayload{
			Key: s.Key, ID: s.ID, Variant: s.Variant, Name: s.Name, Source: s.Source, Path: s.Path,
			Feature: s.Feature, Intent: s.Intent, Status: s.Status, Diagnostic: s.Diagnostic, RunID: s.RunID,
			Golden: s.Golden, Passed: s.Passed, Failed: s.Failed, ParseError: s.ParseError, Cols: s.Cols, Rows: s.Rows,
		}
		sp.Snapshots = make([]SnapshotPayload, 0, len(s.Snapshots))
		for _, snap := range s.Snapshots {
			p := SnapshotPayload{
				Name: snap.Name, Status: snap.Status, Cols: snap.Cols, Rows: snap.Rows, Error: snap.Error,
				Golden: snap.Golden, Changed: snap.Changed, StyleOnly: snap.StyleOnly, Regions: snap.Regions, Diff: snap.Diff,
			}
			if snap.Screen != nil {
				p.Grid, p.Styles, p.Links = CompactScreen(*snap.Screen)
				p.Cursor = snap.Screen.Cursor
			}
			if snap.GoldenScreen != nil && (snap.Golden == GoldenChanged || snap.StyleOnly > 0) {
				p.GoldenGrid, p.GoldenSt, _ = CompactScreen(*snap.GoldenScreen)
			}
			if p.Grid == nil {
				p.Grid = [][]string{}
			}
			sp.Snapshots = append(sp.Snapshots, p)
		}
		out.Stories = append(out.Stories, sp)
	}
	return out
}

// RenderHTML builds a self-contained catalog page: Alpine UI, utility CSS,
// and the snapshot JSON all inlined so the file works offline.
func RenderHTML(cat Catalog) string {
	return RenderHTMLPayload(BuildPayload(cat, false, ""))
}

// RenderHTMLPayload renders the page from a prepared payload (used by the
// live server, which sets Live and refreshes the payload over /events).
func RenderHTMLPayload(payload PagePayload) string {
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte(`{"schemaVersion":1,"stories":[]}`)
	}
	var buf bytes.Buffer
	_ = pageTmpl.Execute(&buf, pageData{
		CSS:     template.CSS(storiesCSS),
		Payload: template.JS(body),
		App:     template.JS(appJS),
		Alpine:  template.JS(alpineJS),
	})
	return buf.String()
}
