package artifacts

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

func TestRenderRunMarkdownIncludesUsefulOperatorContext(t *testing.T) {
	result := RunResult{
		RunID:      "2026-demo",
		SpecName:   "tui_smoke",
		Status:     StatusFailed,
		DurationMS: 123,
		Target: spec.Target{
			Cmd: []string{"./bin/app", "--mode", "smoke test"},
			Cwd: ".",
		},
		Terminal: spec.Terminal{Cols: 120, Rows: 36, Profile: "xterm-256color"},
		Outcomes: []OutcomeResult{
			{ID: "ready", Status: OutcomePassed, Message: "screen contains \"ready\""},
			{ID: "quit", Status: OutcomeFailed, Message: "expected process exit code 0, got 1", Evidence: "outcomes/quit.md"},
		},
		Artifacts: map[string]string{
			"agentContext":      "agent_context.md",
			"failureDiagnostic": "diagnostics/failure.md",
			"finalScreenText":   "screens/final.txt",
		},
		RunDir:   "/tmp/glyphrun/demo",
		ExitCode: 1,
	}

	md := RenderRunMarkdown(result)
	for _, want := range []string{
		"# Glyphrun Run: failed",
		"## Summary",
		"- target: `./bin/app --mode 'smoke test'`",
		"- failed: 1",
		"## Failure Focus",
		"`quit`: expected process exit code 0, got 1",
		"diagnostics/failure.md",
		"## Key Artifacts",
		"glyph context 2026-demo --format md",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestRenderRunMarkdownIncludesContractAndMetadata(t *testing.T) {
	result := RunResult{
		RunID:        "2026-demo",
		SpecName:     "tui_smoke",
		ContractHash: "sha256:abcdef",
		CoversSymbol: "github.com/org/repo.Handler.ServeHTTP",
		Metadata: &spec.Metadata{
			Feature:  "auth",
			Owner:    "team-a",
			Priority: "high",
			Tags:     []string{"login", "critical"},
		},
		Status:     StatusPassed,
		DurationMS: 42,
		Target:     spec.Target{Cmd: []string{"./bin/app"}},
		Terminal:   spec.Terminal{Cols: 80, Rows: 24, Profile: "xterm-256color"},
		Outcomes:   []OutcomeResult{{ID: "ok", Status: OutcomePassed}},
		Artifacts:  map[string]string{},
		RunDir:     "/tmp/glyphrun/demo",
		ExitCode:   0,
	}

	md := RenderRunMarkdown(result)
	for _, want := range []string{
		"- contract: `sha256:abcdef`",
		"- covers symbol: `github.com/org/repo.Handler.ServeHTTP`",
		"- feature: `auth`",
		"- owner: `team-a`",
		"- tags: login, critical",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestRenderRunMarkdownOmitsEmptyContractAndMetadata(t *testing.T) {
	result := RunResult{
		RunID:     "2026-demo",
		SpecName:  "tui_smoke",
		Status:    StatusPassed,
		Target:    spec.Target{Cmd: []string{"./bin/app"}},
		Terminal:  spec.Terminal{Cols: 80, Rows: 24, Profile: "xterm-256color"},
		Outcomes:  []OutcomeResult{{ID: "ok", Status: OutcomePassed}},
		Artifacts: map[string]string{},
		RunDir:    "/tmp/glyphrun/demo",
		ExitCode:  0,
	}

	md := RenderRunMarkdown(result)
	for _, absent := range []string{
		"- contract:",
		"- covers symbol:",
		"- feature:",
		"- owner:",
		"- tags:",
	} {
		if strings.Contains(md, absent) {
			t.Fatalf("markdown should not contain %q:\n%s", absent, md)
		}
	}
}

func TestRenderAgentContextIncludesContractAndCoversSymbol(t *testing.T) {
	specVal := spec.Spec{
		Name:   "tui_smoke",
		Intent: "test something",
	}
	result := RunResult{
		RunID:        "2026-demo",
		SpecName:     "tui_smoke",
		ContractHash: "sha256:abcdef",
		CoversSymbol: "Handler.ServeHTTP",
		Status:       StatusPassed,
		Target:       spec.Target{Cmd: []string{"./bin/app"}},
		Terminal:     spec.Terminal{Cols: 80, Rows: 24, Profile: "xterm-256color"},
		Outcomes:     []OutcomeResult{{ID: "ok", Status: OutcomePassed}},
		Artifacts:    map[string]string{},
		RunDir:       "/tmp/glyphrun/demo",
		ExitCode:     0,
	}

	md := RenderAgentContext(specVal, result, "final screen", nil)
	for _, want := range []string{
		"- contract: `sha256:abcdef`",
		"- covers symbol: `Handler.ServeHTTP`",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("agent context missing %q:\n%s", want, md)
		}
	}
}
