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

func TestSnapshotSVGColors(t *testing.T) {
	s := snap("x")
	s.Cells[0].Style = terminal.Style{Fg: "red", Bg: "blue"}
	out := SnapshotSVG(s, DefaultOptions())
	// Named colors resolve through the palette to hex.
	if !strings.Contains(out, `fill="#cc342b"`) { // red
		t.Errorf("expected red foreground hex in output:\n%s", out)
	}
	if !strings.Contains(out, `fill="#3971ed"/>`) { // blue background rect
		t.Errorf("expected blue background rect in output:\n%s", out)
	}
}

func TestSnapshotSVGTruecolorAnd256(t *testing.T) {
	s := snap("ab")
	s.Cells[0].Style = terminal.Style{Fg: "#ff8800"} // truecolor passthrough
	s.Cells[1].Style = terminal.Style{Fg: "201"}     // 256-palette index
	out := SnapshotSVG(s, DefaultOptions())
	if !strings.Contains(out, `fill="#ff8800"`) {
		t.Errorf("expected truecolor fg passthrough:\n%s", out)
	}
	if !strings.Contains(out, xterm256Hex(201)) {
		t.Errorf("expected 256-index color %s in output:\n%s", xterm256Hex(201), out)
	}
}

func TestXterm256Hex(t *testing.T) {
	tests := map[int]string{
		16:  "#000000",
		231: "#ffffff",
		232: "#080808",
		255: "#eeeeee",
	}
	for idx, want := range tests {
		if got := xterm256Hex(idx); got != want {
			t.Errorf("xterm256Hex(%d) = %s, want %s", idx, got, want)
		}
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

func TestSnapshotSVGDefaultOmitsInspectOverlays(t *testing.T) {
	s := snap("hi  ", "    ")
	out := SnapshotSVG(s, DefaultOptions())
	for _, banned := range []string{"id=\"grid\"", "id=\"rulers\"", "id=\"regions\"", "·"} {
		if strings.Contains(out, banned) {
			t.Errorf("default SVG must not contain inspect overlay %q:\n%s", banned, out)
		}
	}
}

func TestSnapshotSVGShowSpacesDrawsMiddleDots(t *testing.T) {
	s := snap("a b")
	opts := DefaultOptions()
	opts.ShowSpaces = true
	out := SnapshotSVG(s, opts)
	if !strings.Contains(out, "·") {
		t.Fatalf("ShowSpaces should draw middle dots for space cells:\n%s", out)
	}
	if !strings.Contains(out, "<text") {
		t.Fatalf("ShowSpaces should still emit <text> for mixed runs:\n%s", out)
	}
}

func TestSnapshotSVGShowSpacesPaintsBlankRuns(t *testing.T) {
	s := snap("   ")
	opts := DefaultOptions()
	opts.ShowSpaces = true
	out := SnapshotSVG(s, opts)
	if !strings.Contains(out, "<text") {
		t.Fatalf("ShowSpaces should emit <text> for a blank row:\n%s", out)
	}
	if !strings.Contains(out, "···") && !strings.Contains(out, "·") {
		t.Fatalf("blank row with ShowSpaces should contain middle dots:\n%s", out)
	}
}

func TestSnapshotSVGShowGridDeterministic(t *testing.T) {
	s := snap("ab", "cd")
	opts := DefaultOptions()
	opts.ShowGrid = true
	a := SnapshotSVG(s, opts)
	b := SnapshotSVG(s, opts)
	if a != b {
		t.Fatalf("grid overlay is not deterministic")
	}
	if !strings.Contains(a, `id="grid"`) {
		t.Fatalf("ShowGrid should emit a grid group:\n%s", a)
	}
}

func TestSnapshotSVGShowRulers(t *testing.T) {
	s := snap("hello")
	opts := DefaultOptions()
	opts.ShowRulers = true
	out := SnapshotSVG(s, opts)
	if !strings.Contains(out, `id="rulers"`) {
		t.Fatalf("ShowRulers should emit a rulers group:\n%s", out)
	}
	plain := SnapshotSVG(s, DefaultOptions())
	if !(strings.Contains(out, `width="`) && len(out) > len(plain)) {
		t.Fatalf("rulers should grow the SVG compared to the default render")
	}
}

func TestSnapshotSVGRegions(t *testing.T) {
	s := snap("hello")
	opts := DefaultOptions()
	opts.Regions = []RegionHighlight{{X: 1, Y: 0, Width: 3, Height: 1}}
	out := SnapshotSVG(s, opts)
	if !strings.Contains(out, `id="regions"`) {
		t.Fatalf("Regions should emit a regions group:\n%s", out)
	}
	if !strings.Contains(out, `data-x="1"`) || !strings.Contains(out, `data-y="0"`) {
		t.Fatalf("region rect should carry cell coordinates:\n%s", out)
	}
}

// TestSnapshotSVGUsesTerminalDefaultColors checks that colors the target set
// with OSC 10/11 replace the theme's canvas colors, that cursor and reverse
// video follow them, and that unset defaults leave the theme untouched.
func TestSnapshotSVGUsesTerminalDefaultColors(t *testing.T) {
	base := snap("hi")
	base.Cursor = terminal.Cursor{X: 0, Y: 0, Visible: true}
	withReverse := snap("hi")
	withReverse.Cursor = terminal.Cursor{Visible: false}
	withReverse.Cells[0].Style.Reverse = true

	tests := []struct {
		name      string
		snap      terminal.ScreenSnapshot
		fg, bg    string
		want      []string
		wantNot   []string
		wantEqual bool // output must equal the render of the snapshot without defaults
	}{
		{
			name: "background and foreground",
			snap: base, fg: "#d8dee9", bg: "#2e3440",
			want:    []string{`<rect width="40" height="40" fill="#2e3440"/>`, `fill="#d8dee9"`, `stroke="#d8dee9"`},
			wantNot: []string{"#1d1f21", "#c5c8c6"},
		},
		{
			name: "background only keeps the theme foreground",
			snap: base, bg: "#2e3440",
			want:    []string{`fill="#2e3440"`, `fill="#c5c8c6"`},
			wantNot: []string{`<rect width="40" height="40" fill="#1d1f21"/>`},
		},
		{
			name: "reverse video swaps the app's colors",
			snap: withReverse, fg: "#d8dee9", bg: "#2e3440",
			want: []string{`<rect x="10" y="10" width="10" height="20" fill="#d8dee9"/>`, `fill="#2e3440"`},
		},
		{
			name: "invalid values are ignored", snap: base, fg: `"/><script>`, bg: "red",
			wantEqual: true,
		},
		{name: "unset changes nothing", snap: base, wantEqual: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.snap
			s.DefaultFg, s.DefaultBg = tc.fg, tc.bg
			out := SnapshotSVG(s, DefaultOptions())
			if again := SnapshotSVG(s, DefaultOptions()); again != out {
				t.Fatal("render with default colors is not deterministic")
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in:\n%s", w, out)
				}
			}
			for _, w := range tc.wantNot {
				if strings.Contains(out, w) {
					t.Errorf("unexpected %q in:\n%s", w, out)
				}
			}
			if tc.wantEqual {
				plain := tc.snap
				plain.DefaultFg, plain.DefaultBg = "", ""
				if want := SnapshotSVG(plain, DefaultOptions()); out != want {
					t.Errorf("output changed without valid defaults:\n%s\nwant:\n%s", out, want)
				}
			}
		})
	}
}

// TestSnapshotSVGDefaultColorsKeepCellColors makes sure explicit cell colors are
// not replaced by the terminal defaults, only the canvas and unset colors are.
func TestSnapshotSVGDefaultColorsKeepCellColors(t *testing.T) {
	s := snap("ab")
	s.DefaultBg, s.DefaultFg = "#2e3440", "#d8dee9"
	s.Cells[1].Style = terminal.Style{Bg: "#3b4252", Fg: "red"}
	out := SnapshotSVG(s, DefaultOptions())
	for _, w := range []string{`fill="#3b4252"`, `fill="#2e3440"`, `fill="#cc342b"`} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
}
