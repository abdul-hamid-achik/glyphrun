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

func TestSimpleEmulatorIgnoresOSCTitleSequences(t *testing.T) {
	em := NewEmulator(20, 3)
	if _, err := em.Feed([]byte("\x1b]2;LOCAL AGENT\ahello\x1b]2;\a")); err != nil {
		t.Fatal(err)
	}
	text := em.Screen().Text()
	if strings.Contains(text, "2;") || strings.Contains(text, "LOCAL AGENT") {
		t.Fatalf("OSC title leaked into screen: %q", text)
	}
	if !strings.Contains(text, "hello") {
		t.Fatalf("screen text = %q", text)
	}
}

func TestSimpleEmulatorTracksBracketedPasteMode(t *testing.T) {
	em := NewEmulator(20, 3)
	if em.BracketedPasteMode() {
		t.Fatal("bracketed paste should start disabled")
	}
	if _, err := em.Feed([]byte("\x1b[?2004h")); err != nil {
		t.Fatal(err)
	}
	if !em.BracketedPasteMode() {
		t.Fatal("bracketed paste should be enabled")
	}
	if _, err := em.Feed([]byte("\x1b[?2004l")); err != nil {
		t.Fatal(err)
	}
	if em.BracketedPasteMode() {
		t.Fatal("bracketed paste should be disabled")
	}
}

func TestSimpleEmulatorTracksAlternateScreenMode(t *testing.T) {
	em := NewEmulator(20, 3)
	if em.AlternateScreenMode() || em.AlternateScreenUsed() {
		t.Fatal("alternate screen should start disabled and unused")
	}
	if _, err := em.Feed([]byte("main\x1b[?1049halt")); err != nil {
		t.Fatal(err)
	}
	if !em.AlternateScreenMode() {
		t.Fatal("alternate screen should be active")
	}
	if !em.AlternateScreenUsed() {
		t.Fatal("alternate screen should be marked used")
	}
	if strings.Contains(em.Screen().Text(), "main") {
		t.Fatalf("entering alternate screen should clear current screen: %q", em.Screen().Text())
	}
	if _, err := em.Feed([]byte("\x1b[?1049l")); err != nil {
		t.Fatal(err)
	}
	if em.AlternateScreenMode() {
		t.Fatal("alternate screen should be inactive after reset")
	}
	if !em.AlternateScreenUsed() {
		t.Fatal("alternate screen usage should remain recorded")
	}
}
