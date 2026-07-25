package privacy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRejectsPrivateMarkersInTrackedContent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		contents string
	}{
		{name: "private product name", contents: "graph" + "ite"},
		{name: "private ticket", contents: "OP" + "G-1234"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newTrackedRepo(t, "notes.md", tc.contents)
			output, err := runCheck(t, repo)
			if err == nil {
				t.Fatal("privacy check succeeded for a private marker")
			}
			if !strings.Contains(output, "notes.md") {
				t.Fatalf("privacy check output = %q, want matching path", output)
			}
		})
	}
}

func TestCheckRejectsPrivateMarkersInTrackedPaths(t *testing.T) {
	t.Parallel()

	repo := newTrackedRepo(t, "OP"+"G-1234-notes.md", "safe content")
	output, err := runCheck(t, repo)
	if err == nil {
		t.Fatal("privacy check succeeded for a private path")
	}
	if !strings.Contains(output, "notes.md") {
		t.Fatalf("privacy check output = %q, want matching path", output)
	}
}

func TestCheckAllowsNeutralReservedIdentifier(t *testing.T) {
	t.Parallel()

	repo := newTrackedRepo(t, "notes.md", "EXAMPLE-1234")
	output, err := runCheck(t, repo)
	if err != nil {
		t.Fatalf("privacy check failed unexpectedly: %v\n%s", err, output)
	}
}

func newTrackedRepo(t *testing.T, name, contents string) string {
	t.Helper()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "--", name}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	return repo
}

func runCheck(t *testing.T, repo string) (string, error) {
	t.Helper()

	path, err := filepath.Abs(filepath.Join("..", "..", "scripts", "privacy-check.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path, repo)
	output, err := cmd.CombinedOutput()
	return string(output), err
}
