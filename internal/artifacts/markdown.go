package artifacts

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

func RenderRunMarkdown(result RunResult) string {
	var b strings.Builder
	passed, failed := outcomeCounts(result.Outcomes)
	fmt.Fprintf(&b, "# Glyphrun Run: %s\n\n", result.Status)
	b.WriteString("## Summary\n\n")
	fmt.Fprintf(&b, "- status: %s\n", result.Status)
	if result.ErrorKind != "" {
		fmt.Fprintf(&b, "- errorKind: `%s`\n", result.ErrorKind)
	}
	if result.Diagnostic != "" {
		fmt.Fprintf(&b, "- diagnostic: %s\n", result.Diagnostic)
	}
	if result.TargetExit != nil {
		fmt.Fprintf(&b, "- target exit code: %d\n", result.TargetExit.ExitCode)
		if result.TargetExit.LastPtyLine != "" {
			fmt.Fprintf(&b, "- lastPtyLine: %q\n", result.TargetExit.LastPtyLine)
		}
	}
	if len(result.NextActions) > 0 {
		b.WriteString("\n## Next actions\n\n")
		for _, na := range result.NextActions {
			if na.Command != "" {
				fmt.Fprintf(&b, "- `%s` — %s\n", na.Command, na.Reason)
			} else {
				fmt.Fprintf(&b, "- %s\n", na.Reason)
			}
		}
	}
	fmt.Fprintf(&b, "- spec: `%s`\n", result.SpecName)
	if strings.TrimSpace(result.Intent) != "" {
		fmt.Fprintf(&b, "- intent: %s\n", strings.TrimSpace(result.Intent))
	}
	if result.ContractHash != "" {
		fmt.Fprintf(&b, "- contract: `%s`\n", result.ContractHash)
	}
	if result.CoversSymbol != "" {
		fmt.Fprintf(&b, "- covers symbol: `%s`\n", result.CoversSymbol)
	}
	if result.Metadata != nil {
		if result.Metadata.Feature != "" {
			fmt.Fprintf(&b, "- feature: `%s`\n", result.Metadata.Feature)
		}
		if result.Metadata.Owner != "" {
			fmt.Fprintf(&b, "- owner: `%s`\n", result.Metadata.Owner)
		}
		if len(result.Metadata.Tags) > 0 {
			fmt.Fprintf(&b, "- tags: %s\n", strings.Join(result.Metadata.Tags, ", "))
		}
	}
	fmt.Fprintf(&b, "- run: `%s`\n", result.RunID)
	fmt.Fprintf(&b, "- target: `%s`\n", shellJoin(result.Target.Cmd))
	if result.Target.Cwd != "" {
		fmt.Fprintf(&b, "- cwd: `%s`\n", result.Target.Cwd)
	}
	if len(result.Target.Env) > 0 {
		fmt.Fprintf(&b, "- env overrides: %d\n", len(result.Target.Env))
	}
	fmt.Fprintf(&b, "- exit code: %d\n", result.ExitCode)
	fmt.Fprintf(&b, "- duration: %dms\n", result.DurationMS)
	fmt.Fprintf(&b, "- terminal: %dx%d %s\n", result.Terminal.Cols, result.Terminal.Rows, result.Terminal.Profile)
	fmt.Fprintf(&b, "- artifacts: %s\n", result.RunDir)
	if result.StartedAt != "" {
		fmt.Fprintf(&b, "- started: %s\n", result.StartedAt)
	}
	if result.EndedAt != "" {
		fmt.Fprintf(&b, "- ended: %s\n", result.EndedAt)
	}
	fmt.Fprintf(&b, "\n## Outcome Summary\n\n")
	fmt.Fprintf(&b, "- passed: %d\n", passed)
	fmt.Fprintf(&b, "- failed: %d\n", failed)
	fmt.Fprintf(&b, "- total: %d\n\n", len(result.Outcomes))

	if failed > 0 {
		b.WriteString("## Failure Focus\n\n")
		for _, outcome := range result.Outcomes {
			if outcome.Status != OutcomeFailed {
				continue
			}
			fmt.Fprintf(&b, "- `%s`", outcome.ID)
			if outcome.Message != "" {
				fmt.Fprintf(&b, ": %s", outcome.Message)
			}
			if outcome.Evidence != "" {
				fmt.Fprintf(&b, " (`%s`)", outcome.Evidence)
			}
			b.WriteByte('\n')
		}
		if result.Artifacts["failureDiagnostic"] != "" {
			fmt.Fprintf(&b, "- diagnostic: `%s`\n", result.Artifacts["failureDiagnostic"])
		}
		b.WriteByte('\n')
	}

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
	b.WriteString("\n## Key Artifacts\n\n")
	for _, key := range orderedArtifactKeys(result.Artifacts) {
		fmt.Fprintf(&b, "- %s: `%s`\n", artifactLabel(key), result.Artifacts[key])
	}
	b.WriteString("\n## Next Commands\n\n")
	fmt.Fprintf(&b, "- `glyph context %s --format md`\n", result.RunID)
	if result.Artifacts["finalScreenText"] != "" {
		fmt.Fprintf(&b, "- `sed -n '1,120p' %s/%s`\n", result.RunDir, result.Artifacts["finalScreenText"])
	}
	fmt.Fprintf(&b, "- `glyph run <spec> --format json`\n")
	return b.String()
}

