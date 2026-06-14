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
		next := ((e.cursor.X / 8) + 1) * 8
		for e.cursor.X < next {
			e.putRune(' ')
		}
	default:
		if r >= 0x20 {
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
		return
	}
	if !strings.HasPrefix(seq, "[") {
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
		e.cursor.Y = clamp(row-1, 0, e.rows-1)
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
	if strings.HasSuffix(body, "m") {
		e.applySGR(strings.TrimSuffix(body, "m"))
		return
	}
}

func (e *SimpleEmulator) applyPrivateMode(body string) {
	enable := strings.HasSuffix(body, "h")
	args := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(body, "?"), "h"), "l")
	for _, arg := range strings.Split(args, ";") {
		switch arg {
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
		case "2004":
			e.bracketedPaste = enable
		}
	}
}

func (e *SimpleEmulator) applySGR(args string) {
	if args == "" {
		args = "0"
	}
	for _, part := range strings.Split(args, ";") {
		code := csiNumber(part, 0)
		switch code {
		case 0:
			e.style = Style{}
		case 1:
			e.style.Bold = true
		case 2:
			e.style.Dim = true
		case 3:
			e.style.Italic = true
		case 4:
			e.style.Underline = true
		case 7:
			e.style.Reverse = true
		case 22:
			e.style.Bold = false
			e.style.Dim = false
		case 23:
			e.style.Italic = false
		case 24:
			e.style.Underline = false
		case 27:
			e.style.Reverse = false
		}
	}
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
	}
	if e.cursor.X >= e.cols-1 {
		// Last column: stay put and defer the wrap (VT100 autowrap quirk).
		e.cursor.X = e.cols - 1
		e.pendingWrap = true
	} else {
		e.cursor.X++
	}
}

func (e *SimpleEmulator) newLine() {
	e.pendingWrap = false
	e.cursor.X = 0
	e.cursor.Y++
	if e.cursor.Y >= e.rows {
		copy(e.cells[0:], e.cells[1:])
		y := e.rows - 1
		e.cells[y] = make([]Cell, e.cols)
		for x := 0; x < e.cols; x++ {
			e.cells[y][x] = Cell{X: x, Y: y, Char: " ", Width: 1}
		}
		for yy := 0; yy < e.rows; yy++ {
			for x := 0; x < e.cols; x++ {
				e.cells[yy][x].X = x
				e.cells[yy][x].Y = yy
			}
		}
		e.cursor.Y = e.rows - 1
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
