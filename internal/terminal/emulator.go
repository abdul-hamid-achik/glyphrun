package terminal

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type SimpleEmulator struct {
	mu      sync.RWMutex
	cols    int
	rows    int
	cursor  Cursor
	cells   [][]Cell
	seq     int64
	escMode bool
	escBuf  []byte
	style   Style

	// pendingWrap implements the VT100 deferred-autowrap quirk: writing a
	// glyph into the last column does NOT advance to the next row. Instead
	// the cursor stays on the last column with this flag set, and the wrap
	// only happens when the next glyph is printed. Real terminals (xterm,
	// etc.) behave this way, and full-screen TUIs (e.g. Bubble Tea) rely on
	// it to paint the bottom-right cell without scrolling. Without it, any
	// frame that fills the full terminal width drifts by a row.
	pendingWrap bool

	bracketedPaste      bool
	alternateScreen     bool
	alternateScreenUsed bool

	// link is the currently open OSC 8 hyperlink URI. It is attached to every
	// glyph printed until the link is closed (OSC 8 with an empty URI). Unlike
	// SGR attributes it is not reset by SGR 0.
	link string

	// mouseTracking is set when the target requests mouse reporting (private
	// modes 1000/1002/1003). mouseSGR is set for the SGR extended encoding
	// (1006). They let the runner gate and encode `mouse:` steps correctly.
	mouseTracking bool
	mouseSGR      bool

	// scrollTop/scrollBottom bound the DECSTBM scroll region (inclusive,
	// 0-based). Line feeds, reverse index, IL/DL, and SU/SD all operate within
	// this region. They default to the full screen.
	scrollTop    int
	scrollBottom int
	// originMode (DECOM) makes cursor row addressing relative to scrollTop and
	// confines the cursor to the scroll region.
	originMode bool

	// Saved cursor/style state for DECSC/DECRC (ESC 7 / ESC 8) and the
	// SCO save/restore (CSI s / CSI u).
	savedCursor Cursor
	savedStyle  Style
	savedOrigin bool
	hasSaved    bool
}

func NewEmulator(cols, rows int) *SimpleEmulator {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	e := &SimpleEmulator{cursor: Cursor{Visible: true}}
	_ = e.Resize(cols, rows)
	return e
}

func (e *SimpleEmulator) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("terminal size must be positive")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cols = cols
	e.rows = rows
	e.cells = make([][]Cell, rows)
	for y := 0; y < rows; y++ {
		e.cells[y] = make([]Cell, cols)
		for x := 0; x < cols; x++ {
			e.cells[y][x] = Cell{X: x, Y: y, Char: " ", Width: 1}
		}
	}
	e.cursor = Cursor{X: 0, Y: 0, Visible: true}
	e.pendingWrap = false
	e.scrollTop = 0
	e.scrollBottom = rows - 1
	e.originMode = false
	return nil
}

func (e *SimpleEmulator) Feed(data []byte) ([]Frame, error) {
	raw := append([]byte(nil), data...)
	e.mu.Lock()
	defer e.mu.Unlock()
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			r = rune(data[0])
		}
		data = data[size:]
		e.feedRune(r)
	}
	e.seq++
	snap := e.snapshotLocked()
	frame := Frame{
		Seq:            e.seq,
		Time:           time.Now().UTC().Format(time.RFC3339Nano),
		Kind:           "output",
		Screen:         &snap,
		RawBytesBase64: base64.StdEncoding.EncodeToString(raw),
	}
	return []Frame{frame}, nil
}

func (e *SimpleEmulator) Screen() Screen {
	e.mu.RLock()
	defer e.mu.RUnlock()
	snap := e.snapshotLocked()
	return snapshotScreen{snapshot: snap}
}

func (e *SimpleEmulator) BracketedPasteMode() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.bracketedPaste
}

func (e *SimpleEmulator) AlternateScreenMode() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.alternateScreen
}

func (e *SimpleEmulator) AlternateScreenUsed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.alternateScreenUsed
}

