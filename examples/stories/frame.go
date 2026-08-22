package main

import "charm.land/lipgloss/v2"

// Frame is the shared chrome: header, hairline, body, optional composer, status.
// Height is filled so stories inspect the full 80×24 grid, not a cropped dump.
type Frame struct {
	Header   Header
	Body     string
	Composer *Composer
	Keys     string
	Width    int
	Height   int
}

func (f Frame) Render() string {
	w, h := f.Width, f.Height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	header := f.Header.Render(w)
	rule := renderRule(w)
	keys := muteStyle.Width(w).MaxWidth(w).Render(f.Keys)
	chrome := 3 // header + rule + keys
	var composer string
	if f.Composer != nil {
		composer = f.Composer.Render(w)
		chrome += 2
	}
	bodyH := h - chrome
	if bodyH < 1 {
		bodyH = 1
	}
	body := lipgloss.NewStyle().Width(w).Height(bodyH).MaxHeight(bodyH).Align(lipgloss.Left, lipgloss.Top).Render(f.Body)
	parts := []string{header, rule, body}
	if f.Composer != nil {
		parts = append(parts, rule, composer)
	}
	parts = append(parts, keys)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
