package spec

import (
	"os"
	"path/filepath"
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
