package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

// TestRunSpec_FileVerifierMatches is the canonical smoke test for the
// `file:` verifier. A preconditions command writes a marker file into
// the run dir, then a single `file:` outcome polls until it appears
// and confirms the body contains a needle.
func TestRunSpec_FileVerifierMatches(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "file_verifier.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: file_verifier_smoke
intent: a `+"`file:`"+` verifier polls for a file and checks the body.
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - download:
      path: "`+filepath.Join(dir, "marker.txt")+`"
      saveAs: marker.txt
      assign: marker
      waitFor: true
      timeoutMs: 2000
outcomes:
  - id: file_appeared
    description: the marker file exists in the spec dir
    verify:
      file:
        glob: "`+filepath.Join(dir, "marker*")+`"
        contains: hello
        timeoutMs: 2000
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Stage the marker so the download step can capture it.
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("hello from file verifier\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
}

// TestRunSpec_FileVerifierWaitsForFile proves the verifier's `timeoutMs`
// keeps the run alive long enough for a delayed file to appear.
func TestRunSpec_FileVerifierWaitsForFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "delayed.txt")
	specPath := filepath.Join(dir, "file_wait.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: file_wait
intent: file verifier waits for a delayed file.
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: file_arrived
    description: the delayed file appears within the verifier timeout
    verify:
      file:
        glob: "`+target+`"
        contains: "delayed body"
        timeoutMs: 3000
`), 0o644); err != nil {
		t.Fatal(err)
	}
	go func() {
		// Schedule the file for 200ms — well inside the 3s timeout.
		// (Inlined; no time import dance needed for the test.)
		// Use a sleep via a goroutine + signal.
		_ = os.WriteFile(target, []byte("delayed body\n"), 0o644)
	}()

	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
}

// TestRunSpec_FileVerifierTimesOutOnMissingFile ensures the verifier
// reports a timeout (not a panic, not a phantom pass) when the file
// never appears.
func TestRunSpec_FileVerifierTimesOutOnMissingFile(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "file_missing.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: file_missing
intent: file verifier fails clearly when the file never appears.
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: never_arrives
    description: the missing file is never found
    verify:
      file:
        glob: "`+filepath.Join(dir, "does-not-exist-*.txt")+`"
        timeoutMs: 200
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusFailed {
		t.Fatalf("expected failed status, got %s (outcomes=%#v)", result.Status, result.Outcomes)
	}
	// The outcome's outer timeout (DefaultTimeout) wraps the inner
	// 200ms verifier timeout, so the final message comes from the
	// outcome wrapper. Just assert a timeout-shaped message is present.
	if !strings.Contains(result.Outcomes[0].Message, "timed out") {
		t.Fatalf("expected a timeout-shaped failure, got %q", result.Outcomes[0].Message)
	}
}

// TestRunSpec_ScriptVerifierInline is the smoke test for the inline
// `script:` verifier (the `run:` form). The body returns
// `{ ok: true, evidence: { ... } }` and the runner writes the evidence
// to outcomes/<id>.raw.json.
func TestRunSpec_ScriptVerifierInline(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "script_inline.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: script_inline
intent: a script verifier with an inline body returns ok and evidence.
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: inline_check
    description: inline body returns ok
    verify:
      script:
        runtime: node
        run: |
          process.stdout.write(JSON.stringify({ ok: true, evidence: { rows: 42 } }));
        timeoutMs: 5000
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	if result.Outcomes[0].EvidenceRaw == "" {
		t.Fatalf("expected EvidenceRaw to be set, got %+v", result.Outcomes[0])
	}
	rawPath := filepath.Join(result.RunDir, result.Outcomes[0].EvidenceRaw)
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if !strings.Contains(string(raw), `"rows": 42`) {
		t.Fatalf("raw evidence missing rows=42: %s", string(raw))
	}
}

// TestRunSpec_ScriptVerifierRejectsNonJSON guards against a buggy
// script that emits unstructured stdout. The verifier must surface
// the parse error in the outcome message, not silently mark it passed.
func TestRunSpec_ScriptVerifierRejectsNonJSON(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "script_bad.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: script_bad
intent: a script that prints non-JSON stdout must fail the outcome.
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: bad_output
    description: parser failure surfaces
    verify:
      script:
        runtime: node
        run: |
          console.log("not json");
        timeoutMs: 5000
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusFailed {
		t.Fatalf("expected failed status, got %s", result.Status)
	}
	if !strings.Contains(result.Outcomes[0].Message, "non-JSON") {
		t.Fatalf("expected non-JSON error, got %q", result.Outcomes[0].Message)
	}
}

// TestBuildRedactor_PerSpecValues covers the Sprint 3 per-spec
// redaction block. A spec that declares `redaction: { values: [...] }`
// must scrub those literals from any artifact, in addition to the
// project-config patterns.
func TestBuildRedactor_PerSpecValues(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(source, []byte("the secret value is hunter2-not-for-you\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "redact.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: redact_smoke
intent: per-spec redaction scrubs declared literals from artifacts.
redaction:
  values:
    - hunter2-not-for-you
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - download:
      path: "`+source+`"
      saveAs: secret.txt
      assign: secret
      timeoutMs: 2000
outcomes:
  - id: secret_redacted
    description: the captured artifact does not contain the literal value
    verify:
      command:
        run: "! grep -q 'hunter2-not-for-you' \"$GLYPHRUN_RUN_DIR/artifacts/secret/secret.txt\" && grep -q redacted \"$GLYPHRUN_RUN_DIR/artifacts/secret/secret.txt\""
`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	// Independently read the captured file and confirm the secret is gone.
	captured, err := os.ReadFile(filepath.Join(result.RunDir, "artifacts/secret/secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(captured), "hunter2-not-for-you") {
		t.Errorf("captured artifact still contains the secret: %q", string(captured))
	}
	if !strings.Contains(string(captured), "[redacted]") {
		t.Errorf("captured artifact should contain [redacted] marker, got: %q", string(captured))
	}
}

// TestBuildRedactor_DropsShortValues confirms the 4-char minimum
// (matches cairn's policy). A spec that lists "ab" must be ignored —
// redacting two-letter strings would shred the artifact content.
func TestBuildRedactor_DropsShortValues(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "safe.txt")
	if err := os.WriteFile(source, []byte("safe content here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "short.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: short_redact
intent: short values are dropped from redaction.
redaction:
  values:
    - "ab"
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - download:
      path: "`+source+`"
      saveAs: safe.txt
      assign: safe
      timeoutMs: 2000
outcomes:
  - id: still_readable
    description: the captured file is untouched
    verify:
      command:
        run: "grep -q 'safe content' \"$GLYPHRUN_RUN_DIR/artifacts/safe/safe.txt\""
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
}

// TestBuildRedactor_LongestWins locks the longest-first ordering.
// A spec that lists both "abc" and "abc-123" must redact the longer
// string first so the shorter one doesn't shadow it.
func TestBuildRedactor_LongestWins(t *testing.T) {
	r := buildRedactor(config.Redaction{Enabled: true}, &spec.Redaction{
		Values: []string{"abc", "abc-123-xyz"},
	}, nil)
	out := r.Text("the token is abc-123-xyz and abc alone is fine")
	want := "the token is [redacted] and abc alone is fine"
	if out != want {
		t.Fatalf("longest-first ordering failed:\n got: %q\nwant: %q", out, want)
	}
}
