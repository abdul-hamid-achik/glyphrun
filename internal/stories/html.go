package stories

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html/template"
)

// Static UI assets. stories.css is hand-written Tailwind-shaped utilities
// (same class names as page.html) so a future Vue + Tailwind app can reuse
// the markup. Alpine is vendored. Nothing here requires a JS/CSS toolchain
// at `task verify` or at `glyph stories --html` time.

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

// RenderHTML builds a self-contained catalog page: Alpine UI, utility CSS,
// and the snapshot JSON all inlined so the file works offline.
func RenderHTML(cat Catalog) string {
	type snapPayload struct {
		Name      string          `json:"name"`
		Status    string          `json:"status"`
		Cols      int             `json:"cols"`
		Rows      int             `json:"rows"`
		Error     string          `json:"error,omitempty"`
		SVGPlain  string          `json:"svgPlain"`
		SVGGrid   string          `json:"svgGrid"`
		SVGSpaces string          `json:"svgSpaces"`
		Cells     json.RawMessage `json:"cells"`
	}
	type storyPayload struct {
		Name      string        `json:"name"`
		Feature   string        `json:"feature"`
		Status    string        `json:"status"`
		RunID     string        `json:"runId"`
		Snapshots []snapPayload `json:"snapshots"`
	}
	payload := make([]storyPayload, 0, len(cat.Stories))
	for _, s := range cat.Stories {
		sp := storyPayload{Name: s.Name, Feature: s.Feature, Status: s.Status, RunID: s.RunID}
		for _, snap := range s.Snapshots {
			cells, _ := json.Marshal(snap.Cells)
			if cells == nil {
				cells = []byte("[]")
			}
			sp.Snapshots = append(sp.Snapshots, snapPayload{
				Name:      snap.Name,
				Status:    snap.Status,
				Cols:      snap.Cols,
				Rows:      snap.Rows,
				Error:     snap.Error,
				SVGPlain:  snap.SVGPlain,
				SVGGrid:   snap.SVGGrid,
				SVGSpaces: snap.SVGSpaces,
				Cells:     cells,
			})
		}
		payload = append(payload, sp)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte("[]")
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
