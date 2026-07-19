package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/runner"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/spf13/cobra"
)

type runProgressListener struct {
	w     io.Writer
	color bool
}

func makeRunProgressListener(cmd *cobra.Command, opts *globalOptions, format outputFormat, parallel int, mode string) (runner.ProgressListener, error) {
	enabled, err := progressEnabled(cmd.ErrOrStderr(), opts, format, parallel, mode)
	if err != nil || !enabled {
		return nil, err
	}
	return &runProgressListener{
		w:     cmd.ErrOrStderr(),
		color: colorEnabled(cmd.ErrOrStderr(), opts),
	}, nil
}

func progressEnabled(w io.Writer, opts *globalOptions, format outputFormat, parallel int, mode string) (bool, error) {
	if opts != nil && opts.quiet {
		return false, nil
	}
	switch strings.ToLower(os.Getenv("GLYPHRUN_PROGRESS")) {
	case "always", "1", "true", "yes", "on":
		if mode == "auto" {
			mode = "always"
		}
	case "never", "0", "false", "no", "off":
		if mode == "auto" {
			mode = "never"
		}
	}
	switch mode {
	case "never":
		return false, nil
	case "always":
		// Parallel progress is supported: lines are prefixed with the spec name.
		return true, nil
	case "auto":
		if format != formatMD {
			return false, nil
		}
		return isTerminalWriter(w), nil
	default:
		return false, fmt.Errorf("unsupported --progress %q", mode)
	}
}

func (l *runProgressListener) OnRunStart(s spec.Spec, runID string, runDir string) {
	l.line("%s %s", l.heading("Glyphrun progress"), s.Name)
	l.line("  run: %s", runID)
	l.line("  target: %s", strings.Join(s.Target.Cmd, " "))
	l.line("  artifacts: %s", runDir)
}

func (l *runProgressListener) OnPreconditionStart(index int, command spec.Command) {
	if index == 0 {
		l.line("")
		l.line("%s", l.section("Preconditions"))
	}
	l.line("  %s %s", l.dim("RUN"), truncateProgress(command.Run, 90))
}

func (l *runProgressListener) OnPreconditionFinish(index int, command spec.Command, status runner.ProgressStatus, duration time.Duration, message string) {
	l.statusLine(status, "precondition", index+1, duration, truncateProgress(command.Run, 70), message)
}

func (l *runProgressListener) OnStepStart(index int, step spec.Step) {
	if index == 0 {
		l.line("")
		l.line("%s", l.section("Steps"))
	}
	l.line("  %s step %d %s", l.dim("RUN"), index+1, stepSummary(step))
}

func (l *runProgressListener) OnStepFinish(index int, step spec.Step, status runner.ProgressStatus, duration time.Duration, message string) {
	l.statusLine(status, "step", index+1, duration, stepSummary(step), message)
}

func (l *runProgressListener) OnOutcomesStart(total int) {
	l.line("")
	l.line("%s (%d)", l.section("Outcomes"), total)
}

func (l *runProgressListener) OnOutcomeFinish(outcome spec.Outcome, result artifacts.OutcomeResult) {
	status := runner.ProgressPassed
	if result.Status != artifacts.OutcomePassed {
		status = runner.ProgressFailed
	}
	l.statusLine(status, "outcome", 0, 0, outcome.ID, result.Message)
}

func (l *runProgressListener) OnRunEnd(result artifacts.RunResult) {
	passed, failed := 0, 0
	for _, outcome := range result.Outcomes {
		if outcome.Status == artifacts.OutcomePassed {
			passed++
		} else {
			failed++
		}
	}
	l.line("")
	status := string(result.Status)
	switch result.Status {
	case artifacts.StatusPassed:
		status = l.ok("PASSED")
	case artifacts.StatusFailed:
		status = l.fail("FAILED")
	case artifacts.StatusErrored:
		status = l.warn("ERRORED")
	}
	l.line("%s %d passed, %d failed, %dms", status, passed, failed, result.DurationMS)
	if result.Artifacts["agentContext"] != "" {
		l.line("  context: %s/%s", result.RunDir, result.Artifacts["agentContext"])
	}
}

