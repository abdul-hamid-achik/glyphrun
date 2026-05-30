package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFileSubstitutesPlaceholders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.yml")
	err := os.WriteFile(path, []byte(`version: 1
name: placeholder_test
intent: checks placeholders
target:
  cmd: ["${vars.bin}"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "${env.READY_TEXT}"
outcomes:
  - id: ready
    description: ready text is visible
    verify:
      screen:
        contains: "${env.READY_TEXT}"
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseFile(path, ParseOptions{
		ProjectRoot: dir,
		Vars:        map[string]string{"bin": "/bin/echo"},
		Env:         map[string]string{"READY_TEXT": "ready"},
		DefaultTerminal: Terminal{
			Cols:    80,
			Rows:    24,
			Profile: "xterm-256color",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Resolved.Target.Cmd[0] != "/bin/echo" {
		t.Fatalf("cmd not substituted: %#v", parsed.Resolved.Target.Cmd)
	}
	if parsed.Resolved.Steps[0].Wait.Screen.Contains != "ready" {
		t.Fatalf("screen wait not substituted")
	}
}

func TestParseFileRejectsMultipleStepActionsWithSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yml")
	err := os.WriteFile(path, []byte(`version: 1
name: invalid_step
intent: invalid specs are rejected before runtime.
target:
  cmd: ["/bin/echo", "ready"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - press: "enter"
    type: "oops"
outcomes:
  - id: ready
    description: ready text is visible
    verify:
      screen:
        contains: "ready"
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseFile(path, ParseOptions{
		ProjectRoot:     repoRoot(t),
		DefaultTerminal: Terminal{Cols: 80, Rows: 24, Profile: "xterm-256color"},
	})
	if err == nil {
		t.Fatal("expected schema validation error")
	}
	if !strings.Contains(err.Error(), "oneOf") {
		t.Fatalf("expected oneOf schema error, got %v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}