func RenderAgentContext(s spec.Spec, result RunResult, finalScreen string, recentEvents []Event) string {
	var b strings.Builder
	passed, failed := outcomeCounts(result.Outcomes)
	fmt.Fprintf(&b, "# Glyphrun Agent Context\n\n")
	b.WriteString("## Run\n\n")
	fmt.Fprintf(&b, "- spec: `%s`\n", result.SpecName)
	if result.ContractHash != "" {
		fmt.Fprintf(&b, "- contract: `%s`\n", result.ContractHash)
	}
	if result.CoversSymbol != "" {
		fmt.Fprintf(&b, "- covers symbol: `%s`\n", result.CoversSymbol)
	}
	fmt.Fprintf(&b, "- run: `%s`\n", result.RunID)
	fmt.Fprintf(&b, "- status: %s\n", result.Status)
	if result.ErrorKind != "" {
		fmt.Fprintf(&b, "- errorKind: `%s`\n", result.ErrorKind)
	}
	if result.Diagnostic != "" {
		fmt.Fprintf(&b, "- diagnostic: %s\n", result.Diagnostic)
	}
	if result.TargetExit != nil {
		fmt.Fprintf(&b, "- target exit code: %d\n", result.TargetExit.ExitCode)
		if result.TargetExit.LastPtyLine != "" {
			fmt.Fprintf(&b, "- lastPtyLine: %q\n", result.TargetExit.LastPtyLine)
		}
	}
	fmt.Fprintf(&b, "- target: `%s`\n", shellJoin(result.Target.Cmd))
	fmt.Fprintf(&b, "- command run: `glyph run <spec> --format json`\n")
	if result.Target.Cwd != "" {
		fmt.Fprintf(&b, "- target cwd: `%s`\n", result.Target.Cwd)
	}
	fmt.Fprintf(&b, "- terminal: %dx%d %s\n", result.Terminal.Cols, result.Terminal.Rows, result.Terminal.Profile)
	fmt.Fprintf(&b, "- duration: %dms\n", result.DurationMS)
	fmt.Fprintf(&b, "- exit code: %d\n", result.ExitCode)
	fmt.Fprintf(&b, "- outcomes: %d passed, %d failed\n", passed, failed)
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
	if failed > 0 {
		b.WriteString("\n## Failure Focus\n\n")
		for _, outcome := range result.Outcomes {
			if outcome.Status != OutcomeFailed {
				continue
			}
			fmt.Fprintf(&b, "- `%s`", outcome.ID)
			if outcome.Message != "" {
				fmt.Fprintf(&b, ": %s", outcome.Message)
			}
			if outcome.Evidence != "" {
				fmt.Fprintf(&b, " (`%s`)", outcome.Evidence)
			}
			b.WriteByte('\n')
		}
		if result.Artifacts["failureDiagnostic"] != "" {
			fmt.Fprintf(&b, "- diagnostic: `%s`\n", result.Artifacts["failureDiagnostic"])
		}
		if result.Artifacts["finalScreenText"] != "" {
			fmt.Fprintf(&b, "- final screen: `%s`\n", result.Artifacts["finalScreenText"])
		}
		if result.Artifacts["frames"] != "" {
			fmt.Fprintf(&b, "- frame timeline: `%s`\n", result.Artifacts["frames"])
		}
		if result.Artifacts["rawPtyLog"] != "" {
			fmt.Fprintf(&b, "- raw PTY log: `%s`\n", result.Artifacts["rawPtyLog"])
		}
	}
	if len(recentEvents) > 0 {
		b.WriteString("\n## Recent Events\n\n")
		for _, event := range recentEvents {
			fmt.Fprintf(&b, "- %s %s", event.TS, event.Type)
			if event.Name != "" {
				fmt.Fprintf(&b, " `%s`", event.Name)
			}
			if event.Info != "" {
				fmt.Fprintf(&b, ": %s", event.Info)
			}
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n## Final Screen\n\n```text\n")
	b.WriteString(finalScreen)
	b.WriteString("\n```\n\n")
	b.WriteString("## Artifact Paths\n\n")
	for _, key := range orderedArtifactKeys(result.Artifacts) {
		fmt.Fprintf(&b, "- %s: %s\n", key, result.Artifacts[key])
	}
	if len(result.NamedArtifacts) > 0 {
		b.WriteString("\n## Named Artifacts\n\n")
		b.WriteString("Reference these from later steps and outcome `command:` verifiers via `${artifacts.<name>.path}` or `${artifacts.<name>.relativePath}`.\n\n")
		names := make([]string, 0, len(result.NamedArtifacts))
		for name := range result.NamedArtifacts {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			art := result.NamedArtifacts[name]
			fmt.Fprintf(&b, "- `%s` (%s): %s\n", name, art.Kind, art.RelativePath)
		}
	}
	b.WriteString("\n## Suggested Commands\n\n")
	fmt.Fprintf(&b, "- `glyph context %s --format md`\n", result.RunID)
	if result.Artifacts["failureDiagnostic"] != "" {
		fmt.Fprintf(&b, "- `sed -n '1,180p' %s/%s`\n", result.RunDir, result.Artifacts["failureDiagnostic"])
	}
	if result.Artifacts["finalScreenText"] != "" {
		fmt.Fprintf(&b, "- `sed -n '1,160p' %s/%s`\n", result.RunDir, result.Artifacts["finalScreenText"])
	}
	fmt.Fprintf(&b, "- `tail -n 20 %s/%s`\n", result.RunDir, result.Artifacts["events"])
	fmt.Fprintf(&b, "- `glyph run <spec> --format json`\n")
	return b.String()
}

func outcomeCounts(outcomes []OutcomeResult) (int, int) {
	var passed, failed int
	for _, outcome := range outcomes {
		if outcome.Status == OutcomePassed {
			passed++
		} else {
			failed++
		}
	}
	return passed, failed
}

func orderedArtifactKeys(artifacts map[string]string) []string {
	if len(artifacts) == 0 {
		return nil
	}
	preferred := []string{
		"agentContext",
		"failureDiagnostic",
		"environmentDiagnostic",
		"finalScreenText",
		"finalScreenJSON",
		"finalScreenSVG",
		"events",
		"frames",
		"rawPtyLog",
		"inputRawLog",
	}
	seen := map[string]bool{}
	var keys []string
	for _, key := range preferred {
		if artifacts[key] != "" {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	var rest []string
	for key := range artifacts {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

func artifactLabel(key string) string {
	if strings.HasPrefix(key, "snapshot:") {
		return "snapshot " + strings.TrimPrefix(key, "snapshot:")
	}
	switch key {
	case "agentContext":
		return "agent context"
	case "failureDiagnostic":
		return "failure diagnostic"
	case "environmentDiagnostic":
		return "environment diagnostic"
	case "finalScreenText":
		return "final screen text"
	case "finalScreenJSON":
		return "final screen JSON"
	case "finalScreenSVG":
		return "final screen SVG"
	case "rawPtyLog":
		return "raw PTY log"
	case "inputRawLog":
		return "input log"
	default:
		return key
	}
}

func shellJoin(args []string) string {
	if len(args) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	for _, r := range arg {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			continue
		case strings.ContainsRune("@%_+=:,./-", r):
			continue
		default:
			return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
		}
	}
	return arg
}