func (l *runProgressListener) statusLine(status runner.ProgressStatus, kind string, index int, duration time.Duration, label string, message string) {
	head := "OK"
	switch status {
	case runner.ProgressFailed:
		head = "FAIL"
	case runner.ProgressSkipped:
		head = "SKIP"
	}
	head = l.status(head, status)
	name := kind
	if index > 0 {
		name = fmt.Sprintf("%s %d", kind, index)
	}
	if duration > 0 {
		l.line("  %s %s %s %s", head, name, formatProgressDuration(duration), label)
	} else {
		l.line("  %s %s %s", head, name, label)
	}
	if message != "" && status != runner.ProgressPassed {
		l.line("      %s", truncateProgress(message, 160))
	}
}

func (l *runProgressListener) line(format string, args ...any) {
	fmt.Fprintf(l.w, format+"\n", args...)
}

func (l *runProgressListener) status(text string, status runner.ProgressStatus) string {
	if !l.color {
		return text
	}
	switch status {
	case runner.ProgressPassed:
		return ansiGreen + ansiBold + text + ansiReset
	case runner.ProgressFailed:
		return ansiRed + ansiBold + text + ansiReset
	case runner.ProgressSkipped:
		return ansiYellow + ansiBold + text + ansiReset
	default:
		return text
	}
}

func (l *runProgressListener) heading(text string) string {
	if !l.color {
		return text
	}
	return ansiBold + ansiCyan + text + ansiReset
}

func (l *runProgressListener) section(text string) string {
	if !l.color {
		return text
	}
	return ansiBold + ansiBlue + text + ansiReset
}

func (l *runProgressListener) dim(text string) string {
	if !l.color {
		return text
	}
	return ansiDim + text + ansiReset
}

func (l *runProgressListener) ok(text string) string {
	return l.status(text, runner.ProgressPassed)
}

func (l *runProgressListener) fail(text string) string {
	return l.status(text, runner.ProgressFailed)
}

func (l *runProgressListener) warn(text string) string {
	return l.status(text, runner.ProgressSkipped)
}

func stepSummary(step spec.Step) string {
	prefix := ""
	if step.When != nil {
		prefix = "if " + verifySummary(*step.When.AsVerify()) + " then "
	}
	if id := strings.TrimSpace(step.ID); id != "" {
		prefix = "[" + id + "] " + prefix
	}
	switch {
	case step.Press != "":
		return prefix + "press " + step.Press
	case step.Type != "":
		return prefix + "type " + strconvQuote(step.Type)
	case step.Paste != "":
		return prefix + "paste " + strconvQuote(step.Paste)
	case step.Send != nil:
		return prefix + "send bytes"
	case step.Mouse != nil:
		button := step.Mouse.Button
		if button == "" {
			button = "left"
		}
		return fmt.Sprintf("%smouse %s %d,%d", prefix, button, step.Mouse.X, step.Mouse.Y)
	case step.Wait != nil:
		return prefix + "wait " + waitSummary(*step.Wait)
	case step.Resize != nil:
		return fmt.Sprintf("%sresize %dx%d", prefix, step.Resize.Cols, step.Resize.Rows)
	case step.Snapshot != "":
		return prefix + "snapshot " + step.Snapshot
	case step.Use != "":
		return prefix + "use " + step.Use
	case step.Download != nil:
		return prefix + downloadSummary(*step.Download)
	case step.Transform != nil:
		return prefix + transformSummary(*step.Transform)
	case step.Monitor != nil:
		return prefix + monitorStepSummary(*step.Monitor)
	case len(step.Batch) > 0:
		return prefix + batchSummary(step.Batch)
	default:
		return prefix + "unknown"
	}
}

func downloadSummary(d spec.DownloadStep) string {
	assign := d.Assign
	if assign == "" {
		assign = "auto"
	}
	tag := "download -> " + assign
	if d.WaitFor {
		tag += " (wait)"
	}
	return tag
}

