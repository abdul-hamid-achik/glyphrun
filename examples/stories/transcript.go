package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	roleYou   = "you"
	roleGlyph = "glyph"
	roleTool  = "tool"
)

// Msg is one turn in the agent transcript.
type Msg struct {
	Role string
	Body string
	Meta string // tool name
	Tag  string // running, failed, …
}

func RenderTranscript(msgs []Msg, emptyTitle, emptyHint string, width int) string {
	if len(msgs) == 0 {
		parts := []string{inkStyle.Render(emptyTitle)}
		if emptyHint != "" {
			parts = append(parts, "", muteStyle.Width(width).Render(emptyHint))
		}
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}
	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteByte('\n')
			b.WriteByte('\n')
		}
		b.WriteString(renderMsg(m, width))
	}
	return b.String()
}

func renderMsg(m Msg, width int) string {
	switch m.Role {
	case roleTool:
		left := muteStyle.Render("tool  ") + inkStyle.Render(m.Meta)
		right := statusStyle(m.Tag).Render(m.Tag)
		head := splitBar(left, right, width)
		if m.Body == "" {
			return head
		}
		body := muteStyle.Width(width).Render("  " + m.Body)
		return head + "\n" + body
	case roleGlyph:
		head := accentStyle.Bold(true).Render(roleGlyph)
		body := inkStyle.Width(width).Render(indent(m.Body, 2))
		return head + "\n" + body
	default:
		head := muteStyle.Render(m.Role)
		body := inkStyle.Width(width).Render(indent(m.Body, 2))
		return head + "\n" + body
	}
}

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}
