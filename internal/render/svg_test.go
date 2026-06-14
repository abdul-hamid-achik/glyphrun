package render

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// snap builds a small snapshot from rows of text with all-default styles.
func snap(rows ...string) terminal.ScreenSnapshot {
	cols := 0
	for _, r := range rows {
		if len([]rune(r)) > cols {
			cols = len([]rune(r))
		}
	}
	out := terminal.ScreenSnapshot{Cols: cols, Rows: len(rows)}
	for y, r := range rows {
		runes := []rune(r)
		for x := 0; x < cols; x++ {
			ch := " "
			if x < len(runes) {
				ch = string(runes[x])
			}
			out.Cells = append(out.Cells, terminal.Cell{X: x, Y: y, Char: ch, Width: 1})
		}
	}
	out.Text = strings.Join(rows, "\n")
	return out
}

func TestSnapshotSVGDeterministic(t *testing.T) {
	s := snap("hello", "world")
	a := SnapshotSVG(s, DefaultOptions())
	b := SnapshotSVG(s, DefaultOptions())
	if a != b {
		t.Fatalf("render is not deterministic:\n%s\n---\n%s", a, b)
	}
}

func TestSnapshotSVGContainsText(t *testing.T) {
	tests := []struct {
		name string
		rows []string
		want []string
	}{
		{name: "plain", rows: []string{"hello"}, want: []string{"<svg", "hello", "</svg>"}},
		{name: "two rows", rows: []string{"ab", "cd"}, want: []string{">ab</text>", ">cd</text>"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := SnapshotSVG(snap(tc.rows...), DefaultOptions())
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("expected SVG to contain %q\ngot:\n%s", w, out)
				}
			}
		})
	}
}

func TestSnapshotSVGEscapesXML(t *testing.T) {
	s := snap("a<b&c>d")
	out := SnapshotSVG(s, DefaultOptions())
	if strings.Contains(out, "a<b&c>d") {
		t.Fatalf("raw XML metacharacters leaked into output:\n%s", out)
	}
	for _, want := range []string{"&lt;", "&amp;", "&gt;"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected escaped %q in output", want)
		}
	}
}

func TestSnapshotSVGStyleAttributes(t *testing.T) {
	s := snap("x")
	s.Cells[0].Style = terminal.Style{Bold: true, Underline: true, Italic: true}
	out := SnapshotSVG(s, DefaultOptions())
	for _, want := range []string{`font-weight="bold"`, `text-decoration="underline"`, `font-style="italic"`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in styled output:\n%s", want, out)
		}
	}
}

func TestSnapshotSVGReverseSwapsColors(t *testing.T) {
	s := snap("x")
	s.Cells[0].Style = terminal.Style{Reverse: true}
	out := SnapshotSVG(s, DefaultOptions())
	// Reverse should paint a background rect using the theme foreground color.
	if !strings.Contains(out, `fill="`+DefaultTheme().Foreground+`"/>`) {
		t.Errorf("expected reverse cell to paint a foreground-colored background rect:\n%s", out)
	}
}

func TestSnapshotSVGBlankScreenHasNoText(t *testing.T) {
	s := snap("   ", "   ")
	out := SnapshotSVG(s, DefaultOptions())
	if strings.Contains(out, "<text") {
		t.Errorf("blank screen should emit no <text> elements:\n%s", out)
	}
}

func TestSnapshotSVGCursorOutline(t *testing.T) {
	s := snap("hi")
	s.Cursor = terminal.Cursor{X: 1, Y: 0, Visible: true}
	out := SnapshotSVG(s, DefaultOptions())
	if !strings.Contains(out, `stroke="`+DefaultTheme().Cursor+`"`) {
		t.Errorf("expected cursor outline in output:\n%s", out)
	}
	hidden := snap("hi")
	hidden.Cursor = terminal.Cursor{X: 1, Y: 0, Visible: false}
	if strings.Contains(SnapshotSVG(hidden, DefaultOptions()), `stroke-width="1"`) {
		t.Errorf("hidden cursor should not render an outline")
	}
}