func transformSummary(t spec.TransformStep) string {
	assign := t.Assign
	if assign == "" {
		assign = "auto"
	}
	runtime := t.Runtime
	if runtime == "" {
		runtime = "shell"
	}
	return fmt.Sprintf("transform(%s) -> %s", runtime, assign)
}

func monitorStepSummary(m spec.MonitorStep) string {
	name := m.SaveAs
	if name == "" {
		name = "monitor"
	}
	parts := []string{"snapshot"}
	if m.Tree {
		parts = append(parts, "tree")
	}
	if p := strings.TrimSpace(m.Profile); p != "" {
		parts = append(parts, "profile:"+p)
	}
	return fmt.Sprintf("monitor(%s) -> %s", strings.Join(parts, "+"), name)
}

func batchSummary(steps []spec.Step) string {
	parts := make([]string, 0, len(steps))
	for _, sub := range steps {
		switch {
		case sub.Press != "":
			parts = append(parts, "press "+sub.Press)
		case sub.Type != "":
			parts = append(parts, "type "+strconvQuote(sub.Type))
		case sub.Paste != "":
			parts = append(parts, "paste "+strconvQuote(sub.Paste))
		case sub.Send != nil:
			parts = append(parts, "send")
		case sub.Wait != nil:
			parts = append(parts, "wait")
		}
	}
	return "batch[" + strings.Join(parts, " | ") + "]"
}

func waitSummary(wait spec.WaitStep) string {
	switch {
	case wait.Screen != nil:
		return "screen " + screenConditionSummary(*wait.Screen)
	case wait.Process != nil:
		return "process " + processConditionSummary(*wait.Process)
	case wait.Idle != nil:
		return fmt.Sprintf("idle %dms", wait.Idle.QuietForMS)
	default:
		return "condition"
	}
}

func verifySummary(verify spec.Verify) string {
	switch {
	case verify.Screen != nil:
		return "screen " + screenConditionSummary(*verify.Screen)
	case verify.Region != nil:
		return "region " + screenConditionSummary(spec.ScreenCondition{
			Contains:    verify.Region.Contains,
			NotContains: verify.Region.NotContains,
			Regex:       verify.Region.Regex,
		})
	case verify.Cell != nil:
		return fmt.Sprintf("cell %d,%d", verify.Cell.X, verify.Cell.Y)
	case verify.Cursor != nil:
		return "cursor"
	case verify.Process != nil:
		return "process " + processConditionSummary(*verify.Process)
	case verify.Snapshot != nil:
		return "snapshot " + verify.Snapshot.Name
	case verify.Command != nil:
		return "command " + truncateProgress(verify.Command.Run, 40)
	default:
		return "condition"
	}
}

func screenConditionSummary(cond spec.ScreenCondition) string {
	switch {
	case cond.Contains != "":
		return "contains " + strconvQuote(cond.Contains)
	case cond.NotContains != "":
		return "not contains " + strconvQuote(cond.NotContains)
	case cond.Regex != "":
		return "matches " + strconvQuote(cond.Regex)
	default:
		return "condition"
	}
}

func processConditionSummary(cond spec.ProcessCondition) string {
	if cond.ExitCode != nil {
		return fmt.Sprintf("exitCode=%d", *cond.ExitCode)
	}
	if cond.Exited != nil {
		return fmt.Sprintf("exited=%v", *cond.Exited)
	}
	return "condition"
}

func formatProgressDuration(duration time.Duration) string {
	if duration < time.Millisecond {
		return "<1ms"
	}
	if duration < time.Second {
		return duration.Round(time.Millisecond).String()
	}
	return duration.Round(100 * time.Millisecond).String()
}

func truncateProgress(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	if max <= 1 {
		return value[:max]
	}
	return value[:max-1] + "..."
}

func strconvQuote(value string) string {
	value = strings.ReplaceAll(value, "\n", `\n`)
	return `"` + truncateProgress(value, 60) + `"`
}
