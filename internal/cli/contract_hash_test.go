package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestClassifyRunError_ContractHashMismatch guards the exit-code
// mapping for spec.ContractHashMismatchError. The CLI must surface
// this as exit 6 (cairn convention) so CI dashboards can mark the
// failure with a distinct signal from a generic parse error (4)
// or a behavior failure (1).
func TestClassifyRunError_ContractHashMismatch(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "mismatch.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: hash_mismatch_smoke
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
	opts := &globalOptions{format: "json", artifactRoot: filepath.Join(dir, "runs")}
	// Run the real CLI path so the parser + classifier are both
	// exercised. The contract-hash mismatch fires inside
	// spec.ParseFile, which is what we want to test.
	_ = io.Discard
	_, _, err := runSpecs(context.Background(), []string{specPath}, 1, opts, false, nil, nil)
	if err == nil {
		t.Fatal("expected error from mismatched contract hash, got nil")
	}
	classified := classifyRunError(err)
	ee, ok := classified.(exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T (%v)", classified, classified)
	}
	if ee.code != 6 {
		t.Errorf("expected exit code 6 (cairn contract-hash), got %d", ee.code)
	}
	if !strings.Contains(ee.err.Error(), "contractHash mismatch") {
		t.Errorf("error message should mention the mismatch, got %q", ee.err.Error())
	}
}
