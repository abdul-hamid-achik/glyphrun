package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestStoryViewsFitAndCopy(t *testing.T) {
	cases := []struct {
		id   string
		ctor func() tea.Model
		want []string
	}{
		{"list/empty", NewListEmpty, []string{"Inbox", "(empty)", "start a thread from a spec", "idle"}},
		{"list/rows", NewListRows, []string{"Inbox", "> hello", "  drafts", "  sent"}},
		{"list/error", NewListError, []string{"Inbox", "failed to load", "failed"}},
		{"agent/empty", NewAgentEmpty, []string{"glyph", "inbox layout", "No turns yet", "ask about a cell"}},
		{"agent/messages", NewAgentMessages, []string{"where is the Inbox title?", "Cell 0,0 is I"}},
		{"agent/streaming", NewAgentStreaming, []string{"20 wide", "streaming", "waiting on tokens"}},
		{"agent/tool", NewAgentTool, []string{"glyph run", "story_list_empty.yml", "running"}},
		{"agent/error", NewAgentError, []string{"PTY closed", "failed", "ask about a cell"}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			content := tc.ctor().View().Content
			for _, want := range tc.want {
				if !strings.Contains(content, want) {
					t.Fatalf("missing %q in\n%s", want, content)
				}
			}
			assertFrame(t, content)
		})
	}
}

func TestListRowsJMovesSelection(t *testing.T) {
	m := NewListRows()
	next, _ := m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	content := next.View().Content
	if !strings.Contains(content, "> drafts") {
		t.Fatalf("expected drafts selected, got\n%s", content)
	}
}

func assertFrame(t *testing.T, content string) {
	t.Helper()
	lines := strings.Split(content, "\n")
	if len(lines) != defaultHeight {
		t.Fatalf("lines = %d, want %d\n%s", len(lines), defaultHeight, content)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > defaultWidth {
			t.Fatalf("line %d width %d > %d: %q", i, w, defaultWidth, line)
		}
	}
}
