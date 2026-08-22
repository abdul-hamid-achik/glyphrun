package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type listModel struct {
	width, height int
	status        string
	items         []string
	selected      int
	empty         bool
	err           string
	hint          string
	keys          string
}

func NewListEmpty() tea.Model {
	return listModel{
		width: defaultWidth, height: defaultHeight,
		status: statusIdle,
		empty:  true,
		hint:   "start a thread from a spec",
		keys:   "n new · j/k move · q quit",
	}
}

func NewListRows() tea.Model {
	return listModel{
		width: defaultWidth, height: defaultHeight,
		status:   statusIdle,
		items:    []string{"hello", "drafts", "sent"},
		selected: 0,
		keys:     "n new · j/k move · q quit",
	}
}

func NewListError() tea.Model {
	return listModel{
		width: defaultWidth, height: defaultHeight,
		status: statusFailed,
		err:    "failed to load",
		keys:   "r retry · q quit",
	}
}

func (m listModel) Init() tea.Cmd { return nil }

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if cmd := quitOnQ(msg); cmd != nil {
		return m, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if len(m.items) == 0 {
			break
		}
		switch msg.String() {
		case "j", "down":
			if m.selected < len(m.items)-1 {
				m.selected++
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
		}
	}
	return m, nil
}

func (m listModel) View() tea.View {
	return storyView(m.frame().Render())
}

func (m listModel) frame() Frame {
	return Frame{
		Header: Header{Title: "Inbox", Status: m.status},
		Body:   m.body(),
		Keys:   m.keys,
		Width:  m.width,
		Height: m.height,
	}
}

func (m listModel) body() string {
	w := m.width
	if w <= 0 {
		w = defaultWidth
	}
	if m.err != "" {
		return lipgloss.JoinVertical(lipgloss.Left,
			dangerStyle.Render("! "+m.err),
			"",
			muteStyle.Render("retry with r, or q to quit"),
		)
	}
	if m.empty || len(m.items) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			muteStyle.Render("(empty)"),
			"",
			muteStyle.Render(m.hint),
		)
	}
	var b strings.Builder
	for i, item := range m.items {
		prefix := "  "
		st := inkStyle
		if i == m.selected {
			prefix = "> "
			st = selectedStyle
		}
		b.WriteString(st.Width(w).Render(prefix + item))
		if i < len(m.items)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
