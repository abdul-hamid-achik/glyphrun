package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
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
