package terminal

import (
	"reflect"
	"strings"
	"testing"
)

// screenRows returns every row of the screen as trimmed text.
func screenRows(s Screen) []string {
	sz := s.Size()
	rows := make([]string, sz.Rows)
	for y := range rows {
		rows[y] = rowText(s, y, sz.Cols)
	}
	return rows
}

func scrollbackLines(em *SimpleEmulator) []string {
	text := em.ScrollbackText()
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// TestSimpleEmulatorResizeKeepsScreen pins xterm's resize: no reflow, the
// cells that fit stay, and the cursor row stays on screen by pushing the top
// rows into the scrollback.
func TestSimpleEmulatorResizeKeepsScreen(t *testing.T) {
	tests := []struct {
		name       string
		cols, rows int
		input      string
		newCols    int
		newRows    int
		wantRows   []string
		wantX      int
		wantY      int
		wantScroll []string
	}{
		{
			name: "width grows", cols: 5, rows: 3, input: "abc\r\ndef",
			newCols: 8, newRows: 3,
			wantRows: []string{"abc", "def", ""}, wantX: 3, wantY: 1,
		},
		{
			name: "width shrinks cuts each row and clamps the cursor", cols: 8, rows: 3,
			input:   "abcdefgh\r\nijklmn",
			newCols: 4, newRows: 3,
			wantRows: []string{"abcd", "ijkl", ""}, wantX: 3, wantY: 1,
		},
		{
			name: "width shrinks, cursor already inside", cols: 8, rows: 2,
			input:   "abcdefgh\x1b[1;2H",
			newCols: 4, newRows: 2,
			wantRows: []string{"abcd", ""}, wantX: 1, wantY: 0,
		},
		{
			name: "rows grow with blank rows at the bottom", cols: 4, rows: 2,
			input:   "a\r\nb",
			newCols: 4, newRows: 4,
			wantRows: []string{"a", "b", "", ""}, wantX: 1, wantY: 1,
		},
		{
			name: "rows grow without pulling lines back from the scrollback", cols: 4, rows: 2,
			input:   "a\r\nb\r\nc\r\nd",
			newCols: 4, newRows: 4,
			wantRows: []string{"c", "d", "", ""}, wantX: 1, wantY: 1,
			wantScroll: []string{"a", "b"},
		},
		{
			name: "rows shrink with the cursor on top drops rows from the bottom", cols: 4, rows: 4,
			input:   "l1\r\nl2\r\nl3\r\nl4\x1b[H",
			newCols: 4, newRows: 2,
			wantRows: []string{"l1", "l2"}, wantX: 0, wantY: 0,
		},
		{
			name: "rows shrink with the cursor at the bottom push the top rows", cols: 4, rows: 5,
			input:   "l1\r\nl2\r\nl3\r\nl4\r\nl5",
			newCols: 4, newRows: 3,
			wantRows: []string{"l3", "l4", "l5"}, wantX: 2, wantY: 2,
			wantScroll: []string{"l1", "l2"},
		},
		{
			name: "rows shrink with the cursor in the middle keeps it on the last row", cols: 4, rows: 5,
			input:   "l1\r\nl2\r\nl3\r\nl4\r\nl5\x1b[4;1H",
			newCols: 4, newRows: 3,
			wantRows: []string{"l2", "l3", "l4"}, wantX: 0, wantY: 2,
			wantScroll: []string{"l1"},
		},
		{
			name: "both axes shrink", cols: 6, rows: 4,
			input:   "abcdef\r\nghijkl\r\nmnopqr\r\nstuvwx",
			newCols: 3, newRows: 2,
			wantRows: []string{"mno", "stu"}, wantX: 2, wantY: 1,
			wantScroll: []string{"abcdef", "ghijkl"},
		},
		{
			name: "rows shrink to a single row", cols: 4, rows: 3,
			input:   "a\r\nb\r\nc",
			newCols: 4, newRows: 1,
			wantRows: []string{"c"}, wantX: 1, wantY: 0,
			wantScroll: []string{"a", "b"},
		},
		{
			name: "wide character cut in half becomes a blank", cols: 6, rows: 1,
			input:   "a雪b",
			newCols: 2, newRows: 1,
			wantRows: []string{"a"}, wantX: 1, wantY: 0,
		},
		{
			name: "wide character that still fits is kept", cols: 6, rows: 1,
			input:   "a雪b",
			newCols: 3, newRows: 1,
			wantRows: []string{"a雪"}, wantX: 2, wantY: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			em := NewEmulator(tc.cols, tc.rows)
			feedAll(t, em, tc.input)
			if err := em.Resize(tc.newCols, tc.newRows); err != nil {
				t.Fatal(err)
			}
			scr := em.Screen()
			if sz := scr.Size(); sz.Cols != tc.newCols || sz.Rows != tc.newRows {
				t.Fatalf("size = %dx%d, want %dx%d", sz.Cols, sz.Rows, tc.newCols, tc.newRows)
			}
			if got := screenRows(scr); !reflect.DeepEqual(got, tc.wantRows) {
				t.Errorf("rows = %q, want %q", got, tc.wantRows)
			}
			if c := scr.Cursor(); c.X != tc.wantX || c.Y != tc.wantY || !c.Visible {
				t.Errorf("cursor = %+v, want (%d,%d) visible", c, tc.wantX, tc.wantY)
			}
			if got := scrollbackLines(em); !reflect.DeepEqual(got, tc.wantScroll) {
				t.Errorf("scrollback = %q, want %q", got, tc.wantScroll)
			}
			// Every cell carries its own coordinates after the move.
			for y := 0; y < tc.newRows; y++ {
				for x := 0; x < tc.newCols; x++ {
					if c := scr.Cell(x, y); c.X != x || c.Y != y {
						t.Fatalf("cell (%d,%d) reports (%d,%d)", x, y, c.X, c.Y)
					}
				}
			}
		})
	}
}

