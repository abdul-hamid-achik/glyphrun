package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoriesCommandJSONAndHTMLFormatGuard(t *testing.T) {
	dir := t.TempDir()
	specBody := `version: 1
name: story_alpha
metadata:
  feature: list
  tags: [story]
intent: layout
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: ok
    description: smoke
    verify:
      command:
        run: "true"
`
	if err := os.WriteFile(filepath.Join(dir, "alpha.yml"), []byte(specBody), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := &globalOptions{format: "json"}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--format", "json", "stories", dir, "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("stories json: %v", err)
	}
	var payload struct {
		SchemaVersion int `json:"schemaVersion"`
		Stories       []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"stories"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if payload.SchemaVersion != 1 || len(payload.Stories) != 1 || payload.Stories[0].Name != "story_alpha" {
		t.Fatalf("payload = %+v", payload)
	}

	opts = &globalOptions{format: "json"}
	cmd = newRootCommand(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--format", "json", "stories", dir, "--html"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected exit 2 for --html --format json")
	}
	ee, ok := err.(exitError)
	if !ok || ee.code != 2 {
		t.Fatalf("err = %v, want exitError 2", err)
	}

	opts = &globalOptions{format: "json"}
	cmd = newRootCommand(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--format", "json", "stories", dir, "--tui"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected exit 2 for --tui --format json")
	}
	ee, ok = err.(exitError)
	if !ok || ee.code != 2 {
		t.Fatalf("err = %v, want exitError 2", err)
	}
}

func TestStoriesInitAndSpecScaffoldStory(t *testing.T) {
	dir := t.TempDir()
	opts := &globalOptions{format: "json"}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--format", "json", "stories", "init", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init: %v\n%s", err, stdout.String())
	}
	for _, rel := range []string{"stories/main.go", "stories/list_empty.go", "specs/stories/list_empty.yml"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}

	stdout.Reset()
	cmd = newRootCommand(&globalOptions{})
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"spec", "scaffold", "--kind", "story"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scaffold story: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"tags: [story", "snapshot: list_empty", "./bin/stories"} {
		if !strings.Contains(out, want) {
			t.Fatalf("scaffold missing %q:\n%s", want, out)
		}
	}
}
