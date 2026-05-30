package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/input"
	"github.com/abdul-hamid-achik/glyphrun/internal/ptyrunner"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal/adapters/gote"
)

const (
	DefaultTimeout      = 5 * time.Second
	DefaultPollInterval = 25 * time.Millisecond
	CleanupTimeout      = 2 * time.Second
)

type Options struct {
	SpecPath        string
	ConfigPath      string
	Environment     string
	ArtifactRoot    string
	UpdateSnapshots bool
	Listener        ProgressListener
}

type ProgressStatus string

const (
	ProgressPassed  ProgressStatus = "passed"
	ProgressFailed  ProgressStatus = "failed"
	ProgressSkipped ProgressStatus = "skipped"
)

type ProgressListener interface {
	OnRunStart(s spec.Spec, runID string, runDir string)
	OnPreconditionStart(index int, command spec.Command)
	OnPreconditionFinish(index int, command spec.Command, status ProgressStatus, duration time.Duration, message string)
	OnStepStart(index int, step spec.Step)
	OnStepFinish(index int, step spec.Step, status ProgressStatus, duration time.Duration, message string)
	OnOutcomesStart(total int)
	OnOutcomeFinish(outcome spec.Outcome, result artifacts.OutcomeResult)
	OnRunEnd(result artifacts.RunResult)
}

func RunSpec(ctx context.Context, opts Options) (artifacts.RunResult, error) {
	runtime, err := config.LoadRuntime(opts.SpecPath, opts.ConfigPath, opts.Environment)
	if err != nil {
		return artifacts.RunResult{}, err
	}
	parse, err := spec.ParseFile(opts.SpecPath, runtime.SpecParseOptions())
	if err != nil {
		return artifacts.RunResult{}, err
	}
	resolved := parse.Resolved
	started := time.Now().UTC()
	runID := makeRunID(started, resolved.Name)
	artifactRoot := opts.ArtifactRoot
	if artifactRoot == "" {
		artifactRoot = runtime.Config.ArtifactRoot
	}
	if !filepath.IsAbs(artifactRoot) {
		artifactRoot = filepath.Join(runtime.ProjectRoot, artifactRoot)
	}
	runDir := filepath.Join(artifactRoot, runID)
	writer := artifacts.NewWriter(runDir, artifacts.NewRedactor(runtime.Config.Redaction))
	if err := writer.EnsureDirs(); err != nil {
		return artifacts.RunResult{}, err
	}
	_ = writer.AppendEvent(event("run.started", resolved.Name, ""))
	_ = writer.WriteResolvedSpec(resolved)

	state := newRunState(resolved, runtime, writer, opts.UpdateSnapshots, opts.Listener)
	if opts.Listener != nil {
		opts.Listener.OnRunStart(resolved, runID, runDir)
	}
	if err := state.runPreconditions(ctx); err != nil {
		return state.finish(started, artifacts.StatusErrored, nil, fmt.Sprintf("precondition failed: %v", err)), nil
	}

	if err := state.startTarget(); err != nil {
		return state.finish(started, artifacts.StatusErrored, nil, fmt.Sprintf("target failed to start: %v", err)), nil
	}
	defer state.cleanup()

	for idx, step := range resolved.Steps {
		name := fmt.Sprintf("step.%d", idx+1)
		_ = writer.AppendEvent(event("step.started", name, describeStep(step)))
		stepStart := time.Now()
		if state.listener != nil {
			state.listener.OnStepStart(idx, step)
		}
		result, err := state.executeStep(ctx, step)
		if err != nil {
			_ = writer.AppendEvent(event("step.failed", name, err.Error()))
			if state.listener != nil {
				state.listener.OnStepFinish(idx, step, ProgressFailed, time.Since(stepStart), err.Error())
			}
			result := state.evaluateOutcomes(ctx)
			return state.finish(started, artifacts.StatusFailed, result, fmt.Sprintf("step %d failed: %v", idx+1, err)), nil
		}
		if result.Skipped {
			_ = writer.AppendEvent(event("step.skipped", name, result.Message))
			if state.listener != nil {
				state.listener.OnStepFinish(idx, step, ProgressSkipped, time.Since(stepStart), result.Message)
			}
			continue
		}
		if state.listener != nil {
			state.listener.OnStepFinish(idx, step, ProgressPassed, time.Since(stepStart), "")
		}
		_ = writer.AppendEvent(event("step.finished", name, ""))
	}

	outcomes := state.evaluateOutcomes(ctx)
	status := artifacts.StatusPassed
	for _, outcome := range outcomes {
		if outcome.Status != artifacts.OutcomePassed {
			status = artifacts.StatusFailed
			break
		}
	}
	return state.finish(started, status, outcomes, ""), nil
}

