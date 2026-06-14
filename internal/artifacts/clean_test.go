package artifacts

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// baseTime returns a stable epoch for test files. Using a fixed
// base (rather than time.Now()) keeps the test deterministic across
// runs regardless of system clock.
func baseTime() time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

// TestPruneRuns_KeepsNewest is the canonical smoke test for the
// retention engine. We stage 5 fake run directories with descending
// modification times, ask the pruner to keep 2, and verify the 3
// oldest are removed.
func TestPruneRuns_KeepsNewest(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		// Use a stable, sorted-by-string name with explicit modtimes
		// so the test is deterministic regardless of clock skew.
		name := "2026-01-01T00-00-0" + strconv.Itoa(i) + "Z-r" + strconv.Itoa(i)
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		// Newest = i=4, oldest = i=0. Set modtime accordingly.
		mt := baseTime().Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(path, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	report, err := PruneRuns(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	if report.Pruned != 3 {
		t.Errorf("expected 3 pruned, got %d (kept=%d)", report.Pruned, report.Kept)
	}
	if report.Kept != 2 {
		t.Errorf("expected 2 kept, got %d", report.Kept)
	}
	// Verify the survivors are the two newest.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 surviving dirs, got %d", len(entries))
	}
	wantNewest := []string{"2026-01-01T00-00-03Z-r3", "2026-01-01T00-00-04Z-r4"}
	for i, e := range entries {
		if e.Name() != wantNewest[i] {
			t.Errorf("survivor %d: got %q, want %q", i, e.Name(), wantNewest[i])
		}
	}
}

// TestPruneRuns_NoOpWhenUnderLimit confirms the pruner doesn't touch
// the artifact root when the run count is below the keep threshold.
func TestPruneRuns_NoOpWhenUnderLimit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, "2026-01-01T00-00-0"+strconv.Itoa(i)+"Z-r")
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	report, err := PruneRuns(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if report.Pruned != 0 {
		t.Errorf("expected no pruning, got %d", report.Pruned)
	}
	if report.Kept != 3 {
		t.Errorf("expected kept=3, got %d", report.Kept)
	}
}

// TestPruneRuns_DisabledWhenKeepZero covers the default-config case:
// Retention.KeepRuns is 0, so PruneRuns is a no-op regardless of how
// many runs are sitting on disk. This is the "you opted out of
// auto-cleanup" path.
func TestPruneRuns_DisabledWhenKeepZero(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		path := filepath.Join(dir, "2026-01-01T00-00-0"+strconv.Itoa(i)+"Z-r")
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	report, err := PruneRuns(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Pruned != 0 || report.Kept != 0 {
		t.Errorf("expected no-op with keepRuns=0, got pruned=%d kept=%d", report.Pruned, report.Kept)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 5 {
		t.Errorf("expected 5 dirs untouched, got %d", len(entries))
	}
}

// TestPruneRuns_MissingDirIsNoop guards the first-run case where the
// artifact root doesn't exist yet. The runner must not crash on the
// first invocation.
func TestPruneRuns_MissingDirIsNoop(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	report, err := PruneRuns(dir, 5)
	if err != nil {
		t.Errorf("expected no error for missing dir, got %v", err)
	}
	if report.Pruned != 0 || report.Kept != 0 {
		t.Errorf("expected no-op, got pruned=%d kept=%d", report.Pruned, report.Kept)
	}
}

// TestCleanAll_OnlyRemovesRunDirs is the safety net: --all must not
// touch hidden files (`.DS_Store`), top-level files (a stray
// `junit.xml`), or subdirectories that don't follow the run-dir
// naming convention.
func TestCleanAll_OnlyRemovesRunDirs(t *testing.T) {
	dir := t.TempDir()
	// Two run dirs to remove.
	for _, name := range []string{"2026-01-01T00-00-00Z-r1", "2026-01-01T00-00-01Z-r2"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Things to preserve.
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshots", "keep.txt"), []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := CleanAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Pruned != 2 {
		t.Errorf("expected 2 pruned, got %d", report.Pruned)
	}
	// Verify preservation.
	if _, err := os.Stat(filepath.Join(dir, ".DS_Store")); err != nil {
		t.Errorf(".DS_Store should be preserved, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "snapshots", "keep.txt")); err != nil {
		t.Errorf("snapshots/keep.txt should be preserved, got %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("expected 2 surviving entries (.DS_Store + snapshots), got %d", len(entries))
	}
}
