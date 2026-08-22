package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

func TestRunStoriesRejectsEmpty(t *testing.T) {
	if err := RunStories(nil); err == nil {
		t.Fatal("expected error for empty catalog")
	}
}

func TestCatalogAltScreen(t *testing.T) {
	m := newCatalog([]Story{{Name: "x"}})
	if !m.View().AltScreen {
		t.Fatal("stories --tui must use the alt screen")
	}
}

func TestCatalogNavigationAndSpaces(t *testing.T) {
	snap := &terminal.ScreenSnapshot{Cols: 3, Rows: 1, Cells: []terminal.Cell{
		{X: 0, Y: 0, Char: "a", Width: 1},
		{X: 1, Y: 0, Char: " ", Width: 1},
		{X: 2, Y: 0, Char: "b", Width: 1},
	}}
	items := []Story{
		{Name: "first", Feature: "list", Status: "run", Snapshots: []StorySnap{
			{Name: "home", Status: "ok", Screen: snap},
			{Name: "final", Status: "ok", Screen: snap},
		}},
		{Name: "second", Status: "not_run"},
	}
	m := newCatalog(items)
	key := func(text string) tea.KeyPressMsg {
		var code rune
		if text != "" {
			code = []rune(text)[0]
		}
		return tea.KeyPressMsg{Code: code, Text: text}
	}
	step := func(m catalogModel, k tea.KeyPressMsg) catalogModel {
		next, _ := m.Update(k)
		return next.(catalogModel)
	}

	out := m.render()
	if !strings.Contains(out, "first") || !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Fatalf("render missing story or screen:\n%s", out)
	}

	m = step(m, key("]"))
	if m.snap != 1 {
		t.Fatalf("] should advance snapshot, snap=%d", m.snap)
	}
	m = step(m, key("["))
	if m.snap != 0 {
		t.Fatalf("[ should go back, snap=%d", m.snap)
	}
	m = step(m, key("j"))
	if m.idx != 1 || m.snap != 0 {
		t.Fatalf("j should move to next story and reset snap: idx=%d snap=%d", m.idx, m.snap)
	}
	if !strings.Contains(m.render(), "not run") {
		t.Fatalf("second story should show not-run hint:\n%s", m.render())
	}

	m = step(m, key("k"))
	if m.idx != 0 {
		t.Fatalf("k should return to first story, idx=%d", m.idx)
	}
	m = step(m, key("s"))
	if !m.showSpaces {
		t.Fatal("s should toggle spaces")
	}
	painted := paintScreen(snap, true, true)
	if !strings.Contains(painted, "·") {
		t.Fatalf("spaces overlay should use middle dots:\n%q", painted)
	}
}

func TestPaintScreenSkipBlankAndSpaces(t *testing.T) {
	tall := screenGrid(2, 3, map[[2]int]string{
		{0, 0}: "a", {1, 0}: "b",
		{0, 2}: "c", {1, 2}: "d",
	})
	compact := paintScreen(tall, false, true)
	if compact != "ab\ncd" {
		t.Fatalf("skipBlank should drop the all-space middle row, got %q", compact)
	}
	if strings.Count(compact, "\n") != 1 || strings.Contains(compact, "\n  \n") {
		t.Fatalf("skipBlank left an interior blank row:\n%q", compact)
	}
	withSpaces := paintScreen(tall, true, true)
	if !strings.Contains(withSpaces, "·") {
		t.Fatalf("spaces overlay should keep blank rows as dots:\n%q", withSpaces)
	}
	if strings.Count(withSpaces, "\n") != 2 {
		t.Fatalf("spaces overlay should keep 3 rows, got %q", withSpaces)
	}
}

func TestPaintScreenReplayKeepsBlankRows(t *testing.T) {
	tall := screenGrid(2, 3, map[[2]int]string{
		{0, 0}: "a", {1, 0}: "b",
		{0, 2}: "c", {1, 2}: "d",
	})
	// Same painter path as replay --tui (spaces off, skip-blank off).
	full := renderScreen(tall)
	if full != "ab\n  \ncd" {
		t.Fatalf("replay must keep the all-space middle row, got %q", full)
	}
}

