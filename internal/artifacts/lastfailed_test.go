package artifacts

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteReadLastFailed_RoundTrip covers the Sprint 3 --rerun-failed
// feature's storage layer. The runner writes the failed spec's name
// to <artifactRoot>/.last-failed.txt on every non-passing run and
// drops it on passing runs. The CLI's `--rerun-failed` reads the
// file to scope the next invocation.
func TestWriteReadLastFailed_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	// Initial empty: ReadLastFailed should return nil without error.
	got, err := ReadLastFailed(dir)
	if err != nil {
		t.Fatalf("read empty: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing file, got %v", got)
	}
	// Write two names.
	if err := WriteLastFailed(dir, []string{"alpha", "beta"}); err != nil {
		t.Fatal(err)
	}
	got, err = ReadLastFailed(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "beta"}
	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, got[i], want[i])
		}
	}
	// Verify the file lives at the conventional name.
	if _, err := os.Stat(filepath.Join(dir, LastFailedFile)); err != nil {
		t.Errorf("expected %s to exist, got %v", LastFailedFile, err)
	}
}

// TestWriteLastFailed_DedupsAndSorts confirms the writer normalizes
// the list: it dedups (the runner appends, so dupes are possible)
// and sorts alphabetically so diffs are stable.
func TestWriteLastFailed_DedupsAndSorts(t *testing.T) {
	dir := t.TempDir()
	// Write in a deliberately unsorted order with a duplicate.
	in := []string{"charlie", "alpha", "charlie", "bravo"}
	if err := WriteLastFailed(dir, in); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, LastFailedFile))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	want := "alpha\nbravo\ncharlie\n"
	if got != want {
		t.Errorf("dedup+sort failed:\n got: %q\nwant: %q", got, want)
	}
}

// TestWriteLastFailed_SkipsEmptyNames guards the writer's "drop
// blanks" filter. A blank name would otherwise show up in the
// --rerun-failed output as `- ` (a bulleted blank), which is hard
// to act on.
func TestWriteLastFailed_SkipsEmptyNames(t *testing.T) {
	dir := t.TempDir()
	if err := WriteLastFailed(dir, []string{"alpha", "", " ", "beta"}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLastFailed(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "beta"}
	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestReadLastFailed_HandlesTrailingNewline confirms ReadLastFailed
// doesn't surface a phantom empty name on a file that ends with
// a trailing newline (the canonical shape the writer produces).
func TestReadLastFailed_HandlesTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, LastFailedFile), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLastFailed(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Errorf("expected [alpha beta], got %v", got)
	}
}
