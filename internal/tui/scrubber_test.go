package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

func TestClampIndex(t *testing.T) {
	tests := []struct {
		i, n, want int
	}{
		{0, 0, 0},
		{5, 0, 0},
		{-1, 3, 0},
		{1, 3, 1},
		{3, 3, 2},
		{99, 3, 2},
	}
	for _, tc := range tests {
		if got := clampIndex(tc.i, tc.n); got != tc.want {
			t.Errorf("clampIndex(%d,%d) = %d, want %d", tc.i, tc.n, got, tc.want)
		}
	}
}

func TestRenderScreenNil(t *testing.T) {
	out := renderScreen(nil)
	if !strings.Contains(out, "no screen captured") {
		t.Errorf("expected placeholder for nil screen, got %q", out)
	}
}

func TestRenderScreenText(t *testing.T) {
	snap := &terminal.ScreenSnapshot{Cols: 3, Rows: 2}
	for y, row := range []string{"abc", "de "} {
		for x, ch := range row {
			snap.Cells = append(snap.Cells, terminal.Cell{X: x, Y: y, Char: string(ch), Width: 1})
		}
	}
	// Give one cell a style so the styled-run branch is exercised.
	snap.Cells[0].Style = terminal.Style{Fg: "red", Bold: true}

	out := renderScreen(snap)
	// The plain characters must survive regardless of the active color profile.
	if !strings.Contains(out, "a") || !strings.Contains(out, "bc") || !strings.Contains(out, "de") {
		t.Errorf("rendered screen missing characters:\n%q", out)
	}
	if !strings.Contains(out, "\n") {
		t.Errorf("expected a row separator in rendered screen:\n%q", out)
	}
}

func TestModelNavigation(t *testing.T) {
	frames := make([]terminal.Frame, 3)
	for i := range frames {
		frames[i] = terminal.Frame{Seq: int64(i)}
	}
	m := New(frames, Meta{Spec: "demo"})

	key := func(code rune, text string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code, Text: text} }

	// right advances, clamped at the end.
	step := func(m model, k tea.KeyPressMsg) model {
		next, _ := m.Update(k)
		return next.(model)
	}
	m = step(m, key('l', "l"))
	if m.idx != 1 {
		t.Fatalf("after right: idx = %d, want 1", m.idx)
	}
	m = step(m, key('l', "l"))
	m = step(m, key('l', "l"))
	if m.idx != 2 {
		t.Fatalf("right should clamp at last frame: idx = %d, want 2", m.idx)
	}
	// left goes back.
	m = step(m, key('h', "h"))
	if m.idx != 1 {
		t.Fatalf("after left: idx = %d, want 1", m.idx)
	}

	// q returns a quit command.
	if _, cmd := m.Update(key('q', "q")); cmd == nil {
		t.Errorf("expected a quit command on q")
	}

	// space toggles playback and schedules a tick.
	played, cmd := m.Update(key(' ', " "))
	if !played.(model).playing {
		t.Errorf("space should start playback")
	}
	if cmd == nil {
		t.Errorf("starting playback should schedule a tick")
	}
}

func TestLipColor(t *testing.T) {
	if _, ok := lipColor(""); ok {
		t.Errorf("empty color should report not-ok")
	}
	for _, v := range []string{"red", "brightblue", "201", "#ff8800"} {
		if _, ok := lipColor(v); !ok {
			t.Errorf("expected %q to resolve to a color", v)
		}
	}
}