type runState struct {
	spec            spec.Spec
	runtime         config.Runtime
	writer          *artifacts.Writer
	emulator        terminal.Emulator
	session         *ptyrunner.Session
	updateSnapshots bool
	listener        ProgressListener

	mu        sync.Mutex
	rawPTY    []byte
	inputLog  []byte
	frames    []terminal.Frame
	snapshots map[string]terminal.ScreenSnapshot
}

func newRunState(s spec.Spec, rt config.Runtime, writer *artifacts.Writer, updateSnapshots bool, listener ProgressListener) *runState {
	return &runState{
		spec:            s,
		runtime:         rt,
		writer:          writer,
		emulator:        gote.New(s.Terminal.Cols, s.Terminal.Rows),
		updateSnapshots: updateSnapshots,
		listener:        listener,
		snapshots:       map[string]terminal.ScreenSnapshot{},
	}
}

func (s *runState) runPreconditions(ctx context.Context) error {
	for idx, command := range s.spec.Preconditions.Commands {
		start := time.Now()
		if s.listener != nil {
			s.listener.OnPreconditionStart(idx, command)
		}
		if err := s.runShellCommand(ctx, command.Run, command.Cwd, command.TimeoutMS); err != nil {
			if s.listener != nil {
				s.listener.OnPreconditionFinish(idx, command, ProgressFailed, time.Since(start), err.Error())
			}
			return err
		}
		if s.listener != nil {
			s.listener.OnPreconditionFinish(idx, command, ProgressPassed, time.Since(start), "")
		}
	}
	return nil
}

func (s *runState) startTarget() error {
	env := map[string]string{}
	for k, v := range s.runtime.Env {
		env[k] = v
	}
	env["TERM"] = s.spec.Terminal.Profile
	env["GLYPHRUN"] = "1"
	for k, v := range s.spec.Target.Env {
		env[k] = v
	}
	cwd := resolveProjectPath(s.runtime.ProjectRoot, s.spec.Target.Cwd)
	session, err := ptyrunner.Start(ptyrunner.Options{
		Cmd:  s.spec.Target.Cmd,
		Cwd:  cwd,
		Env:  env,
		Cols: s.spec.Terminal.Cols,
		Rows: s.spec.Terminal.Rows,
		OnOutput: func(data []byte) {
			s.mu.Lock()
			max := s.runtime.Config.Artifacts.MaxRawLogBytes
			if max <= 0 || int64(len(s.rawPTY)+len(data)) <= max {
				s.rawPTY = append(s.rawPTY, data...)
			}
			s.mu.Unlock()
			frames, _ := s.emulator.Feed(data)
			s.mu.Lock()
			s.frames = append(s.frames, frames...)
			s.mu.Unlock()
		},
	})
	if err != nil {
		return err
	}
	s.session = session
	return nil
}

type stepExecutionResult struct {
	Skipped bool
	Message string
}

