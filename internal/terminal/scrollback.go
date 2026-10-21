package terminal

import "strings"

// DefaultScrollbackLines is how many scrolled-off rows an emulator keeps when
// it is not configured otherwise. It matches the project config default
// (terminal.scrollbackLines).
const DefaultScrollbackLines = 10000

// Options configures an emulator. The zero value is the default configuration.
type Options struct {
	// ScrollbackLines caps the scrollback buffer. Zero means
	// DefaultScrollbackLines; a negative value disables scrollback, so every
	// row that scrolls off the top is counted as dropped.
	ScrollbackLines int
}

// ScrollbackRow is one row that scrolled off the top of the main screen.
type ScrollbackRow struct {
	// Text is the row's text as the screen would show it: wide characters once,
	// trailing blanks trimmed.
	Text string `json:"text"`
	// Cells are the row's cells with their styles and links. Plain trailing
	// blanks are not kept, so a row can be shorter than the terminal was wide;
	// a missing cell reads as a blank. X is the column and Y the row's index in
	// the scrollback, oldest first.
	Cells []Cell `json:"cells"`
}

// ScrollbackInfo summarizes the scrollback buffer without copying it.
type ScrollbackInfo struct {
	// Lines is how many rows the buffer holds now.
	Lines int `json:"lines"`
	// Dropped is how many rows were discarded because the buffer was full (or
	// disabled). Rows removed by CSI 3 J or a full reset are not counted.
	Dropped int `json:"dropped"`
	// Limit is the buffer's capacity in rows; zero means scrollback is off.
	Limit int `json:"limit"`
}

// ScrollbackReader is implemented by emulators that keep scrollback. The
// runner reaches it through a type assertion, like the mode reporters, so an
// Emulator without scrollback stays valid.
type ScrollbackReader interface {
	// ScrollbackInfo reports the buffer's size, drop counter and capacity.
	ScrollbackInfo() ScrollbackInfo
	// ScrollbackText returns the buffered rows' text, oldest first, one row per
	// line, trailing blanks trimmed. Unlike the screen text, trailing blank
	// rows are kept so Lines matches the number of rows in the buffer.
	ScrollbackText() string
	// Scrollback returns a deep copy of the buffered rows with their cells.
	Scrollback() []ScrollbackRow
}

// scrollbackRow is the stored form of a row.
type scrollbackRow struct {
	text  string
	cells []Cell
}

// scrollbackBuffer holds the rows that scrolled off the top of the main screen,
// oldest first. It is not safe for concurrent use; the emulator guards it.
type scrollbackBuffer struct {
	limit   int
	rows    []scrollbackRow
	dropped int

	// text caches the joined row text between mutations.
	text      string
	textValid bool
}

func newScrollbackBuffer(lines int) scrollbackBuffer {
	b := scrollbackBuffer{}
	b.setLimit(lines)
	return b
}

// setLimit applies Options.ScrollbackLines semantics and trims to the new cap.
func (b *scrollbackBuffer) setLimit(lines int) {
	switch {
	case lines == 0:
		lines = DefaultScrollbackLines
	case lines < 0:
		lines = 0
	}
	b.limit = lines
	b.trim()
}

// push appends a row, evicting the oldest rows beyond the limit.
func (b *scrollbackBuffer) push(row scrollbackRow) {
	if b.limit == 0 {
		b.dropped++
		return
	}
	b.rows = append(b.rows, row)
	b.textValid = false
	b.trim()
}

func (b *scrollbackBuffer) trim() {
	over := len(b.rows) - b.limit
	if over <= 0 {
		return
	}
	for i := 0; i < over; i++ {
		b.rows[i] = scrollbackRow{} // release the cells for the collector
	}
	b.rows = b.rows[over:]
	b.dropped += over
	b.textValid = false
	// Re-slicing from the front leaves the evicted prefix in the backing array
	// until append next grows it, which copies only the live rows.
}

// clear empties the buffer. The drop counter keeps its value: it counts rows
// lost to the cap, and a clear is the target's own choice.
func (b *scrollbackBuffer) clear() {
	for i := range b.rows {
		b.rows[i] = scrollbackRow{}
	}
	b.rows = nil
	b.text, b.textValid = "", false
}

func (b *scrollbackBuffer) info() ScrollbackInfo {
	return ScrollbackInfo{Lines: len(b.rows), Dropped: b.dropped, Limit: b.limit}
}

func (b *scrollbackBuffer) joinedText() string {
	if b.textValid {
		return b.text
	}
	var sb strings.Builder
	for i, row := range b.rows {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(row.text)
	}
	b.text, b.textValid = sb.String(), true
	return b.text
}

func (b *scrollbackBuffer) copyRows() []ScrollbackRow {
	out := make([]ScrollbackRow, len(b.rows))
	for i, row := range b.rows {
		cells := make([]Cell, len(row.cells))
		copy(cells, row.cells)
		for x := range cells {
			cells[x].Y = i
		}
		out[i] = ScrollbackRow{Text: row.text, Cells: cells}
	}
	return out
}

// cellsText renders a row of cells as text the way the screen does: a
// continuation cell adds nothing, an empty character reads as a blank, and
// trailing blanks are trimmed.
func cellsText(cells []Cell) string {
	var b strings.Builder
	for _, cell := range cells {
		switch {
		case cell.IsContinuation():
		case cell.Char == "":
			b.WriteByte(' ')
		default:
			b.WriteString(cell.Char)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// isPlainBlank reports whether a cell is an unstyled, unlinked blank, which a
// scrollback row does not need to store.
func isPlainBlank(c Cell) bool {
	return (c.Char == " " || c.Char == "") && c.Width <= 1 && !c.IsContinuation() && c.Style == (Style{}) && c.Link == ""
}

// snapshotScrollbackRow copies a screen row into its stored form.
func snapshotScrollbackRow(cells []Cell) scrollbackRow {
	end := len(cells)
	for end > 0 && isPlainBlank(cells[end-1]) {
		end--
	}
	kept := make([]Cell, end)
	copy(kept, cells[:end])
	return scrollbackRow{text: cellsText(kept), cells: kept}
}

// pushScrolledRows records the top n rows of the main screen as they scroll
// off. It is called before the rows are overwritten.
func (e *SimpleEmulator) pushScrolledRows(n int) {
	for y := 0; y < n; y++ {
		e.scrollback.push(snapshotScrollbackRow(e.cells[y]))
	}
}

// ScrollbackInfo implements ScrollbackReader.
func (e *SimpleEmulator) ScrollbackInfo() ScrollbackInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.scrollback.info()
}

// ScrollbackText implements ScrollbackReader.
func (e *SimpleEmulator) ScrollbackText() string {
	// The joined text is cached, so this takes the write lock.
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.scrollback.joinedText()
}

// Scrollback implements ScrollbackReader.
func (e *SimpleEmulator) Scrollback() []ScrollbackRow {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.scrollback.copyRows()
}

// SetScrollbackLines changes the scrollback capacity with the semantics of
// Options.ScrollbackLines, discarding the oldest rows when it shrinks.
func (e *SimpleEmulator) SetScrollbackLines(lines int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scrollback.setLimit(lines)
}
