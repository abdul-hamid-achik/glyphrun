package main

import (
	tea "charm.land/bubbletea/v2"
)

type agentModel struct {
	width, height int
	brand, title  string
	status        string
	msgs          []Msg
	emptyTitle    string
	emptyHint     string
	composer      Composer
	keys          string
}

func NewAgentEmpty() tea.Model {
	return agentModel{
		width: defaultWidth, height: defaultHeight,
		brand:      "glyph",
		title:      "inbox layout",
		status:     statusIdle,
		emptyTitle: "No turns yet",
		emptyHint:  "Ask where a title sits, or how wide a region is.",
		composer:   Composer{Placeholder: "ask about a cell", Focused: true},
		keys:       "enter send · q quit",
	}
}

func NewAgentMessages() tea.Model {
	return agentModel{
		width: defaultWidth, height: defaultHeight,
		brand:  "glyph",
		title:  "inbox layout",
		status: statusIdle,
		msgs: []Msg{
			{Role: roleYou, Body: "where is the Inbox title?"},
			{Role: roleGlyph, Body: "Cell 0,0 is I. The empty copy sits\non row 2."},
		},
		composer: Composer{Placeholder: "ask about a cell", Focused: true},
		keys:     "enter send · q quit",
	}
}

func NewAgentStreaming() tea.Model {
	return agentModel{
		width: defaultWidth, height: defaultHeight,
		brand:  "glyph",
		title:  "inbox layout",
		status: statusStreaming,
		msgs: []Msg{
			{Role: roleYou, Body: "how wide is the selected row?"},
			{Role: roleGlyph, Body: "Region (0,1) is 20 wide"},
		},
		composer: Composer{Waiting: "waiting on tokens"},
		keys:     "esc cancel · q quit",
	}
}

func NewAgentTool() tea.Model {
	return agentModel{
		width: defaultWidth, height: defaultHeight,
		brand:  "glyph",
		title:  "inbox layout",
		status: statusRunning,
		msgs: []Msg{
			{Role: roleYou, Body: "run the empty inbox story"},
			{Role: roleTool, Meta: "glyph run", Tag: statusRunning, Body: "examples/specs/story_list_empty.yml"},
		},
		composer: Composer{Waiting: "waiting on glyph run"},
		keys:     "esc cancel · q quit",
	}
}

func NewAgentError() tea.Model {
	return agentModel{
		width: defaultWidth, height: defaultHeight,
		brand:  "glyph",
		title:  "inbox layout",
		status: statusFailed,
		msgs: []Msg{
			{Role: roleYou, Body: "open the last snapshot"},
			{Role: roleGlyph, Body: "PTY closed before the ready string.\nCheck target.cmd and re-run."},
		},
		composer: Composer{Placeholder: "ask about a cell", Focused: true},
		keys:     "enter send · q quit",
	}
}

func (m agentModel) Init() tea.Cmd { return nil }

func (m agentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if cmd := quitOnQ(msg); cmd != nil {
		return m, cmd
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
	}
	return m, nil
}

func (m agentModel) View() tea.View {
	return storyView(m.frame().Render())
}

func (m agentModel) frame() Frame {
	w := m.width
	if w <= 0 {
		w = defaultWidth
	}
	c := m.composer
	return Frame{
		Header:   Header{Brand: m.brand, Title: m.title, Status: m.status},
		Body:     RenderTranscript(m.msgs, m.emptyTitle, m.emptyHint, w),
		Composer: &c,
		Keys:     m.keys,
		Width:    m.width,
		Height:   m.height,
	}
}
