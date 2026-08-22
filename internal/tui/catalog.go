package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// Story is one catalog entry for the stories TUI. The CLI maps the stories
// package catalog onto this type so this package stays free of spec/artifact
// knowledge.
type Story struct {
	Name      string
	Feature   string
	Status    string
	RunID     string
	Snapshots []StorySnap
}

// StorySnap is a named screen belonging to a story.
type StorySnap struct {
	Name   string
	Status string
	Error  string
	Screen *terminal.ScreenSnapshot
}

type catalogModel struct {
	items      []Story
	idx        int
	snap       int
	showSpaces bool
	width      int
	height     int
}

const catalogSidebarWidth = 28

// RunStories launches the catalog browser. It paints snapshots with the same
// cell renderer as replay --tui, so the preview matches the host terminal.
func RunStories(items []Story) error {
	if len(items) == 0 {
		return fmt.Errorf("no stories to browse")
	}
	_, err := tea.NewProgram(newCatalog(items)).Run()
	return err
}

func newCatalog(items []Story) catalogModel {
	return catalogModel{items: items}
}

func (m catalogModel) Init() tea.Cmd { return nil }

func (m catalogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			m.idx = clampIndex(m.idx+1, len(m.items))
			m.snap = 0
		case "k", "up":
			m.idx = clampIndex(m.idx-1, len(m.items))
			m.snap = 0
		case "g", "home":
			m.idx = 0
			m.snap = 0
		case "G", "end":
			m.idx = clampIndex(len(m.items)-1, len(m.items))
			m.snap = 0
		case "l", "right", "]":
			m.snap = clampIndex(m.snap+1, len(m.current().Snapshots))
		case "h", "left", "[":
			m.snap = clampIndex(m.snap-1, len(m.current().Snapshots))
		case "s":
			m.showSpaces = !m.showSpaces
		}
	}
	return m, nil
}

func (m catalogModel) current() Story {
	if len(m.items) == 0 {
		return Story{}
	}
	return m.items[clampIndex(m.idx, len(m.items))]
}

func (m catalogModel) currentSnap() (StorySnap, bool) {
	st := m.current()
	if len(st.Snapshots) == 0 {
		return StorySnap{}, false
	}
	return st.Snapshots[clampIndex(m.snap, len(st.Snapshots))], true
}

func (m catalogModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m catalogModel) layout() (sideW, mainW, h int) {
	h = m.height
	if m.width <= 0 {
		return catalogSidebarWidth, 0, h
	}
	const gap = 3 // " │ "
	sideW = catalogSidebarWidth
	if m.width < sideW+gap+24 {
		sideW = max(16, m.width/3)
	}
	mainW = m.width - sideW - gap
	if mainW < 1 {
		mainW = 1
	}
	return sideW, mainW, h
}

func (m catalogModel) render() string {
	sideW, mainW, h := m.layout()
	leftStyle := lipgloss.NewStyle().Width(sideW).MaxWidth(sideW)
	if h > 0 {
		leftStyle = leftStyle.Height(h).MaxHeight(h)
	}
	left := leftStyle.Render(m.renderSidebar())
	right := m.renderMain(mainW, h)
	if h <= 0 {
		h = max(lipgloss.Height(left), lipgloss.Height(right))
	}
	bar := strings.TrimSuffix(strings.Repeat(dimStyle.Render("│")+"\n", max(h, 1)), "\n")
	out := lipgloss.JoinHorizontal(lipgloss.Top, left, " "+bar+" ", right)
	if m.width > 0 && h > 0 {
		out = lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).Height(h).MaxHeight(h).Render(out)
	}
	return out
}

func (m catalogModel) renderSidebar() string {
	var b strings.Builder
	b.WriteString(dimStyle.Bold(true).Render("GLYPH STORIES"))
	prev := "\x00"
	for i, s := range m.items {
		feat := s.Feature
		if feat == "" {
			feat = "ungrouped"
		}
		if feat != prev {
			b.WriteString("\n\n")
			b.WriteString(dimStyle.Render(strings.ToUpper(feat)))
			b.WriteByte('\n')
			prev = feat
		}
		mark := "  "
		name := dimStyle.Render(s.Name)
		if i == m.idx {
			mark = "▸ "
			name = headerStyle.Render(s.Name)
		}
		b.WriteString(mark)
		b.WriteString(name)
		b.WriteByte('\n')
		meta := s.Status
		if n := len(s.Snapshots); n > 0 {
			meta += fmt.Sprintf(" · %d snap", n)
		}
		b.WriteString(dimStyle.Render("  " + meta))
		b.WriteByte('\n')
	}
	return b.String()
}

