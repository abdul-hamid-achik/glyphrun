package terminal

// CellDiff is one cell whose character or style differs between two
// snapshots. Before is the cell from the first snapshot (the golden or the
// previous run), After is the cell from the second (the current screen).
type CellDiff struct {
	X      int  `json:"x"`
	Y      int  `json:"y"`
	Before Cell `json:"before"`
	After  Cell `json:"after"`
}

// SnapshotDiff is the cell-level comparison of two screen snapshots. It is
// the visual-regression primitive behind stories: a golden screen against the
// screen a run just captured.
type SnapshotDiff struct {
	Cols        int        `json:"cols"`
	Rows        int        `json:"rows"`
	SizeChanged bool       `json:"sizeChanged"`
	Changed     []CellDiff `json:"changed"`
}

// Equal reports whether the two snapshots rendered identically.
func (d SnapshotDiff) Equal() bool {
	return !d.SizeChanged && len(d.Changed) == 0
}

// DiffSnapshots compares two snapshots cell by cell over the union of their
// grids. A missing cell reads as a blank default-style cell, so a size change
// still yields a per-cell list that a renderer can highlight. Cursor position
// is not part of the comparison: goldens assert the picture, not the caret.
func DiffSnapshots(before, after ScreenSnapshot) SnapshotDiff {
	cols := max(before.Cols, after.Cols)
	rows := max(before.Rows, after.Rows)
	out := SnapshotDiff{
		Cols:        cols,
		Rows:        rows,
		SizeChanged: before.Cols != after.Cols || before.Rows != after.Rows,
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			a := cellAt(before, x, y)
			b := cellAt(after, x, y)
			if !cellsEqual(a, b) {
				out.Changed = append(out.Changed, CellDiff{X: x, Y: y, Before: a, After: b})
			}
		}
	}
	return out
}

func cellAt(snap ScreenSnapshot, x, y int) Cell {
	blank := Cell{X: x, Y: y, Char: " ", Width: 1}
	if x < 0 || y < 0 || x >= snap.Cols || y >= snap.Rows {
		return blank
	}
	idx := y*snap.Cols + x
	if idx < 0 || idx >= len(snap.Cells) {
		return blank
	}
	c := snap.Cells[idx]
	if c.Char == "" {
		c.Char = " "
	}
	return c
}

func cellsEqual(a, b Cell) bool {
	return a.Char == b.Char && a.Style == b.Style && a.Link == b.Link
}
