package terminal_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal/adapters/fake"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal/adapters/gote"
)

// adapters lists both emulator adapters. They must keep scrollback the same
// way, so every table below runs against each.
var adapters = []struct {
	name string
	new  func(cols, rows int, opts terminal.Options) terminal.Emulator
}{
	{"gote", gote.NewWithOptions},
	{"fake", fake.NewWithOptions},
}

func feed(t *testing.T, em terminal.Emulator, chunks ...string) {
	t.Helper()
	for _, c := range chunks {
		if _, err := em.Feed([]byte(c)); err != nil {
			t.Fatal(err)
		}
	}
}

func reader(t *testing.T, em terminal.Emulator) terminal.ScrollbackReader {
	t.Helper()
	r, ok := em.(terminal.ScrollbackReader)
	if !ok {
		t.Fatalf("%T does not implement terminal.ScrollbackReader", em)
	}
	return r
}

func TestScrollbackCollectsRowsThatScrollOffTheTop(t *testing.T) {
	const esc = "\x1b"
	tests := []struct {
		name       string
		cols, rows int
		opts       terminal.Options
		input      []string
		wantLines  []string // scrollback text, oldest first
		wantScreen string
		wantDrop   int
	}{
		{
			name: "line feed at the bottom margin", cols: 10, rows: 3,
			input:     []string{"a\r\nb\r\nc\r\nd\r\ne"},
			wantLines: []string{"a", "b"}, wantScreen: "c\nd\ne",
		},
		{
			name: "nothing scrolls while the screen has room", cols: 10, rows: 3,
			input:     []string{"a\r\nb\r\nc"},
			wantLines: nil, wantScreen: "a\nb\nc",
		},
		{
			name: "IND at the bottom margin", cols: 10, rows: 2,
			input:     []string{"a\r\nb", esc + "D" + "c"},
			wantLines: []string{"a"}, wantScreen: "b\n c",
		},
		{
			name: "NEL at the bottom margin", cols: 10, rows: 2,
			input:     []string{"a\r\nb", esc + "E" + "c"},
			wantLines: []string{"a"}, wantScreen: "b\nc",
		},
		{
			name: "SU scrolls n rows off", cols: 10, rows: 4,
			input:     []string{"a\r\nb\r\nc\r\nd", esc + "[2S"},
			wantLines: []string{"a", "b"}, wantScreen: "c\nd",
		},
		{
			name: "SU larger than the screen adds each row once", cols: 10, rows: 3,
			input:     []string{"a\r\nb\r\nc", esc + "[99S"},
			wantLines: []string{"a", "b", "c"}, wantScreen: "",
		},
		{
			name: "autowrap at the bottom-right cell scrolls", cols: 2, rows: 2,
			input:     []string{"abcde"},
			wantLines: []string{"ab"}, wantScreen: "cd\ne",
		},
		{
			name: "painting the bottom-right cell does not scroll", cols: 2, rows: 2,
			input:     []string{"abcd"},
			wantLines: nil, wantScreen: "ab\ncd",
		},
		{
			name: "region below row 0 adds nothing", cols: 10, rows: 4,
			input:     []string{"a\r\nb\r\nc\r\nd", esc + "[2;3r", esc + "[3;1H\n"},
			wantLines: nil, wantScreen: "a\nc\n\nd",
		},
		{
			name: "region reaching the bottom but not starting at row 0 adds nothing", cols: 10, rows: 3,
			input:     []string{"a\r\nb\r\nc", esc + "[2;3r", esc + "[3;1H\n"},
			wantLines: nil, wantScreen: "a\nc",
		},
		{
			name: "region starting at row 0 adds the top row", cols: 10, rows: 4,
			input:     []string{"a\r\nb\r\nc\r\nd", esc + "[1;2r", esc + "[2;1H\n"},
			wantLines: []string{"a"}, wantScreen: "b\n\nc\nd",
		},
		{
			name: "SU in a region below row 0 adds nothing", cols: 10, rows: 4,
			input:     []string{"a\r\nb\r\nc\r\nd", esc + "[2;4r", esc + "[1S"},
			wantLines: nil, wantScreen: "a\nc\nd",
		},
		{
			name: "SU is clamped to a short region", cols: 10, rows: 4,
			input:     []string{"a\r\nb\r\nc\r\nd", esc + "[1;2r", esc + "[9S"},
			wantLines: []string{"a", "b"}, wantScreen: "\n\nc\nd",
		},
		{
			name: "DL does not add", cols: 10, rows: 3,
			input:     []string{"a\r\nb\r\nc", esc + "[1;1H", esc + "[2M"},
			wantLines: nil, wantScreen: "c",
		},
		{
			name: "reverse index does not add", cols: 10, rows: 3,
			input:     []string{"a\r\nb\r\nc", esc + "[1;1H", esc + "M"},
			wantLines: nil, wantScreen: "\na\nb",
		},
		{
			name: "a blank row is kept, trailing blanks are trimmed", cols: 10, rows: 2,
			input:     []string{"a   \r\n\r\nb\r\nc"},
			wantLines: []string{"a", ""}, wantScreen: "b\nc",
		},
		{
			name: "wide characters read once", cols: 10, rows: 1,
			input:     []string{"a雪b\n"},
			wantLines: []string{"a雪b"}, wantScreen: "",
		},
		{
			name: "ED 2 keeps the scrollback", cols: 10, rows: 2,
			input:     []string{"a\r\nb\r\nc", esc + "[2J"},
			wantLines: []string{"a"}, wantScreen: "",
		},
		{
			name: "ED 3 clears the scrollback", cols: 10, rows: 2,
			input:     []string{"a\r\nb\r\nc", esc + "[3J"},
			wantLines: nil, wantScreen: "",
		},
		{
			name: "RIS clears the scrollback", cols: 10, rows: 2,
			input:     []string{"a\r\nb\r\nc", esc + "c"},
			wantLines: nil, wantScreen: "",
		},
		{
			name: "cap drops the oldest rows and counts them", cols: 10, rows: 2,
			opts:      terminal.Options{ScrollbackLines: 3},
			input:     []string{"1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n7"},
			wantLines: []string{"3", "4", "5"}, wantScreen: "6\n7", wantDrop: 2,
		},
		{
			name: "a negative limit disables scrollback", cols: 10, rows: 2,
			opts:      terminal.Options{ScrollbackLines: -1},
			input:     []string{"1\r\n2\r\n3\r\n4"},
			wantLines: nil, wantScreen: "3\n4", wantDrop: 2,
		},
		{
			name: "input split across feeds", cols: 10, rows: 2,
			input:     []string{"a\r", "\nb\r\n", "c\r\nd"},
			wantLines: []string{"a", "b"}, wantScreen: "c\nd",
		},
	}
	for _, ad := range adapters {
		for _, tc := range tests {
			t.Run(ad.name+"/"+tc.name, func(t *testing.T) {
				em := ad.new(tc.cols, tc.rows, tc.opts)
				feed(t, em, tc.input...)
				r := reader(t, em)
				info := r.ScrollbackInfo()
				if info.Lines != len(tc.wantLines) {
					t.Errorf("Lines = %d, want %d (text %q)", info.Lines, len(tc.wantLines), r.ScrollbackText())
				}
				if info.Dropped != tc.wantDrop {
					t.Errorf("Dropped = %d, want %d", info.Dropped, tc.wantDrop)
				}
				if got, want := r.ScrollbackText(), strings.Join(tc.wantLines, "\n"); got != want {
					t.Errorf("ScrollbackText = %q, want %q", got, want)
				}
				rows := r.Scrollback()
				if len(rows) != len(tc.wantLines) {
					t.Fatalf("Scrollback() has %d rows, want %d", len(rows), len(tc.wantLines))
				}
				for i, row := range rows {
					if row.Text != tc.wantLines[i] {
						t.Errorf("row %d text = %q, want %q", i, row.Text, tc.wantLines[i])
					}
				}
				if got := em.Screen().Text(); got != tc.wantScreen {
					t.Errorf("screen = %q, want %q", got, tc.wantScreen)
				}
			})
		}
	}
}