func (e *SimpleEmulator) feedRune(r rune) {
	if e.escMode {
		e.escBuf = append(e.escBuf, string(r)...)
		if escapeComplete(e.escBuf, r) {
			e.applyEscape(string(e.escBuf))
			e.escMode = false
			e.escBuf = nil
		}
		return
	}
	switch r {
	case '\x1b':
		e.escMode = true
		e.escBuf = nil
	case '\r':
		e.pendingWrap = false
		e.cursor.X = 0
	case '\n':
		e.newLine()
	case '\b':
		e.pendingWrap = false
		if e.cursor.X > 0 {
			e.cursor.X--
		}
	case '\t':
		// Tab is cursor movement, not a write: advance to the next 8-column
		// tab stop without overwriting the cells it passes over.
		e.pendingWrap = false
		e.cursor.X = clamp(((e.cursor.X/8)+1)*8, 0, e.cols-1)
	default:
		// Skip C0 controls below 0x20 and DEL (0x7f); both are non-printing.
		if r >= 0x20 && r != 0x7f {
			e.putRune(r)
		}
	}
}

func escapeComplete(buf []byte, r rune) bool {
	if len(buf) == 0 {
		return false
	}
	if buf[0] == ']' {
		return r == '\a' || strings.HasSuffix(string(buf), "\x1b\\")
	}
	if buf[0] != '[' {
		return true
	}
	if len(buf) == 1 {
		return false
	}
	return r >= '@' && r <= '~'
}

