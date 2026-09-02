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
	Name       string // display label: story id (+ @variant) or spec name
	SpecName   string
	Feature    string
	Status     string
	RunID      string
	Golden     string // match | changed | missing | none
	Diagnostic string
	Snapshots  []StorySnap
}

// StorySnap is a named screen belonging to a story. Before is the committed
// golden when it differs; Changed lists the differing cells.
type StorySnap struct {
	Name          string
	Status        string
	Error         string
	Golden        string
	CursorChanged bool
	Screen        *terminal.ScreenSnapshot
	Before        *terminal.ScreenSnapshot
	Changed       []terminal.CellDiff
}

type catalogModel struct {
	items      []Story
	idx        int
	snap       int
	showSpaces bool
	showDiff   bool
	showGolden bool
	showRulers bool
	width      int
	height     int
}

const catalogSidebarWidth = 30

var (
	diffStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#7f1d1d")).Foreground(lipgloss.Color("#ffffff"))
	changedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#fda4af"))
	missingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#fcd34d"))
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#6ee7b7"))
)

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
	m := catalogModel{items: items, showDiff: true}
	return m
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
			m.showGolden = false
		case "k", "up":
			m.idx = clampIndex(m.idx-1, len(m.items))
			m.snap = 0
			m.showGolden = false
		case "g", "home":
			m.idx = 0
			m.snap = 0
			m.showGolden = false
		case "G", "end":
			m.idx = clampIndex(len(m.items)-1, len(m.items))
			m.snap = 0
			m.showGolden = false
		case "l", "right", "]":
			m.snap = clampIndex(m.snap+1, len(m.current().Snapshots))
			m.showGolden = false
		case "h", "left", "[":
			m.snap = clampIndex(m.snap-1, len(m.current().Snapshots))
			m.showGolden = false
		case "s":
			m.showSpaces = !m.showSpaces
		case "d":
			m.showDiff = !m.showDiff
		case "o":
			if snap, ok := m.currentSnap(); ok && snap.Before != nil {
				m.showGolden = !m.showGolden
			}
		case "r":
			m.showRulers = !m.showRulers
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

func goldenMark(golden string) string {
	switch golden {
	case "changed":
		return changedStyle.Render("±")
	case "missing":
		return missingStyle.Render("?")
	case "match":
		return okStyle.Render("✓")
	}
	return " "
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
		b.WriteString(goldenMark(s.Golden))
		b.WriteByte(' ')
		b.WriteString(name)
		b.WriteByte('\n')
		meta := s.Status
		if n := len(s.Snapshots); n > 0 {
			meta += fmt.Sprintf(" · %d snap", n)
		}
		b.WriteString(dimStyle.Render("    " + meta))
		b.WriteByte('\n')
	}
	return b.String()
}

func (m catalogModel) keysLine() string {
	return "j/k stories · [/] snapshots · s spaces · d diff · o golden · r rulers · q quit"
}