func TestScrollbackDefaultLimit(t *testing.T) {
	for _, ad := range adapters {
		t.Run(ad.name, func(t *testing.T) {
			em := ad.new(10, 2, terminal.Options{})
			if got := reader(t, em).ScrollbackInfo().Limit; got != terminal.DefaultScrollbackLines {
				t.Fatalf("default limit = %d, want %d", got, terminal.DefaultScrollbackLines)
			}
			if got := terminal.NewEmulator(10, 2).ScrollbackInfo().Limit; got != terminal.DefaultScrollbackLines {
				t.Fatalf("NewEmulator limit = %d, want %d", got, terminal.DefaultScrollbackLines)
			}
		})
	}
}

func TestScrollbackAlternateScreen(t *testing.T) {
	const enter, leave = "\x1b[?1049h", "\x1b[?1049l"
	var many strings.Builder
	for i := 0; i < 20; i++ {
		many.WriteString("alt\r\n")
	}
	for _, ad := range adapters {
		t.Run(ad.name, func(t *testing.T) {
			em := ad.new(10, 3, terminal.Options{})
			r := reader(t, em)
			feed(t, em, "m1\r\nm2\r\nm3\r\nm4") // m1 scrolls into scrollback
			if got := r.ScrollbackText(); got != "m1" {
				t.Fatalf("before the alternate screen: scrollback = %q", got)
			}
			feed(t, em, enter, many.String())
			if got := r.ScrollbackText(); got != "m1" {
				t.Fatalf("on the alternate screen: scrollback = %q, want it untouched", got)
			}
			feed(t, em, "\x1b[3S") // SU on the alternate screen adds nothing either
			if got := r.ScrollbackInfo(); got.Lines != 1 || got.Dropped != 0 {
				t.Fatalf("after SU on the alternate screen: %+v", got)
			}
			feed(t, em, leave)
			if got := r.ScrollbackText(); got != "m1" {
				t.Fatalf("after leaving: scrollback = %q, want it intact", got)
			}
			// The main screen scrolls into the scrollback again after leaving.
			feed(t, em, "\x1b[3;1H\r\nnext\r\nmore")
			if info := r.ScrollbackInfo(); info.Lines <= 1 {
				t.Fatalf("after leaving: scrollback stopped growing: %+v", info)
			}
		})
	}
}

