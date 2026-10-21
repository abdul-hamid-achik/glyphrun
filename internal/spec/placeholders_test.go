package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func placeholderOpts() ParseOptions {
	return ParseOptions{
		ProjectRoot:  "/proj",
		Vars:         map[string]string{"bin": "./app", "hello-bin": "./hello", "x": "VX"},
		Env:          map[string]string{"READY": "ready", "HOME": "/home/me"},
		ConfigValues: map[string]string{"artifactRoot": ".glyphrun/runs"},
	}
}

func TestIsPlaceholderKey(t *testing.T) {
	cases := map[string]bool{
		// Glyphrun keys.
		"projectRoot":              true,
		"vars.bin":                 true,
		"vars.hello-bin":           true,
		"vars.a.b":                 true,
		"vars.1st":                 true,
		"env.READY_TEXT":           true,
		"config.artifactRoot":      true,
		"artifacts.report.path":    true,
		"artifacts.r.relativePath": true,
		"vars.nmae":                true, // shaped like a key, so a typo stays an error
		// Shell parameter expansions and other text.
		"HOME":         false,
		"PWD":          false,
		"projectroot":  false,
		"var#*:":       false,
		"x:-y":         false,
		"#arr":         false,
		"arr[@]":       false,
		"1":            false,
		"@":            false,
		"vars.":        false,
		"vars":         false,
		"vars.x:-d":    false,
		"vars.x#pre":   false,
		"env.A/b/c":    false,
		"vars. x":      false,
		"artifacts.":   false,
		"Vars.x":       false,
		"projectRoot:": false,
		"a + b":        false,
		"":             false,
	}
	for in, want := range cases {
		if got := IsPlaceholderKey(in); got != want {
			t.Errorf("IsPlaceholderKey(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSubstitutePlaceholdersTable(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string // substring of the error; empty means success
	}{
		{name: "projectRoot", in: "cd ${projectRoot}/x", want: "cd /proj/x"},
		{name: "vars", in: "${vars.bin} ${vars.hello-bin}", want: "./app ./hello"},
		{name: "env", in: "${env.READY}", want: "ready"},
		{name: "config", in: "${config.artifactRoot}", want: ".glyphrun/runs"},
		{name: "plain text without placeholders", in: "echo hi", want: "echo hi"},

		// (a) not key-shaped: left untouched.
		{name: "shell strip prefix", in: `v=${var#*:}`, want: `v=${var#*:}`},
		{name: "shell default", in: `echo ${x:-y}`, want: `echo ${x:-y}`},
		{name: "shell length", in: `echo ${#arr}`, want: `echo ${#arr}`},
		{name: "shell array", in: `echo "${arr[@]}"`, want: `echo "${arr[@]}"`},
		{name: "shell substitution", in: `echo ${path//a/b}`, want: `echo ${path//a/b}`},
		{name: "bare shell variable", in: `echo ${HOME} ${PWD}`, want: `echo ${HOME} ${PWD}`},
		{name: "positional", in: `echo ${1} ${@}`, want: `echo ${1} ${@}`},
		{name: "key with a shell operator is not a key", in: `${vars.x:-d}`, want: `${vars.x:-d}`},
		{name: "javascript expression", in: "`${a + b}`", want: "`${a + b}`"},
		{name: "wrong case is not a key", in: `${Vars.x} ${projectroot}`, want: `${Vars.x} ${projectroot}`},
		{name: "namespace without a name", in: `${vars.}`, want: `${vars.}`},
		{name: "unclosed brace", in: `${vars.x`, want: `${vars.x`},
		{name: "empty braces", in: `${}`, want: `${}`},
		{name: "dollar alone and PID", in: `echo $ $$ $HOME`, want: `echo $ $$ $HOME`},
		{name: "mixed: keys resolve, shell passes", in: `${vars.bin} ${var#*:} ${env.READY}`, want: `./app ${var#*:} ready`},
		{name: "key inside a shell default", in: `${v:-${vars.x}}`, want: `${v:-VX}`},
		{name: "key inside a shell pattern", in: `${v//${vars.x}/y}`, want: `${v//VX/y}`},

		// (b) $${...} yields a literal ${...}.
		{name: "escape a key-shaped span", in: `echo $${vars.x}`, want: `echo ${vars.x}`},
		{name: "escape projectRoot", in: `echo $${projectRoot}`, want: `echo ${projectRoot}`},
		{name: "escape a bare identifier", in: `echo $${HOME}`, want: `echo ${HOME}`},
		{name: "escape a shell expansion", in: `echo $${var#*:}`, want: `echo ${var#*:}`},
		{name: "escape of an undefined key is not an error", in: `$${vars.nope}`, want: `${vars.nope}`},
		{name: "escape next to a real key", in: `$${vars.x} ${vars.x}`, want: `${vars.x} VX`},
		{name: "escape beside a PID", in: `$$${vars.x}`, want: `$${vars.x}`},
		{name: "escaped artifact key is literal text for the runner to resolve", in: `$${artifacts.a.path} ${artifacts.a.path}`, want: `${artifacts.a.path} ${artifacts.a.path}`},
		{name: "double dollar without braces is untouched", in: `echo $$ {vars.x}`, want: `echo $$ {vars.x}`},

		// Runtime keys pass through.
		{name: "artifact key", in: `${artifacts.report.path}`, want: `${artifacts.report.path}`},

		// (c) a typo in a real key is still an error.
		{name: "typo in vars", in: `${vars.nmae}`, wantErr: "vars.nmae"},
		{name: "missing env", in: `${env.MISSING}`, wantErr: "env.MISSING"},
		{name: "missing config", in: `${config.nope}`, wantErr: "config.nope"},
		{name: "typo among valid spans", in: `${vars.bin} ${var#*:} ${vars.binn}`, wantErr: "vars.binn"},
		{name: "typo inside a shell default", in: `${v:-${vars.nmae}}`, wantErr: "vars.nmae"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SubstitutePlaceholders(tc.in, "memory://test", placeholderOpts())
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("got %q, want an error containing %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), "unresolved placeholder") || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want unresolved placeholder %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestSubstitutePlaceholdersReportsEveryMissingKey(t *testing.T) {
	_, err := SubstitutePlaceholders(`${vars.a} ${HOME} ${env.b}`, "memory://test", placeholderOpts())
	if err == nil || !strings.Contains(err.Error(), "vars.a, env.b") || strings.Contains(err.Error(), "HOME") {
		t.Fatalf("error = %v", err)
	}
}

// A script with shell parameter expansions parses, and the expansions reach the
// verifier command byte for byte.
func TestParseFileKeepsShellParameterExpansions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.yml")
	err := os.WriteFile(path, []byte(`version: 1
name: shell_expansions
intent: scripts may use shell parameter expansion
target:
  cmd: ["${vars.bin}"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: ready
outcomes:
  - id: ready
    description: a script uses ${var#*:}, ${x:-y} and $${HOME}
    verify:
      command:
        run: 'line="a:b"; echo "${line#*:} ${HOME:-none} $${vars.bin} ${vars.bin}"'
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ParseFile(path, ParseOptions{Vars: map[string]string{"bin": "./app"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Spec.Outcomes[0].Description; got != "a script uses ${var#*:}, ${x:-y} and ${HOME}" {
		t.Errorf("description = %q", got)
	}
	run := res.Spec.Outcomes[0].Verify.Command.Run
	if want := `line="a:b"; echo "${line#*:} ${HOME:-none} ${vars.bin} ./app"`; run != want {
		t.Errorf("run = %q, want %q", run, want)
	}
}