func (s *runState) executeStep(ctx context.Context, step spec.Step) (stepExecutionResult, error) {
	if step.When != nil {
		ok, message := s.checkVerify(ctx, *step.When)
		if !ok {
			return stepExecutionResult{Skipped: true, Message: message}, nil
		}
	}
	switch {
	case step.Press != "":
		data, err := input.KeyBytes(step.Press)
		if err != nil {
			return stepExecutionResult{}, err
		}
		return stepExecutionResult{}, s.writeInput(data)
	case step.Type != "":
		return stepExecutionResult{}, s.writeInput([]byte(step.Type))
	case step.Paste != "":
		return stepExecutionResult{}, s.writeInput([]byte(step.Paste))
	case step.Send != nil:
		data, err := decodeEscaped(step.Send.Bytes)
		if err != nil {
			return stepExecutionResult{}, err
		}
		return stepExecutionResult{}, s.writeInput(data)
	case step.Wait != nil:
		return stepExecutionResult{}, s.waitFor(ctx, *step.Wait)
	case step.Resize != nil:
		if err := s.session.Resize(step.Resize.Cols, step.Resize.Rows); err != nil {
			return stepExecutionResult{}, err
		}
		return stepExecutionResult{}, s.emulator.Resize(step.Resize.Cols, step.Resize.Rows)
	case step.Snapshot != "":
		return stepExecutionResult{}, s.captureSnapshot(step.Snapshot)
	default:
		return stepExecutionResult{}, fmt.Errorf("unsupported step")
	}
}

func (s *runState) captureSnapshot(name string) error {
	snapshot := s.normalizeSnapshot(s.emulator.Screen().Snapshot())
	s.mu.Lock()
	s.snapshots[name] = snapshot
	s.mu.Unlock()
	if s.runtime.Config.Artifacts.Snapshots {
		if err := s.writer.WriteSnapshot(name, snapshot); err != nil {
			return err
		}
	}
	if s.updateSnapshots {
		return s.writeCommittedSnapshot(name, snapshot)
	}
	return nil
}

func (s *runState) writeInput(data []byte) error {
	if s.session == nil {
		return fmt.Errorf("target session has not started")
	}
	s.mu.Lock()
	s.inputLog = append(s.inputLog, data...)
	s.mu.Unlock()
	return s.session.Write(data)
}

func (s *runState) waitFor(ctx context.Context, wait spec.WaitStep) error {
	timeout := timeoutFromMS(wait.TimeoutMS)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(DefaultPollInterval)
	defer tick.Stop()
	for {
		ok, message := s.checkWait(wait)
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out after %s: %s", timeout, message)
		case <-tick.C:
		}
	}
}

func (s *runState) checkWait(wait spec.WaitStep) (bool, string) {
	if wait.Screen != nil {
		return checkScreen(s.screenText(), *wait.Screen)
	}
	if wait.Process != nil {
		return checkProcess(s.session.ExitState(), *wait.Process)
	}
	if wait.Idle != nil {
		quiet := time.Duration(wait.Idle.QuietForMS) * time.Millisecond
		return time.Since(s.session.LastOutputAt()) >= quiet, "waiting for terminal output to become idle"
	}
	return false, "wait has no condition"
}

func (s *runState) evaluateOutcomes(ctx context.Context) []artifacts.OutcomeResult {
	if s.listener != nil {
		s.listener.OnOutcomesStart(len(s.spec.Outcomes))
	}
	results := make([]artifacts.OutcomeResult, 0, len(s.spec.Outcomes))
	for _, outcome := range s.spec.Outcomes {
		result := s.evaluateOutcome(ctx, outcome)
		results = append(results, result)
		if s.listener != nil {
			s.listener.OnOutcomeFinish(outcome, result)
		}
		_ = s.writer.WriteOutcome(result, map[string]any{
			"id":     outcome.ID,
			"verify": outcome.Verify,
		})
		if result.Status == artifacts.OutcomePassed {
			_ = s.writer.AppendEvent(event("outcome.passed", outcome.ID, result.Message))
		} else {
			_ = s.writer.AppendEvent(event("outcome.failed", outcome.ID, result.Message))
		}
	}
	return results
}