func (e *SimpleEmulator) applyEscape(seq string) {
	if seq == "" {
		return
	}
	if strings.HasPrefix(seq, "]") {
		e.applyOSC(seq)
		return
	}
	if !strings.HasPrefix(seq, "[") {
		// Non-CSI ("ESC <byte>") sequences.
		switch seq {
		case "7": // DECSC — save cursor
			e.saveCursor()
		case "8": // DECRC — restore cursor
			e.restoreCursor()
		case "D": // IND — index (line feed, no carriage return)
			e.index()
		case "M": // RI — reverse index
			e.reverseIndex()
		case "E": // NEL — next line (CR + index)
			e.cursor.X = 0
			e.index()
		}
		return
	}
	body := strings.TrimPrefix(seq, "[")
	if strings.HasPrefix(body, "?") {
		e.applyPrivateMode(body)
		return
	}
	if strings.HasSuffix(body, "J") {
		e.clearScreen(csiNumber(strings.TrimSuffix(body, "J"), 0))
		return
	}
	if strings.HasSuffix(body, "K") {
		e.clearLine(csiNumber(strings.TrimSuffix(body, "K"), 0))
		return
	}
	if strings.HasSuffix(body, "H") || strings.HasSuffix(body, "f") {
		args := strings.TrimRight(body, "Hf")
		row, col := 1, 1
		if args != "" {
			parts := strings.Split(args, ";")
			if len(parts) > 0 && parts[0] != "" {
				row, _ = strconv.Atoi(parts[0])
			}
			if len(parts) > 1 && parts[1] != "" {
				col, _ = strconv.Atoi(parts[1])
			}
		}
		e.pendingWrap = false
		if e.originMode {
			// Row is relative to the scroll region and confined to it.
			e.cursor.Y = clamp(e.scrollTop+row-1, e.scrollTop, e.scrollBottom)
		} else {
			e.cursor.Y = clamp(row-1, 0, e.rows-1)
		}
		e.cursor.X = clamp(col-1, 0, e.cols-1)
		return
	}
	if strings.HasSuffix(body, "A") {
		e.pendingWrap = false
		e.cursor.Y = clamp(e.cursor.Y-csiNumber(strings.TrimSuffix(body, "A"), 1), 0, e.rows-1)
		return
	}
	if strings.HasSuffix(body, "B") {
		e.pendingWrap = false
		e.cursor.Y = clamp(e.cursor.Y+csiNumber(strings.TrimSuffix(body, "B"), 1), 0, e.rows-1)
		return
	}
	if strings.HasSuffix(body, "C") {
		e.pendingWrap = false
		e.cursor.X = clamp(e.cursor.X+csiNumber(strings.TrimSuffix(body, "C"), 1), 0, e.cols-1)
		return
	}
	if strings.HasSuffix(body, "D") {
		e.pendingWrap = false
		e.cursor.X = clamp(e.cursor.X-csiNumber(strings.TrimSuffix(body, "D"), 1), 0, e.cols-1)
		return
	}
	if strings.HasSuffix(body, "G") {
		// CHA — cursor horizontal absolute (1-based column).
		e.pendingWrap = false
		e.cursor.X = clamp(csiNumber(strings.TrimSuffix(body, "G"), 1)-1, 0, e.cols-1)
		return
	}
	if strings.HasSuffix(body, "d") {
		// VPA — line position absolute (1-based row), column unchanged.
		// Bubble Tea's renderer uses this to jump between repaint regions;
		// without it, diff frames write to the wrong row.
		e.pendingWrap = false
		e.cursor.Y = clamp(csiNumber(strings.TrimSuffix(body, "d"), 1)-1, 0, e.rows-1)
		return
	}
	if strings.HasSuffix(body, "X") {
		// ECH — erase N characters from the cursor without moving it.
		// Erasing cancels any deferred autowrap so a subsequent print does
		// not spuriously advance to the next line.
		e.pendingWrap = false
		n := csiNumber(strings.TrimSuffix(body, "X"), 1)
		if e.cursor.Y >= 0 && e.cursor.Y < e.rows {
			for i := 0; i < n; i++ {
				x := e.cursor.X + i
				if x >= 0 && x < e.cols {
					e.cells[e.cursor.Y][x] = Cell{X: x, Y: e.cursor.Y, Char: " ", Width: 1}
				}
			}
		}
		return
	}
	if strings.HasSuffix(body, "r") {
		// DECSTBM — set top/bottom scroll margins.
		e.setScrollRegion(strings.TrimSuffix(body, "r"))
		return
	}
	if strings.HasSuffix(body, "L") {
		// IL — insert blank lines at the cursor within the scroll region.
		e.insertLines(csiNumber(strings.TrimSuffix(body, "L"), 1))
		return
	}
	if strings.HasSuffix(body, "M") {
		// DL — delete lines at the cursor within the scroll region.
		e.deleteLines(csiNumber(strings.TrimSuffix(body, "M"), 1))
		return
	}
	if strings.HasSuffix(body, "@") {
		// ICH — insert blank characters at the cursor.
		e.insertChars(csiNumber(strings.TrimSuffix(body, "@"), 1))
		return
	}
	if strings.HasSuffix(body, "P") {
		// DCH — delete characters at the cursor.
		e.deleteChars(csiNumber(strings.TrimSuffix(body, "P"), 1))
		return
	}
	if strings.HasSuffix(body, "S") {
		// SU — scroll the region up.
		e.scrollUp(csiNumber(strings.TrimSuffix(body, "S"), 1))
		return
	}
	if strings.HasSuffix(body, "T") {
		// SD — scroll the region down.
		e.scrollDown(csiNumber(strings.TrimSuffix(body, "T"), 1))
		return
	}
	if strings.HasSuffix(body, "s") {
		// SCOSC — save cursor (SCO variant of DECSC).
		e.saveCursor()
		return
	}
	if strings.HasSuffix(body, "u") {
		// SCORC — restore cursor (SCO variant of DECRC).
		e.restoreCursor()
		return
	}
	if strings.HasSuffix(body, "m") {
		e.applySGR(strings.TrimSuffix(body, "m"))
		return
	}
}

// applyOSC handles Operating System Command sequences. Only OSC 8 (hyperlinks)
// changes state today; other OSCs (e.g. window title) are ignored. The format
// is `8 ; params ; URI`; an empty URI closes the current link.
func (e *SimpleEmulator) applyOSC(seq string) {
	body := strings.TrimPrefix(seq, "]")
	body = strings.TrimSuffix(body, "\a")
	body = strings.TrimSuffix(body, "\x1b\\")
	if !strings.HasPrefix(body, "8;") {
		return
	}
	rest := strings.TrimPrefix(body, "8;")
	idx := strings.IndexByte(rest, ';')
	if idx < 0 {
		e.link = ""
		return
	}
	e.link = rest[idx+1:]
}

