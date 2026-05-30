package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCommandCreatesStarterFiles(t *testing.T) {
	dir := t.TempDir()
	opts := &globalOptions{format: "json"}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{
		"init", dir,
		"--cmd", "./bin/local-agent",
		"--build", "go build -o ./bin/local-agent ./cmd/local-agent",
		"--ready", "Welcome to LOCAL AGENT",
		"--name", "local_agent_smoke",
		"--format", "json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v\n%s", err, stdout.String())
	}
	for _, path := range []string{
		filepath.Join(dir, "glyphrun.config.yml"),
		filepath.Join(dir, "specs", "glyphrun", "smoke.yml"),
		filepath.Join(dir, ".gitignore"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	specData, err := os.ReadFile(filepath.Join(dir, "specs", "glyphrun", "smoke.yml"))
	if err != nil {
		t.Fatal(err)
	}
	specText := string(specData)
	for _, want := range []string{
		"name: local_agent_smoke",
		`cmd: ["/bin/sh", "-lc", "./bin/local-agent"]`,
		`run: "go build -o ./bin/local-agent ./cmd/local-agent"`,
		`contains: "Welcome to LOCAL AGENT"`,
	} {
		if !strings.Contains(specText, want) {
			t.Fatalf("starter spec missing %q:\n%s", want, specText)
		}
	}
	if !strings.Contains(stdout.String(), `"specPath"`) {
		t.Fatalf("json output missing specPath: %s", stdout.String())
	}
}

func TestInitCommandIsIdempotentWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if _, err := initProject(dir, initProjectOptions{TargetCmd: "./first", ReadyText: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := initProject(dir, initProjectOptions{TargetCmd: "./second", ReadyText: "two"}); err != nil {
		t.Fatal(err)
	}
	specData, err := os.ReadFile(filepath.Join(dir, "specs", "glyphrun", "smoke.yml"))
	if err != nil {
		t.Fatal(err)
	}
	specText := string(specData)
	if strings.Contains(specText, "./second") || strings.Contains(specText, "two") {
		t.Fatalf("second init should not overwrite without force:\n%s", specText)
	}
	result, err := initProject(dir, initProjectOptions{TargetCmd: "./third", ReadyText: "three", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Updated) == 0 {
		t.Fatalf("force init should report updated files: %#v", result)
	}
	specData, err = os.ReadFile(filepath.Join(dir, "specs", "glyphrun", "smoke.yml"))
	if err != nil {
		t.Fatal(err)
	}
	specText = string(specData)
	if !strings.Contains(specText, "./third") || !strings.Contains(specText, "three") {
		t.Fatalf("force init should overwrite starter spec:\n%s", specText)
	}
}
