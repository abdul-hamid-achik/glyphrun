package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunCommandReturnsOutcomeFailureExitCode(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "fail.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: failing_outcome
intent: target prints ready but outcome expects missing.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\n'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: missing
    description: missing text is visible
    verify:
      screen:
        contains: "missing"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := &globalOptions{format: "json", artifactRoot: filepath.Join(dir, "runs")}
	cmd := newRootCommand(opts)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"run", specPath, "--format", "json"})
	err := cmd.Execute()
	exit, ok := err.(exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T %v", err, err)
	}
	if exit.code != 1 {
		t.Fatalf("exit code = %d, stderr = %s stdout = %s", exit.code, stderr.String(), stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"status": "failed"`)) {
		t.Fatalf("stdout did not include failed result: %s", stdout.String())
	}
}

func TestRunCommandParallelPreservesAllResults(t *testing.T) {
	dir := t.TempDir()
	specA := writePassingCLISpec(t, dir, "a")
	specB := writePassingCLISpec(t, dir, "b")
	opts := &globalOptions{format: "json", artifactRoot: filepath.Join(dir, "runs")}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"run", specA, specB, "--parallel", "2", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run failed: %v\n%s", err, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"specName": "a"`)) || !bytes.Contains(stdout.Bytes(), []byte(`"specName": "b"`)) {
		t.Fatalf("stdout missing batch results: %s", stdout.String())
	}
}

func writePassingCLISpec(t *testing.T, dir string, name string) string {
	t.Helper()
	path := filepath.Join(dir, name+".yml")
	if err := os.WriteFile(path, []byte(`version: 1
name: `+name+`
intent: target prints ready and exits.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\n'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: ready
    description: ready is visible
    verify:
      screen:
        contains: "ready"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
