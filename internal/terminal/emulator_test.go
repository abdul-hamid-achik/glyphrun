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

// TestSimpleEmulatorSplitRuneAcrossFeeds guards the PTY-chunk-boundary fix: a
// multi-byte glyph (here "•", 0xE2 0x80 0xA2) can arrive split across two
// Feed() calls. It must decode as ONE cell and advance the cursor by ONE
// column — not be mangled into raw bytes (which corrupts the cell and desyncs
// every later column on the frame).
func TestSimpleEmulatorSplitRuneAcrossFeeds(t *testing.T) {
	bullet := []byte("•") // 0xE2 0x80 0xA2
	splits := []struct {
		name string
		a, b []byte
	}{
		{"after-1-byte", bullet[:1], bullet[1:]},
		{"after-2-bytes", bullet[:2], bullet[2:]},
	}
	for _, sp := range splits {
		t.Run(sp.name, func(t *testing.T) {
			em := NewEmulator(10, 1)
			if _, err := em.Feed(sp.a); err != nil {
				t.Fatal(err)
			}
			if _, err := em.Feed(sp.b); err != nil {
				t.Fatal(err)
			}
			if got := em.Screen().Cell(0, 0).Char; got != "•" {
				t.Fatalf("cell 0,0 = %q, want bullet", got)
			}
			if got := em.Screen().Cell(1, 0).Char; got != " " {
				t.Fatalf("cell 1,0 = %q, want blank (split bytes leaked into extra cells)", got)
			}
			if x := em.Screen().Cursor().X; x != 1 {
				t.Fatalf("cursor X = %d, want 1 (column desync from a split rune)", x)
			}
		})
	}
}

// TestSimpleEmulatorSplitRuneMidLine proves a split rune in the middle of a
// line does not shift the cells after it (the bug that produced "32 Projects"
// and stale count digits in full-screen TUIs).
func TestSimpleEmulatorSplitRuneMidLine(t *testing.T) {
	em := NewEmulator(10, 1)
	bullet := []byte("•")
	if _, err := em.Feed(append([]byte("AB"), bullet[:2]...)); err != nil {
		t.Fatal(err)
	}
	if _, err := em.Feed(append(append([]byte(nil), bullet[2:]...), []byte("CD")...)); err != nil {
		t.Fatal(err)
	}
	if got := rowText(em.Screen(), 0, 5); got != "AB•CD" {
		t.Fatalf("row = %q, want %q", got, "AB•CD")
	}
	if x := em.Screen().Cursor().X; x != 5 {
		t.Fatalf("cursor X = %d, want 5", x)
	}
}

// TestSimpleEmulatorInvalidByteNotBuffered makes sure a genuinely invalid byte
// (not an incomplete-but-valid prefix) is processed immediately, never stuck in
// the fragment buffer waiting for bytes that will never come.
func TestSimpleEmulatorInvalidByteNotBuffered(t *testing.T) {
	em := NewEmulator(10, 1)
	if _, err := em.Feed([]byte{0xff}); err != nil { // 0xff can't start any rune
		t.Fatal(err)
	}
	if _, err := em.Feed([]byte("X")); err != nil {
		t.Fatal(err)
	}
	if got := em.Screen().Cell(1, 0).Char; got != "X" {
		t.Fatalf("cell 1,0 = %q, want X (invalid byte was wrongly buffered)", got)
	}
}

