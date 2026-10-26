package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

// snapshotNormalizeSpec is a spec whose target prints line. The snapshot
// verifier compares the screen against the committed golden; outcome and
// snapshot are the normalize blocks placed on the outcome and on the
// verify.snapshot condition (empty strings omit them).
func snapshotNormalizeSpec(line, outcomeNormalize, snapshotNormalize string) string {
	return fmt.Sprintf(`version: 1
name: snapshot_normalize
intent: volatile screen text is masked on both sides before a snapshot compare.
target:
  cmd: ["/bin/sh", "-lc", "printf '%s\\n'"]
terminal:
  cols: 80
  rows: 4
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "build"
      timeoutMs: 2000
  - snapshot: home
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: home_snapshot
    description: the home snapshot matches after masking volatile text
%s    verify:
      snapshot:
        name: home
        mode: text
%s`, line, indent(outcomeNormalize, "    "), indent(snapshotNormalize, "        "))
}

func indent(block, prefix string) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(block, "\n") {
		if line != "" {
			b.WriteString(prefix + line)
		}
	}
	return b.String()
}

const outcomeRunNormalize = `normalize:
  replace:
    - regex: "run-[0-9]+"
      with: "run-<id>"
`

const snapshotStateNormalize = `normalize:
  replace:
    - regex: "(ready|done)"
      with: "<state>"
`

// runSnapshotNormalize records the golden from golden and then compares it
// with a run that prints actual, both specs sharing the spec name so they use
// the same committed snapshot.
func runSnapshotNormalize(t *testing.T, golden, actual, outcomeNormalize, snapshotNormalize string) artifacts.RunStatus {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "glyphrun.config.yml")
	if err := os.WriteFile(configPath, []byte("version: 1\nartifactRoot: runs\nsnapshotRoot: snapshots\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goldenSpec := filepath.Join(dir, "golden.yml")
	if err := os.WriteFile(goldenSpec, []byte(snapshotNormalizeSpec(golden, outcomeNormalize, snapshotNormalize)), 0o644); err != nil {
		t.Fatal(err)
	}
	recorded, err := RunSpec(context.Background(), Options{SpecPath: goldenSpec, ConfigPath: configPath, UpdateSnapshots: true})
	if err != nil {
		t.Fatal(err)
	}
	if recorded.Status != artifacts.StatusPassed {
		t.Fatalf("record status = %s, outcomes = %#v", recorded.Status, recorded.Outcomes)
	}
	actualSpec := filepath.Join(dir, "actual.yml")
	if err := os.WriteFile(actualSpec, []byte(snapshotNormalizeSpec(actual, outcomeNormalize, snapshotNormalize)), 0o644); err != nil {
		t.Fatal(err)
	}
	checked, err := RunSpec(context.Background(), Options{SpecPath: actualSpec, ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	if checked.Status != artifacts.StatusPassed {
		t.Logf("outcomes = %#v", checked.Outcomes)
	}
	return checked.Status
}

func TestSnapshotOutcomeLevelNormalizeMasksBothSides(t *testing.T) {
	// The outcome block is the only mask. Before the fix the snapshot verifier
	// ignored it, so the run id differed and the compare failed.
	status := runSnapshotNormalize(t, "build run-123", "build run-456", outcomeRunNormalize, "")
	if status != artifacts.StatusPassed {
		t.Fatalf("outcome-level normalize did not mask the snapshot: status = %s", status)
	}
}

func TestSnapshotNormalizeMergesOutcomeAndSnapshotLevels(t *testing.T) {
	// Each level masks a different volatile word; the compare passes only
	// when both apply. Outcome rules run first, then the snapshot's own rules.
	status := runSnapshotNormalize(t, "build run-123 ready", "build run-456 done", outcomeRunNormalize, snapshotStateNormalize)
	if status != artifacts.StatusPassed {
		t.Fatalf("merged normalize did not mask both levels: status = %s", status)
	}
	// Without the outcome level the run id still differs, so it must fail.
	if status := runSnapshotNormalize(t, "build run-123 ready", "build run-456 done", "", snapshotStateNormalize); status == artifacts.StatusPassed {
		t.Fatal("snapshot-only normalize masked the outcome's run id, which it should not")
	}
}

func TestSnapshotLevelNormalizeStillMasks(t *testing.T) {
	// Control: the snapshot's own block keeps working on its own.
	status := runSnapshotNormalize(t, "build run-123", "build run-456", "", outcomeRunNormalize)
	if status != artifacts.StatusPassed {
		t.Fatalf("snapshot-level normalize regressed: status = %s", status)
	}
}
