package artifacts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadLastFailed_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadLastFailed(dir)
	if err != nil {
		t.Fatalf("read empty: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing file, got %v", got)
	}
	if err := WriteLastFailed(dir, []FailedSpec{
		{Name: "alpha", Path: "specs/alpha.yml"},
		{Name: "beta", Path: "specs/beta.yml"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err = ReadLastFailed(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d (%v)", len(got), got)
	}
	if got[0].Name != "alpha" || got[0].Path != "specs/alpha.yml" {
		t.Errorf("entry 0: %+v", got[0])
	}
	if got[1].Name != "beta" || got[1].Path != "specs/beta.yml" {
		t.Errorf("entry 1: %+v", got[1])
	}
	if _, err := os.Stat(filepath.Join(dir, LastFailedJSON)); err != nil {
		t.Errorf("expected %s: %v", LastFailedJSON, err)
	}
	if _, err := os.Stat(filepath.Join(dir, LastFailedFile)); err != nil {
		t.Errorf("expected %s: %v", LastFailedFile, err)
	}
}

func TestWriteLastFailed_DedupsAndSorts(t *testing.T) {
	dir := t.TempDir()
	in := []FailedSpec{
		{Name: "charlie", Path: "c.yml"},
		{Name: "alpha", Path: "a.yml"},
		{Name: "charlie", Path: "c.yml"},
		{Name: "bravo", Path: "b.yml"},
	}
	if err := WriteLastFailed(dir, in); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLastFailed(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 after dedup, got %d (%v)", len(got), got)
	}
	if got[0].Name != "alpha" || got[1].Name != "bravo" || got[2].Name != "charlie" {
		t.Errorf("sort order: %+v", got)
	}
}

func TestWriteLastFailed_SkipsEmptyNames(t *testing.T) {
	dir := t.TempDir()
	if err := WriteLastFailed(dir, []FailedSpec{
		{Name: "alpha", Path: "a.yml"},
		{Name: ""},
		{Name: " ", Path: ""},
		{Name: "beta", Path: "b.yml"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLastFailed(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d (%v)", len(got), got)
	}
}

func TestReadLastFailed_LegacyTextFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, LastFailedFile), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLastFailed(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Errorf("expected name-only [alpha beta], got %+v", got)
	}
	if got[0].Path != "" {
		t.Errorf("legacy entries should have empty path, got %q", got[0].Path)
	}
}

func TestFailedPaths(t *testing.T) {
	entries := []FailedSpec{
		{Name: "a", Path: "a.yml"},
		{Name: "b"},
		{Name: "c", Path: "c.yml"},
		{Name: "a2", Path: "a.yml"},
	}
	paths := FailedPaths(entries)
	if len(paths) != 2 || paths[0] != "a.yml" || paths[1] != "c.yml" {
		t.Errorf("FailedPaths = %v", paths)
	}
}
