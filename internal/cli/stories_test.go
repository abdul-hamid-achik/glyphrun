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
			Golden string `json:"golden"`
			Source string `json:"source"`
		} `json:"stories"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if payload.SchemaVersion != 1 || len(payload.Stories) != 1 || payload.Stories[0].Name != "story_alpha" || payload.Stories[0].Source != "spec" || payload.Stories[0].Golden != "none" {
		t.Fatalf("payload = %+v", payload)
	}

	for _, flag := range []string{"--html", "--tui"} {
		opts = &globalOptions{format: "json"}
		cmd = newRootCommand(opts)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs([]string{"--format", "json", "stories", dir, flag})
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected exit 2 for %s --format json", flag)
		}
		ee, ok := err.(exitError)
		if !ok || ee.code != 2 {
			t.Fatalf("err = %v, want exitError 2", err)
		}
	}

	// run --watch and serve are interactive: structured formats are refused.
	for _, args := range [][]string{
		{"--format", "json", "stories", "run", dir, "--watch"},
		{"--format", "yaml", "stories", "serve", dir},
	} {
		cmd = newRootCommand(&globalOptions{format: args[1]})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs(args)
		err := cmd.Execute()
		ee, ok := err.(exitError)
		if !ok || ee.code != 2 {
			t.Fatalf("%v: err = %v, want exitError 2", args, err)
		}
	}
}

func TestStoriesRunManifestCreatesGoldenAndIndex(t *testing.T) {
	dir := t.TempDir()
	manifest := `version: 1
kind: stories
harness:
  cmd: ["/bin/sh", "-c", "printf 'Inbox\\n\\n(empty)\\n'; exit 0"]
defaults:
  terminal: { cols: 40, rows: 8, profile: xterm-256color }
  quit: ""
stories:
  - id: list/empty
    ready: { contains: "(empty)" }
`
	if err := os.WriteFile(filepath.Join(dir, "stories.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nartifactRoot: " + filepath.Join(dir, "runs") + "\nsnapshotRoot: " + filepath.Join(dir, "goldens") + "\nstoriesRoot: " + filepath.Join(dir, "index") + "\n"
	cfgPath := filepath.Join(dir, "glyphrun.config.yml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func() (string, error) {
		cmd := newRootCommand(&globalOptions{format: "json", configPath: cfgPath})
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"--format", "json", "--config", cfgPath, "stories", "run", dir, "--progress", "never"})
		err := cmd.Execute()
		return stdout.String(), err
	}
	out, err := run()
	if err != nil {
		t.Fatalf("first run: %v\n%s", err, out)
	}
	var report struct {
		Passed         int `json:"passed"`
		GoldensCreated int `json:"goldensCreated"`
		Results        []struct {
			Job struct {
				Key      string `json:"key"`
				SpecName string `json:"specName"`
			} `json:"job"`
			GoldenCreated bool `json:"goldenCreated"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("report json: %v\n%s", err, out)
	}
	if report.Passed != 1 || report.GoldensCreated != 1 || len(report.Results) != 1 || report.Results[0].Job.Key != "list/empty" || !report.Results[0].GoldenCreated {
		t.Fatalf("first report = %+v\n%s", report, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "goldens", "story_list_empty", "empty.txt")); err != nil {
		t.Fatalf("golden not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "index", "story_list_empty", "latest.json")); err != nil {
		t.Fatalf("index not written: %v", err)
	}

	// Second run compares against the golden: nothing new, still passing.
	out, err = run()
	if err != nil {
		t.Fatalf("second run: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Passed != 1 || report.GoldensCreated != 0 {
		t.Fatalf("second report = %+v", report)
	}

	// The catalog resolves through the index and reports the golden match.
	cmd := newRootCommand(&globalOptions{format: "json", configPath: cfgPath})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--format", "json", "--config", cfgPath, "stories", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("stories: %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"golden": "match"`) || !strings.Contains(stdout.String(), `"key": "list/empty"`) {
		t.Fatalf("catalog = %s", stdout.String())
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
	for _, rel := range []string{"stories/main.go", "stories/list_empty.go", "stories.yml"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	shDir := t.TempDir()
	cmd = newRootCommand(&globalOptions{format: "json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--format", "json", "stories", "init", shDir, "--lang", "sh"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init sh: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shDir, "stories", "story.sh")); err != nil {
		t.Fatalf("missing sh harness: %v", err)
	}

	stdout.Reset()
	cmd = newRootCommand(&globalOptions{})
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"spec", "scaffold", "--kind", "story"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scaffold story: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"kind: stories", "id: list/empty", "./bin/stories", "golden: true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("scaffold missing %q:\n%s", want, out)
		}
	}
}