func (s *runState) evaluateOutcome(ctx context.Context, outcome spec.Outcome) artifacts.OutcomeResult {
	timeout := DefaultTimeout
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(DefaultPollInterval)
	defer tick.Stop()
	var last string
	for {
		ok, message := s.checkVerify(ctx, outcome.Verify)
		if ok {
			return artifacts.OutcomeResult{ID: outcome.ID, Status: artifacts.OutcomePassed, Message: message, Evidence: "outcomes/" + outcome.ID + ".md"}
		}
		last = message
		select {
		case <-ctx.Done():
			return artifacts.OutcomeResult{ID: outcome.ID, Status: artifacts.OutcomeFailed, Message: ctx.Err().Error(), Evidence: "outcomes/" + outcome.ID + ".md"}
		case <-deadline.C:
			return artifacts.OutcomeResult{ID: outcome.ID, Status: artifacts.OutcomeFailed, Message: last, Evidence: "outcomes/" + outcome.ID + ".md"}
		case <-tick.C:
		}
	}
}

func (s *runState) checkVerify(ctx context.Context, verify spec.Verify) (bool, string) {
	screen := s.emulator.Screen()
	switch {
	case verify.Screen != nil:
		return checkScreen(s.screenText(), *verify.Screen)
	case verify.Region != nil:
		cond := verify.Region
		return checkScreen(normalizeText(screen.Region(cond.X, cond.Y, cond.Width, cond.Height).Text(), s.normalizeConfig()), spec.ScreenCondition{
			Contains:    cond.Contains,
			NotContains: cond.NotContains,
			Regex:       cond.Regex,
		})
	case verify.Cell != nil:
		cell := screen.Cell(verify.Cell.X, verify.Cell.Y)
		if verify.Cell.Char != "" && cell.Char != verify.Cell.Char {
			return false, fmt.Sprintf("expected cell %d,%d to be %q, got %q", verify.Cell.X, verify.Cell.Y, verify.Cell.Char, cell.Char)
		}
		if verify.Cell.Style != nil {
			if ok, message := checkStyle(cell.Style, *verify.Cell.Style); !ok {
				return false, fmt.Sprintf("cell %d,%d style mismatch: %s", verify.Cell.X, verify.Cell.Y, message)
			}
		}
		return true, "cell matched"
	case verify.Cursor != nil:
		cursor := screen.Cursor()
		if cursor.X != verify.Cursor.X || cursor.Y != verify.Cursor.Y {
			return false, fmt.Sprintf("expected cursor %d,%d, got %d,%d", verify.Cursor.X, verify.Cursor.Y, cursor.X, cursor.Y)
		}
		if verify.Cursor.Visible != nil && cursor.Visible != *verify.Cursor.Visible {
			return false, fmt.Sprintf("expected cursor visible=%v, got %v", *verify.Cursor.Visible, cursor.Visible)
		}
		return true, "cursor matched"
	case verify.Process != nil:
		return checkProcess(s.session.ExitState(), *verify.Process)
	case verify.Snapshot != nil:
		return s.checkSnapshot(*verify.Snapshot)
	case verify.Command != nil:
		err := s.runShellCommand(ctx, verify.Command.Run, verify.Command.Cwd, verify.Command.TimeoutMS)
		if err != nil {
			return false, err.Error()
		}
		return true, "command verifier passed"
	default:
		return false, "unsupported verifier"
	}
}

