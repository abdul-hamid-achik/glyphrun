package terminal

import (
	"strings"
	"testing"
)

func TestSimpleEmulatorFeedsAnsiScreen(t *testing.T) {
	em := NewEmulator(20, 5)
	if _, err := em.Feed([]byte("\x1b[2J\x1b[Hhello\nworld")); err != nil {
		t.Fatal(err)
	}
	text := em.Screen().Text()
	if !strings.Contains(text, "hello") || !strings.Contains(text, "world") {
		t.Fatalf("screen text = %q", text)
	}
}

func TestSimpleEmulatorCursorMovementAndStyles(t *testing.T) {
	em := NewEmulator(10, 3)
	if _, err := em.Feed([]byte("ab\x1b[Dc\x1b[1mZ")); err != nil {
		t.Fatal(err)
	}
	screen := em.Screen()
	if got := screen.Cell(1, 0).Char; got != "c" {
		t.Fatalf("cell 1,0 = %q", got)
	}
	if got := screen.Cell(2, 0); got.Char != "Z" || !got.Style.Bold {
		t.Fatalf("styled cell = %#v", got)
	}
	if _, err := em.Feed([]byte("\x1b[?25l")); err != nil {
		t.Fatal(err)
	}
	if em.Screen().Cursor().Visible {
		t.Fatal("cursor should be hidden")
	}
}
