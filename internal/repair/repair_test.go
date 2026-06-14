package repair

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

func TestLongestCommonSubstring(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"hello world", "hello there", 6}, // "hello "
		{"abc", "xyz", 0},
		{"", "abc", 0},
		{"ready", "the app is ready now", 5},
	}
	for _, tc := range tests {
		if got := longestCommonSubstring(tc.a, tc.b); got != tc.want {
			t.Errorf("longestCommonSubstring(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestStepIndexFromName(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"step.3", 3},
		{"step.12", 12},
		{"nope", 0},
		{"step.x", 0},
	}
	for _, tc := range tests {
		if got := stepIndexFromName(tc.name); got != tc.want {
			t.Errorf("stepIndexFromName(%q) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestPropose(t *testing.T) {
	zero := 0
	steps := []spec.Step{
		{Wait: &spec.WaitStep{Screen: &spec.ScreenCondition{Contains: "OLD GREETING"}}},
		{Press: "q"},
		{Wait: &spec.WaitStep{Process: &spec.ProcessCondition{ExitCode: &zero}}},
	}
	screen := "Welcome to the new greeting\npress q to quit"

	t.Run("stale screen string", func(t *testing.T) {
		ps := propose(steps, []stepFailure{{index: 1, message: "timed out"}}, screen)
		if len(ps) != 1 || ps[0].Current != "OLD GREETING" || ps[0].Proposed == "" {
			t.Fatalf("expected a contains rewrite, got %+v", ps)
		}
	})

	t.Run("present string is timing not stale", func(t *testing.T) {
		ps := propose(steps, []stepFailure{{index: 1, message: "timed out"}}, "OLD GREETING is here")
		if len(ps) != 1 || ps[0].Proposed != "" {
			t.Fatalf("expected a no-rewrite timing proposal, got %+v", ps)
		}
	})

	t.Run("process wait hints missing interaction", func(t *testing.T) {
		ps := propose(steps, []stepFailure{{index: 3, message: "timed out"}}, screen)
		if len(ps) != 1 || ps[0].Kind != "wait.process" {
			t.Fatalf("expected a wait.process hint, got %+v", ps)
		}
	})
}

func TestEditStepWaitContains(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.yml")
	content := `version: 1
name: demo
intent: a thing
target:
  cmd: ["./app"]
terminal:
  cols: 80
  rows: 24
steps:
  - wait:
      screen:
        contains: OLD
  - press: q
outcomes:
  - id: visible
    description: shows OLD
    verify:
      screen:
        contains: OLD
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := editStepWaitContains(path, 1, "NEW"); err != nil {
		t.Fatalf("editStepWaitContains: %v", err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	// The step's contains must become NEW; the outcome's contains must remain
	// OLD (the contract is never touched).
	if !strings.Contains(text, "contains: NEW") {
		t.Errorf("expected step contains rewritten to NEW:\n%s", text)
	}
	if !strings.Contains(text, "contains: OLD") {
		t.Errorf("expected the outcome contains to remain OLD (contract untouched):\n%s", text)
	}
}
