package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitStoriesGoWritesAndSkips(t *testing.T) {
	dir := t.TempDir()
	first, err := InitStories(dir, "go")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Written) != 3 {
		t.Fatalf("written = %v, want 3 files", first.Written)
	}
	if first.ManifestPath != filepath.Join(dir, "stories.yml") {
		t.Fatalf("manifest path = %q", first.ManifestPath)
	}
	second, err := InitStories(dir, "go")
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
	manifest, err := os.ReadFile(first.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind: stories", "id: list/empty", "go build -o ./bin/stories", "golden: true"} {
		if !strings.Contains(string(manifest), want) {
			t.Fatalf("manifest missing %q", want)
		}
	}
}

func TestInitStoriesShWritesExecutableHarness(t *testing.T) {
	dir := t.TempDir()
	res, err := InitStories(dir, "sh")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 2 {
		t.Fatalf("written = %v, want harness + manifest", res.Written)
	}
	info, err := os.Stat(filepath.Join(dir, "stories", "story.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o100 == 0 {
		t.Fatalf("sh harness should be executable, mode %v", info.Mode())
	}
	manifest, _ := os.ReadFile(res.ManifestPath)
	if !strings.Contains(string(manifest), `["sh", "./stories/story.sh"]`) || strings.Contains(string(manifest), "build:") {
		t.Fatalf("sh manifest = %s", manifest)
	}
	if _, err := InitStories(dir, "rust"); err == nil {
		t.Fatal("expected unsupported lang error")
	}
}

func TestStoryManifestYAMLIsAStoriesDocument(t *testing.T) {
	if !strings.Contains(StoryManifestYAML("go"), "kind: stories") {
		t.Fatal("manifest YAML missing kind")
	}
}