// TestSimpleEmulatorBareLineFeedPreservesColumn guards the LF-vs-CRLF fix: a
// bare line feed moves DOWN one row but keeps the column (the default
// LNM-reset behavior). Only an explicit carriage return resets the column.
// Full-screen renderers emit a bare LF to step down while keeping the column;
// treating it as CR+LF put the next write at column 0 and corrupted the frame.
func TestSimpleEmulatorBareLineFeedPreservesColumn(t *testing.T) {
	em := NewEmulator(20, 5)
	// CUP to row 1 col 6 -> (5,0); print A (cursor 6,0); bare LF -> (6,1); print B.
	if _, err := em.Feed([]byte("\x1b[1;6HA\nB")); err != nil {
		t.Fatal(err)
	}
	if got := em.Screen().Cell(5, 0).Char; got != "A" {
		t.Fatalf("A should be at (5,0), got %q", got)
	}
	if got := em.Screen().Cell(6, 1).Char; got != "B" {
		t.Fatalf("bare LF must preserve the column: B should be at (6,1), got %q at (6,1) (column reset to 0?)", got)
	}

	// A carriage return DOES reset the column: CR+LF puts B at (0,1).
	em2 := NewEmulator(20, 5)
	if _, err := em2.Feed([]byte("\x1b[1;6HA\r\nB")); err != nil {
		t.Fatal(err)
	}
	if got := em2.Screen().Cell(0, 1).Char; got != "B" {
		t.Fatalf("CR+LF should put B at (0,1), got %q", got)
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

// rowText reads a screen row as trimmed text, independent of snapshot joining.
func rowText(s Screen, y, cols int) string {
	var b strings.Builder
	for x := 0; x < cols; x++ {
		b.WriteString(s.Cell(x, y).Char)
	}
	return strings.TrimRight(b.String(), " ")
}

func TestSimpleEmulatorInsertChars(t *testing.T) {
	em := NewEmulator(6, 1)
	// "abcdef", move to column 3 (CSI 3G -> x=2), insert one blank.
	if _, err := em.Feed([]byte("abcdef\x1b[3G\x1b[@")); err != nil {
		t.Fatal(err)
	}
	if got := rowText(em.Screen(), 0, 6); got != "ab cde" {
		t.Fatalf("ICH row = %q, want %q", got, "ab cde")
	}
}

func TestSimpleEmulatorDeleteChars(t *testing.T) {
	em := NewEmulator(6, 1)
	// "abcdef", move to column 3 (x=2), delete one char.
	if _, err := em.Feed([]byte("abcdef\x1b[3G\x1b[P")); err != nil {
		t.Fatal(err)
	}
	if got := rowText(em.Screen(), 0, 6); got != "abdef" {
		t.Fatalf("DCH row = %q, want %q", got, "abdef")
	}
}

func TestSimpleEmulatorScrollRegionLineFeed(t *testing.T) {
	em := NewEmulator(3, 4)
	// Scroll region rows 2-3 (0-based 1-2); fill all rows; LF at bottom margin
	// scrolls only within the region.
	seq := "\x1b[2;3r" + // set region (homes cursor to 0,0)
		"AAA" + "\x1b[2;1HBBB" + "\x1b[3;1HCCC" + "\x1b[4;1HDDD" +
		"\x1b[3;1H" + "\n" // cursor to region bottom, line feed
	if _, err := em.Feed([]byte(seq)); err != nil {
		t.Fatal(err)
	}
	s := em.Screen()
	for y, want := range []string{"AAA", "CCC", "", "DDD"} {
		if got := rowText(s, y, 3); got != want {
			t.Errorf("after scroll: row %d = %q, want %q", y, got, want)
		}
	}
}

func TestSimpleEmulatorReverseIndexAtTop(t *testing.T) {
	em := NewEmulator(3, 3)
	// Fill rows, home cursor, reverse index (ESC M) scrolls the screen down.
	if _, err := em.Feed([]byte("AAA\x1b[2;1HBBB\x1b[3;1HCCC\x1b[H\x1bM")); err != nil {
		t.Fatal(err)
	}
	s := em.Screen()
	for y, want := range []string{"", "AAA", "BBB"} {
		if got := rowText(s, y, 3); got != want {
			t.Errorf("after RI: row %d = %q, want %q", y, got, want)
		}
	}
}

func TestSimpleEmulatorInsertLines(t *testing.T) {
	em := NewEmulator(3, 4)
	// Fill rows; move to row 2 (y=1); insert one blank line.
	if _, err := em.Feed([]byte("AAA\x1b[2;1HBBB\x1b[3;1HCCC\x1b[4;1HDDD\x1b[2;1H\x1b[L")); err != nil {
		t.Fatal(err)
	}
	s := em.Screen()
	for y, want := range []string{"AAA", "", "BBB", "CCC"} { // DDD pushed off
		if got := rowText(s, y, 3); got != want {
			t.Errorf("after IL: row %d = %q, want %q", y, got, want)
		}
	}
}

func TestSimpleEmulatorDeleteLines(t *testing.T) {
	em := NewEmulator(3, 4)
	// Fill rows; move to row 2 (y=1); delete that line.
	if _, err := em.Feed([]byte("AAA\x1b[2;1HBBB\x1b[3;1HCCC\x1b[4;1HDDD\x1b[2;1H\x1b[M")); err != nil {
		t.Fatal(err)
	}
	s := em.Screen()
	for y, want := range []string{"AAA", "CCC", "DDD", ""} {
		if got := rowText(s, y, 3); got != want {
			t.Errorf("after DL: row %d = %q, want %q", y, got, want)
		}
	}
}

func TestSimpleEmulatorSaveRestoreCursor(t *testing.T) {
	em := NewEmulator(5, 3)
	// Move to (x=2,y=1), set bold+red, save (ESC 7). Move home, reset SGR.
	// Restore (ESC 8) and type: the glyph lands at the saved spot with the
	// saved style.
	if _, err := em.Feed([]byte("\x1b[2;3H\x1b[1;31m\x1b7\x1b[H\x1b[0m\x1b8Z")); err != nil {
		t.Fatal(err)
	}
	cell := em.Screen().Cell(2, 1)
	if cell.Char != "Z" || !cell.Style.Bold || cell.Style.Fg != "red" {
		t.Fatalf("restored cell = %#v, want Z bold red at (2,1)", cell)
	}
}

func TestSimpleEmulatorOriginMode(t *testing.T) {
	em := NewEmulator(4, 5)
	// Region rows 2-4 (0-based 1-3), origin mode on. CUP 1;1 maps to the
	// region top; an out-of-region row clamps to the bottom margin.
	if _, err := em.Feed([]byte("\x1b[2;4r\x1b[?6h\x1b[1;1HX\x1b[10;1HY")); err != nil {
		t.Fatal(err)
	}
	s := em.Screen()
	if got := s.Cell(0, 1).Char; got != "X" {
		t.Errorf("origin CUP 1;1 wrote to %q, want X at row 1", got)
	}
	if got := s.Cell(0, 3).Char; got != "Y" {
		t.Errorf("origin CUP 10;1 should clamp to bottom margin (row 3), got %q", got)
	}
}

func TestSimpleEmulatorConsumesStringSequences(t *testing.T) {
	tests := []struct {
		name string
		seq  string
	}{
		{name: "DCS/Sixel", seq: "AB\x1bPq#0;2;0;0;0~~@@vvvv\x1b\\CD"},
		{name: "APC/Kitty", seq: "AB\x1b_Gf=24,s=10,v=10;payloadbytes\x1b\\CD"},
		{name: "PM", seq: "AB\x1b^private message\x1b\\CD"},
		{name: "SOS", seq: "AB\x1bXstart of string\x1b\\CD"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			em := NewEmulator(40, 1)
			if _, err := em.Feed([]byte(tc.seq)); err != nil {
				t.Fatal(err)
			}
			got := rowText(em.Screen(), 0, 40)
			if got != "ABCD" {
				t.Errorf("string sequence leaked onto screen: row = %q, want %q", got, "ABCD")
			}
		})
	}
}

func TestSimpleEmulatorTracksMouseModes(t *testing.T) {
	em := NewEmulator(20, 3)
	if em.MouseTrackingMode() || em.MouseSGRMode() {
		t.Fatal("mouse modes should start disabled")
	}
	if _, err := em.Feed([]byte("\x1b[?1000h\x1b[?1006h")); err != nil {
		t.Fatal(err)
	}
	if !em.MouseTrackingMode() {
		t.Error("expected mouse tracking enabled after ?1000h")
	}
	if !em.MouseSGRMode() {
		t.Error("expected SGR mouse encoding enabled after ?1006h")
	}
	if _, err := em.Feed([]byte("\x1b[?1000l\x1b[?1006l")); err != nil {
		t.Fatal(err)
	}
	if em.MouseTrackingMode() || em.MouseSGRMode() {
		t.Error("expected mouse modes disabled after reset")
	}
}

func TestSimpleEmulatorTracksOSC8Hyperlinks(t *testing.T) {
	em := NewEmulator(40, 1)
	// "go ⟨docs⟩ end" where docs links to the project URL.
	seq := "go \x1b]8;;https://glyphrun.dev\x1b\\docs\x1b]8;;\x1b\\ end"
	if _, err := em.Feed([]byte(seq)); err != nil {
		t.Fatal(err)
	}
	s := em.Screen()
	// "go " (cols 0-2) carry no link; "docs" (cols 3-6) do; " end" does not.
	if l := s.Cell(0, 0).Link; l != "" {
		t.Errorf("cell 0 should have no link, got %q", l)
	}
	for x := 3; x <= 6; x++ {
		if l := s.Cell(x, 0).Link; l != "https://glyphrun.dev" {
			t.Errorf("cell %d link = %q, want the project URL", x, l)
		}
	}
	if l := s.Cell(8, 0).Link; l != "" {
		t.Errorf("cell after the link should have no link, got %q", l)
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

// TestSimpleEmulatorCursorBackwardTabulation guards CBT (CSI Z): Bubble Tea
// v2's cell-diff renderer moves backward to an 8-column tab stop before
// rewriting a styled segment. Ignoring CBT left the cursor in place, so the
// segment landed to the right of its true cells and tore an otherwise-static
// row (observed as "Recovery rery rt review" in downstream terminal specs).
func TestSimpleEmulatorCursorBackwardTabulation(t *testing.T) {
	em := NewEmulator(80, 3)
	if _, err := em.Feed([]byte("\x1b[2J\x1b[H")); err != nil {
		t.Fatal(err)
	}
	// Column math is 0-based internally: after CUP to column 47 (1-based),
	// one CBT lands on column 40 (0-based), the previous 8-column stop.
	if _, err := em.Feed([]byte("\x1b[1;47H\x1b[Z")); err != nil {
		t.Fatal(err)
	}
	if got := em.Screen().Cursor().X; got != 40 {
		t.Fatalf("CBT cursor column = %d, want 40", got)
	}
	// A count moves that many stops; clamping stops at column zero.
	if _, err := em.Feed([]byte("\x1b[1;47H\x1b[3Z")); err != nil {
		t.Fatal(err)
	}
	if got := em.Screen().Cursor().X; got != 24 {
		t.Fatalf("CBT×3 cursor column = %d, want 24", got)
	}
	if _, err := em.Feed([]byte("\x1b[1;3H\x1b[9Z")); err != nil {
		t.Fatal(err)
	}
	if got := em.Screen().Cursor().X; got != 0 {
		t.Fatalf("CBT overshoot cursor column = %d, want 0", got)
	}
	// CHT is the forward counterpart.
	if _, err := em.Feed([]byte("\x1b[1;1H\x1b[2I")); err != nil {
		t.Fatal(err)
	}
	if got := em.Screen().Cursor().X; got != 16 {
		t.Fatalf("CHT×2 cursor column = %d, want 16", got)
	}
}

// TestSimpleEmulatorCBTSegmentRewriteReplay replays the downstream failure
// byte-for-byte: a row is written, the cursor ends past it, LF LF preserves
// the column, CBT moves back to the tab stop, and a partially-underlined
// segment rewrite must overlay the exact cells it targets.
func TestSimpleEmulatorCBTSegmentRewriteReplay(t *testing.T) {
	em := NewEmulator(80, 24)
	feed := "\x1b[2J" +
		"\x1b[3;19HRecovery receipt review" + // row write: title spans 0-based cols 18-40
		"\x1b[1;20H" + // cursor parked on the filter input row
		"filter" + // ends at 0-based column 25
		"\n\n" + // LF preserves the column; lands on row 3 (0-based 2), column 25
		"\x1b[Z" + // CBT → previous 8-column stop, column 24
		"\x1b[4mry \x1b[24mr" // segment rewrite: title chars 6-9 ("ry r") live at cols 24-27
	if _, err := em.Feed([]byte(feed)); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(em.Screen().Text(), "\n")
	row := ""
	if len(lines) > 2 {
		row = lines[2]
	}
	if !strings.Contains(row, "Recovery receipt review") {
		t.Fatalf("segment rewrite tore the row: %q", row)
	}
}
