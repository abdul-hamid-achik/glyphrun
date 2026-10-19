package terminal

import (
	"reflect"
	"strings"
	"testing"
)

func TestTextColumns(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{"", nil},
		{"ab", []string{"a", "b"}},
		{"a雪b", []string{"a", "雪", "", "b"}},
		{"🔩x", []string{"🔩", "", "x"}},
		{"éx", []string{"é", "x"}},
		{"👨‍👩‍👧", []string{"👨‍👩‍👧", ""}},
		{"●▲", []string{"●", "▲", ""}},
		{"雪́", []string{"雪́", ""}},
	}
	for _, tt := range tests {
		got := TextColumns(tt.line)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("TextColumns(%q) = %q, want %q", tt.line, got, tt.want)
		}
		if joined := strings.Join(got, ""); joined != tt.line {
			t.Errorf("TextColumns(%q) joins back to %q", tt.line, joined)
		}
	}
}

// TextColumns lays text out exactly as the emulator does, so code holding only
// a golden's text addresses the same columns as code holding its cells.
func TestTextColumnsMatchesTheEmulator(t *testing.T) {
	line := "a雪b🔩c é 👨‍👩‍👧 ●▲ 日本"
	em := NewEmulator(40, 1)
	feedAll(t, em, line)
	scr := em.Screen()
	for x, col := range TextColumns(line) {
		if c := scr.Cell(x, 0); c.Char != col {
			t.Errorf("column %d: emulator %q, TextColumns %q", x, c.Char, col)
		}
	}
}

// A character that changes width changes what the screen shows even when the
// cells around it read the same.
func TestDiffSnapshotsSeesAWidthChange(t *testing.T) {
	wide := NewEmulator(4, 1)
	feedAll(t, wide, "雪")
	narrow := NewEmulator(4, 1)
	feedAll(t, narrow, "雪") // then pretend an old golden stored it one column wide
	old := narrow.Screen().Snapshot()
	old.Cells[0].Width = 1
	old.Cells[1] = Cell{X: 1, Y: 0, Char: " ", Width: 1}
	d := DiffSnapshots(old, wide.Screen().Snapshot())
	if d.Equal() || len(d.Changed) != 1 || d.Changed[0].X != 1 {
		t.Fatalf("diff = %+v, want exactly the continuation column changed", d.Changed)
	}
	if same := DiffSnapshots(wide.Screen().Snapshot(), wide.Screen().Snapshot()); !same.Equal() {
		t.Fatalf("a snapshot differs from itself: %+v", same.Changed)
	}
}
