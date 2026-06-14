package ghreport

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

func TestRenderPassing(t *testing.T) {
	results := []artifacts.RunResult{{
		SpecName: "hello_quits",
		Status:   artifacts.StatusPassed,
		ExitCode: 0,
		Outcomes: []artifacts.OutcomeResult{
			{ID: "ready", Status: artifacts.OutcomePassed},
			{ID: "exit", Status: artifacts.OutcomePassed},
		},
	}}
	md := Render(results)
	for _, want := range []string{"✅ passed", "`hello_quits`", "2✓ 0✗"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in comment:\n%s", want, md)
		}
	}
	if strings.Contains(md, "### ❌") {
		t.Errorf("passing run should have no failure detail section:\n%s", md)
	}
}

func TestRenderFailing(t *testing.T) {
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
	md := Render(results)
	for _, want := range []string{"❌ failed", "### ❌ broken", "`ready`: not found", "final.svg"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in comment:\n%s", want, md)
		}
	}
}

func TestRenderEmpty(t *testing.T) {
	if md := Render(nil); !strings.Contains(md, "No runs found") {
		t.Errorf("expected empty-state message, got:\n%s", md)
	}
}