func TestSimpleEmulatorResizeCutWideCharacterKeepsStyle(t *testing.T) {
	em := NewEmulator(6, 1)
	feedAll(t, em, "a\x1b[44m雪\x1b[0mb")
	if err := em.Resize(2, 1); err != nil {
		t.Fatal(err)
	}
	c := em.Screen().Cell(1, 0)
	if c.Char != " " || c.Width != 1 || c.Style.Bg == "" {
		t.Errorf("cut half = %+v, want a blank of width 1 that keeps its background", c)
	}
}

func TestSimpleEmulatorResizeKeepsStylesAndLinks(t *testing.T) {
	em := NewEmulator(6, 2)
	feedAll(t, em, "\x1b[1;31mred\x1b[0m\r\n\x1b]8;;https://example.com\x1b\\go\x1b]8;;\x1b\\")
	if err := em.Resize(10, 3); err != nil {
		t.Fatal(err)
	}
	scr := em.Screen()
	if c := scr.Cell(0, 0); c.Char != "r" || !c.Style.Bold || c.Style.Fg == "" {
		t.Errorf("styled cell = %+v", c)
	}
	if c := scr.Cell(1, 1); c.Char != "o" || c.Link != "https://example.com" {
		t.Errorf("linked cell = %+v", c)
	}
	// Padding is plain blanks.
	if c := scr.Cell(9, 2); c.Char != " " || c.Width != 1 || c.Style != (Style{}) {
		t.Errorf("padding cell = %+v", c)
	}
}