func (s *runState) checkSnapshot(cond spec.SnapshotCondition) (bool, string) {
	s.mu.Lock()
	current, ok := s.snapshots[cond.Name]
	s.mu.Unlock()
	if !ok {
		return false, fmt.Sprintf("snapshot %q was not captured in this run", cond.Name)
	}
	committedPath := s.committedSnapshotTextPath(cond.Name)
	if _, err := os.Stat(committedPath); err != nil {
		if s.updateSnapshots {
			if err := s.writeCommittedSnapshot(cond.Name, current); err != nil {
				return false, fmt.Sprintf("failed to update snapshot %q: %v", cond.Name, err)
			}
			return true, fmt.Sprintf("snapshot %q created", cond.Name)
		}
		return false, fmt.Sprintf("committed snapshot %q not found at %s", cond.Name, committedPath)
	}
	expected, err := os.ReadFile(committedPath)
	if err != nil {
		return false, fmt.Sprintf("failed to read committed snapshot %q: %v", cond.Name, err)
	}
	expectedText := strings.TrimRight(string(expected), "\n")
	actualText := strings.TrimRight(current.Text, "\n")
	if expectedText != actualText {
		if s.updateSnapshots {
			if err := s.writeCommittedSnapshot(cond.Name, current); err != nil {
				return false, fmt.Sprintf("failed to update snapshot %q: %v", cond.Name, err)
			}
			return true, fmt.Sprintf("snapshot %q updated", cond.Name)
		}
		return false, fmt.Sprintf("snapshot %q mismatch\nexpected:\n%s\nactual:\n%s", cond.Name, expectedText, actualText)
	}
	return true, fmt.Sprintf("snapshot %q matched", cond.Name)
}

func (s *runState) screenText() string {
	return s.normalizeSnapshot(s.emulator.Screen().Snapshot()).Text
}

func (s *runState) normalizeSnapshot(snapshot terminal.ScreenSnapshot) terminal.ScreenSnapshot {
	snapshot.Text = normalizeSnapshotText(snapshot, s.normalizeConfig())
	return snapshot
}

func (s *runState) normalizeConfig() config.Normalize {
	out := s.runtime.Config.Terminal.Normalize
	if s.spec.Normalize == nil {
		return out
	}
	if s.spec.Normalize.TrimRight != nil {
		out.TrimRight = *s.spec.Normalize.TrimRight
	}
	if s.spec.Normalize.NormalizeLineEndings != nil {
		out.NormalizeLineEndings = *s.spec.Normalize.NormalizeLineEndings
	}
	if s.spec.Normalize.StripAnsiTitle != nil {
		out.StripAnsiTitle = *s.spec.Normalize.StripAnsiTitle
	}
	out.Replace = append(out.Replace, s.spec.Normalize.Replace...)
	out.IgnoreRegions = append(out.IgnoreRegions, s.spec.Normalize.IgnoreRegions...)
	return out
}

func normalizeSnapshotText(snapshot terminal.ScreenSnapshot, normalize config.Normalize) string {
	if len(normalize.IgnoreRegions) == 0 {
		return normalizeText(snapshot.Text, normalize)
	}
	lines := make([][]rune, snapshot.Rows)
	for y := 0; y < snapshot.Rows; y++ {
		lines[y] = make([]rune, snapshot.Cols)
		for x := 0; x < snapshot.Cols; x++ {
			ch := ' '
			idx := y*snapshot.Cols + x
			if idx >= 0 && idx < len(snapshot.Cells) && snapshot.Cells[idx].Char != "" {
				cell := snapshot.Cells[idx]
				runes := []rune(cell.Char)
				if len(runes) > 0 {
					ch = runes[0]
				}
			}
			lines[y][x] = ch
		}
	}
	for _, region := range normalize.IgnoreRegions {
		for y := region.Y; y < region.Y+region.Height && y < len(lines); y++ {
			if y < 0 {
				continue
			}
			for x := region.X; x < region.X+region.Width && x < len(lines[y]); x++ {
				if x >= 0 {
					lines[y][x] = ' '
				}
			}
		}
	}
	var b strings.Builder
	for y, line := range lines {
		if y > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(string(line))
	}
	return normalizeText(b.String(), normalize)
}