func (m catalogModel) renderMain(mainW, h int) string {
	st := m.current()
	toolbar := m.renderToolbar()
	keys := dimStyle.Render("j/k stories · [/] snapshots · s spaces · q quit")
	if mainW > 0 {
		fit := lipgloss.NewStyle().Width(mainW).MaxWidth(mainW)
		toolbar = fit.Render(toolbar)
		keys = dimStyle.Width(mainW).MaxWidth(mainW).Render("j/k stories · [/] snapshots · s spaces · q quit")
	}

	var chrome, body string
	snap, ok := m.currentSnap()
	switch {
	case !ok:
		if st.Status == "not_run" {
			body = dimStyle.Render("not run — glyph run " + st.Name)
		} else {
			body = dimStyle.Render("no snapshot")
		}
	case snap.Status != "ok" || snap.Screen == nil:
		msg := snap.Error
		if msg == "" {
			msg = "unreadable snapshot"
		}
		chrome = m.renderChrome(snap, mainW)
		body = dimStyle.Render(msg)
	default:
		chrome = m.renderChrome(snap, mainW)
		body = paintScreen(snap.Screen, m.showSpaces, true)
	}
	if mainW > 0 {
		body = lipgloss.NewStyle().Width(mainW).MaxWidth(mainW).Render(body)
	}
	if h > 0 {
		chromeH := 0
		if chrome != "" {
			chromeH = lipgloss.Height(chrome)
		}
		bodyH := h - 1 - chromeH - 1 // toolbar + keys
		if bodyH < 1 {
			bodyH = 1
		}
		body = lipgloss.NewStyle().Width(max(mainW, 1)).Height(bodyH).MaxHeight(bodyH).Align(lipgloss.Left, lipgloss.Top).Render(body)
		parts := []string{toolbar}
		if chrome != "" {
			parts = append(parts, chrome)
		}
		parts = append(parts, body, keys)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}
	var b strings.Builder
	b.WriteString(toolbar)
	b.WriteByte('\n')
	if chrome != "" {
		b.WriteString(chrome)
		b.WriteByte('\n')
	}
	b.WriteString(body)
	b.WriteByte('\n')
	b.WriteString(keys)
	return b.String()
}

func (m catalogModel) renderToolbar() string {
	plain, spaces := "plain", "spaces"
	if m.showSpaces {
		spaces = headerStyle.Render(spaces)
		plain = dimStyle.Render(plain)
	} else {
		plain = headerStyle.Render(plain)
		spaces = dimStyle.Render(spaces)
	}
	st := m.current()
	pills := make([]string, 0, len(st.Snapshots))
	for i, sn := range st.Snapshots {
		label := sn.Name
		if i == m.snap {
			label = headerStyle.Render(label)
		} else {
			label = dimStyle.Render(label)
		}
		pills = append(pills, label)
	}
	line := plain + "  " + spaces
	if len(pills) > 0 {
		line += "    " + strings.Join(pills, "  ")
	}
	return line
}

func (m catalogModel) renderChrome(snap StorySnap, width int) string {
	st := m.current()
	title := st.Name
	if st.RunID != "" {
		title += " · " + st.RunID
	}
	size := ""
	if snap.Screen != nil {
		size = fmt.Sprintf("%d×%d", snap.Screen.Cols, snap.Screen.Rows)
	}
	left := trafficDots() + "  " + dimStyle.Render(title)
	right := dimStyle.Render(size)
	top := left
	if size != "" && width <= 0 {
		top = left + "  " + right
	}
	if width > 0 {
		top = splitBar(left, right, width)
	}
	ruleW := lipgloss.Width(top)
	if width > 0 {
		ruleW = width
	} else if ruleW < 40 {
		ruleW = 40
	}
	return top + "\n" + dimStyle.Render(strings.Repeat("─", ruleW))
}

func splitBar(left, right string, width int) string {
	if width < 1 {
		return left
	}
	rw := lipgloss.Width(right)
	if rw >= width {
		return lipgloss.NewStyle().MaxWidth(width).Render(right)
	}
	left = lipgloss.NewStyle().MaxWidth(width - rw).Render(left)
	gap := width - lipgloss.Width(left) - rw
	if gap < 0 {
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + right
}

func trafficDots() string {
	r := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f56")).Render("●")
	y := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffbd2e")).Render("●")
	g := lipgloss.NewStyle().Foreground(lipgloss.Color("#27c93f")).Render("●")
	return r + " " + y + " " + g
}

func paintScreen(snap *terminal.ScreenSnapshot, showSpaces, skipBlank bool) string {
	if snap == nil {
		return dimStyle.Render("(no screen captured for this frame)")
	}
	cols, rows := snap.Cols, snap.Rows
	var b strings.Builder
	wrote := false
	for y := 0; y < rows; y++ {
		if skipBlank && !showSpaces && rowEmpty(snap, y, cols) {
			continue
		}
		if wrote {
			b.WriteByte('\n')
		}
		wrote = true
		x := 0
		for x < cols {
			st := cellAt(snap, x, y, cols).Style
			var run strings.Builder
			for x < cols {
				c := cellAt(snap, x, y, cols)
				if c.Style != st {
					break
				}
				ch := charOf(c)
				if showSpaces && ch == " " {
					ch = "·"
				}
				run.WriteString(ch)
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

func rowEmpty(snap *terminal.ScreenSnapshot, y, cols int) bool {
	for x := 0; x < cols; x++ {
		if charOf(cellAt(snap, x, y, cols)) != " " {
			return false
		}
	}
	return true
}
