package main

import "charm.land/lipgloss/v2"

// Composer is the single-line prompt at the bottom of an agent session.
type Composer struct {
	Placeholder string
	Value       string
	Focused     bool
	Waiting     string
}

func (c Composer) Render(width int) string {
	if c.Waiting != "" {
		return muteStyle.Width(width).MaxWidth(width).Render(c.Waiting)
	}
	body, bodySt := c.Placeholder, muteStyle
	if c.Value != "" {
		body, bodySt = c.Value, inkStyle
	}
	prompt := muteStyle.Render("> ")
	if c.Focused {
		prompt = inkStyle.Reverse(true).Render(">") + " "
	}
	line := prompt + bodySt.Render(body)
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(line)
}
