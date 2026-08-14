package inventory

import (
	"regexp"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// Item is a compact authoring hint derived from a rendered screen.
type Item struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Row     int    `json:"row"`
	Text    string `json:"text"`
	Hotkey  string `json:"hotkey,omitempty"`
	Suggest string `json:"suggest,omitempty"`
}

// Report is the `glyph snapshot inventory` payload.
type Report struct {
	SchemaVersion int    `json:"schemaVersion"`
	Cols          int    `json:"cols"`
	Rows          int    `json:"rows"`
	Cursor        string `json:"cursor"`
	Items         []Item `json:"items"`
}

var hotkeyRe = regexp.MustCompile(`(?i)(?:\^([A-Z])|<([A-Z0-9]+)>|\b([a-z])\s*[:=]\s*|\[([A-Z]+)\])`)

// FromSnapshot extracts row-level text, prompt markers, and hotkey-ish tokens.
func FromSnapshot(snap terminal.ScreenSnapshot) Report {
	lines := strings.Split(snap.Text, "\n")
	items := make([]Item, 0)
	for i, raw := range lines {
		line := strings.TrimRight(raw, " ")
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		item := Item{
			Kind:    "row",
			Name:    "row_" + itoa(i),
			Row:     i,
			Text:    trim,
			Suggest: `wait: { screen: { contains: "` + escapeYAML(clip(trim, 40)) + `" } }`,
		}
		if hk := firstHotkey(trim); hk != "" {
			item.Kind = "hotkey"
			item.Hotkey = hk
			item.Suggest = `press: "` + hk + `"`
		} else if isPrompt(trim) {
			item.Kind = "prompt"
		}
		items = append(items, item)
	}
	return Report{
		SchemaVersion: 1,
		Cols:          snap.Cols,
		Rows:          snap.Rows,
		Cursor:        itoa(snap.Cursor.Y) + "," + itoa(snap.Cursor.X),
		Items:         items,
	}
}

func firstHotkey(s string) string {
	m := hotkeyRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	for _, g := range m[1:] {
		if g != "" {
			if len(g) == 1 && g[0] >= 'A' && g[0] <= 'Z' && strings.Contains(s, "^") {
				return "ctrl+" + strings.ToLower(g)
			}
			return strings.ToLower(g)
		}
	}
	return ""
}

func isPrompt(s string) bool {
	if s == "" {
		return false
	}
	last := s[len(s)-1]
	return last == '>' || last == '$' || last == ':' || last == '?'
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n]))
}

func escapeYAML(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