func (m catalogModel) renderMain(mainW, h int) string {
	st := m.current()
	toolbar := m.renderToolbar()
	keys := dimStyle.Render(m.keysLine())
	if mainW > 0 {
		fit := lipgloss.NewStyle().Width(mainW).MaxWidth(mainW)
		toolbar = fit.Render(toolbar)
		keys = dimStyle.Width(mainW).MaxWidth(mainW).Render(m.keysLine())
	}

	var chrome, body string
	snap, ok := m.currentSnap()
	switch {
	case !ok:
		if st.Status == "not_run" {
			body = dimStyle.Render("not run — glyph stories run --only " + st.Name)
		} else {
			msg := "no snapshot"
			if st.Diagnostic != "" {
				msg += " — " + st.Diagnostic
			}
			body = dimStyle.Render(msg)
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
		screen := snap.Screen
		if m.showGolden && snap.Before != nil {
			screen = snap.Before
		}
		var changed map[[2]int]bool
		if m.showDiff && len(snap.Changed) > 0 {
			changed = make(map[[2]int]bool, len(snap.Changed))
			for _, c := range snap.Changed {
				changed[[2]int{c.X, c.Y}] = true
			}
		}
		body = paintScreenOpts(screen, paintOptions{spaces: m.showSpaces, skipBlank: true, rulers: m.showRulers, changed: changed})
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
	toggle := func(label string, on bool) string {
		if on {
			return headerStyle.Render(label)
		}
		return dimStyle.Render(label)
	}
	st := m.current()
	pills := make([]string, 0, len(st.Snapshots))
	for i, sn := range st.Snapshots {
		label := sn.Name
		if sn.Golden == "changed" {
			label += " ±" + snapDelta(sn)
		}
		if i == m.snap {
			label = headerStyle.Render(label)
		} else {
			label = dimStyle.Render(label)
		}
		pills = append(pills, label)
	}
	line := toggle("plain", !m.showSpaces) + "  " + toggle("spaces", m.showSpaces) + "  " + toggle("diff", m.showDiff) + "  " + toggle("rulers", m.showRulers)
	if snap, ok := m.currentSnap(); ok && snap.Before != nil {
		line += "  " + toggle("golden", m.showGolden)
	}
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
	if m.showGolden && snap.Before != nil {
		title += " · golden"
	}
	size := ""
	if snap.Screen != nil {
		size = fmt.Sprintf("%d×%d", snap.Screen.Cols, snap.Screen.Rows)
	}
	if snap.Golden == "changed" {
		size = changedStyle.Render(snapDelta(snap)+" changed") + "  " + size
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

func snapDelta(snap StorySnap) string {
	if snap.CursorChanged {
		if len(snap.Changed) > 0 {
			return fmt.Sprintf("%d+cursor", len(snap.Changed))
		}
		return "cursor"
	}
	return fmt.Sprintf("%d", len(snap.Changed))
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

// paintOptions control how a screen is painted into the host terminal.
type paintOptions struct {
	spaces    bool
	skipBlank bool
	rulers    bool
	changed   map[[2]int]bool
}

func paintScreen(snap *terminal.ScreenSnapshot, showSpaces, skipBlank bool) string {
	return paintScreenOpts(snap, paintOptions{spaces: showSpaces, skipBlank: skipBlank})
}

// paintScreenOpts renders the cell grid with lipgloss styles. Changed cells
// (a golden diff) are painted with the diff style so a regression stands out
// in the terminal the same way it does in the HTML overlay; rulers add a
// column header and row numbers.
func paintScreenOpts(snap *terminal.ScreenSnapshot, opts paintOptions) string {
	if snap == nil {
		return dimStyle.Render("(no screen captured for this frame)")
	}
	cols, rows := snap.Cols, snap.Rows
	var b strings.Builder
	wrote := false
	if opts.rulers {
		b.WriteString(dimStyle.Render("    " + columnRuler(cols)))
		wrote = true
	}
	for y := 0; y < rows; y++ {
		if opts.skipBlank && !opts.spaces && !opts.rulers && rowEmpty(snap, y, cols) && !rowChanged(opts.changed, y, cols) {
			continue
		}
		if wrote {
			b.WriteByte('\n')
		}
		wrote = true
		if opts.rulers {
			b.WriteString(dimStyle.Render(fmt.Sprintf("%3d ", y)))
		}
		x := 0
		for x < cols {
			st := cellAt(snap, x, y, cols).Style
			marked := opts.changed[[2]int{x, y}]
			var run strings.Builder
			for x < cols {
				c := cellAt(snap, x, y, cols)
				if c.Style != st || opts.changed[[2]int{x, y}] != marked {
					break
				}
				ch := charOf(c)
				if opts.spaces && ch == " " {
					ch = "·"
				}
				run.WriteString(ch)
				x++
			}
			switch {
			case marked:
				b.WriteString(diffStyle.Render(run.String()))
			case st == (terminal.Style{}):
				b.WriteString(run.String())
			default:
				b.WriteString(cellStyle(st).Render(run.String()))
			}
		}
	}
	return b.String()
}

func columnRuler(cols int) string {
	var b strings.Builder
	for x := 0; x < cols; x++ {
		switch {
		case x%10 == 0:
			label := fmt.Sprintf("%d", x)
			b.WriteString(label)
			x += len(label) - 1
		default:
			b.WriteByte('·')
		}
	}
	return b.String()
}

func rowChanged(changed map[[2]int]bool, y, cols int) bool {
	if len(changed) == 0 {
		return false
	}
	for x := 0; x < cols; x++ {
		if changed[[2]int{x, y}] {
			return true
		}
	}
	return false
}

func rowEmpty(snap *terminal.ScreenSnapshot, y, cols int) bool {
	for x := 0; x < cols; x++ {
		if charOf(cellAt(snap, x, y, cols)) != " " {
			return false
		}
	}
	return true
}