func normalizeText(text string, normalize config.Normalize) string {
	out := text
	if normalize.NormalizeLineEndings {
		out = strings.ReplaceAll(out, "\r\n", "\n")
		out = strings.ReplaceAll(out, "\r", "\n")
	}
	if normalize.StripAnsiTitle {
		out = regexp.MustCompile(`\x1b\][^\a]*(\a|\x1b\\)`).ReplaceAllString(out, "")
	}
	for _, replacement := range normalize.Replace {
		re, err := regexp.Compile(replacement.Regex)
		if err == nil {
			out = re.ReplaceAllString(out, replacement.With)
		}
	}
	if normalize.TrimRight {
		lines := strings.Split(out, "\n")
		for i := range lines {
			lines[i] = strings.TrimRight(lines[i], " \t")
		}
		out = strings.Join(lines, "\n")
	}
	return strings.TrimRight(out, "\n")
}

func (s *runState) writeCommittedSnapshot(name string, snapshot terminal.ScreenSnapshot) error {
	textPath := s.committedSnapshotTextPath(name)
	jsonPath := strings.TrimSuffix(textPath, ".txt") + ".json"
	if err := os.MkdirAll(filepath.Dir(textPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(textPath, []byte(strings.TrimRight(snapshot.Text, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(jsonPath, data, 0o644)
}

func (s *runState) committedSnapshotTextPath(name string) string {
	root := s.runtime.Config.SnapshotRoot
	if root == "" {
		root = config.DefaultSnapshotRoot
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(s.runtime.ProjectRoot, root)
	}
	return filepath.Join(root, sanitize(s.spec.Name), sanitize(name)+".txt")
}

func (s *runState) runShellCommand(ctx context.Context, command string, cwd string, timeoutMS int) error {
	timeout := timeoutFromMS(timeoutMS)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "/bin/sh", "-lc", command)
	cmd.Dir = resolveProjectPath(s.runtime.ProjectRoot, cwd)
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			return fmt.Errorf("%q timed out after %s", command, timeout)
		}
		return fmt.Errorf("%q failed: %v %s", command, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (s *runState) cleanup() {
	if s.session != nil {
		_ = s.session.Cleanup(CleanupTimeout)
	}
}

func (s *runState) finish(started time.Time, status artifacts.RunStatus, outcomes []artifacts.OutcomeResult, diagnostic string) artifacts.RunResult {
	if outcomes == nil {
		outcomes = []artifacts.OutcomeResult{}
	}
	s.cleanup()
	finalSnapshot := s.normalizeSnapshot(s.emulator.Screen().Snapshot())
	ended := time.Now().UTC()
	exitCode := 0
	if status == artifacts.StatusFailed {
		exitCode = 1
	}
	if status == artifacts.StatusErrored {
		exitCode = 2
	}
	result := artifacts.RunResult{
		SchemaVersion: 1,
		RunID:         makeRunID(started, s.spec.Name),
		SpecName:      s.spec.Name,
		Status:        status,
		StartedAt:     started.Format(time.RFC3339Nano),
		EndedAt:       ended.Format(time.RFC3339Nano),
		DurationMS:    ended.Sub(started).Milliseconds(),
		Target:        s.spec.Target,
		Terminal:      s.spec.Terminal,
		Outcomes:      outcomes,
		RunDir:        s.writer.RunDir,
		ExitCode:      exitCode,
		Artifacts:     map[string]string{"events": "events.ndjson"},
	}
	if s.runtime.Config.Artifacts.AgentContext {
		result.Artifacts["agentContext"] = "agent_context.md"
	}
	if s.runtime.Config.Artifacts.FinalScreen {
		result.Artifacts["finalScreenText"] = "screens/final.txt"
		result.Artifacts["finalScreenJSON"] = "screens/final.json"
	}
	if s.runtime.Config.Artifacts.Frames {
		result.Artifacts["frames"] = "frames/frames.ndjson"
	}
	if s.runtime.Config.Artifacts.RawLog {
		result.Artifacts["rawPtyLog"] = "raw/pty.raw.log"
		result.Artifacts["inputRawLog"] = "raw/input.raw.log"
	}
	if s.runtime.Config.Artifacts.Snapshots {
		s.mu.Lock()
		snapshotNames := make([]string, 0, len(s.snapshots))
		for name := range s.snapshots {
			snapshotNames = append(snapshotNames, name)
		}
		s.mu.Unlock()
		sort.Strings(snapshotNames)
		for _, name := range snapshotNames {
			result.Artifacts["snapshot:"+name] = "snapshots/" + artifacts.SafeName(name) + ".txt"
		}
	}
	if diagnostic != "" {
		result.Artifacts["failureDiagnostic"] = "diagnostics/failure.md"
		_ = s.writer.WriteDiagnostic("failure", "## Failure\n\n"+diagnostic+"\n\n## Final Screen\n\n```text\n"+finalSnapshot.Text+"\n```\n")
	} else if status == artifacts.StatusFailed {
		result.Artifacts["failureDiagnostic"] = "diagnostics/failure.md"
		_ = s.writer.WriteDiagnostic("failure", renderOutcomeFailureDiagnostic(outcomes, finalSnapshot.Text))
	}
	if s.runtime.Config.Artifacts.FinalScreen {
		_ = s.writer.WriteFinalScreen(finalSnapshot)
	}
	s.mu.Lock()
	rawPTY := append([]byte(nil), s.rawPTY...)
	inputLog := append([]byte(nil), s.inputLog...)
	frames := append([]terminal.Frame(nil), s.frames...)
	s.mu.Unlock()
	if s.runtime.Config.Artifacts.RawLog {
		_ = s.writer.WriteRawPTY(rawPTY)
		_ = s.writer.WriteInputLog(inputLog)
	}
	if s.runtime.Config.Artifacts.Frames {
		_ = s.writer.WriteFrames(frames)
	}
	if s.runtime.Config.Artifacts.AgentContext {
		_ = s.writer.WriteAgentContext(s.spec, result, finalSnapshot.Text)
	}
	_ = s.writer.WriteOutcomesIndex(result)
	_ = s.writer.WriteRun(result)
	if status == artifacts.StatusPassed {
		_ = s.writer.AppendEvent(event("run.passed", s.spec.Name, ""))
	} else if status == artifacts.StatusFailed {
		_ = s.writer.AppendEvent(event("run.failed", s.spec.Name, diagnostic))
	} else {
		_ = s.writer.AppendEvent(event("run.errored", s.spec.Name, diagnostic))
	}
	if s.listener != nil {
		s.listener.OnRunEnd(result)
	}
	return result
}

func renderOutcomeFailureDiagnostic(outcomes []artifacts.OutcomeResult, finalScreen string) string {
	var b strings.Builder
	b.WriteString("## Failed Outcomes\n\n")
	for _, outcome := range outcomes {
		if outcome.Status == artifacts.OutcomeFailed {
			b.WriteString("- ")
			b.WriteString(outcome.ID)
			if outcome.Message != "" {
				b.WriteString(": ")
				b.WriteString(outcome.Message)
			}
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n## Final Screen\n\n```text\n")
	b.WriteString(finalScreen)
	b.WriteString("\n```\n")
	return b.String()
}

func checkScreen(text string, cond spec.ScreenCondition) (bool, string) {
	switch {
	case cond.Contains != "":
		if strings.Contains(text, cond.Contains) {
			return true, fmt.Sprintf("screen contains %q", cond.Contains)
		}
		return false, fmt.Sprintf("expected screen to contain %q", cond.Contains)
	case cond.NotContains != "":
		if !strings.Contains(text, cond.NotContains) {
			return true, fmt.Sprintf("screen does not contain %q", cond.NotContains)
		}
		return false, fmt.Sprintf("expected screen not to contain %q", cond.NotContains)
	case cond.Regex != "":
		re, err := regexp.Compile(cond.Regex)
		if err != nil {
			return false, err.Error()
		}
		if re.MatchString(text) {
			return true, fmt.Sprintf("screen matches %q", cond.Regex)
		}
		return false, fmt.Sprintf("expected screen to match %q", cond.Regex)
	default:
		return false, "screen condition is empty"
	}
}

func checkStyle(actual terminal.Style, expected spec.Style) (bool, string) {
	if expected.Fg != "" && actual.Fg != expected.Fg {
		return false, fmt.Sprintf("expected fg %q, got %q", expected.Fg, actual.Fg)
	}
	if expected.Bg != "" && actual.Bg != expected.Bg {
		return false, fmt.Sprintf("expected bg %q, got %q", expected.Bg, actual.Bg)
	}
	if expected.Bold != nil && actual.Bold != *expected.Bold {
		return false, fmt.Sprintf("expected bold=%v, got %v", *expected.Bold, actual.Bold)
	}
	if expected.Dim != nil && actual.Dim != *expected.Dim {
		return false, fmt.Sprintf("expected dim=%v, got %v", *expected.Dim, actual.Dim)
	}
	if expected.Italic != nil && actual.Italic != *expected.Italic {
		return false, fmt.Sprintf("expected italic=%v, got %v", *expected.Italic, actual.Italic)
	}
	if expected.Underline != nil && actual.Underline != *expected.Underline {
		return false, fmt.Sprintf("expected underline=%v, got %v", *expected.Underline, actual.Underline)
	}
	if expected.Reverse != nil && actual.Reverse != *expected.Reverse {
		return false, fmt.Sprintf("expected reverse=%v, got %v", *expected.Reverse, actual.Reverse)
	}
	return true, "style matched"
}

func checkProcess(state ptyrunner.ExitState, cond spec.ProcessCondition) (bool, string) {
	if cond.Exited != nil {
		if state.Exited == *cond.Exited {
			return true, fmt.Sprintf("process exited=%v", state.Exited)
		}
		return false, fmt.Sprintf("expected process exited=%v, got %v", *cond.Exited, state.Exited)
	}
	if cond.ExitCode != nil {
		if state.Exited && state.ExitCode == *cond.ExitCode {
			return true, fmt.Sprintf("process exit code is %d", *cond.ExitCode)
		}
		if !state.Exited {
			return false, fmt.Sprintf("expected process exit code %d but process is still running", *cond.ExitCode)
		}
		return false, fmt.Sprintf("expected process exit code %d, got %d", *cond.ExitCode, state.ExitCode)
	}
	return false, "process condition is empty"
}

func timeoutFromMS(ms int) time.Duration {
	if ms <= 0 {
		return DefaultTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

func resolveProjectPath(projectRoot string, path string) string {
	if path == "" {
		path = "."
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(projectRoot, path)
}

func decodeEscaped(input string) ([]byte, error) {
	unquoted, err := strconv.Unquote(`"` + strings.ReplaceAll(input, `"`, `\"`) + `"`)
	if err != nil {
		return nil, err
	}
	return []byte(unquoted), nil
}

func event(kind string, name string, info string) artifacts.Event {
	return artifacts.Event{TS: time.Now().UTC().Format(time.RFC3339Nano), Type: kind, Name: name, Info: info}
}

func describeStep(step spec.Step) string {
	data, err := json.Marshal(step)
	if err != nil {
		return ""
	}
	return string(data)
}

func makeRunID(t time.Time, name string) string {
	return t.UTC().Format("2006-01-02T15-04-05Z") + "-" + strconv.FormatInt(t.UnixNano()%1e9, 36) + "-" + sanitize(name)
}

func sanitize(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "run"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
