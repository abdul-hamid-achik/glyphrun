package terminal

import (
	"encoding/json"
	"strings"
	"testing"
)

// wideCell describes one expected cell: its text and width. A continuation
// (the right half of a wide character) is {"", 0}.
type wideCell struct {
	char  string
	width int
}

func assertCells(t *testing.T, s Screen, y int, want []wideCell) {
	t.Helper()
	for x, w := range want {
		c := s.Cell(x, y)
		if c.Char != w.char || c.Width != w.width {
			t.Errorf("cell (%d,%d) = {%q, %d}, want {%q, %d}", x, y, c.Char, c.Width, w.char, w.width)
		}
	}
}

func assertCursor(t *testing.T, s Screen, x, y int) {
	t.Helper()
	if c := s.Cursor(); c.X != x || c.Y != y {
		t.Errorf("cursor = (%d,%d), want (%d,%d)", c.X, c.Y, x, y)
	}
}

// East Asian wide characters and emoji take two columns, as in every terminal
// and in the width model Bubble Tea v2 lays out with. The character sits in the
// first cell; the second is a continuation that contributes no text.
func TestSimpleEmulatorWideCharactersTakeTwoCells(t *testing.T) {
	em := NewEmulator(10, 2)
	feedAll(t, em, "a雪b🔩c")
	scr := em.Screen()
	assertCells(t, scr, 0, []wideCell{{"a", 1}, {"雪", 2}, {"", 0}, {"b", 1}, {"🔩", 2}, {"", 0}, {"c", 1}, {" ", 1}})
	assertCursor(t, scr, 7, 0)
	if got := scr.Text(); got != "a雪b🔩c" {
		t.Errorf("Text() = %q, want %q", got, "a雪b🔩c")
	}
}

// East Asian Ambiguous characters — the dots, triangles, box drawing and Nerd
// Font private-use icons TUIs draw with — stay one column whatever the locale.
func TestSimpleEmulatorAmbiguousCharactersStayNarrow(t *testing.T) {
	em := NewEmulator(10, 1)
	feedAll(t, em, "●▲×─\U000F0219")
	scr := em.Screen()
	assertCells(t, scr, 0, []wideCell{{"●", 1}, {"▲", 1}, {"×", 1}, {"─", 1}, {"", 1}, {"\U000F0219", 1}})
	assertCursor(t, scr, 6, 0)
}

// A rune that continues the previous grapheme cluster joins its cell instead of
// taking one of its own, whether or not it arrives in the same read.
func TestSimpleEmulatorJoinsGraphemeClusters(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
		want   []wideCell
		cursor int
	}{
		{"combining accent", []string{"éx"}, []wideCell{{"é", 1}, {"x", 1}}, 2},
		{"combining accent in the next read", []string{"e", "́", "x"}, []wideCell{{"é", 1}, {"x", 1}}, 2},
		{"emoji ZWJ sequence", []string{"👨‍👩‍👧x"}, []wideCell{{"👨‍👩‍👧", 2}, {"", 0}, {"x", 1}}, 3},
		{"skin tone modifier", []string{"👍\U0001F3FDx"}, []wideCell{{"👍\U0001F3FD", 2}, {"", 0}, {"x", 1}}, 3},
		{"flag", []string{"🇲🇽x"}, []wideCell{{"🇲🇽", 1}, {"x", 1}}, 2},
		{"variation selector", []string{"❤️x"}, []wideCell{{"❤️", 1}, {"x", 1}}, 2},
		{"accent on a wide character", []string{"雪́x"}, []wideCell{{"雪́", 2}, {"", 0}, {"x", 1}}, 3},
		// A cursor move ends the cluster: there is no cell to attach to, so the
		// stray mark takes no cell either.
		{"mark after a cursor move", []string{"e\x1b[Ćx"}, []wideCell{{"e", 1}, {" ", 1}, {"x", 1}}, 3},
		{"mark at the start of a line", []string{"́x"}, []wideCell{{"x", 1}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			em := NewEmulator(10, 2)
			feedAll(t, em, tt.chunks...)
			scr := em.Screen()
			assertCells(t, scr, 0, tt.want)
			assertCursor(t, scr, tt.cursor, 0)
		})
	}
}

// A wide character never splits across the right margin.
func TestSimpleEmulatorWideCharacterAtTheRightMargin(t *testing.T) {
	t.Run("autowrap wraps it whole", func(t *testing.T) {
		em := NewEmulator(5, 2)
		feedAll(t, em, "abcd雪")
		scr := em.Screen()
		if got := rowText(scr, 0, 5); got != "abcd" {
			t.Errorf("row 0 = %q, want %q", got, "abcd")
		}
		assertCells(t, scr, 1, []wideCell{{"雪", 2}, {"", 0}})
		assertCursor(t, scr, 2, 1)
	})
	t.Run("without autowrap it is dropped", func(t *testing.T) {
		em := NewEmulator(5, 2)
		feedAll(t, em, "\x1b[?7labcd雪")
		scr := em.Screen()
		assertCells(t, scr, 0, []wideCell{{"a", 1}, {"b", 1}, {"c", 1}, {"d", 1}, {" ", 1}})
		assertCursor(t, scr, 4, 0)
	})
	t.Run("filling the last two columns defers the wrap", func(t *testing.T) {
		em := NewEmulator(4, 2)
		feedAll(t, em, "ab雪")
		scr := em.Screen()
		assertCells(t, scr, 0, []wideCell{{"a", 1}, {"b", 1}, {"雪", 2}, {"", 0}})
		assertCursor(t, scr, 3, 0)
		feedAll(t, em, "c")
		if got := rowText(em.Screen(), 1, 4); got != "c" {
			t.Errorf("row 1 = %q, want %q", got, "c")
		}
	})
	t.Run("a one-column screen cannot hold it", func(t *testing.T) {
		em := NewEmulator(1, 2)
		feedAll(t, em, "雪x")
		assertCells(t, em.Screen(), 0, []wideCell{{"x", 1}})
	})
}

