package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

// TestRerunFailedJSONIsParseable guards that `glyph run --rerun-failed
// --format json` emits valid JSON rather than the human Markdown report.
// JSON/YAML output must always be machine-parseable.
func TestRerunFailedJSONIsParseable(t *testing.T) {
	dir := t.TempDir()
	if err := artifacts.WriteLastFailed(dir, []string{"spec_a", "spec_b"}); err != nil {
		t.Fatal(err)
	}

	opts := &globalOptions{format: "json"}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	// run requires a positional arg even though the rerun-failed branch
	// returns before it is used.
	cmd.SetArgs([]string{"run", "unused.yml", "--rerun-failed", "--format", "json", "--artifact-root", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rerun-failed failed: %v\n%s", err, stdout.String())
	}

	var out struct {
		SchemaVersion  int      `json:"schemaVersion"`
		LastFailedFile string   `json:"lastFailedFile"`
		Failed         []string `json:"failed"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(out.Failed) != 2 || out.Failed[0] != "spec_a" || out.Failed[1] != "spec_b" {
		t.Errorf("failed list = %v, want [spec_a spec_b]", out.Failed)
	}
	if out.LastFailedFile != filepath.Join(dir, artifacts.LastFailedFile) {
		t.Errorf("lastFailedFile = %q, want %q", out.LastFailedFile, filepath.Join(dir, artifacts.LastFailedFile))
	}
}

// TestRerunFailedJSONEmptyIsParseable guards the empty case: no recorded
// failures must still produce parseable JSON with an empty list, not a
// plain-text "no failures" line.
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
		Failed []string `json:"failed"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("empty output is not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(out.Failed) != 0 {
		t.Errorf("failed list = %v, want empty", out.Failed)
	}
}
