package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitGoWritesAndSkips(t *testing.T) {
	dir := t.TempDir()
	first, err := InitGo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Written) != 3 {
		t.Fatalf("written = %v, want 3 files", first.Written)
	}
	if !first.Stamped {
		t.Fatalf("expected starter spec to be stamped")
	}
	second, err := InitGo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Written) != 0 || len(second.Skipped) != 3 {
		t.Fatalf("second init written=%v skipped=%v", second.Written, second.Skipped)
	}
	main := filepath.Join(dir, "stories", "main.go")
	data, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "list/empty") {
		t.Fatalf("harness missing registry id")
	}
}

func TestStorySpecYAMLHasStoryTag(t *testing.T) {
	if !strings.Contains(StorySpecYAML(), "tags: [story") {
		t.Fatal("story YAML missing story tag")
	}
}