func (e *SimpleEmulator) applyPrivateMode(body string) {
	enable := strings.HasSuffix(body, "h")
	args := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(body, "?"), "h"), "l")
	for _, arg := range strings.Split(args, ";") {
		switch arg {
		case "6":
			// DECOM — origin mode. Toggling it homes the cursor to the
			// (region) origin.
			e.originMode = enable
			e.pendingWrap = false
			e.cursor.X = 0
			if enable {
				e.cursor.Y = e.scrollTop
			} else {
				e.cursor.Y = 0
			}
		case "25":
			e.cursor.Visible = enable
		case "1049":
			e.alternateScreen = enable
			if enable {
				e.alternateScreenUsed = true
				e.clear()
				e.pendingWrap = false
				e.cursor.X = 0
				e.cursor.Y = 0
			}
		case "1000", "1002", "1003":
			// Mouse tracking: normal (1000), button-event (1002), any-event
			// (1003). We track whether any mode is on, not which.
			e.mouseTracking = enable
		case "1006":
			// SGR extended mouse encoding.
			e.mouseSGR = enable
		case "2004":
			e.bracketedPaste = enable
		}
	}
}

// MouseTrackingMode reports whether the target requested mouse reporting.
func (e *SimpleEmulator) MouseTrackingMode() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mouseTracking
}

// MouseSGRMode reports whether the target enabled the SGR (1006) mouse encoding.
func (e *SimpleEmulator) MouseSGRMode() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mouseSGR
}

func (e *SimpleEmulator) applySGR(args string) {
	if args == "" {
		args = "0"
	}
	// Parse the parameter list to ints up front so multi-parameter colors
	// (38;5;n and 38;2;r;g;b, and their 48 background twins) can consume the
	// parameters that follow the introducer.
	parts := strings.Split(args, ";")
	nums := make([]int, len(parts))
	for i, p := range parts {
		if p == "" {
			nums[i] = 0 // ANSI: an empty parameter defaults to 0
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		nums[i] = n
	}
	for i := 0; i < len(nums); i++ {
		code := nums[i]
		switch {
		case code == 0:
			e.style = Style{}
		case code == 1:
			e.style.Bold = true
		case code == 2:
			e.style.Dim = true
		case code == 3:
			e.style.Italic = true
		case code == 4:
			e.style.Underline = true
		case code == 7:
			e.style.Reverse = true
		case code == 22:
			e.style.Bold = false
			e.style.Dim = false
		case code == 23:
			e.style.Italic = false
		case code == 24:
			e.style.Underline = false
		case code == 27:
			e.style.Reverse = false
		case code >= 30 && code <= 37:
			e.style.Fg = standardColorName(code - 30)
		case code == 38:
			if c, adv := extendedColor(nums[i+1:]); c != "" {
				e.style.Fg = c
				i += adv
			}
		case code == 39:
			e.style.Fg = ""
		case code >= 40 && code <= 47:
			e.style.Bg = standardColorName(code - 40)
		case code == 48:
			if c, adv := extendedColor(nums[i+1:]); c != "" {
				e.style.Bg = c
				i += adv
			}
		case code == 49:
			e.style.Bg = ""
		case code >= 90 && code <= 97:
			e.style.Fg = brightColorName(code - 90)
		case code >= 100 && code <= 107:
			e.style.Bg = brightColorName(code - 100)
		}
	}
}

// extendedColor parses the parameters following an SGR 38/48 introducer and
// returns the canonical color string plus how many parameters it consumed.
// 5;n selects a 256-color palette index; 2;r;g;b selects a truecolor value.
func extendedColor(rest []int) (string, int) {
	if len(rest) == 0 {
		return "", 0
	}
	switch rest[0] {
	case 5:
		if len(rest) < 2 {
			return "", 0
		}
		return colorForIndex(rest[1]), 2
	case 2:
		if len(rest) < 4 {
			return "", 0
		}
		return fmt.Sprintf("#%02x%02x%02x", clampByte(rest[1]), clampByte(rest[2]), clampByte(rest[3])), 4
	}
	return "", 0
}

// colorForIndex maps a 256-color palette index to its canonical string. The
// first 16 entries reuse the named colors so `cell: { style: { fg } }`
// assertions read the same whether the app used SGR 31 or 38;5;1; indices
// 16-255 are stored as their decimal index.
func colorForIndex(n int) string {
	switch {
	case n < 0 || n > 255:
		return ""
	case n < 8:
		return standardColorName(n)
	case n < 16:
		return brightColorName(n - 8)
	default:
		return strconv.Itoa(n)
	}
}

var standardColorNames = [8]string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}

