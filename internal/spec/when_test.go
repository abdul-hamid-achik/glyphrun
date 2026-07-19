package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWhenShorthand(t *testing.T) {
	cases := []struct {
		in     string
		wantOK bool
		check  func(Verify) bool
	}{
		{`screen.contains:"hello"`, true, func(v Verify) bool {
			return v.Screen != nil && v.Screen.Contains == "hello"
		}},
		{`screen.contains:hello`, true, func(v Verify) bool {
			return v.Screen != nil && v.Screen.Contains == "hello"
		}},
		{`screen.matches:"^Ready"`, true, func(v Verify) bool {
			return v.Screen != nil && v.Screen.Matches == "^Ready"
		}},
		{`screen.equals:ready`, true, func(v Verify) bool {
			return v.Screen != nil && v.Screen.Equals == "ready"
		}},
		{`process.exited:true`, true, func(v Verify) bool {
			return v.Process != nil && v.Process.Exited != nil && *v.Process.Exited
		}},
		{`process.exitCode:0`, true, func(v Verify) bool {
			return v.Process != nil && v.Process.ExitCode != nil && *v.Process.ExitCode == 0
		}},
		{`not-a-thing:x`, false, nil},
		{``, false, nil},
	}
	for _, tc := range cases {
		v, err := ParseWhenShorthand(tc.in)
		if tc.wantOK && err != nil {
			t.Errorf("%q: unexpected err %v", tc.in, err)
			continue
		}
		if !tc.wantOK && err == nil {
			t.Errorf("%q: expected error", tc.in)
			continue
		}
		if tc.wantOK && tc.check != nil && !tc.check(v) {
			t.Errorf("%q: check failed for %+v", tc.in, v)
		}
	}
}

func TestParseFileAcceptsWhenShorthand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "when.yml")
	body := `version: 1
name: when_shorthand
intent: when shorthand works
target:
  cmd: ["/bin/echo", "ready"]
steps:
  - when: 'screen.notContains:"ready"'
    type: "skip me"
  - wait:
      process:
        exitCode: 0
outcomes:
  - id: ok
    description: exits
    verify:
      process:
        exitCode: 0
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
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
		t.Fatalf("when shorthand not parsed: %#v", parsed.Resolved.Steps[0])
	}
	if parsed.Resolved.Steps[0].When.Screen.NotContains != "ready" {
		t.Errorf("NotContains = %q", parsed.Resolved.Steps[0].When.Screen.NotContains)
	}
}
