package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

// TestSpecVerifyContractHashMismatchExitCode guards that `glyph spec verify`
// surfaces a stale contract hash as exit 6 (contract hash mismatch), matching
// the `glyph run` path and the documented exit-code table. Code 5 is reserved
// for "alternate-screen mode not entered", so a mismatch must not collide with it.
func TestSpecVerifyContractHashMismatchExitCode(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "mismatch.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: hash_mismatch_verify
contractHash: sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
intent: a spec with a stale contract hash.
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - type: "x"
outcomes:
  - id: ok
    description: placeholder
    verify:
      command:
        run: "true"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := &globalOptions{format: "json"}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"spec", "verify", specPath, "--format", "json"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error from mismatched contract hash, got nil\n%s", stdout.String())
	}
	ee, ok := err.(exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T (%v)", err, err)
	}
	if ee.code != 6 {
		t.Errorf("expected exit code 6 (contract-hash mismatch), got %d", ee.code)
	}
}

// TestSpecVerifyEmitsIntent guards that `glyph spec verify --format json`
// surfaces the spec's intent so an external indexer (codemap semantic spec
// catalog, FEATURES feature 6) can consume intent + outcomes from one call.
func TestSpecVerifyEmitsIntent(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "with_intent.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: with_intent
intent: |
  the user can quit the app with q.
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: ok
    description: placeholder
    verify:
      command:
        run: "true"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := &globalOptions{}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"spec", "verify", specPath, "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"intent": "the user can quit the app with q."`)) {
		t.Fatalf("verify json missing trimmed intent:\n%s", stdout.String())
	}
}

// TestSpecScaffoldCoversSymbol guards that `glyph spec scaffold
// --coversSymbol <sym>` writes the binding into the starter spec (so the stub
// is immediately selectable by `glyph affected-specs`), that it is omitted when
// not requested, and that --kind action rejects it (actions have no contract).
func TestSpecScaffoldCoversSymbol(t *testing.T) {
	t.Run("spec kind writes coversSymbol", func(t *testing.T) {
		opts := &globalOptions{}
		cmd := newRootCommand(opts)
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetArgs([]string{"spec", "scaffold", "--coversSymbol", "github.com/org/repo.Handler.ServeHTTP"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
		out := stdout.String()
		if !strings.Contains(out, "coversSymbol: github.com/org/repo.Handler.ServeHTTP\n") {
			t.Fatalf("scaffold missing coversSymbol line:\n%s", out)
		}
		// The stub must still parse as a valid spec: write it and verify.
		dir := t.TempDir()
		specPath := filepath.Join(dir, "stub.yml")
		if err := os.WriteFile(specPath, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
		rt, err := config.LoadRuntime(specPath, "", "")
		if err != nil {
			t.Fatalf("load runtime: %v", err)
		}
		parsed, err := spec.ParseFile(specPath, rt.SpecParseOptions())
		if err != nil {
			t.Fatalf("scaffolded stub does not parse: %v\n%s", err, out)
		}
		if parsed.Spec.CoversSymbol != "github.com/org/repo.Handler.ServeHTTP" {
			t.Errorf("parsed CoversSymbol = %q, want the symbol", parsed.Spec.CoversSymbol)
		}
	})

	t.Run("spec kind omits coversSymbol when not set", func(t *testing.T) {
		opts := &globalOptions{}
		cmd := newRootCommand(opts)
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetArgs([]string{"spec", "scaffold"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
		if strings.Contains(stdout.String(), "coversSymbol:") {
			t.Errorf("scaffold should not emit coversSymbol when unset:\n%s", stdout.String())
		}
	})

	t.Run("action kind rejects coversSymbol", func(t *testing.T) {
		opts := &globalOptions{}
		cmd := newRootCommand(opts)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs([]string{"spec", "scaffold", "--kind", "action", "--coversSymbol", "X"})
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error for --kind action --coversSymbol, got nil")
		}
		ee, ok := err.(exitError)
		if !ok || ee.code != 2 {
			t.Errorf("err = %v, want exitError code 2", err)
		}
	})
}
