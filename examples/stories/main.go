package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

var registry = map[string]func() tea.Model{
	"list/empty":      NewListEmpty,
	"list/rows":       NewListRows,
	"list/error":      NewListError,
	"agent/empty":     NewAgentEmpty,
	"agent/messages":  NewAgentMessages,
	"agent/streaming": NewAgentStreaming,
	"agent/tool":      NewAgentTool,
	"agent/error":     NewAgentError,
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--list" || os.Args[1] == "-list") {
		ids := make([]string, 0, len(registry))
		for id := range registry {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		fmt.Print(strings.Join(ids, "\n"))
		if len(ids) > 0 {
			fmt.Println()
		}
		return
	}
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <story-id> | --list\n", os.Args[0])
		os.Exit(2)
	}
	id := os.Args[1]
	ctor, ok := registry[id]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown story %q\n", id)
		os.Exit(1)
	}
	p := tea.NewProgram(ctor())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func storyView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
