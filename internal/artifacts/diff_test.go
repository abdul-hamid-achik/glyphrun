package artifacts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiffRunDirs(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	for _, runDir := range []string{a, b} {
		if err := os.MkdirAll(filepath.Join(runDir, "screens"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(a, "run.json"), []byte(`{"schemaVersion":1,"runId":"a","specName":"demo","status":"passed","outcomes":[{"id":"ready","status":"passed","message":"ok"}],"artifacts":{"finalScreenText":"screens/final.txt"},"exitCode":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "run.json"), []byte(`{"schemaVersion":1,"runId":"b","specName":"demo","status":"failed","outcomes":[{"id":"ready","status":"failed","message":"missing"}],"artifacts":{"finalScreenText":"screens/final.txt"},"exitCode":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a, "screens", "final.txt"), []byte("ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "screens", "final.txt"), []byte("broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff, err := DiffRunDirs(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Changed || !diff.StatusChanged || !diff.FinalScreenChanged {
		t.Fatalf("unexpected diff: %#v", diff)
	}
	if len(diff.OutcomeDiffs) != 1 {
		t.Fatalf("outcome diffs = %#v", diff.OutcomeDiffs)
	}
}
