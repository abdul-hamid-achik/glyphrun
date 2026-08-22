package scaffold

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

// StoryInitResult records files written or skipped by InitGo.
type StoryInitResult struct {
	SchemaVersion int      `json:"schemaVersion" yaml:"schemaVersion"`
	Lang          string   `json:"lang"`
	Dir           string   `json:"dir"`
	Written       []string `json:"written,omitempty" yaml:"written,omitempty"`
	Skipped       []string `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	SpecPath      string   `json:"specPath,omitempty" yaml:"specPath,omitempty"`
	Stamped       bool     `json:"stamped" yaml:"stamped"`
}

// StorySpecYAML is the starter spec that targets ./bin/stories list/empty.
func StorySpecYAML() string {
	return `version: 1
name: story_list_empty
metadata:
  feature: list
  tags: [story, empty]

intent: |
  the list story renders an empty inbox with the title at the origin.

target:
  cmd: ["./bin/stories", "list/empty"]
  cwd: "."

terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
  alternateScreen: require

preconditions:
  commands:
    - run: "go build -o ./bin/stories ./stories"
      timeoutMs: 30000

steps:
  - wait:
      screen:
        contains: "Inbox"
      timeoutMs: 5000
  - snapshot: list_empty
  - press: "q"
  - wait:
      process:
        exitCode: 0
      timeoutMs: 3000

outcomes:
  - id: title_at_origin
    description: the title starts at cell 0,0
    verify:
      cell:
        x: 0
        y: 0
        char: "I"
  - id: empty_copy
    description: the empty state copy is visible
    verify:
      region:
        x: 0
        y: 2
        width: 80
        height: 3
        contains: "(empty)"
`
}

// InitGo writes a Bubble Tea v2 story harness and a stamped starter spec.
// Existing files are left alone.
func InitGo(dir string) (*StoryInitResult, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	result := &StoryInitResult{SchemaVersion: 1, Lang: "go", Dir: abs}
	files := []struct {
		rel  string
		body string
	}{
		{filepath.Join("stories", "main.go"), goStoriesMain},
		{filepath.Join("stories", "list_empty.go"), goListEmpty},
		{filepath.Join("specs", "stories", "list_empty.yml"), StorySpecYAML()},
	}
	for _, f := range files {
		path := filepath.Join(abs, f.rel)
		if _, err := os.Stat(path); err == nil {
			result.Skipped = append(result.Skipped, f.rel)
			if filepath.Base(f.rel) == "list_empty.yml" {
				result.SpecPath = path
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return result, err
		}
		if err := os.WriteFile(path, []byte(f.body), 0o644); err != nil {
			return result, err
		}
		result.Written = append(result.Written, f.rel)
		if filepath.Base(f.rel) == "list_empty.yml" {
			result.SpecPath = path
		}
	}
	if result.SpecPath != "" {
		rt, err := config.LoadRuntime(result.SpecPath, "", "")
		if err == nil {
			parseOpts := rt.SpecParseOptions()
			parseOpts.AllowHashMismatch = true
			parsed, err := spec.ParseFile(result.SpecPath, parseOpts)
			if err == nil {
				if err := spec.StampContractHash(parsed.Path, parsed.ContractHash); err == nil {
					result.Stamped = true
				}
			}
		}
	}
	if len(result.Written) == 0 && len(result.Skipped) == 0 {
		return result, fmt.Errorf("nothing to write")
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
