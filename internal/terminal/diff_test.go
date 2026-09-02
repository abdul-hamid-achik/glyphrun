package terminal

import "testing"

func grid(cols, rows int, text string) ScreenSnapshot {
	snap := ScreenSnapshot{Cols: cols, Rows: rows}
	runes := []rune(text)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			ch := " "
			if i := y*cols + x; i < len(runes) {
				ch = string(runes[i])
			}
			snap.Cells = append(snap.Cells, Cell{X: x, Y: y, Char: ch, Width: 1})
		}
	}
	return snap
}

func TestDiffSnapshotsEqual(t *testing.T) {
	a := grid(3, 2, "abcdef")
	d := DiffSnapshots(a, a)
	if !d.Equal() || len(d.Changed) != 0 || d.SizeChanged {
		t.Fatalf("identical snapshots should be equal: %+v", d)
	}
}

func TestDiffSnapshotsFindsCharAndStyleChanges(t *testing.T) {
	a := grid(3, 2, "abcdef")
	b := grid(3, 2, "abXdef")
	b.Cells[4].Style.Bold = true
	d := DiffSnapshots(a, b)
	if d.Equal() {
		t.Fatal("expected changes")
	}
	if len(d.Changed) != 2 {
		t.Fatalf("changed = %+v, want 2 entries", d.Changed)
	}
	if d.Changed[0].X != 2 || d.Changed[0].Y != 0 || d.Changed[0].Before.Char != "c" || d.Changed[0].After.Char != "X" {
		t.Fatalf("first change = %+v", d.Changed[0])
	}
	if d.Changed[1].X != 1 || d.Changed[1].Y != 1 || !d.Changed[1].After.Style.Bold {
		t.Fatalf("second change = %+v", d.Changed[1])
	}
}

func TestDiffSnapshotsSizeChange(t *testing.T) {
	a := grid(2, 1, "ab")
	b := grid(3, 1, "abc")
	d := DiffSnapshots(a, b)
	if !d.SizeChanged || d.Cols != 3 || d.Rows != 1 {
		t.Fatalf("size diff = %+v", d)
	}
	if len(d.Changed) != 1 || d.Changed[0].X != 2 || d.Changed[0].Before.Char != " " {
		t.Fatalf("changed = %+v", d.Changed)
	}
}

func TestDiffSnapshotsTreatsEmptyCharAsBlank(t *testing.T) {
	a := grid(1, 1, " ")
	b := grid(1, 1, "x")
	b.Cells[0].Char = ""
	if d := DiffSnapshots(a, b); !d.Equal() {
		t.Fatalf("empty char should equal blank: %+v", d)
	}
}
