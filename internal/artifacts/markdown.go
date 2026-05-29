package artifacts

import (
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

func RenderRunMarkdown(result RunResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Run %s\n\n", result.Status)
	fmt.Fprintf(&b, "- run: %s\n", result.RunID)
	fmt.Fprintf(&b, "- spec: %s\n", result.SpecName)
	fmt.Fprintf(&b, "- duration: %dms\n", result.DurationMS)
	fmt.Fprintf(&b, "- artifacts: %s\n\n", result.RunDir)
	b.WriteString("## Outcomes\n\n")
	for _, outcome := range result.Outcomes {
		mark := "PASS"
		if outcome.Status != OutcomePassed {
			mark = "FAIL"
		}
		fmt.Fprintf(&b, "- %s %s", mark, outcome.ID)
		if outcome.Message != "" {
			fmt.Fprintf(&b, ": %s", outcome.Message)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func RenderAgentContext(s spec.Spec, result RunResult, finalScreen string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Glyphrun Agent Context\n\n")
	fmt.Fprintf(&b, "- spec name: %s\n", result.SpecName)
	fmt.Fprintf(&b, "- run id: %s\n", result.RunID)
	fmt.Fprintf(&b, "- status: %s\n", result.Status)
	fmt.Fprintf(&b, "- target command: `%s`\n", strings.Join(result.Target.Cmd, " "))
	fmt.Fprintf(&b, "- terminal: %dx%d %s\n", result.Terminal.Cols, result.Terminal.Rows, result.Terminal.Profile)
	fmt.Fprintf(&b, "- run dir: %s\n\n", result.RunDir)
	b.WriteString("## Intent\n\n")
	b.WriteString(strings.TrimSpace(s.Intent))
	b.WriteString("\n\n## Outcomes\n\n")
	for _, outcome := range result.Outcomes {
		fmt.Fprintf(&b, "- %s: %s", outcome.ID, outcome.Status)
		if outcome.Message != "" {
			fmt.Fprintf(&b, " - %s", outcome.Message)
		}
		b.WriteByte('\n')
	}
	b.WriteString("\n## Final Screen\n\n```text\n")
	b.WriteString(finalScreen)
	b.WriteString("\n```\n\n")
	b.WriteString("## Artifact Paths\n\n")
	for name, path := range result.Artifacts {
		fmt.Fprintf(&b, "- %s: %s\n", name, path)
	}
	b.WriteString("\n## Suggested Commands\n\n")
	fmt.Fprintf(&b, "- `glyph context %s --format md`\n", result.RunID)
	fmt.Fprintf(&b, "- `glyph run <spec> --format json`\n")
	return b.String()
}
