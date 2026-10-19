package stories

import (
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

func TestCompactScreenMarksContinuationCells(t *testing.T) {
	em := terminal.NewEmulator(6, 1)
	if _, err := em.Feed([]byte("a雪b")); err != nil {
		t.Fatal(err)
	}
	grid, _, _ := CompactScreen(em.Screen().Snapshot())
	want := []string{"a", "雪", "", "b", " ", " "}
	for x, ch := range want {
		if grid[0][x] != ch {
			t.Errorf("grid[0][%d] = %q, want %q", x, grid[0][x], ch)
		}
	}
}

// A text golden turns back into the same cells the emulator produced, so a
// golden with wide characters compares cell for cell against a capture.
func TestScreenFromTextLaysOutWideCharacters(t *testing.T) {
	em := terminal.NewEmulator(8, 2)
	if _, err := em.Feed([]byte("a雪b🔩\r\néx")); err != nil {
		t.Fatal(err)
	}
	live := em.Screen().Snapshot()
	fromText := screenFromText(live.Text, 8, 2)
	if d := terminal.DiffSnapshots(fromText, live); !d.Equal() {
		t.Fatalf("golden text and capture differ: %+v", d.Changed)
	}
}
