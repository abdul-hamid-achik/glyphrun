package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
)

// StoryInitResult records files written or skipped by InitStories.
type StoryInitResult struct {
	SchemaVersion int      `json:"schemaVersion" yaml:"schemaVersion"`
	Lang          string   `json:"lang"`
	Dir           string   `json:"dir"`
	Written       []string `json:"written,omitempty" yaml:"written,omitempty"`
	Skipped       []string `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	ManifestPath  string   `json:"manifestPath,omitempty" yaml:"manifestPath,omitempty"`
}

// StoryManifestYAML is the starter stories.yml. It is what `glyph spec
// scaffold --kind story` prints and what `glyph stories init` writes.
func StoryManifestYAML(lang string) string {
	cmd, build, watch := `["./bin/stories"]`, `go build -o ./bin/stories ./stories`, `["stories"]`
	if lang == "sh" {
		cmd, build, watch = `["sh", "./stories/story.sh"]`, "", `["stories"]`
	}
	buildLine := ""
	if build != "" {
		buildLine = "  build: \"" + build + "\"\n  buildTimeoutMs: 60000\n"
	}
	return `# Glyphrun stories manifest — one harness, many isolated TUI states.
# Run:   glyph stories run            (builds once, runs every story, captures goldens)
# Live:  glyph stories serve --watch  (rerun / diff / accept goldens in the browser)
version: 1
kind: stories
name: app

harness:
  # The story id is appended as the last argv item: ./bin/stories list/empty
  cmd: ` + cmd + `
  cwd: "."
` + buildLine + `  watch: ` + watch + `

defaults:
  terminal:
    cols: 80
    rows: 24
    profile: xterm-256color
    alternateScreen: require
  readyTimeoutMs: 5000
  quit: "q"          # key pressed after the snapshot; "" leaves the app running
  golden: true       # every story keeps a committed golden screen

stories:
  - id: list/empty
    intent: the list renders an empty inbox with the title at the origin.
    ready: { contains: "Inbox" }
    outcomes:
      - id: title_at_origin
        description: the title starts at cell 0,0
        verify:
          cell: { x: 0, y: 0, char: "I" }
    variants:
      - name: wide
        terminal: { cols: 120, rows: 40 }
`
}

// InitStories writes a story harness plus a stories.yml manifest. lang is
// "go" (Bubble Tea v2) or "sh" (POSIX shell, no toolchain). Existing files
// are left alone so re-running init is safe.
func InitStories(dir, lang string) (*StoryInitResult, error) {
	if dir == "" {
		dir = "."
	}
	if lang == "" {
		lang = "go"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	var files []struct {
		rel  string
		body string
		mode os.FileMode
	}
	switch lang {
	case "go":
		files = append(files,
			struct {
				rel  string
				body string
				mode os.FileMode
			}{filepath.Join("stories", "main.go"), goStoriesMain, 0o644},
			struct {
				rel  string
				body string
				mode os.FileMode
			}{filepath.Join("stories", "list_empty.go"), goListEmpty, 0o644},
		)
	case "sh":
		files = append(files, struct {
			rel  string
			body string
			mode os.FileMode
		}{filepath.Join("stories", "story.sh"), shStoriesHarness, 0o755})
	default:
		return nil, fmt.Errorf("unsupported --lang %q (supported: go, sh)", lang)
	}
	files = append(files, struct {
		rel  string
		body string
		mode os.FileMode
	}{"stories.yml", StoryManifestYAML(lang), 0o644})

	result := &StoryInitResult{SchemaVersion: 1, Lang: lang, Dir: abs}
	for _, f := range files {
		path := filepath.Join(abs, f.rel)
		if f.rel == "stories.yml" {
			result.ManifestPath = path
		}
		if _, err := os.Stat(path); err == nil {
			result.Skipped = append(result.Skipped, f.rel)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return result, err
		}
		if err := os.WriteFile(path, []byte(f.body), f.mode); err != nil {
			return result, err
		}
		result.Written = append(result.Written, f.rel)
	}
	return result, nil
}

const goStoriesMain = `package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// registry maps story ids to constructors. Add one entry per isolated state;
// list the same ids in stories.yml so glyph can run and catalog them.
var registry = map[string]func() tea.Model{
	"list/empty": NewListEmpty,
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
`

const goListEmpty = `package main

import tea "charm.land/bubbletea/v2"

type listEmpty struct{}

func NewListEmpty() tea.Model { return listEmpty{} }

func (listEmpty) Init() tea.Cmd { return nil }

func (m listEmpty) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (listEmpty) View() tea.View {
	v := tea.NewView("Inbox\n\n(empty)\n")
	v.AltScreen = true
	return v
}
`

// shStoriesHarness is a dependency-free harness: it enters the alternate
// screen, paints one state per story id, and waits for q. It proves that
// stories are black-box — any language that can print escape sequences works.
const shStoriesHarness = `#!/bin/sh
# Glyphrun story harness (POSIX shell). Usage: story.sh <story-id> | --list
set -eu

list() {
  printf '%s\n' list/empty
}

paint_list_empty() {
  printf 'Inbox\r\n'
  printf '\r\n'
  printf '(empty)\r\n'
}

case "${1:-}" in
  --list|-list) list; exit 0 ;;
  "") printf 'usage: %s <story-id> | --list\n' "$0" >&2; exit 2 ;;
esac

# Alternate screen + clear + home, restored on exit.
printf '\033[?1049h\033[2J\033[H'
trap 'printf "\033[?1049l"' EXIT

case "$1" in
  list/empty) paint_list_empty ;;
  *) printf 'unknown story %s\n' "$1" >&2; exit 1 ;;
esac

# Wait for q (raw mode so a single key arrives without Enter).
if [ -t 0 ]; then
  stty -echo -icanon min 1 time 0 2>/dev/null || true
fi
while :; do
  key=$(dd bs=1 count=1 2>/dev/null) || break
  [ -z "$key" ] && break   # stdin closed: leave instead of spinning
  [ "$key" = "q" ] && break
done
`
