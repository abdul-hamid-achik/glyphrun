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

func TestSimpleEmulatorTracksColors(t *testing.T) {
	tests := []struct {
		name   string
		seq    string
		wantFg string
		wantBg string
	}{
		{name: "standard fg", seq: "\x1b[31mX", wantFg: "red"},
		{name: "standard bg", seq: "\x1b[42mX", wantBg: "green"},
		{name: "bright fg", seq: "\x1b[94mX", wantFg: "brightblue"},
		{name: "bright bg", seq: "\x1b[103mX", wantBg: "brightyellow"},
		{name: "256 named", seq: "\x1b[38;5;1mX", wantFg: "red"},
		{name: "256 index", seq: "\x1b[38;5;201mX", wantFg: "201"},
		{name: "truecolor", seq: "\x1b[38;2;255;136;0mX", wantFg: "#ff8800"},
		{name: "fg and bg", seq: "\x1b[31;44mX", wantFg: "red", wantBg: "blue"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			em := NewEmulator(10, 1)
			if _, err := em.Feed([]byte(tc.seq)); err != nil {
				t.Fatal(err)
			}
			cell := em.Screen().Cell(0, 0)
			if cell.Style.Fg != tc.wantFg {
				t.Errorf("fg = %q, want %q", cell.Style.Fg, tc.wantFg)
			}
			if cell.Style.Bg != tc.wantBg {
				t.Errorf("bg = %q, want %q", cell.Style.Bg, tc.wantBg)
			}
		})
	}
}

func TestSimpleEmulatorColorResets(t *testing.T) {
	em := NewEmulator(10, 1)
	// Set fg+bg, then reset fg (39) and bg (49) individually.
	if _, err := em.Feed([]byte("\x1b[31;44mA\x1b[39mB\x1b[49mC")); err != nil {
		t.Fatal(err)
	}
	screen := em.Screen()
	if a := screen.Cell(0, 0).Style; a.Fg != "red" || a.Bg != "blue" {
		t.Errorf("cell A = %+v, want fg red bg blue", a)
	}
	if b := screen.Cell(1, 0).Style; b.Fg != "" || b.Bg != "blue" {
		t.Errorf("cell B = %+v, want fg reset, bg blue", b)
	}
	if c := screen.Cell(2, 0).Style; c.Fg != "" || c.Bg != "" {
		t.Errorf("cell C = %+v, want both reset", c)
	}
	// SGR 0 clears everything.
	if _, err := em.Feed([]byte("\x1b[1;31mD\x1b[0mE")); err != nil {
		t.Fatal(err)
	}
	if e := em.Screen().Cell(4, 0).Style; e.Fg != "" || e.Bold {
		t.Errorf("cell E = %+v, want fully reset", e)
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

func TestSimpleEmulatorTabDoesNotOverwriteCells(t *testing.T) {
	em := NewEmulator(20, 3)
	// Write text, return to column 0, then tab forward past it. The tab must
	// advance the cursor without blanking the cells it moves over.
	if _, err := em.Feed([]byte("abcdef\r\tZ")); err != nil {
		t.Fatal(err)
	}
	screen := em.Screen()
	if got := screen.Cell(2, 0).Char; got != "c" {
		t.Fatalf("tab overwrote cell 2,0: got %q, want \"c\"", got)
	}
	// The next tab stop after column 0 is column 8, so Z lands there.
	if got := screen.Cell(8, 0).Char; got != "Z" {
		t.Fatalf("cell 8,0 = %q, want \"Z\"", got)
	}
}

func TestSimpleEmulatorIgnoresDEL(t *testing.T) {
	em := NewEmulator(10, 2)
	if _, err := em.Feed([]byte("a\x7fb")); err != nil {
		t.Fatal(err)
	}
	screen := em.Screen()
	if got := screen.Cell(0, 0).Char; got != "a" {
		t.Fatalf("cell 0,0 = %q, want \"a\"", got)
	}
	// DEL must be a no-op: b follows a directly with no DEL glyph between.
	if got := screen.Cell(1, 0).Char; got != "b" {
		t.Fatalf("cell 1,0 = %q, want \"b\" (DEL should be ignored)", got)
	}
}

func TestSimpleEmulatorECHCancelsPendingWrap(t *testing.T) {
	em := NewEmulator(5, 3)
	// Fill the row exactly; the last write leaves a deferred autowrap pending.
	// ECH then erases at the cursor; a subsequent print must not wrap to row 1.
	if _, err := em.Feed([]byte("abcde\x1b[XZ")); err != nil {
		t.Fatal(err)
	}
	screen := em.Screen()
	if got := screen.Cell(4, 0).Char; got != "Z" {
		t.Fatalf("cell 4,0 = %q, want \"Z\" (ECH should cancel pending wrap)", got)
	}
	for x := 0; x < 5; x++ {
		if got := screen.Cell(x, 1).Char; strings.TrimSpace(got) != "" {
			t.Fatalf("row 1 should be empty, found %q at col %d", got, x)
		}
	}
}
