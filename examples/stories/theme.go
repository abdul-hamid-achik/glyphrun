package main

import (
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

// Palette matches the glyph stories inspect page: ink, mist, sky. Not purple.
var (
	ink = compat.AdaptiveColor{
		Light: lipgloss.Color("#1a1d24"),
		Dark:  lipgloss.Color("#e4e4e7"),
	}
	mute = compat.AdaptiveColor{
		Light: lipgloss.Color("#5c6370"),
		Dark:  lipgloss.Color("#7d8590"),
	}
	line = compat.AdaptiveColor{
		Light: lipgloss.Color("#c5cad3"),
		Dark:  lipgloss.Color("#2a2e36"),
	}
	accent = compat.AdaptiveColor{
		Light: lipgloss.Color("#0b6e8f"),
		Dark:  lipgloss.Color("#7dd3fc"),
	}
	danger = compat.AdaptiveColor{
		Light: lipgloss.Color("#b42318"),
		Dark:  lipgloss.Color("#f87171"),
	}
)

const (
	statusIdle      = "idle"
	statusStreaming = "streaming"
	statusRunning   = "running"
	statusFailed    = "failed"
)

var (
	inkStyle      = lipgloss.NewStyle().Foreground(ink)
	muteStyle     = lipgloss.NewStyle().Foreground(mute)
	accentStyle   = lipgloss.NewStyle().Foreground(accent)
	dangerStyle   = lipgloss.NewStyle().Foreground(danger)
	brandStyle    = lipgloss.NewStyle().Foreground(accent).Bold(true)
	titleStyle    = lipgloss.NewStyle().Foreground(ink).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	ruleStyle     = lipgloss.NewStyle().Foreground(line)
)

func statusStyle(kind string) lipgloss.Style {
	switch kind {
	case statusFailed:
		return dangerStyle
	case statusStreaming, statusRunning:
		return accentStyle
	default:
		return muteStyle
	}
}

func renderRule(width int) string {
	if width < 1 {
		return ""
	}
	return ruleStyle.Render(strings.Repeat("─", width))
}

func splitBar(left, right string, width int) string {
	if width < 1 {
		return ""
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

const (
	defaultWidth  = 80
	defaultHeight = 24
)
