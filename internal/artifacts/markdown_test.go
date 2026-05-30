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
