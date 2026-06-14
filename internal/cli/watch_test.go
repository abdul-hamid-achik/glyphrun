package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWatchRootsDedup(t *testing.T) {
	dir := t.TempDir()
	specA := filepath.Join(dir, "a.yml")
	specB := filepath.Join(dir, "b.yml")
	roots := watchRoots([]string{specA, specB}, nil)
	if len(roots) != 1 {
		t.Fatalf("expected specs in the same dir to dedup to 1 root, got %d: %v", len(roots), roots)
	}
	absDir, _ := filepath.Abs(dir)
	if roots[0] != absDir {
		t.Errorf("expected root %q, got %q", absDir, roots[0])
	}
}

func TestWatchRootsExtraPaths(t *testing.T) {
	dir := t.TempDir()
	extra := t.TempDir()
	roots := watchRoots([]string{filepath.Join(dir, "a.yml")}, []string{extra})
	if len(roots) != 2 {
		t.Fatalf("expected spec dir + extra path = 2 roots, got %d: %v", len(roots), roots)
	}
}

func TestFingerprintDetectsChange(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "spec.yml")
	if err := os.WriteFile(f, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	roots := []string{dir}
	before := fingerprint(roots)
	// Rewrite with different size + content; mod time advances too.
	if err := os.WriteFile(f, []byte("two longer contents"), 0o644); err != nil {
		t.Fatal(err)
	}
	after := fingerprint(roots)
	if before == after {
		t.Errorf("expected fingerprint to change after file edit (before=%d after=%d)", before, after)
	}
}

func TestFingerprintIgnoresExcludedDirs(t *testing.T) {
	dir := t.TempDir()
	roots := []string{dir}
	base := fingerprint(roots)
	// Writing under an excluded dir (.glyphrun) must not change the print.
	excluded := filepath.Join(dir, ".glyphrun", "runs")
	if err := os.MkdirAll(excluded, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(excluded, "run.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := fingerprint(roots); got != base {
		t.Errorf("expected fingerprint to ignore .glyphrun output (base=%d got=%d)", base, got)
	}
}