func TestScrollbackSurvivesResize(t *testing.T) {
	for _, ad := range adapters {
		t.Run(ad.name, func(t *testing.T) {
			em := ad.new(10, 2, terminal.Options{})
			feed(t, em, "a\r\nb\r\nc")
			if err := em.Resize(20, 5); err != nil {
				t.Fatal(err)
			}
			r := reader(t, em)
			if got := r.ScrollbackText(); got != "a" {
				t.Fatalf("scrollback after resize = %q", got)
			}
			// Rows are not reflowed: the old row keeps its width.
			if got := len(r.Scrollback()[0].Cells); got != 1 {
				t.Fatalf("stored cells = %d, want 1 (trailing blanks trimmed)", got)
			}
		})
	}
}

func TestScrollbackKeepsCellsAndStyles(t *testing.T) {
	for _, ad := range adapters {
		t.Run(ad.name, func(t *testing.T) {
			em := ad.new(10, 2, terminal.Options{})
			feed(t, em,
				"\x1b[1;31mred\x1b[0m ",
				"\x1b]8;;https://example.com\x1b\\go\x1b]8;;\x1b\\",
				"\x1b[44m  \x1b[0m", // a styled blank is kept
				"\r\nx\r\ny")
			rows := reader(t, em).Scrollback()
			if len(rows) != 1 {
				t.Fatalf("rows = %d", len(rows))
			}
			row := rows[0]
			if row.Text != "red go" {
				t.Fatalf("text = %q", row.Text)
			}
			c := row.Cells
			if len(c) != 8 {
				t.Fatalf("cells = %d, want 8 (3+1+2+2 styled blanks, plain tail trimmed)", len(c))
			}
			if !c[0].Style.Bold || c[0].Style.Fg == "" || c[0].Char != "r" {
				t.Errorf("cell 0 = %+v, want bold colored r", c[0])
			}
			if c[4].Link != "https://example.com" || c[4].Char != "g" {
				t.Errorf("cell 4 = %+v, want linked g", c[4])
			}
			if c[7].Style.Bg == "" || c[6].Style.Bg == "" {
				t.Errorf("cells 6,7 = %+v %+v, want styled blanks", c[6], c[7])
			}
			for x, cell := range c {
				if cell.X != x || cell.Y != 0 {
					t.Errorf("cell %d has X=%d Y=%d", x, cell.X, cell.Y)
				}
			}
		})
	}
}

func TestScrollbackCopiesAreIndependent(t *testing.T) {
	em := terminal.NewEmulator(10, 2)
	feed(t, em, "a\r\nb\r\nc")
	rows := em.Scrollback()
	rows[0].Cells[0].Char = "Z"
	rows[0].Text = "Z"
	if got := em.ScrollbackText(); got != "a" {
		t.Fatalf("mutating a copy changed the buffer: %q", got)
	}
	// Screen snapshots do not carry scrollback.
	snap := em.Screen().Snapshot()
	if strings.Contains(snap.Text, "a") {
		t.Fatalf("snapshot text leaked scrollback: %q", snap.Text)
	}
}

func TestSetScrollbackLinesShrinks(t *testing.T) {
	em := terminal.NewEmulator(10, 2)
	feed(t, em, "1\r\n2\r\n3\r\n4\r\n5")
	em.SetScrollbackLines(2)
	if got := em.ScrollbackText(); got != "2\n3" {
		t.Fatalf("scrollback = %q", got)
	}
	if got := em.ScrollbackInfo(); got.Dropped != 1 || got.Limit != 2 {
		t.Fatalf("info = %+v", got)
	}
}

func BenchmarkScrollbackFeed(b *testing.B) {
	line := []byte(strings.Repeat("x", 78) + "\r\n")
	em := terminal.NewEmulator(80, 24)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = em.Feed(line)
	}
}
