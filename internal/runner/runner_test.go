package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

func TestRunSpecShellPTY(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "shell.yml")
	err := os.WriteFile(specPath, []byte(`version: 1
name: shell_quits
intent: a shell target prints ready and exits after q.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\n'; while IFS= read -r line; do if [ \"$line\" = q ]; then exit 0; fi; done"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 2000
  - type: "q"
  - press: "enter"
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: ready_visible
    description: ready is visible
    verify:
      screen:
        contains: "ready"
  - id: clean_exit
    description: process exits
    verify:
      process:
        exitCode: 0
`), 0o644)
	if err != nil {
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
	if _, err := os.Stat(filepath.Join(result.RunDir, "agent_context.md")); err != nil {
		t.Fatal(err)
	}
}

func TestRunSpecUpdatesAndComparesSnapshots(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "glyphrun.config.yml")
	if err := os.WriteFile(configPath, []byte(`version: 1
artifactRoot: runs
snapshotRoot: snapshots
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
`), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "snapshot.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: snapshot_demo
intent: a target prints ready and the screen snapshot is stable.
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
  - snapshot: home
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: home_snapshot
    description: the home snapshot matches the committed snapshot
    verify:
      snapshot:
        name: home
        mode: text
`), 0o644); err != nil {
		t.Fatal(err)
	}

	updated, err := RunSpec(context.Background(), Options{
		SpecPath:        specPath,
		ConfigPath:      configPath,
		UpdateSnapshots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != artifacts.StatusPassed {
		t.Fatalf("update status = %s, outcomes = %#v", updated.Status, updated.Outcomes)
	}
	if _, err := os.Stat(filepath.Join(dir, "snapshots", "snapshot_demo", "home.txt")); err != nil {
		t.Fatal(err)
	}

	checked, err := RunSpec(context.Background(), Options{
		SpecPath:   specPath,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked.Status != artifacts.StatusPassed {
		t.Fatalf("compare status = %s, outcomes = %#v", checked.Status, checked.Outcomes)
	}
	if checked.Artifacts["snapshot:home"] != "snapshots/home.txt" {
		t.Fatalf("snapshot artifact missing: %#v", checked.Artifacts)
	}
}

func TestRunSpecHonorsArtifactFlagsAndNormalization(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "glyphrun.config.yml")
	if err := os.WriteFile(configPath, []byte(`version: 1
artifactRoot: runs
snapshotRoot: snapshots
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
  normalize:
    trimRight: true
    normalizeLineEndings: true
    replace:
      - regex: "run-[0-9]+"
        with: "run-<id>"
artifacts:
  rawLog: false
  frames: false
  finalScreen: false
  snapshots: false
  agentContext: false
`), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "normalize.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: normalize_demo
intent: dynamic output can be normalized.
target:
  cmd: ["/bin/sh", "-lc", "printf 'run-123\n'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "run-<id>"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: normalized
    description: normalized output is visible
    verify:
      screen:
        contains: "run-<id>"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{SpecPath: specPath, ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	if _, ok := result.Artifacts["rawPtyLog"]; ok {
		t.Fatalf("raw artifact should be disabled: %#v", result.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(result.RunDir, "raw", "pty.raw.log")); !os.IsNotExist(err) {
		t.Fatalf("raw log should not exist, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(result.RunDir, "agent_context.md")); !os.IsNotExist(err) {
		t.Fatalf("agent context should not exist, err=%v", err)
	}
}

func TestRunSpecChecksCellStyle(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "style.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: style_demo
intent: styled terminal cells can be asserted.
target:
  cmd: ["/bin/sh", "-lc", "printf '\\033[1m>\\033[0m ready\\n'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "> ready"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: prompt_bold
    description: the prompt marker is bold
    verify:
      cell:
        x: 0
        y: 0
        char: ">"
        style:
          bold: true
  - id: next_cell_plain
    description: the next cell is not bold after reset
    verify:
      cell:
        x: 1
        y: 0
        char: " "
        style:
          bold: false
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
