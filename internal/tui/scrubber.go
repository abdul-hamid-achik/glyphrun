// Package tui provides an interactive frame scrubber for replaying a recorded
// PTY session frame by frame.
//
// This is the one place glyphrun takes a TUI dependency (Bubble Tea v2). It is
// isolated here and reached only through `glyph replay --tui`, so the rest of
// the binary stays dependency-light. The scrubber reads the deterministic
// frame snapshots the runner already captures (frames/frames.ndjson) and lets a
// human time-travel through them — step, jump, and play back — to see exactly
// when the screen changed.
package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// Meta labels the session being scrubbed.
type Meta struct {
	RunID string
	Spec  string
}

const playInterval = 120 * time.Millisecond

type tickMsg time.Time

type model struct {
	frames  []terminal.Frame
	meta    Meta
	idx     int
	playing bool
}

// New builds the scrubber model. Exposed for tests; production code uses Run.
func New(frames []terminal.Frame, meta Meta) model {
	return model{frames: frames, meta: meta}
}

// Run launches the interactive scrubber, blocking until the user quits.
func Run(frames []terminal.Frame, meta Meta) error {
	if len(frames) == 0 {
		return fmt.Errorf("no frames to replay")
	}
	_, err := tea.NewProgram(New(frames, meta)).Run()
	return err
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "right", "l":
			m.idx = clampIndex(m.idx+1, len(m.frames))
		case "left", "h":
			m.idx = clampIndex(m.idx-1, len(m.frames))
		case "pgdown", "ctrl+d":
			m.idx = clampIndex(m.idx+10, len(m.frames))
		case "pgup", "ctrl+u":
			m.idx = clampIndex(m.idx-10, len(m.frames))
		case "home", "g":
			m.idx = 0
		case "end", "G":
			m.idx = clampIndex(len(m.frames)-1, len(m.frames))
		case " ", "space":
			m.playing = !m.playing
			if m.playing {
				// Restart from the beginning if we're parked at the end.
				if m.idx >= len(m.frames)-1 {
					m.idx = 0
				}
				return m, tick()
			}
		default:
			if msg.Code == ' ' { // some terminals report space without a name
				m.playing = !m.playing
				if m.playing {
					return m, tick()
				}
			}
		}
		return m, nil
	case tickMsg:
		if !m.playing {
			return m, nil
		}
		if m.idx >= len(m.frames)-1 {
			m.playing = false
			return m, nil
		}
		m.idx++
		return m, tick()
	}
	return m, nil
}

func (m model) View() tea.View {
	return tea.NewView(m.render())
}

func tick() tea.Cmd {
	return tea.Tick(playInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// clampIndex keeps a frame index within [0, n-1] (or 0 when there are none).
func clampIndex(i, n int) int {
	if n <= 0 {
		return 0
	}
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
)

func (m model) render() string {
	frame := m.frames[clampIndex(m.idx, len(m.frames))]

	status := "paused"
	if m.playing {
		status = "playing"
	}
	header := headerStyle.Render(fmt.Sprintf("glyph replay — %s", m.meta.Spec))
	pos := fmt.Sprintf("frame %d/%d · seq %d · %s · %s",
		m.idx+1, len(m.frames), frame.Seq, frame.Kind, status)

	var b strings.Builder
	b.WriteString(header)
	b.WriteByte('\n')
	b.WriteString(dimStyle.Render(pos))
	if frame.Time != "" {
		b.WriteString(dimStyle.Render("  " + frame.Time))
	}
	b.WriteString("\n\n")
	b.WriteString(renderScreen(frame.Screen))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("←/→ step · pgup/pgdn ±10 · home/end · space play/pause · q quit"))
	return b.String()
}

// renderScreen paints a frame's screen snapshot with per-cell color and
// attributes, grouping runs of identical style for compactness.
func renderScreen(snap *terminal.ScreenSnapshot) string {
	if snap == nil {
		return dimStyle.Render("(no screen captured for this frame)")
	}
	cols, rows := snap.Cols, snap.Rows
	var b strings.Builder
	for y := 0; y < rows; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		x := 0
		for x < cols {
			st := cellAt(snap, x, y, cols).Style
			var run strings.Builder
			for x < cols {
				c := cellAt(snap, x, y, cols)
				if c.Style != st {
					break
				}
				run.WriteString(charOf(c))
				x++
			}
			if st == (terminal.Style{}) {
				b.WriteString(run.String())
			} else {
				b.WriteString(cellStyle(st).Render(run.String()))
			}
		}
	}
	return b.String()
}

func cellAt(snap *terminal.ScreenSnapshot, x, y, cols int) terminal.Cell {
	idx := y*cols + x
	if idx < 0 || idx >= len(snap.Cells) {
		return terminal.Cell{X: x, Y: y, Char: " ", Width: 1}
	}
	return snap.Cells[idx]
}

func charOf(c terminal.Cell) string {
	if c.Char == "" {
		return " "
	}
	return c.Char
}

func cellStyle(st terminal.Style) lipgloss.Style {
	s := lipgloss.NewStyle()
	if c, ok := lipColor(st.Fg); ok {
		s = s.Foreground(c)
	}
	if c, ok := lipColor(st.Bg); ok {
		s = s.Background(c)
	}
	if st.Bold {
		s = s.Bold(true)
	}
	if st.Dim {
		s = s.Faint(true)
	}
	if st.Italic {
		s = s.Italic(true)
	}
	if st.Underline {
		s = s.Underline(true)
	}
	if st.Reverse {
		s = s.Reverse(true)
	}
	return s
}

// ansiIndexByName maps glyphrun's canonical 16-color names to ANSI indices that
// lipgloss.Color understands. 256-palette indices and "#hex" truecolor pass
// through unchanged.
var ansiIndexByName = map[string]string{
	"black": "0", "red": "1", "green": "2", "yellow": "3",
	"blue": "4", "magenta": "5", "cyan": "6", "white": "7",
	"brightblack": "8", "brightred": "9", "brightgreen": "10", "brightyellow": "11",
	"brightblue": "12", "brightmagenta": "13", "brightcyan": "14", "brightwhite": "15",
}

func lipColor(v string) (color.Color, bool) {
	if v == "" {
		return nil, false
	}
	if idx, ok := ansiIndexByName[v]; ok {
		return lipgloss.Color(idx), true
	}
	return lipgloss.Color(v), true // "#rrggbb" or a 0-255 palette index
}
