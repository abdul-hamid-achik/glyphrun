package runner

import (
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// makeScreen spins up a SimpleEmulator and feeds `body`. We size
// the screen to fit the body — the SimpleEmulator's newLine()
// scrolls when the cursor hits the bottom row, so a body that
// exactly fills the rows will leave the last cell blank after the
// implicit wrap. Tests use 2× the body length in cols to leave
// headroom and avoid scroll surprises.
func makeScreen(t *testing.T, cols, rows int, body string) terminal.Emulator {
	t.Helper()
	em := terminal.NewEmulator(cols, rows)
	if _, err := em.Feed([]byte(body)); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	return em
}

func intPtr(v int) *int { return &v }

func TestCheckCount_NonEmptyFullScreen(t *testing.T) {
	// 20×2: two lines, each starting at column 0. A newline that returns to
	// column 0 is CR+LF ("\r\n") — a bare "\n" is a line feed only and keeps
	// the column (LNM-reset default). Non-blank (non-space) cells:
	// "hello world" = 10, "foo bar baz" = 9 → 19.
	emu := makeScreen(t, 20, 2, "hello world\r\nfoo bar baz")
	s := &runState{emulator: emu}
	ok, msg, raw := s.checkCount(emu.Screen(), spec.CountCondition{Equals: intPtr(19)})
	if !ok {
		t.Fatalf("expected 19 matched cells, got fail: %s", msg)
	}
	if raw == nil {
		t.Error("expected evidence payload, got nil")
	}
}

func TestCheckCount_NonEmptyFailsOnAllBlank(t *testing.T) {
	emu := makeScreen(t, 5, 1, "     ")
	s := &runState{emulator: emu}
	ok, msg, _ := s.checkCount(emu.Screen(), spec.CountCondition{Equals: intPtr(0)})
	if !ok {
		t.Fatalf("all-blank screen should match 0 nonEmpty: %s", msg)
	}
}

func TestCheckCount_EqualsRune(t *testing.T) {
	// 10×1: "xx  -  xx" → 4 'x' cells.
	emu := makeScreen(t, 10, 1, "xx--xx")
	s := &runState{emulator: emu}
	ok, msg, _ := s.checkCount(emu.Screen(), spec.CountCondition{
		Matches: "x",
		Equals:  intPtr(4),
	})
	if !ok {
		t.Fatalf("expected 4 'x' cells, got fail: %s", msg)
	}
}

func TestCheckCount_AtLeastRune(t *testing.T) {
	// 10×1: "xyzzz1" → 3 'z' cells. Padding keeps cursor on row 0.
	emu := makeScreen(t, 10, 1, "xyzzz1")
	s := &runState{emulator: emu}
	ok, msg, _ := s.checkCount(emu.Screen(), spec.CountCondition{
		Matches: "z",
		AtLeast: intPtr(3),
	})
	if !ok {
		t.Fatalf("expected at least 3 'z' cells: %s", msg)
	}
}

func TestCheckCount_AtMostFails(t *testing.T) {
	emu := makeScreen(t, 10, 1, "xyzzz1")
	s := &runState{emulator: emu}
	ok, msg, _ := s.checkCount(emu.Screen(), spec.CountCondition{
		Matches: "z",
		AtMost:  intPtr(1),
	})
	if ok {
		t.Fatalf("expected fail (3 'z' > 1), got pass: %s", msg)
	}
}

func TestCheckCount_Between(t *testing.T) {
	// 10×1: "xyzzzz" → 4 'z' cells.
	emu := makeScreen(t, 10, 1, "xyzzzz")
	s := &runState{emulator: emu}
	tight := [2]int{2, 3}
	ok, msg, _ := s.checkCount(emu.Screen(), spec.CountCondition{
		Matches: "z",
		Between: &tight,
	})
	if ok {
		t.Fatalf("expected fail (4 'z' not in [2,3]), got pass: %s", msg)
	}
	loose := [2]int{2, 4}
	ok, _, _ = s.checkCount(emu.Screen(), spec.CountCondition{
		Matches: "z",
		Between: &loose,
	})
	if !ok {
		t.Fatalf("expected pass (4 'z' in [2,4])")
	}
}

func TestCheckCount_RegionOnly(t *testing.T) {
	// 20×2: "abcdef" / "ghijkl". Region x=0,y=0,w=3,h=1
	// contains "abc" — nonEmpty = 3.
	emu := makeScreen(t, 20, 2, "abcdef\nghijkl")
	s := &runState{emulator: emu}
	ok, msg, _ := s.checkCount(emu.Screen(), spec.CountCondition{
		Region: &spec.RegionCondition{X: 0, Y: 0, Width: 3, Height: 1},
		Equals: intPtr(3),
	})
	if !ok {
		t.Fatalf("expected 3 nonEmpty in region, got fail: %s", msg)
	}
}

func TestCheckCount_RejectsMultiCharMatches(t *testing.T) {
	emu := makeScreen(t, 10, 1, "abcd")
	s := &runState{emulator: emu}
	ok, msg, _ := s.checkCount(emu.Screen(), spec.CountCondition{
		Matches: "ab",
		Equals:  intPtr(1),
	})
	if ok {
		t.Fatalf("expected fail (multi-char matches), got pass: %s", msg)
	}
}

func TestCheckCount_EvidenceReturnsCount(t *testing.T) {
	// 10×1: "a   a" → 2 'a' cells.
	emu := makeScreen(t, 10, 1, "a   a")
	s := &runState{emulator: emu}
	_, _, raw := s.checkCount(emu.Screen(), spec.CountCondition{
		Matches: "a",
		Equals:  intPtr(2),
	})
	ev, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected evidence map, got %T", raw)
	}
	if got, _ := ev["matched"].(int); got != 2 {
		t.Errorf("evidence.matched: got %v, want 2", ev["matched"])
	}
}

func TestCheckCount_NoComparatorFails(t *testing.T) {
	emu := makeScreen(t, 10, 1, "abc")
	s := &runState{emulator: emu}
	ok, msg, _ := s.checkCount(emu.Screen(), spec.CountCondition{})
	if ok {
		t.Fatalf("expected fail (no comparator), got pass: %s", msg)
	}
}
