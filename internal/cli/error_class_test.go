package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRunErroredTargetStartErrorKind guards that an errored run (target
// failed to start) carries errorKind=target_start + diagnostic in the JSON
// envelope on stdout, so agents can tell the operator "fix cmd/cwd" instead
// of reporting an ambiguous error.
func TestRunErroredTargetStartErrorKind(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "bad_target.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: bad_target
intent: target does not exist.
target:
  cmd: ["/nonexistent/binary"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      process:
        exitCode: 0
      timeoutMs: 1000
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
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"run", specPath, "--format", "json"})
	err := cmd.Execute()
	ee, ok := err.(exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T %v", err, err)
	}
	if ee.code != 2 {
		t.Fatalf("exit code = %d, stdout = %s", ee.code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"errorKind": "target_start"`)) {
		t.Errorf("stdout missing errorKind=target_start: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"diagnostic":`)) {
		t.Errorf("stdout missing diagnostic field: %s", stdout.String())
	}
}

// TestRunFailedStepFailureErrorKind guards that a failed run caused by a
// step error (non-timeout, non-unsupported-terminal) carries
// errorKind=step_failure.
func TestRunFailedStepFailureErrorKind(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "bad_press.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: bad_press
intent: a step uses an invalid key name.
target:
  cmd: ["/bin/sh", "-lc", "sleep 5"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - press: "nonexistent_key_name"
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
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"run", specPath, "--format", "json"})
	err := cmd.Execute()
	ee, ok := err.(exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T %v", err, err)
	}
	if ee.code != 1 {
		t.Fatalf("exit code = %d, stdout = %s", ee.code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"errorKind": "step_failure"`)) {
		t.Errorf("stdout missing errorKind=step_failure: %s", stdout.String())
	}
}

// TestRunOutcomeFailureNoErrorKind guards that a normal outcome failure
// (behavior contract not met) does NOT carry errorKind — it's a legitimate
// test failure, not a runner error.
func TestRunOutcomeFailureNoErrorKind(t *testing.T) {
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
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"run", specPath, "--format", "json"})
	err := cmd.Execute()
	ee, ok := err.(exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T %v", err, err)
	}
	if ee.code != 1 {
		t.Fatalf("exit code = %d, stdout = %s", ee.code, stdout.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte(`"errorKind"`)) {
		t.Errorf("stdout should NOT contain errorKind for outcome failure: %s", stdout.String())
	}
}

// TestRunContractHashMismatchEmitsEnvelope guards that a contract-hash
// mismatch (exit 6) emits a structured JSON envelope on stdout with
// errorKind, contractHash, and expectedHash — not an empty stdout.
func TestRunContractHashMismatchEmitsEnvelope(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "mismatch.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: hash_mismatch
contractHash: sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
intent: stale hash.
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      process:
        exitCode: 0
      timeoutMs: 1000
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
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"run", specPath, "--format", "json"})
	err := cmd.Execute()
	ee, ok := err.(exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T %v", err, err)
	}
	if ee.code != 6 {
		t.Fatalf("exit code = %d, stdout = %s", ee.code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"errorKind": "contract_hash_mismatch"`)) {
		t.Errorf("stdout missing errorKind=contract_hash_mismatch: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"contractHash":`)) {
		t.Errorf("stdout missing contractHash: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"expectedHash":`)) {
		t.Errorf("stdout missing expectedHash: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"diagnostic":`)) {
		t.Errorf("stdout missing diagnostic: %s", stdout.String())
	}
}

// TestRunSpecParseErrorEmitsEnvelope guards that a spec parse/schema error
// (exit 4) emits a structured JSON envelope on stdout with
// errorKind=spec_parse + diagnostic.
func TestRunSpecParseErrorEmitsEnvelope(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "bad_version.yml")
	if err := os.WriteFile(specPath, []byte(`version: 999
name: bad_version
intent: invalid version.
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      process:
        exitCode: 0
      timeoutMs: 1000
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
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"run", specPath, "--format", "json"})
	err := cmd.Execute()
	ee, ok := err.(exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T %v", err, err)
	}
	if ee.code != 4 {
		t.Fatalf("exit code = %d, stdout = %s", ee.code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"errorKind": "spec_parse"`)) {
		t.Errorf("stdout missing errorKind=spec_parse: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"diagnostic":`)) {
		t.Errorf("stdout missing diagnostic: %s", stdout.String())
	}
}

// TestSpecVerifyContractHashMismatchEmitsEnvelope guards that `glyph spec
// verify` emits the structured JSON envelope on stdout for a contract-hash
// mismatch (exit 6), including contractHash, expectedHash, and specName.
func TestSpecVerifyContractHashMismatchEmitsEnvelope(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "mismatch.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: verify_mismatch
contractHash: sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
intent: stale hash.
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      process:
        exitCode: 0
      timeoutMs: 1000
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
	ee, ok := err.(exitError)
	if !ok {
		t.Fatalf("expected exitError, got %T %v", err, err)
	}
	if ee.code != 6 {
		t.Fatalf("exit code = %d, stdout = %s", ee.code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"errorKind": "contract_hash_mismatch"`)) {
		t.Errorf("stdout missing errorKind=contract_hash_mismatch: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"contractHash":`)) {
		t.Errorf("stdout missing contractHash: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"expectedHash":`)) {
		t.Errorf("stdout missing expectedHash: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"specName": "verify_mismatch"`)) {
		t.Errorf("stdout missing specName=verify_mismatch: %s", stdout.String())
	}
}
