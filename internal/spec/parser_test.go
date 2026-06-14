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

func TestParseFileAcceptsConditionalStep(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conditional.yml")
	err := os.WriteFile(path, []byte(`version: 1
name: conditional_step
intent: conditional steps can skip repair-only input.
target:
  cmd: ["/bin/echo", "ready"]
steps:
  - when:
      screen:
        notContains: "ready"
    type: "only when not ready"
  - wait:
      process:
        exitCode: 0
outcomes:
  - id: clean_exit
    description: command exits
    verify:
      process:
        exitCode: 0
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseFile(path, ParseOptions{
		ProjectRoot:     repoRoot(t),
		DefaultTerminal: Terminal{Cols: 80, Rows: 24, Profile: "xterm-256color"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Resolved.Steps[0].When == nil || parsed.Resolved.Steps[0].When.Screen == nil {
		t.Fatalf("conditional step was not parsed: %#v", parsed.Resolved.Steps[0])
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

func TestSubstitutePlaceholders_LeavesRuntimeArtifactKeysIntact(t *testing.T) {
	in := `version: 1
name: runtime_placeholder_test
intent: artifact placeholders survive parse
target:
  cmd: ["/bin/echo"]
steps:
  - download:
      path: "${artifacts.report.path}"
      saveAs: copy.txt
      assign: copy
`
	out, err := SubstitutePlaceholders(in, "memory://test", ParseOptions{
		Vars:         map[string]string{},
		Env:          map[string]string{},
		ConfigValues: map[string]string{},
	})
	if err != nil {
		t.Fatalf("expected runtime placeholder to pass through, got error: %v", err)
	}
	if !strings.Contains(out, "${artifacts.report.path}") {
		t.Fatalf("expected artifact placeholder to be preserved verbatim, got:\n%s", out)
	}
	if !strings.Contains(out, "assign: copy") {
		t.Fatalf("expected other YAML keys to survive, got:\n%s", out)
	}
}

func TestIsRuntimePlaceholder(t *testing.T) {
	cases := map[string]bool{
		"artifacts.report.path":    true,
		"artifacts.X.relativePath": true,
		"vars.bin":                 false,
		"env.READY_TEXT":           false,
		"config.artifactRoot":      false,
		"projectRoot":              false,
	}
	for in, want := range cases {
		if got := IsRuntimePlaceholder(in); got != want {
			t.Errorf("IsRuntimePlaceholder(%q) = %v, want %v", in, got, want)
		}
	}
}
