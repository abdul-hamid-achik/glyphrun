package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

// fakeFcheapArchive writes a shell script that records every invocation
// (the full argv, which is `store <runDir>`) to logPath, one per line, and
// exits 0. Used to drive clean's archival path end-to-end without a real
// fcheap/file.cheap install. Skipped on non-Unix where /bin/sh is absent.
func fakeFcheapArchive(t *testing.T, logPath string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake fcheap shell script is Unix-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-fcheap")
	script := "#!/bin/sh\necho \"$@\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeCleanConfig writes a glyphrun.config.yml that points at the given
// absolute artifact root, keeps keepRuns newest runs, and routes pruned
// dirs through the given archive command (`args: [store]`).
func writeCleanConfig(t *testing.T, cfgPath, artifactRoot, archiveCmd string, keepRuns int) {
	t.Helper()
	yml := fmt.Sprintf("version: 1\nartifactRoot: %s\nretention:\n  keepRuns: %d\n  archive:\n    enabled: true\n    command: %s\n    args:\n      - store\n",
		artifactRoot, keepRuns, archiveCmd)
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
}

// stageRuns creates n run directories under root with deterministic, strictly
// increasing mtimes so PruneRuns' newest-first sort is stable. Names follow
// the runner's `YYYY-MM-DDTHH-MM-SSZ-...` convention so CleanAll also picks
// them up. Returns the directory paths in creation (oldest→newest) order.
func stageRuns(t *testing.T, root string, n int) []string {
	t.Helper()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dirs := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("2026-01-01T00-00-%02dZ-r%d", i, i)
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		mt := base.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(dir, mt, mt); err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, dir)
	}
	return dirs
}

func runCleanCmd(t *testing.T, cfgPath string, args ...string) (stdout, stderr string) {
	t.Helper()
	opts := &globalOptions{}
	cmd := newRootCommand(opts)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(append([]string{"clean", "--config", cfgPath}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute clean: %v\nstderr: %s", err, errBuf.String())
	}
	return out.String(), errBuf.String()
}

// TestCleanMarkdownRendersArchive guards the markdown renderer directly:
// when the report archived any dirs (or recorded archive errors), the
// `- archived:` and `- archive error:` lines must appear alongside the
// pruned count.
func TestCleanMarkdownRendersArchive(t *testing.T) {
	got := renderCleanMarkdown("/tmp/root", artifacts.CleanReport{
		Pruned:        2,
		Kept:          3,
		Archived:      2,
		ArchiveErrors: []string{"boom"},
	}, false)
	for _, want := range []string{"- pruned: 2", "- archived: 2", "- archive error: boom"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n%s", want, got)
		}
	}
}

// TestCleanCommandNoArchiveSkipsArchival drives `glyph clean --no-archive`
// end-to-end: pruned run dirs are deleted locally, the configured fcheap
// is NOT invoked (its log stays empty), and the markdown output omits the
// `- archived:` line.
func TestCleanCommandNoArchiveSkipsArchival(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake fcheap shell script is Unix-only")
	}
	tmp := t.TempDir()
	runsRoot := filepath.Join(tmp, "runs")
	logPath := filepath.Join(tmp, "archive.log")
	fcheap := fakeFcheapArchive(t, logPath)
	cfgPath := filepath.Join(tmp, "glyphrun.config.yml")
	writeCleanConfig(t, cfgPath, runsRoot, fcheap, 2)
	staged := stageRuns(t, runsRoot, 5)

	stdout, _ := runCleanCmd(t, cfgPath, "--no-archive", "--format", "md")

	// Oldest 3 pruned locally; newest 2 kept.
	for _, dir := range staged[:3] {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("pruned dir %s still exists", dir)
		}
	}
	for _, dir := range staged[3:] {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("kept dir %s removed: %v", dir, err)
		}
	}
	// fcheap must not have been invoked.
	if data, err := os.ReadFile(logPath); err == nil && len(data) > 0 {
		t.Errorf("fcheap invoked under --no-archive; log:\n%s", data)
	}
	if strings.Contains(stdout, "archived") {
		t.Errorf("md output should not mention archived:\n%s", stdout)
	}
	if !strings.Contains(stdout, "- pruned: 3") {
		t.Errorf("md output missing pruned count:\n%s", stdout)
	}
}

// TestCleanCommandArchivesByDefault confirms that without --no-archive the
// clean command routes pruned dirs through the configured fcheap (the log
// records one `store <runDir>` per pruned dir) and the markdown output
// advertises the archived count.
func TestCleanCommandArchivesByDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake fcheap shell script is Unix-only")
	}
	tmp := t.TempDir()
	runsRoot := filepath.Join(tmp, "runs")
	logPath := filepath.Join(tmp, "archive.log")
	fcheap := fakeFcheapArchive(t, logPath)
	cfgPath := filepath.Join(tmp, "glyphrun.config.yml")
	writeCleanConfig(t, cfgPath, runsRoot, fcheap, 2)
	staged := stageRuns(t, runsRoot, 5)

	stdout, _ := runCleanCmd(t, cfgPath, "--format", "md")

	// Oldest 3 pruned locally; newest 2 kept.
	for _, dir := range staged[:3] {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("pruned dir %s still exists", dir)
		}
	}
	// fcheap invoked once per pruned dir.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("fcheap log not written: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("fcheap invocations = %d, want 3:\n%s", len(lines), data)
	}
	for _, dir := range staged[:3] {
		want := "store " + dir
		if !strings.Contains(string(data), want) {
			t.Errorf("fcheap log missing %q\n%s", want, data)
		}
	}
	if !strings.Contains(stdout, "- archived: 3") {
		t.Errorf("md output missing archived count:\n%s", stdout)
	}
}
