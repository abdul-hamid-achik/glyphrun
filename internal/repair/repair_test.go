package repair

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/runner"
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

func TestVerifyRepairAcceptsValidFix(t *testing.T) {
	// End-to-end: a spec whose wait expects "Ready" but the target prints
	// "Welcome" fails; repair proposes "Welcome"; Verify reruns the temp and
	// accepts because the rerun passes.
	dir := t.TempDir()
	specPath := filepath.Join(dir, "verify.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: verify_repair
intent: verify a repair end-to-end
target:
  cmd: ["/bin/sh", "-lc", "printf 'Welcome\n'; sleep 0.3"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "Ready"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: welcome_visible
    description: welcome is visible
    verify:
      screen:
        contains: "Welcome"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Run the failing spec.
	beforeResult, err := runner.RunSpec(context.Background(), runner.Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if beforeResult.Status == artifacts.StatusPassed {
		t.Fatalf("expected the before run to fail (wait for Ready but target prints Welcome), got %s", beforeResult.Status)
	}

	// 2. Analyze the failed run.
	parseOpts := spec.ParseOptions{AllowHashMismatch: true}
	parsed, err := spec.ParseFile(specPath, parseOpts)
	if err != nil {
		t.Fatal(err)
	}
	proposals := Analyze(beforeResult.RunDir, parsed.Resolved.Steps)
	if len(proposals) == 0 {
		t.Fatalf("expected at least one repair proposal, got none; before status=%s diagnostic=%s", beforeResult.Status, beforeResult.Diagnostic)
	}

	// 3. Verify — rerun the temp with the proposed fix.
	vr, verr := Verify(context.Background(), specPath, beforeResult.RunDir, proposals, VerifyOptions{
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if verr != nil {
		t.Fatalf("Verify errored: %v", verr)
	}
	if !vr.Verified {
		t.Errorf("expected verified=true, got false; reason=%s afterRun=%s", vr.Reason, vr.AfterRun)
	}
	if vr.Confidence != "high" {
		t.Errorf("expected confidence=high, got %s", vr.Confidence)
	}
	if vr.AfterRun == "" {
		t.Error("expected a non-empty afterRun run id")
	}
	if vr.Evidence == "" {
		t.Error("expected retained evidence (after run dir)")
	}
	if vr.Replay == "" {
		t.Error("expected a replay command")
	}

	// 4. The original spec should still have "Ready" (Verify never modifies it).
	origData, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(origData), "Ready") {
		t.Errorf("Verify must not modify the original spec; it should still contain 'Ready'")
	}

	// 5. No temp file should remain.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".verify.") || strings.Contains(e.Name(), ".repair-") {
			t.Errorf("temp spec not cleaned up: %s", e.Name())
		}
	}
}
