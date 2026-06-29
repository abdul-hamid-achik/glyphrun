package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/affected"
)

// specBody is a compact helper for building a valid spec fixture with the
// given name + coversSymbol. The target/outcome are the minimal shell stub
// `glyph list` accepts without a parse error.
func specBody(name, coversSymbol string) string {
	cs := ""
	if coversSymbol != "" {
		cs = "coversSymbol: " + coversSymbol + "\n"
	}
	return "version: 1\nname: " + name + "\n" + cs + `intent: ` + name + `
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: ok
    description: smoke check
    verify:
      command:
        run: "true"
`
}

// fakeCodemap writes a shell script that emits the given JSON to stdout and
// returns its path. Used to drive the command end-to-end without a real
// codemap install. Skipped on non-Unix where /bin/sh is absent.
func fakeCodemap(t *testing.T, jsonBody string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake codemap shell script is Unix-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-codemap")
	script := "#!/bin/sh\ncat <<'EOF'\n" + jsonBody + "\nEOF\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestAffectedSpecsCommand_MDOutput drives the full command with a fake
// codemap binary: the md (default) output must be exactly the matched spec
// paths, one per line, so `glyph run $(glyph affected-specs ...)` works.
func TestAffectedSpecsCommand_MDOutput(t *testing.T) {
	dir := t.TempDir()
	specs := map[string]string{
		"run.yml":     specBody("run_spec", "Run"),
		"other.yml":   specBody("other_spec", "Other"),
		"nocover.yml": specBody("nocover_spec", ""),
		"miss.yml":    specBody("miss_spec", "Missing"),
	}
	for name, body := range specs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fake := fakeCodemap(t, `{
		"changed_symbols": [{"symbol":"Run","fqn":"app.Run"}],
		"blast_radius": [{"symbol":"Other","fqn":"app.Other"}]
	}`)
	opts := &globalOptions{}
	cmd := newRootCommand(opts)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"affected-specs", dir, "--codemap", fake, "--quiet", "--format", "md"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, stderr.String())
	}
	// stdout is bare paths, sorted, newline-terminated.
	got := strings.TrimRight(stdout.String(), "\n")
	gotPaths := strings.Split(got, "\n")
	want := []string{filepath.Join(dir, "other.yml"), filepath.Join(dir, "run.yml")}
	if len(gotPaths) != 2 || gotPaths[0] != want[0] || gotPaths[1] != want[1] {
		t.Fatalf("stdout = %q\nwant %v", stdout.String(), want)
	}
}

// TestAffectedSpecsCommand_JSONOutput checks the structured report carries
// matched/unmatched/noCover counts, the mode/since, and per-spec reasons.
func TestAffectedSpecsCommand_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run.yml"), []byte(specBody("run_spec", "Run")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "miss.yml"), []byte(specBody("miss_spec", "Missing")), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := fakeCodemap(t, `{
		"changed_symbols": [{"symbol":"Run","fqn":"app.Run"}],
		"blast_radius": [],
		"resolution": ""
	}`)
	opts := &globalOptions{}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"affected-specs", dir, "--since", "HEAD^", "--codemap", fake, "--quiet", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var report affected.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stdout.String())
	}
	if report.SchemaVersion != 1 {
		t.Errorf("schemaVersion = %d, want 1", report.SchemaVersion)
	}
	if report.Mode != "since" || report.Since != "HEAD^" {
		t.Errorf("mode/since = %q/%q, want since/HEAD^", report.Mode, report.Since)
	}
	if report.Total != 2 || report.Matched != 1 || report.Unmatched != 1 {
		t.Errorf("total/matched/unmatched = %d/%d/%d, want 2/1/1", report.Total, report.Matched, report.Unmatched)
	}
	if len(report.Specs) != 1 || report.Specs[0].MatchedBy != "changed" {
		t.Errorf("specs = %+v, want one run.yml matchedBy changed", report.Specs)
	}
}

// TestAffectedSpecsCommand_MissingCodemap confirms a non-existent --codemap
// path surfaces a clear error (not a panic) so CI fails loudly when codemap
// is misconfigured.
func TestAffectedSpecsCommand_MissingCodemap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run.yml"), []byte(specBody("run_spec", "Run")), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := &globalOptions{}
	cmd := newRootCommand(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"affected-specs", dir, "--codemap", "/no/such/codemap-binary", "--format", "md"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for missing codemap binary, got nil")
	}
	if !strings.Contains(err.Error(), "codemap") {
		t.Errorf("error %q should mention codemap", err.Error())
	}
}

// TestAffectedSpecsCommand_SinceAndStagedConflict guards that passing both
// diff scopes is rejected up front (mirrors codemap review's mutual
// exclusion) instead of silently picking one.
func TestAffectedSpecsCommand_SinceAndStagedConflict(t *testing.T) {
	opts := &globalOptions{}
	cmd := newRootCommand(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"affected-specs", t.TempDir(), "--since", "HEAD^", "--staged", "--format", "md"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for --since + --staged, got nil")
	}
	ee, ok := err.(exitError)
	if !ok || ee.code != 2 {
		t.Errorf("err = %v, want exitError code 2", err)
	}
}
