package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	report, err := PruneRuns(dir, 2, ArchiveConfig{})
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
	report, err := PruneRuns(dir, 10, ArchiveConfig{})
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
	report, err := PruneRuns(dir, 0, ArchiveConfig{})
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
	report, err := PruneRuns(dir, 5, ArchiveConfig{})
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

// stageRunDirsForArchive stages n fake run directories with
// deterministic, descending modtimes (i=4 newest, i=0 oldest) so the
// pruner prunes the oldest n-keep. Each dir carries a marker file so
// the tests can confirm presence/absence after the prune.
func stageRunDirsForArchive(t *testing.T, root string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		name := "2026-01-01T00-00-0" + strconv.Itoa(i) + "Z-r" + strconv.Itoa(i)
		path := filepath.Join(root, name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		mt := baseTime().Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(path, mt, mt); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "marker"), []byte("r"+strconv.Itoa(i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestPruneRuns_ArchiveBeforeDelete covers the archive-enabled happy
// path: with archive.archiveEnabled() true and an exit-0 archive
// script, each pruned directory is sent to the archive command and
// only then deleted locally (move semantics). We stage 5 run dirs and
// keep 2, so 3 are pruned and all 3 must be archived and removed.
func TestPruneRuns_ArchiveBeforeDelete(t *testing.T) {
	dir := t.TempDir()
	stageRunDirsForArchive(t, dir, 5)

	// The archive script records the runDir it was invoked with
	// ($1 = appended positional) to a shared log file. After the
	// prune we confirm the script ran once per pruned dir.
	archiveLog := filepath.Join(t.TempDir(), "archive.log")
	scriptBody := fmt.Sprintf(`printf '%%s\n' "$1" >> %s`, archiveLog)
	script := writeScript(t, scriptBody)

	report, err := PruneRuns(dir, 2, ArchiveConfig{
		Enabled: true,
		Command: script,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Pruned != 3 {
		t.Errorf("expected 3 pruned, got %d", report.Pruned)
	}
	if report.Kept != 2 {
		t.Errorf("expected 2 kept, got %d", report.Kept)
	}
	if report.Archived != 3 {
		t.Errorf("expected 3 archived (one per pruned dir), got %d", report.Archived)
	}
	if len(report.ArchiveErrors) != 0 {
		t.Errorf("expected no archive errors, got %v", report.ArchiveErrors)
	}

	// The 3 oldest dirs must be gone; the 2 newest must survive.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 surviving dirs, got %d", len(entries))
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "2026-01-01T00-00-03Z-r3") &&
			!strings.HasPrefix(name, "2026-01-01T00-00-04Z-r4") {
			t.Errorf("unexpected survivor %q", name)
		}
	}

	// The archive script must have been invoked once per pruned dir.
	logBytes, err := os.ReadFile(archiveLog)
	if err != nil {
		t.Fatalf("archive log not written: %v", err)
	}
	logLines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	if len(logLines) != 3 {
		t.Errorf("expected 3 archive invocations logged, got %d (%q)", len(logLines), string(logBytes))
	}
	// Each pruned dir path (the appended positional) must appear.
	for _, line := range logLines {
		if !strings.HasPrefix(line, dir) {
			t.Errorf("archive log line %q should start with runDir root %q", line, dir)
		}
	}
}

// TestPruneRuns_ArchiveFailureKeepsDir covers the archive failure
// path: when the archive command exits non-zero, the local directory
// is preserved, the path is NOT counted as pruned, Archived stays 0,
// and an entry is recorded in ArchiveErrors. We stage 5 run dirs and
// keep 2; the failing archive script means all 3 candidates are
// preserved, so the disk still holds all 5.
func TestPruneRuns_ArchiveFailureKeepsDir(t *testing.T) {
	dir := t.TempDir()
	stageRunDirsForArchive(t, dir, 5)

	script := writeScript(t, `echo 'archive failed'; exit 1`)

	report, err := PruneRuns(dir, 2, ArchiveConfig{
		Enabled: true,
		Command: script,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Pruned != 0 {
		t.Errorf("expected 0 pruned on archive failure, got %d", report.Pruned)
	}
	if report.Archived != 0 {
		t.Errorf("expected 0 archived on failure, got %d", report.Archived)
	}
	if report.Kept != 2 {
		t.Errorf("expected kept=2 (the keep window), got %d", report.Kept)
	}
	if len(report.ArchiveErrors) != 3 {
		t.Errorf("expected 3 archive errors (one per pruned candidate), got %d: %v", len(report.ArchiveErrors), report.ArchiveErrors)
	}
	if len(report.Paths) != 0 {
		t.Errorf("expected no pruned paths on failure, got %v", report.Paths)
	}

	// All 5 dirs must still exist on disk (3 candidates preserved + 2 kept).
	entries, _ := os.ReadDir(dir)
	if len(entries) != 5 {
		t.Fatalf("expected 5 dirs preserved on archive failure, got %d", len(entries))
	}
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "marker")); err != nil {
			t.Errorf("preserved dir %q lost its marker: %v", e.Name(), err)
		}
	}
}