// A resize clears the deferred wrap, keeps the cursor hidden, and returns the
// scroll region and origin mode to their defaults.
func TestSimpleEmulatorResizeResetsTransientState(t *testing.T) {
	t.Run("pending wrap", func(t *testing.T) {
		em := NewEmulator(5, 2)
		feedAll(t, em, "abcde") // cursor parked on the last column with a wrap pending
		if err := em.Resize(8, 2); err != nil {
			t.Fatal(err)
		}
		feedAll(t, em, "Z")
		scr := em.Screen()
		if got := screenRows(scr); !reflect.DeepEqual(got, []string{"abcdZ", ""}) {
			t.Errorf("rows = %q: the pending wrap survived the resize", got)
		}
		assertCursor(t, scr, 5, 0)
	})
	t.Run("cursor visibility", func(t *testing.T) {
		em := NewEmulator(5, 2)
		feedAll(t, em, "\x1b[?25l")
		if err := em.Resize(6, 3); err != nil {
			t.Fatal(err)
		}
		if em.Screen().Cursor().Visible {
			t.Error("a hidden cursor became visible")
		}
	})
	t.Run("scroll region and origin mode", func(t *testing.T) {
		em := NewEmulator(4, 4)
		feedAll(t, em, "\x1b[2;3r\x1b[?6h")
		if err := em.Resize(4, 6); err != nil {
			t.Fatal(err)
		}
		// With the region back to the full screen, a line feed on the last row
		// scrolls the whole screen and feeds the scrollback; with origin mode
		// off the row address is absolute.
		feedAll(t, em, "\x1b[1;1Htop\x1b[6;1H\n")
		if got := scrollbackLines(em); !reflect.DeepEqual(got, []string{"top"}) {
			t.Errorf("scrollback = %q, want [top]", got)
		}
	})
	t.Run("saved cursor is clamped", func(t *testing.T) {
		em := NewEmulator(10, 6)
		feedAll(t, em, "\x1b[6;10H\x1b7") // DECSC at the bottom-right corner
		if err := em.Resize(4, 3); err != nil {
			t.Fatal(err)
		}
		feedAll(t, em, "\x1b8")
		assertCursor(t, em.Screen(), 3, 2)
	})
}

// The emulator has a single grid for the main and the alternate screen, so the
// alternate screen follows the same rules, except that its rows never enter
// the scrollback. The flag survives, and so does the position to return to.
func TestSimpleEmulatorResizeOnAlternateScreen(t *testing.T) {
	em := NewEmulator(6, 5)
	feedAll(t, em, "main\x1b[3;3H\x1b[?1049h", "\x1b[Ha1\r\na2\r\na3\r\na4\r\na5")
	if err := em.Resize(4, 3); err != nil {
		t.Fatal(err)
	}
	if !em.AlternateScreenMode() || !em.AlternateScreenUsed() {
		t.Fatal("resize changed the alternate-screen state")
	}
	scr := em.Screen()
	if got := screenRows(scr); !reflect.DeepEqual(got, []string{"a3", "a4", "a5"}) {
		t.Errorf("rows = %q", got)
	}
	assertCursor(t, scr, 2, 2)
	if got := scrollbackLines(em); got != nil {
		t.Errorf("scrollback = %q: alternate-screen rows must not enter it", got)
	}
	feedAll(t, em, "\x1b[?1049l")
	assertCursor(t, em.Screen(), 2, 2) // main cursor (2,2) still fits in 4x3
}

func TestSimpleEmulatorResizeAlternateScreenSavedCursorClamped(t *testing.T) {
	em := NewEmulator(10, 6)
	feedAll(t, em, "\x1b[6;9H\x1b[?1049h")
	if err := em.Resize(5, 3); err != nil {
		t.Fatal(err)
	}
	feedAll(t, em, "\x1b[?1049l")
	assertCursor(t, em.Screen(), 4, 2)
}

// Resizing to the same size is a no-op, and a main-screen resize in the
// middle of the scrollback leaves it as it was.
func TestSimpleEmulatorResizeSameSizeIsNoop(t *testing.T) {
	em := NewEmulator(5, 3)
	feedAll(t, em, "a\r\nb\r\nc\r\nd")
	before := em.Screen().Snapshot()
	if err := em.Resize(5, 3); err != nil {
		t.Fatal(err)
	}
	after := em.Screen().Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Errorf("snapshot changed:\n before %+v\n after  %+v", before, after)
	}
	if got := scrollbackLines(em); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("scrollback = %q", got)
	}
}

func TestSimpleEmulatorResizeRejectsNonPositiveSize(t *testing.T) {
	em := NewEmulator(5, 3)
	feedAll(t, em, "abc")
	for _, sz := range [][2]int{{0, 3}, {5, 0}, {-1, -1}} {
		if err := em.Resize(sz[0], sz[1]); err == nil {
			t.Errorf("Resize(%d, %d) succeeded", sz[0], sz[1])
		}
	}
	if got := rowText(em.Screen(), 0, 5); got != "abc" {
		t.Errorf("a rejected resize changed the screen: %q", got)
	}
}
