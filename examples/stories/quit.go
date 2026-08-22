package main

import tea "charm.land/bubbletea/v2"

func quitOnQ(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return tea.Quit
		}
	}
	return nil
}