func standardColorName(n int) string {
	if n < 0 || n > 7 {
		return ""
	}
	return standardColorNames[n]
}

func brightColorName(n int) string {
	if n < 0 || n > 7 {
		return ""
	}
	return "bright" + standardColorNames[n]
}

func clampByte(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func csiNumber(text string, fallback int) int {
	if text == "" {
		return fallback
	}
	value, err := strconv.Atoi(text)
	if err != nil || value == 0 {
		return fallback
	}
	return value
}

func (e *SimpleEmulator) putRune(r rune) {
	if e.cursor.Y < 0 || e.cursor.Y >= e.rows {
		return
	}
	// Resolve a deferred wrap from the previous last-column write before
	// printing this glyph.
	if e.pendingWrap {
		e.newLine()
		e.pendingWrap = false
	}
	if e.cursor.Y >= 0 && e.cursor.Y < e.rows && e.cursor.X >= 0 && e.cursor.X < e.cols {
		e.cells[e.cursor.Y][e.cursor.X].Char = string(r)
		e.cells[e.cursor.Y][e.cursor.X].Style = e.style
		e.cells[e.cursor.Y][e.cursor.X].Link = e.link
	}
	if e.cursor.X >= e.cols-1 {
		// Last column: stay put and defer the wrap (VT100 autowrap quirk).
		e.cursor.X = e.cols - 1
		e.pendingWrap = true
	} else {
		e.cursor.X++
	}
}

// newLine is a line feed with carriage return (the screen's '\n' handling):
// move to column 0 and index down one line, scrolling within the region at the
// bottom margin.
func (e *SimpleEmulator) newLine() {
	e.pendingWrap = false
	e.cursor.X = 0
	e.index()
}

// index (IND) moves the cursor down one line. At the bottom margin it scrolls
// the scroll region up instead of moving past it.
func (e *SimpleEmulator) index() {
	e.pendingWrap = false
	switch {
	case e.cursor.Y == e.scrollBottom:
		e.scrollUp(1)
	case e.cursor.Y < e.rows-1:
		e.cursor.Y++
	}
}

// reverseIndex (RI) moves the cursor up one line. At the top margin it scrolls
// the scroll region down.
func (e *SimpleEmulator) reverseIndex() {
	e.pendingWrap = false
	switch {
	case e.cursor.Y == e.scrollTop:
		e.scrollDown(1)
	case e.cursor.Y > 0:
		e.cursor.Y--
	}
}

// setScrollRegion implements DECSTBM. An empty or invalid region resets to the
// full screen; either way the cursor is homed to the (region) origin.
func (e *SimpleEmulator) setScrollRegion(args string) {
	top, bottom := 1, e.rows
	if args != "" {
		parts := strings.Split(args, ";")
		if len(parts) > 0 && parts[0] != "" {
			top, _ = strconv.Atoi(parts[0])
		}
		if len(parts) > 1 && parts[1] != "" {
			bottom, _ = strconv.Atoi(parts[1])
		}
	}
	t := clamp(top-1, 0, e.rows-1)
	b := clamp(bottom-1, 0, e.rows-1)
	if t >= b {
		e.scrollTop = 0
		e.scrollBottom = e.rows - 1
	} else {
		e.scrollTop = t
		e.scrollBottom = b
	}
	e.pendingWrap = false
	e.cursor.X = 0
	if e.originMode {
		e.cursor.Y = e.scrollTop
	} else {
		e.cursor.Y = 0
	}
}

// scrollUp moves the scroll region's lines up by n, blanking the lines that
// scroll in at the bottom margin.
func (e *SimpleEmulator) scrollUp(n int) {
	top, bot := e.scrollTop, e.scrollBottom
	if n <= 0 || top < 0 || bot >= e.rows || top > bot {
		return
	}
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for y := top; y <= bot; y++ {
		if y+n <= bot {
			copy(e.cells[y], e.cells[y+n])
		} else {
			e.blankRow(y)
		}
	}
	e.reindex()
}

// scrollDown moves the scroll region's lines down by n, blanking the lines that
// scroll in at the top margin.
func (e *SimpleEmulator) scrollDown(n int) {
	top, bot := e.scrollTop, e.scrollBottom
	if n <= 0 || top < 0 || bot >= e.rows || top > bot {
		return
	}
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for y := bot; y >= top; y-- {
		if y-n >= top {
			copy(e.cells[y], e.cells[y-n])
		} else {
			e.blankRow(y)
		}
	}
	e.reindex()
}

// insertLines (IL) opens n blank lines at the cursor row, pushing lines below
// it down within the scroll region. A no-op when the cursor is outside the
// region.
func (e *SimpleEmulator) insertLines(n int) {
	if e.cursor.Y < e.scrollTop || e.cursor.Y > e.scrollBottom {
		return
	}
	top, bot := e.cursor.Y, e.scrollBottom
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for y := bot; y >= top; y-- {
		if y-n >= top {
			copy(e.cells[y], e.cells[y-n])
		} else {
			e.blankRow(y)
		}
	}
	e.reindex()
	e.cursor.X = 0
	e.pendingWrap = false
}

// deleteLines (DL) removes n lines at the cursor row, pulling lines below it up
// within the scroll region. A no-op when the cursor is outside the region.
func (e *SimpleEmulator) deleteLines(n int) {
	if e.cursor.Y < e.scrollTop || e.cursor.Y > e.scrollBottom {
		return
	}
	top, bot := e.cursor.Y, e.scrollBottom
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for y := top; y <= bot; y++ {
		if y+n <= bot {
			copy(e.cells[y], e.cells[y+n])
		} else {
			e.blankRow(y)
		}
	}
	e.reindex()
	e.cursor.X = 0
	e.pendingWrap = false
}

// insertChars (ICH) opens n blank cells at the cursor, shifting the rest of the
// line right; cells pushed past the right edge are lost.
func (e *SimpleEmulator) insertChars(n int) {
	y := e.cursor.Y
	if y < 0 || y >= e.rows || n <= 0 {
		return
	}
	for x := e.cols - 1; x >= e.cursor.X; x-- {
		if src := x - n; src >= e.cursor.X {
			e.cells[y][x] = e.cells[y][src]
			e.cells[y][x].X = x
		} else {
			e.cells[y][x] = Cell{X: x, Y: y, Char: " ", Width: 1}
		}
	}
	e.pendingWrap = false
}

// deleteChars (DCH) removes n cells at the cursor, shifting the rest of the
// line left and blanking the right edge.
func (e *SimpleEmulator) deleteChars(n int) {
	y := e.cursor.Y
	if y < 0 || y >= e.rows || n <= 0 {
		return
	}
	for x := e.cursor.X; x < e.cols; x++ {
		if src := x + n; src < e.cols {
			e.cells[y][x] = e.cells[y][src]
			e.cells[y][x].X = x
		} else {
			e.cells[y][x] = Cell{X: x, Y: y, Char: " ", Width: 1}
		}
	}
	e.pendingWrap = false
}

func (e *SimpleEmulator) saveCursor() {
	e.savedCursor = e.cursor
	e.savedStyle = e.style
	e.savedOrigin = e.originMode
	e.hasSaved = true
}

func (e *SimpleEmulator) restoreCursor() {
	e.pendingWrap = false
	if !e.hasSaved {
		e.cursor.X = 0
		e.cursor.Y = 0
		return
	}
	e.cursor = e.savedCursor
	e.style = e.savedStyle
	e.originMode = e.savedOrigin
}

func (e *SimpleEmulator) blankRow(y int) {
	if y < 0 || y >= e.rows {
		return
	}
	for x := 0; x < e.cols; x++ {
		e.cells[y][x] = Cell{X: x, Y: y, Char: " ", Width: 1}
	}
}

// reindex restores each cell's X/Y after a row-level move (copy shuffles whole
// rows, which carry stale coordinates).
func (e *SimpleEmulator) reindex() {
	for y := 0; y < e.rows; y++ {
		for x := 0; x < e.cols; x++ {
			e.cells[y][x].X = x
			e.cells[y][x].Y = y
		}
	}
}

func (e *SimpleEmulator) clear() {
	for y := 0; y < e.rows; y++ {
		for x := 0; x < e.cols; x++ {
			e.cells[y][x] = Cell{X: x, Y: y, Char: " ", Width: 1}
		}
	}
}

func (e *SimpleEmulator) clearScreen(mode int) {
	switch mode {
	case 1:
		for y := 0; y <= e.cursor.Y; y++ {
			end := e.cols
			if y == e.cursor.Y {
				end = e.cursor.X + 1
			}
			for x := 0; x < end; x++ {
				e.cells[y][x] = Cell{X: x, Y: y, Char: " ", Width: 1}
			}
		}
	case 2, 3:
		e.clear()
	default:
		for y := e.cursor.Y; y < e.rows; y++ {
			start := 0
			if y == e.cursor.Y {
				start = e.cursor.X
			}
			for x := start; x < e.cols; x++ {
				e.cells[y][x] = Cell{X: x, Y: y, Char: " ", Width: 1}
			}
		}
	}
}

func (e *SimpleEmulator) clearLine(mode int) {
	if e.cursor.Y < 0 || e.cursor.Y >= e.rows {
		return
	}
	start, end := e.cursor.X, e.cols
	switch mode {
	case 1:
		start, end = 0, e.cursor.X+1
	case 2:
		start, end = 0, e.cols
	}
	for x := start; x < end && x < e.cols; x++ {
		e.cells[e.cursor.Y][x] = Cell{X: x, Y: e.cursor.Y, Char: " ", Width: 1}
	}
}

func (e *SimpleEmulator) snapshotLocked() ScreenSnapshot {
	cells := make([]Cell, 0, e.cols*e.rows)
	var b strings.Builder
	for y := 0; y < e.rows; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		row := make([]string, e.cols)
		for x := 0; x < e.cols; x++ {
			cell := e.cells[y][x]
			if cell.Char == "" {
				cell.Char = " "
			}
			cells = append(cells, cell)
			row[x] = cell.Char
		}
		b.WriteString(strings.TrimRight(strings.Join(row, ""), " "))
	}
	return ScreenSnapshot{
		Cols:   e.cols,
		Rows:   e.rows,
		Cursor: e.cursor,
		Cells:  cells,
		Text:   strings.TrimRight(b.String(), "\n"),
	}
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

type snapshotScreen struct {
	snapshot ScreenSnapshot
}

func (s snapshotScreen) Size() Size {
	return Size{Cols: s.snapshot.Cols, Rows: s.snapshot.Rows}
}

func (s snapshotScreen) Cursor() Cursor {
	return s.snapshot.Cursor
}

func (s snapshotScreen) Text() string {
	return s.snapshot.Text
}

func (s snapshotScreen) Region(x, y, width, height int) Region {
	cells := make([]Cell, 0, width*height)
	for yy := y; yy < y+height; yy++ {
		for xx := x; xx < x+width; xx++ {
			cells = append(cells, s.Cell(xx, yy))
		}
	}
	return snapshotRegion{width: width, height: height, cells: cells}
}

func (s snapshotScreen) Cell(x, y int) Cell {
	if x < 0 || y < 0 || x >= s.snapshot.Cols || y >= s.snapshot.Rows {
		return Cell{X: x, Y: y, Char: " ", Width: 1}
	}
	idx := y*s.snapshot.Cols + x
	if idx < 0 || idx >= len(s.snapshot.Cells) {
		return Cell{X: x, Y: y, Char: " ", Width: 1}
	}
	return s.snapshot.Cells[idx]
}

func (s snapshotScreen) Snapshot() ScreenSnapshot {
	return s.snapshot
}

type snapshotRegion struct {
	width  int
	height int
	cells  []Cell
}

func (r snapshotRegion) Text() string {
	var b strings.Builder
	for y := 0; y < r.height; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		var row strings.Builder
		for x := 0; x < r.width; x++ {
			idx := y*r.width + x
			if idx >= 0 && idx < len(r.cells) {
				ch := r.cells[idx].Char
				if ch == "" {
					ch = " "
				}
				row.WriteString(ch)
			}
		}
		b.WriteString(strings.TrimRight(row.String(), " "))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (r snapshotRegion) Cells() []Cell {
	out := make([]Cell, len(r.cells))
	copy(out, r.cells)
	return out
}