func TestCatalogRenderTwoPane(t *testing.T) {
	title := "Inbox"
	help := "q quit"
	snap := screenGrid(80, 24, map[[2]int]string{})
	for i, r := range []rune(title) {
		snap.Cells[i].Char = string(r)
	}
	last := 23 * 80
	for i, r := range []rune(help) {
		snap.Cells[last+i].Char = string(r)
	}

	items := make([]Story, 8)
	for i := 0; i < 8; i++ {
		feat := "agent"
		if i >= 4 {
			feat = "list"
		}
		name := "alpha" + string(rune('0'+i))
		items[i] = Story{
			Name: name, Feature: feat, Status: "passed",
			Snapshots: []StorySnap{{Name: "final", Status: "ok", Screen: snap}},
		}
	}
	out := newCatalog(items).render()
	first := strings.Split(out, "\n")[0]
	if !strings.Contains(first, "GLYPH STORIES") || !strings.Contains(first, "plain") {
		t.Fatalf("first line should be sidebar+main, got %q", first)
	}
	if !strings.Contains(out, "AGENT") || !strings.Contains(out, "LIST") {
		t.Fatalf("sidebar missing feature groups:\n%s", out)
	}
	for i := 0; i < 8; i++ {
		if !strings.Contains(out, items[i].Name) {
			t.Fatalf("sidebar missing %s:\n%s", items[i].Name, out)
		}
	}
	i := strings.Index(out, title)
	if i < 0 {
		t.Fatalf("preview missing title %q:\n%s", title, out)
	}
	rest := out[i+len(title):]
	j := strings.Index(rest, help)
	if j < 0 {
		t.Fatalf("preview missing help %q after title:\n%s", help, out)
	}
	between := rest[:j]
	if n := strings.Count(between, "\n"); n != 1 {
		t.Fatalf("Inbox and %q should be adjacent painted lines, got %d newlines between them:\n%q", help, n, between)
	}
}

func TestCatalogWindowSize(t *testing.T) {
	m := newCatalog([]Story{{Name: "x"}})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got := next.(catalogModel)
	if got.width != 120 || got.height != 40 {
		t.Fatalf("size = %d×%d", got.width, got.height)
	}
}

func TestCatalogFillsWindow(t *testing.T) {
	snap := screenGrid(80, 24, map[[2]int]string{{0, 0}: "I"})
	m := newCatalog([]Story{{
		Name: "alpha0", Feature: "list", Status: "passed",
		Snapshots: []StorySnap{{Name: "final", Status: "ok", Screen: snap}},
	}})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	out := next.(catalogModel).render()
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("render height %d, want 24 (window)\n%s", len(lines), out)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > 100 {
			t.Fatalf("line %d width %d > 100: %q", i, w, line)
		}
	}
	if !strings.Contains(lines[0], "GLYPH STORIES") {
		t.Fatalf("sidebar should stay on the left at full height: %q", lines[0])
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "j/k stories") {
		t.Fatalf("keys should sit on the last row: %q", last)
	}

	narrow, _ := next.(catalogModel).Update(tea.WindowSizeMsg{Width: 50, Height: 16})
	small := narrow.(catalogModel).render()
	slines := strings.Split(small, "\n")
	if len(slines) != 16 {
		t.Fatalf("narrow height %d, want 16", len(slines))
	}
	for i, line := range slines {
		if w := lipgloss.Width(line); w > 50 {
			t.Fatalf("narrow line %d width %d > 50", i, w)
		}
	}
}

func screenGrid(cols, rows int, fills map[[2]int]string) *terminal.ScreenSnapshot {
	cells := make([]terminal.Cell, cols*rows)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			ch := " "
			if s, ok := fills[[2]int{x, y}]; ok {
				ch = s
			}
			cells[y*cols+x] = terminal.Cell{X: x, Y: y, Char: ch, Width: 1}
		}
	}
	return &terminal.ScreenSnapshot{Cols: cols, Rows: rows, Cells: cells}
}