// Every operation that changes part of a wide character blanks the half it
// leaves behind, as terminals do, so no cell is left claiming a partner that is
// gone.
func TestSimpleEmulatorEditsDoNotLeaveHalfAWideCharacter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []wideCell
	}{
		{"print over the right half", "a雪b\x1b[1;3Hx", []wideCell{{"a", 1}, {" ", 1}, {"x", 1}, {"b", 1}}},
		{"print over the left half", "a雪b\x1b[1;2Hx", []wideCell{{"a", 1}, {"x", 1}, {" ", 1}, {"b", 1}}},
		{"wide over the right half of another", "a雪b\x1b[1;3H雨", []wideCell{{"a", 1}, {" ", 1}, {"雨", 2}, {"", 0}}},
		{"ECH on the right half", "a雪b\x1b[1;3H\x1b[X", []wideCell{{"a", 1}, {" ", 1}, {" ", 1}, {"b", 1}}},
		{"ECH on the left half", "a雪b\x1b[1;2H\x1b[X", []wideCell{{"a", 1}, {" ", 1}, {" ", 1}, {"b", 1}}},
		{"EL from the right half", "a雪b\x1b[1;3H\x1b[K", []wideCell{{"a", 1}, {" ", 1}, {" ", 1}, {" ", 1}}},
		{"EL to the left half", "a雪b\x1b[1;2H\x1b[1K", []wideCell{{" ", 1}, {" ", 1}, {" ", 1}, {"b", 1}}},
		{"ED below from the right half", "a雪b\x1b[1;3H\x1b[J", []wideCell{{"a", 1}, {" ", 1}, {" ", 1}, {" ", 1}}},
		{"ICH inside", "a雪b\x1b[1;3H\x1b[@", []wideCell{{"a", 1}, {" ", 1}, {" ", 1}, {" ", 1}, {"b", 1}}},
		{"ICH pushes it past the margin", "ab雪\x1b[1;1H\x1b[@", []wideCell{{" ", 1}, {"a", 1}, {"b", 1}, {" ", 1}}},
		{"DCH on the left half", "a雪b\x1b[1;2H\x1b[P", []wideCell{{"a", 1}, {" ", 1}, {"b", 1}}},
		{"DCH before it keeps it whole", "a雪b\x1b[1;1H\x1b[P", []wideCell{{"雪", 2}, {"", 0}, {"b", 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols := 6
			if strings.Contains(tt.name, "margin") {
				cols = 4
			}
			em := NewEmulator(cols, 2)
			feedAll(t, em, tt.input)
			assertCells(t, em.Screen(), 0, tt.want)
		})
	}
}

func TestSimpleEmulatorInsertModeAndRepeatUseTheCharacterWidth(t *testing.T) {
	t.Run("IRM opens two cells", func(t *testing.T) {
		em := NewEmulator(6, 1)
		feedAll(t, em, "ab\x1b[1;1H\x1b[4h雪")
		scr := em.Screen()
		assertCells(t, scr, 0, []wideCell{{"雪", 2}, {"", 0}, {"a", 1}, {"b", 1}})
		assertCursor(t, scr, 2, 0)
	})
	t.Run("REP repeats at two cells", func(t *testing.T) {
		em := NewEmulator(8, 1)
		feedAll(t, em, "雪\x1b[2b")
		scr := em.Screen()
		if got := scr.Text(); got != "雪雪雪" {
			t.Errorf("Text() = %q, want %q", got, "雪雪雪")
		}
		assertCursor(t, scr, 6, 0)
	})
}

// Region text and snapshots carry the character once: the continuation cell is
// kept in the cell grid (with an empty char and width 0) but adds no text.
func TestSimpleEmulatorWideCharacterInRegionsAndSnapshots(t *testing.T) {
	em := NewEmulator(6, 1)
	feedAll(t, em, "a雪b")
	scr := em.Screen()
	for _, tt := range []struct {
		x, w int
		want string
	}{
		{0, 4, "a雪b"},
		{1, 2, "雪"},
		{1, 1, "雪"},
		{2, 2, " b"}, // starts on the right half: that column reads as blank
	} {
		if got := scr.Region(tt.x, 0, tt.w, 1).Text(); got != tt.want {
			t.Errorf("Region(%d,0,%d,1).Text() = %q, want %q", tt.x, tt.w, got, tt.want)
		}
	}

	data, err := json.Marshal(scr.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var snap ScreenSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	if c := snap.Cells[2]; c.Char != "" || c.Width != 0 || !c.IsContinuation() {
		t.Errorf("continuation cell after a JSON round trip = %+v, want empty char and width 0", c)
	}
	if c := snap.Cells[1]; c.Char != "雪" || c.Width != 2 || c.IsContinuation() {
		t.Errorf("wide cell after a JSON round trip = %+v", c)
	}
	if snap.Text != "a雪b" {
		t.Errorf("snapshot text = %q", snap.Text)
	}
}
