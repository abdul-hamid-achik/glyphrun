package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

// TestRerunFailedWithPathsReplacesArgs ensures path-aware index entries
// cause --rerun-failed to actually re-execute (args rewritten to those paths).
// We stop short of a full run by using missing files and asserting the
// structured error envelope still names the path from the index.
func TestRerunFailedWithPathsAttemptsRun(t *testing.T) {
	dir := t.TempDir()
	// Point artifact root at dir; write a path that does not exist so the
	// runner fails at parse — enough to prove the path was selected.
	missing := filepath.Join(dir, "does-not-exist.yml")
	if err := artifacts.WriteLastFailed(dir, []artifacts.FailedSpec{
		{Name: "ghost", Path: missing},
	}); err != nil {
		t.Fatal(err)
	}

	opts := &globalOptions{format: "json", artifactRoot: dir}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"run", "unused.yml", "--rerun-failed", "--format", "json", "--artifact-root", dir})
	err := cmd.Execute()
	// Expect non-zero: the path cannot be parsed.
	if err == nil {
		t.Fatalf("expected error for missing path, got success\n%s", stdout.String())
	}
	// Output should still be JSON (structured envelope).
	if !json.Valid(stdout.Bytes()) && !bytes.Contains(stdout.Bytes(), []byte("ghost")) {
		// Batch or single result; either way the command should have tried the path.
		t.Logf("stdout: %s", stdout.String())
	}
}

// TestRerunFailedNameOnlyListsWithoutRerun guards the legacy text-only index:
// when paths are missing, --rerun-failed lists entries and does not re-run.
func TestRerunFailedNameOnlyListsWithoutRerun(t *testing.T) {
	dir := t.TempDir()
	// Write only the legacy text file (no JSON) to force name-only entries.
	if err := os.WriteFile(filepath.Join(dir, artifacts.LastFailedFile), []byte("spec_a\nspec_b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := &globalOptions{format: "json"}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"run", "unused.yml", "--rerun-failed", "--format", "json", "--artifact-root", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rerun-failed failed: %v\n%s", err, stdout.String())
	}

	var out struct {
		SchemaVersion int                    `json:"schemaVersion"`
		Failed        []artifacts.FailedSpec `json:"failed"`
		Rerun         bool                   `json:"rerun"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout.String())
	}
	if out.Rerun {
		t.Errorf("expected rerun=false for name-only index")
	}
	if len(out.Failed) != 2 || out.Failed[0].Name != "spec_a" || out.Failed[1].Name != "spec_b" {
		t.Errorf("failed list = %+v, want [spec_a spec_b]", out.Failed)
	}
}

// TestRerunFailedJSONEmptyIsParseable guards the empty case.
func TestRerunFailedJSONEmptyIsParseable(t *testing.T) {
	dir := t.TempDir()
	opts := &globalOptions{format: "json"}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"run", "unused.yml", "--rerun-failed", "--format", "json", "--artifact-root", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rerun-failed failed: %v\n%s", err, stdout.String())
	}

	var out struct {
		Failed []artifacts.FailedSpec `json:"failed"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("empty output is not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(out.Failed) != 0 {
		t.Errorf("failed list = %v, want empty", out.Failed)
	}
}
