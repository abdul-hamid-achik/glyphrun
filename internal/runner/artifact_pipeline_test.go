package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

// TestRunSpec_DownloadCapturesFile is the canonical smoke test for the
// new download step. It writes a small file to a known temp path, then
// runs a /bin/sh spec whose only meaningful step is `download: { path,
// waitFor: true, saveAs, assign }`. The spec's outcome then verifies the
// captured copy via a `command:` verifier that reads $GLYPHRUN_RUN_DIR.
func TestRunSpec_DownloadCapturesFile(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(sourcePath, []byte("hello from download\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(dir, "download.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: download_smoke
intent: download step captures a file from disk into a named artifact.
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - download:
      path: "`+sourcePath+`"
      saveAs: copy.txt
      assign: copy
      waitFor: true
      timeoutMs: 2000
outcomes:
  - id: captured
    description: the captured copy exists
    verify:
      command:
        run: "test -s \"$GLYPHRUN_RUN_DIR/artifacts/copy/copy.txt\""
  - id: contents_match
    description: the captured copy has the original contents
    verify:
      command:
        run: "grep -q 'hello from download' \"$GLYPHRUN_RUN_DIR/artifacts/copy/copy.txt\""
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
	captured, ok := result.NamedArtifacts["copy"]
	if !ok {
		t.Fatalf("expected named artifact 'copy', got %#v", result.NamedArtifacts)
	}
	if captured.Kind != "download" {
		t.Fatalf("expected kind=download, got %q", captured.Kind)
	}
	if captured.RelativePath != "artifacts/copy/copy.txt" {
		t.Fatalf("unexpected relative path: %q", captured.RelativePath)
	}
	if data, err := os.ReadFile(captured.Path); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(string(data), "hello from download") {
		t.Fatalf("captured contents wrong: %q", string(data))
	}

	// run.json should also surface the named artifact so a CI consumer
	// can resolve ${artifacts.<name>.*} without re-parsing the dir.
	runJSON, err := os.ReadFile(filepath.Join(result.RunDir, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runJSON), `"namedArtifacts"`) {
		t.Fatalf("run.json missing namedArtifacts: %s", string(runJSON))
	}
}

// TestRunSpec_DownloadWaitsForFile proves `waitFor: true` polls the
// filesystem for the source to appear. We schedule the source file to
// be created 250ms after the spec starts, well inside the 2s timeout.
func TestRunSpec_DownloadWaitsForFile(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "delayed.txt")

	specPath := filepath.Join(dir, "download_wait.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: download_wait
intent: download step waits for a file to appear before capturing.
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - download:
      path: "`+sourcePath+`"
      saveAs: delayed.txt
      assign: delayed
      waitFor: true
      timeoutMs: 3000
outcomes:
  - id: captured
    description: the delayed file was captured
    verify:
      command:
        run: "test -s \"$GLYPHRUN_RUN_DIR/artifacts/delayed/delayed.txt\""
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Schedule the source to be created after a short delay so the
	// download step has to actually poll.
	go func() {
		time.Sleep(250 * time.Millisecond)
		_ = os.WriteFile(sourcePath, []byte("delayed body\n"), 0o644)
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
	if _, ok := result.NamedArtifacts["delayed"]; !ok {
		t.Fatalf("expected named artifact 'delayed'")
	}
}

// TestRunSpec_TransformRunsShellScript exercises the shell runtime of
// the transform step: the spec's transform invokes a tiny sh script
// that uppercases a captured download and writes the result to a
// separate named artifact.
func TestRunSpec_TransformRunsShellScript(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(sourcePath, []byte("hello transform\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transformPath := filepath.Join(dir, "upper.sh")
	if err := os.WriteFile(transformPath, []byte("#!/bin/sh\nset -eu\nif [ -z \"$GLYPHRUN_INPUT\" ] || [ -z \"$GLYPHRUN_OUTPUT\" ]; then\n  echo \"missing env\" >&2\n  exit 2\nfi\nmkdir -p \"$(dirname \"$GLYPHRUN_OUTPUT\")\"\ntr '[:lower:]' '[:upper:]' < \"$GLYPHRUN_INPUT\" > \"$GLYPHRUN_OUTPUT\"\nprintf '{\"ok\":true}\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(dir, "transform.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: transform_smoke
intent: transform step runs a shell script that reads the input artifact.
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - download:
      path: "`+sourcePath+`"
      saveAs: input.txt
      assign: input
      waitFor: true
      timeoutMs: 2000
  - transform:
      runtime: shell
      file: "`+transformPath+`"
      input: "${artifacts.input.path}"
      saveAs: upper.txt
      assign: upper
      timeoutMs: 5000
outcomes:
  - id: transform_wrote_artifact
    description: the transform wrote the upper named artifact
    verify:
      command:
        run: "test -s \"$GLYPHRUN_RUN_DIR/transforms/upper/upper.txt\""
  - id: contents_uppercased
    description: the transform's output is the uppercase of the input
    verify:
      command:
        run: "grep -q '^HELLO TRANSFORM' \"$GLYPHRUN_RUN_DIR/transforms/upper/upper.txt\""
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
	if _, ok := result.NamedArtifacts["input"]; !ok {
		t.Fatalf("expected named artifact 'input'")
	}
	upper, ok := result.NamedArtifacts["upper"]
	if !ok {
		t.Fatalf("expected named artifact 'upper'")
	}
	if upper.Kind != "transform" {
		t.Fatalf("expected kind=transform, got %q", upper.Kind)
	}
	if upper.RelativePath != "transforms/upper/upper.txt" {
		t.Fatalf("unexpected relative path: %q", upper.RelativePath)
	}
}

// TestRunSpec_ArtifactPlaceholderResolution proves the runner resolves
// ${artifacts.<name>.path} against the live named-artifact registry as
// each step runs, not against the static parse-time result.
func TestRunSpec_ArtifactPlaceholderResolution(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(sourcePath, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transformPath := filepath.Join(dir, "copy.sh")
	if err := os.WriteFile(transformPath, []byte("#!/bin/sh\nset -eu\nmkdir -p \"$(dirname \"$GLYPHRUN_OUTPUT\")\"\ncp \"$GLYPHRUN_INPUT\" \"$GLYPHRUN_OUTPUT\"\nprintf '{\"ok\":true}\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(dir, "placeholder.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: artifact_placeholder
intent: transform step receives the captured artifact path via ${artifacts.*}.
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - download:
      path: "`+sourcePath+`"
      saveAs: source.txt
      assign: source
      waitFor: true
      timeoutMs: 2000
  - transform:
      runtime: shell
      file: "`+transformPath+`"
      input: "${artifacts.source.path}"
      saveAs: copied.txt
      assign: copied
      timeoutMs: 5000
outcomes:
  - id: copied_exists
    description: the transform used the captured artifact's path
    verify:
      command:
        run: "test -s \"$GLYPHRUN_RUN_DIR/transforms/copied/copied.txt\""
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

// TestRunSpec_MissingArtifactFailsLoudly ensures a misconfigured
// ${artifacts.*} reference is surfaced as a step error, not silently
// dropped or resolved to an empty path.
func TestRunSpec_MissingArtifactFailsLoudly(t *testing.T) {
	dir := t.TempDir()
	transformPath := filepath.Join(dir, "noop.sh")
	if err := os.WriteFile(transformPath, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(dir, "missing.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: missing_artifact_ref
intent: referencing a non-existent artifact must fail with a clear error.
target:
  cmd: ["/bin/sh", "-lc", "true"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - transform:
      runtime: shell
      file: "`+transformPath+`"
      input: "${artifacts.does_not_exist.path}"
      saveAs: out.txt
      assign: out
      timeoutMs: 2000
outcomes:
  - id: noop
    description: should never be evaluated
    verify:
      command:
        run: "true"
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
	events, _ := os.ReadFile(filepath.Join(result.RunDir, "events.ndjson"))
	if !strings.Contains(string(events), "step.failed") {
		t.Fatalf("expected step.failed event in events.ndjson, got: %s", string(events))
	}
}

// TestRunSpec_BatchDeliversInputInOneBurst is an end-to-end test of the
// batch step against a /bin/sh -i process. The spec sends
// "echo BATCH-MARKER\n" + a marker in a single batch and asserts the
// screen shows the marker. This proves the runner concatenated the
// sub-step bytes before writing them.
func TestRunSpec_BatchDeliversInputInOneBurst(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "batch.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: batch_smoke
intent: batch step concatenates sub-step bytes into a single PTY write.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\\n'; read line; printf 'BATCH-MARKER\\n'; read line; exit 0"]
  cwd: "."
terminal:
  cols: 100
  rows: 30
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 3000
  - batch:
      - type: "echo BATCH-MARKER"
      - press: "enter"
      - wait:
          screen:
            contains: "BATCH-MARKER"
          timeoutMs: 3000
  - type: "exit 0"
  - press: "enter"
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: marker_visible
    description: the batch's echo output appeared on the screen
    verify:
      screen:
        contains: "BATCH-MARKER"
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
		t.Logf("diagnostic: events=%s", readFile(filepath.Join(result.RunDir, "events.ndjson")))
		t.Logf("diagnostic: failure.md=%s", readFile(filepath.Join(result.RunDir, "diagnostics/failure.md")))
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	// Sanity: the input log should contain the concatenated echo command.
	inputLog, err := os.ReadFile(filepath.Join(result.RunDir, "raw/input.raw.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(inputLog), "echo BATCH-MARKER") {
		t.Fatalf("input log missing batch echo: %q", string(inputLog))
	}
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("read failed: %v", err)
	}
	return string(data)
}

// TestArtifactNameFromPath_Stable maps a representative set of paths
// to their stable assign names. The mapping is part of the public
// contract: it's what `assign: <auto>` resolves to in the absence of
// an explicit name.
func TestArtifactNameFromPath_Stable(t *testing.T) {
	cases := map[string]string{
		"/tmp/report.txt":             "report",
		"/var/log/Build-Report.TXT":   "build_report",
		"/var/log/2024/01/report.log": "report",
		"":                            "artifact",
		"///":                         "artifact",
		"/tmp/123-data.bin":           "a_123_data",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := artifactNameFromPath(in); got != want {
				t.Fatalf("artifactNameFromPath(%q) = %q, want %q", in, got, want)
			}
		})
	}
}
