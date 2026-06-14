package cli

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

func TestRenderPRCommentPassing(t *testing.T) {
	results := []artifacts.RunResult{{
		SpecName: "hello_quits",
		Status:   artifacts.StatusPassed,
		ExitCode: 0,
		Outcomes: []artifacts.OutcomeResult{
			{ID: "ready", Status: artifacts.OutcomePassed},
			{ID: "exit", Status: artifacts.OutcomePassed},
		},
	}}
	md := renderPRComment(results)
	for _, want := range []string{"✅ passed", "`hello_quits`", "2✓ 0✗"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in comment:\n%s", want, md)
		}
	}
	if strings.Contains(md, "### ❌") {
		t.Errorf("passing run should have no failure detail section:\n%s", md)
	}
}

func TestRenderPRCommentFailing(t *testing.T) {
	results := []artifacts.RunResult{{
		SpecName: "broken",
		Status:   artifacts.StatusFailed,
		ExitCode: 1,
		Outcomes: []artifacts.OutcomeResult{
			{ID: "ready", Status: artifacts.OutcomeFailed, Message: "not found"},
		},
		Artifacts: map[string]string{"finalScreenSVG": "screens/final.svg"},
		RunDir:    "/tmp/runs/run-broken",
	}}
	md := renderPRComment(results)
	for _, want := range []string{"❌ failed", "### ❌ broken", "`ready`: not found", "final.svg"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in comment:\n%s", want, md)
		}
	}
}

func TestRenderPRCommentEmpty(t *testing.T) {
	md := renderPRComment(nil)
	if !strings.Contains(md, "No runs found") {
		t.Errorf("expected empty-state message, got:\n%s", md)
	}
}
