package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/abdul-hamid-achik/glyphrun/internal/log"
	"github.com/abdul-hamid-achik/glyphrun/internal/procmon"
	"github.com/abdul-hamid-achik/glyphrun/internal/ptyrunner"
	"github.com/abdul-hamid-achik/glyphrun/internal/render"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal/adapters/gote"
	"github.com/abdul-hamid-achik/glyphrun/internal/version"
	"gopkg.in/yaml.v3"
)

const (
	DefaultTimeout      = 5 * time.Second
	DefaultPollInterval = 25 * time.Millisecond
	CleanupTimeout      = 2 * time.Second
)

const (
	exitPassed              = 0
	exitFailed              = 1
	exitErrored             = 2
	exitTimedOut            = 3
	exitSpecParse           = 4
	exitContractHash        = 6
	exitUnsupportedTerminal = 7
)

type runTimeoutError struct {
	Scope   string
	Timeout time.Duration
	Message string
}

func (e runTimeoutError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Scope == "" {
		return fmt.Sprintf("timed out after %s", e.Timeout)
	}
	return fmt.Sprintf("%s timed out after %s", e.Scope, e.Timeout)
}

type unsupportedTerminalError struct {
	Err error
}

func (e unsupportedTerminalError) Error() string {
	if e.Err == nil {
		return "unsupported terminal behavior"
	}
	return "unsupported terminal behavior: " + e.Err.Error()
}

func (e unsupportedTerminalError) Unwrap() error {
	return e.Err
}

type Options struct {
	SpecPath        string
	ConfigPath      string
	Environment     string
	ArtifactRoot    string
	UpdateSnapshots bool
	Listener        ProgressListener
	// Procmon enables opt-in process telemetry of the spawned target via the
	// `monitor` CLI. When nil, no sampling runs and no process artifacts are
	// written — the feature is zero-cost for runs that don't opt in.
	Procmon *ProcmonConfig
}

// ProcmonConfig configures `glyph run --monitor` process-telemetry capture.
type ProcmonConfig struct {
	Bin      string        // path to the monitor binary (default: monitor on $PATH)
	Interval time.Duration // sample interval; clamped to >=50ms, default 250ms
	Profile  string        // optional end-of-run profile type: heap|cpu|goroutine|sample
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
		return parseErrorResult(opts.SpecPath, err), err
	}
	parse, err := spec.ParseFile(opts.SpecPath, runtime.SpecParseOptions())
	if err != nil {
		return parseErrorResult(opts.SpecPath, err), err
	}
	runtime.SpecPath = parse.Path
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

	// Resolve tvault env-group secrets before building the redactor so the
	// resolved values are available to both the run environment and the
	// redactor. The resolution is a no-op when the config has no secrets block.
	var secretValues []string
	if runtime.Secrets != nil {
		secretEnv, values, err := resolveSecrets(ctx, runtime.Secrets, envSlice(runtime.Env))
		if err != nil {
			return earlyError(runDir, started, resolved.Name, fmt.Sprintf("secret resolution failed: %v", err), artifacts.ErrorKindPrecondition, exitErrored), nil
		}
		for k, v := range secretEnv {
			runtime.Env[k] = v
		}
		secretValues = values
	}

	writer := artifacts.NewWriter(runDir, buildRedactor(runtime.Config.Redaction, parse.Spec.Redaction, secretValues))
	if err := writer.EnsureDirs(); err != nil {
		return artifacts.RunResult{}, err
	}
	_ = writer.AppendEvent(event("run.started", resolved.Name, ""))
	_ = writer.WriteResolvedSpec(resolved)

	state := newRunState(resolved, runtime, writer, opts.UpdateSnapshots, opts.Listener)
	state.capturePolicy = resolveCapturePolicy(runtime.Config.Artifacts, parse.Spec.Artifacts, artifacts.StatusPassed)
	if opts.Listener != nil {
		opts.Listener.OnRunStart(resolved, runID, runDir)
	}
	if err := state.runPreconditions(ctx); err != nil {
		return state.finish(started, artifacts.StatusErrored, nil, fmt.Sprintf("precondition failed: %v", err), artifacts.ErrorKindPrecondition, exitCodeForError(err)), nil
	}

	if err := state.startTarget(); err != nil {
		return state.finish(started, artifacts.StatusErrored, nil, fmt.Sprintf("target failed to start: %v", err), artifacts.ErrorKindTargetStart), nil
	}
	state.startProcmon(ctx, opts.Procmon)
	if opts.Procmon != nil && opts.Procmon.Bin != "" {
		state.monitorBin = opts.Procmon.Bin
	} else {
		state.monitorBin = "monitor"
	}
	state.writeTargetPID()
	defer state.cleanup()
	runCtx := ctx
	var cancelRun context.CancelFunc
	if resolved.Target.TimeoutMS > 0 {
		timeout := time.Duration(resolved.Target.TimeoutMS) * time.Millisecond
		runCtx, cancelRun = context.WithTimeout(ctx, timeout)
		defer cancelRun()
	}

	for idx, step := range resolved.Steps {
		name := fmt.Sprintf("step.%d", idx+1)
		_ = writer.AppendEvent(event("step.started", name, describeStep(step)))
		stepStart := time.Now()
		if state.listener != nil {
			state.listener.OnStepStart(idx, step)
		}
		result, err := state.executeStep(runCtx, step)
		if err != nil {
			_ = writer.AppendEvent(event("step.failed", name, err.Error()))
			state.stepResults = append(state.stepResults, artifacts.StepResult{Index: idx + 1, Kind: stepKind(step), Status: "failed", DurationMS: time.Since(stepStart).Milliseconds(), Error: err.Error()})
			if state.listener != nil {
				state.listener.OnStepFinish(idx, step, ProgressFailed, time.Since(stepStart), err.Error())
			}
			if code := exitCodeForError(err); code == exitTimedOut || code == exitUnsupportedTerminal {
				message := fmt.Sprintf("step %d failed: %v", idx+1, err)
				if errors.Is(err, context.DeadlineExceeded) && resolved.Target.TimeoutMS > 0 {
					message = fmt.Sprintf("step %d failed: %s", idx+1, targetTimeoutMessage(resolved.Target.TimeoutMS))
				}
				kind := artifacts.ErrorKindTimeout
				if code == exitUnsupportedTerminal {
					kind = artifacts.ErrorKindUnsupportedTerminal
				}
				return state.finish(started, artifacts.StatusErrored, nil, message, kind, code), nil
			}
			result := state.evaluateOutcomes(ctx)
			return state.finish(started, artifacts.StatusFailed, result, fmt.Sprintf("step %d failed: %v", idx+1, err), artifacts.ErrorKindStepFailure), nil
		}
		if result.Skipped {
			_ = writer.AppendEvent(event("step.skipped", name, result.Message))
			state.stepResults = append(state.stepResults, artifacts.StepResult{Index: idx + 1, Kind: stepKind(step), Status: "skipped", DurationMS: time.Since(stepStart).Milliseconds()})
			if state.listener != nil {
				state.listener.OnStepFinish(idx, step, ProgressSkipped, time.Since(stepStart), result.Message)
			}
			continue
		}
		if state.listener != nil {
			state.listener.OnStepFinish(idx, step, ProgressPassed, time.Since(stepStart), "")
		}
		state.stepResults = append(state.stepResults, artifacts.StepResult{Index: idx + 1, Kind: stepKind(step), Status: "passed", DurationMS: time.Since(stepStart).Milliseconds()})
		_ = writer.AppendEvent(event("step.finished", name, ""))
	}

	// If the target timeout already expired before we reached outcome
	// evaluation, classify the run as a timeout directly. Running
	// evaluateOutcomes here would poll once, immediately observe the
	// cancelled context, and write a spurious outcome.failed event and
	// artifact for every outcome — noise that misrepresents a global
	// timeout as per-outcome failures.
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return state.finish(started, artifacts.StatusErrored, nil, targetTimeoutMessage(resolved.Target.TimeoutMS), artifacts.ErrorKindTimeout, exitTimedOut), nil
	}
	outcomes := state.evaluateOutcomes(runCtx)
	if err := state.terminalError(); err != nil {
		return state.finish(started, artifacts.StatusErrored, outcomes, err.Error(), artifacts.ErrorKindUnsupportedTerminal, exitCodeForError(err)), nil
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		// Deadline fired during outcome evaluation; keep the partial
		// outcome results but report the run as a timeout.
		return state.finish(started, artifacts.StatusErrored, outcomes, targetTimeoutMessage(resolved.Target.TimeoutMS), artifacts.ErrorKindTimeout, exitTimedOut), nil
	}
	status := artifacts.StatusPassed
	for _, outcome := range outcomes {
		if outcome.Status != artifacts.OutcomePassed {
			status = artifacts.StatusFailed
			break
		}
	}
	diagnostic := ""
	if policyFailure := state.terminalPolicyFailure(); policyFailure != "" {
		_ = writer.AppendEvent(event("terminal.policy.failed", "alternateScreen", policyFailure))
		if status == artifacts.StatusPassed {
			status = artifacts.StatusFailed
			diagnostic = policyFailure
		}
	}
	return state.finish(started, status, outcomes, diagnostic, ""), nil
}

// parseErrorResult builds a RunResult for a failure that occurred before the
// run could start — spec parse error, schema validation failure, or contract
// hash mismatch. The result carries errorKind + diagnostic so the CLI can emit
// a structured JSON envelope on stdout (exit 4/6) instead of only a stderr log
// line, letting agents pick an actionable next step.
func parseErrorResult(specPath string, err error) artifacts.RunResult {
	started := time.Now().UTC()
	name := specNameFromPath(specPath)
	kind := artifacts.ErrorKindSpecParse
	contractHash := ""
	expectedHash := ""
	exitCode := exitSpecParse
	var mismatch spec.ContractHashMismatchError
	if errors.As(err, &mismatch) {
		kind = artifacts.ErrorKindContractHashMismatch
		contractHash = mismatch.Actual
		expectedHash = mismatch.Expected
		exitCode = exitContractHash
		if mismatch.SpecName != "" {
			name = mismatch.SpecName
		}
	}
	ended := time.Now().UTC()
	return artifacts.RunResult{
		SchemaVersion: 1,
		RunID:         makeRunID(started, name),
		SpecName:      name,
		Status:        artifacts.StatusErrored,
		ErrorKind:     kind,
		Diagnostic:    err.Error(),
		ContractHash:  contractHash,
		ExpectedHash:  expectedHash,
		StartedAt:     started.Format(time.RFC3339Nano),
		EndedAt:       ended.Format(time.RFC3339Nano),
		DurationMS:    ended.Sub(started).Milliseconds(),
		ExitCode:      exitCode,
		Outcomes:      []artifacts.OutcomeResult{},
		Artifacts:     map[string]string{},
		NextActions:   artifacts.NextActionsFor(kind, name, contractHash, expectedHash),
	}
}

// specNameFromPath extracts the spec name from a YAML spec file's `name:`
// field. If the file cannot be read or the field is absent, it falls back to
// the file basename without extension so the error envelope always carries a
// non-empty specName.
func specNameFromPath(specPath string) string {
	if data, err := os.ReadFile(specPath); err == nil {
		var probe struct {
			Name string `yaml:"name"`
		}
		if yaml.Unmarshal(data, &probe) == nil && strings.TrimSpace(probe.Name) != "" {
			return strings.TrimSpace(probe.Name)
		}
	}
	base := filepath.Base(specPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

type runState struct {
	spec            spec.Spec
	runtime         config.Runtime
	writer          *artifacts.Writer
	emulator        terminal.Emulator
	session         *ptyrunner.Session
	updateSnapshots bool
	listener        ProgressListener
	// capturePolicy is the resolved per-run policy (project config
	// + spec override) used to gate which artifacts are written. The
	// runner populates it once at run start so `finish()` can apply
	// it without re-resolving.
	capturePolicy spec.CapturePolicy

	// cleanupOnce guards session teardown so the deferred cleanup in
	// RunSpec and the explicit call in finish() collapse to a single
	// Session.Cleanup, regardless of which path runs first.
	cleanupOnce sync.Once

	mu             sync.Mutex
	rawPTY         []byte
	inputLog       []byte
	frames         []terminal.Frame
	termErr        error
	snapshots      map[string]terminal.ScreenSnapshot
	namedArtifacts map[string]artifacts.NamedArtifact

	// stepResults accumulates per-step execution records (SPEC §7.3) during the
	// run loop so finish() can include them in RunResult.Steps.
	stepResults []artifacts.StepResult

	// rawPTYTruncated is set the first time a PTY output chunk is
	// dropped because rawPTY has already hit MaxRawLogBytes. finish()
	// appends a marker to the artifact and emits a pty.truncated event
	// so the loss is visible in events.ndjson and agent_context.md.
	rawPTYTruncated bool

	// procmon is the opt-in process-telemetry sampler for the spawned
	// target. nil unless Options.Procmon is set AND the backend exposes a
	// PID. One supervised goroutine ticks SampleOnce into procmon.samples
	// until finalizeProcmon() cancels it; the mutex on procmonRun guards
	// the sample slice across that goroutine and the finish() reader.
	procmon    *procmonRun
	monitorBin string // monitor binary path for `monitor:` steps (default "monitor")
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
		namedArtifacts:  map[string]artifacts.NamedArtifact{},
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
	// GLYPHRUN_RUN_DIR lets a target process (or a shell `command:` verifier)
	// reference the run's absolute path without scanning env vars for it.
	env["GLYPHRUN_RUN_DIR"] = s.writer.RunDir
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
			if max > 0 {
				used := int64(len(s.rawPTY))
				if used >= max {
					// Already at cap — drop the chunk but remember it.
					s.rawPTYTruncated = true
				} else if used+int64(len(data)) > max {
					// Take what fits, then mark truncated.
					s.rawPTY = append(s.rawPTY, data[:max-used]...)
					s.rawPTYTruncated = true
				} else {
					s.rawPTY = append(s.rawPTY, data...)
				}
			} else {
				s.rawPTY = append(s.rawPTY, data...)
			}
			s.mu.Unlock()
			frames, err := s.emulator.Feed(data)
			s.mu.Lock()
			if err != nil && s.termErr == nil {
				s.termErr = unsupportedTerminalError{Err: err}
				s.mu.Unlock()
				return
			}
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
	if err := ctx.Err(); err != nil {
		return stepExecutionResult{}, err
	}
	if err := s.terminalError(); err != nil {
		return stepExecutionResult{}, err
	}
	if step.When != nil {
		ok, message := s.checkVerify(ctx, *step.When, nil)
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
		data := []byte(step.Paste)
		if bracketedPasteEnabled(s.emulator) {
			data = input.BracketedPasteBytes(step.Paste)
		}
		return stepExecutionResult{}, s.writeInput(data)
	case step.Send != nil:
		data, err := decodeEscaped(step.Send.Bytes)
		if err != nil {
			return stepExecutionResult{}, err
		}
		return stepExecutionResult{}, s.writeInput(data)
	case step.Mouse != nil:
		data, err := input.MouseBytes(step.Mouse.X, step.Mouse.Y, step.Mouse.Button, step.Mouse.Action, mouseSGREnabled(s.emulator))
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
	case step.Download != nil:
		return s.executeDownload(ctx, *step.Download)
	case step.Transform != nil:
		return s.executeTransform(ctx, *step.Transform)
	case step.Monitor != nil:
		return s.executeMonitor(ctx, *step.Monitor)
	case len(step.Batch) > 0:
		return s.executeBatch(ctx, step.Batch)
	default:
		return stepExecutionResult{}, fmt.Errorf("unsupported step")
	}
}

// resolveRuntimePlaceholders replaces every ${artifacts.<name>.path} and
// ${artifacts.<name>.relativePath} occurrence in `text` with the current
// value from s.namedArtifacts. Unknown artifact names return a descriptive
// error so missing wiring is loud, not silent.
func (s *runState) resolveRuntimePlaceholders(text string) (string, error) {
	if text == "" {
		return text, nil
	}
	var firstErr error
	out := runtimePlaceholderPattern.ReplaceAllStringFunc(text, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		name, field, ok := splitArtifactKey(key)
		if !ok {
			return match
		}
		s.mu.Lock()
		art, found := s.namedArtifacts[name]
		s.mu.Unlock()
		if !found {
			if firstErr == nil {
				firstErr = fmt.Errorf("artifact %q referenced by %s is not produced by any earlier step", name, match)
			}
			return match
		}
		switch field {
		case "path":
			return art.Path
		case "relativePath":
			return art.RelativePath
		}
		return match
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

var runtimePlaceholderPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func splitArtifactKey(key string) (name, field string, ok bool) {
	const prefix = "artifacts."
	if !strings.HasPrefix(key, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(key, prefix)
	dot := strings.Index(rest, ".")
	if dot < 0 {
		return "", "", false
	}
	return rest[:dot], rest[dot+1:], true
}

// executeDownload captures a file from a known path into the run artifact
// directory. If `waitFor` is set the runner polls for the file to appear
// (defaulting to a short timeout). Any ${artifacts.*} placeholders in the
// path are resolved against the current run state.
func (s *runState) executeDownload(ctx context.Context, d spec.DownloadStep) (stepExecutionResult, error) {
	resolved, err := s.resolveRuntimePlaceholders(d.Path)
	if err != nil {
		return stepExecutionResult{}, err
	}
	timeout := timeoutFromMS(d.TimeoutMS)
	if d.WaitFor {
		// Poll for the file to appear, then wait one more tick for
		// the size to stabilize. A target process that calls
		// `os.WriteFile` (or `O_TRUNC` + write) has a brief window
		// where the file exists at 0 bytes; capturing then would
		// surface an empty artifact. Waiting for size stability across
		// a poll cycle catches that race without adding a hard sleep.
		deadline := time.Now().Add(timeout)
		var prevSize int64 = -1
		stableTicks := 0
		for {
			info, statErr := os.Stat(resolved)
			if statErr == nil {
				if info.Size() == prevSize && info.Size() > 0 {
					stableTicks++
					if stableTicks >= 1 {
						break
					}
				} else {
					stableTicks = 0
					prevSize = info.Size()
				}
			}
			if time.Now().After(deadline) {
				return stepExecutionResult{}, fmt.Errorf("download timed out after %s waiting for %s", timeout, resolved)
			}
			select {
			case <-ctx.Done():
				return stepExecutionResult{}, ctx.Err()
			case <-time.After(25 * time.Millisecond):
			}
		}
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return stepExecutionResult{}, fmt.Errorf("download source not found: %w", err)
	}
	if info.IsDir() {
		return stepExecutionResult{}, fmt.Errorf("download source %s is a directory", resolved)
	}
	assign := d.Assign
	if assign == "" {
		assign = artifactNameFromPath(resolved)
	}
	saveAs := d.SaveAs
	if saveAs == "" {
		saveAs = filepath.Base(resolved)
	}
	relDir := "artifacts/" + assign
	relPath := relDir + "/" + saveAs
	absPath, err := s.writer.ResolvePath(relPath)
	if err != nil {
		return stepExecutionResult{}, err
	}
	src, err := os.Open(resolved)
	if err != nil {
		return stepExecutionResult{}, err
	}
	defer src.Close()
	if err := s.writer.CopyArtifact(relPath, src); err != nil {
		return stepExecutionResult{}, err
	}
	s.mu.Lock()
	s.namedArtifacts[assign] = artifacts.NamedArtifact{
		Kind:         "download",
		Path:         absPath,
		RelativePath: relPath,
	}
	s.mu.Unlock()
	_ = s.writer.AppendEvent(event("artifact.download", assign, relPath))
	return stepExecutionResult{}, nil
}

// executeTransform runs an external script (Node or shell) that produces a
// new named artifact. The script receives a JSON context on argv (Node) or
// via env vars (shell); it must write its output to ctx.output.path. If the
// script returns a JSON object with `{ "ok": false }` the step fails.
func (s *runState) executeTransform(ctx context.Context, t spec.TransformStep) (stepExecutionResult, error) {
	assign := t.Assign
	if assign == "" {
		assign = artifactNameFromPath(t.SaveAs)
	}
	relDir := "transforms/" + assign
	relPath := relDir + "/" + filepath.Base(t.SaveAs)
	absPath, err := s.writer.ResolvePath(relPath)
	if err != nil {
		return stepExecutionResult{}, err
	}
	absDir, err := s.writer.ResolvePath(relDir)
	if err != nil {
		return stepExecutionResult{}, err
	}

	input, err := s.resolveRuntimePlaceholders(t.Input)
	if err != nil {
		return stepExecutionResult{}, err
	}

	fixtures := map[string]string{}
	for k, v := range t.Fixtures {
		resolved, err := s.resolveRuntimePlaceholders(v)
		if err != nil {
			return stepExecutionResult{}, err
		}
		fixtures[k] = resolved
	}

	scriptPath := t.File
	if !filepath.IsAbs(scriptPath) {
		// Resolve relative to the spec file (matches the convention used
		// elsewhere in glyphrun: spec-relative paths).
		scriptPath = filepath.Join(filepath.Dir(s.runtime.SpecPath), scriptPath)
	}

	runtime := t.Runtime
	if runtime == "" {
		runtime = "shell"
	}
	timeout := timeoutFromMS(t.TimeoutMS)

	if err := s.runTransformScript(ctx, runtime, scriptPath, input, absPath, absDir, fixtures, timeout); err != nil {
		return stepExecutionResult{}, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return stepExecutionResult{}, fmt.Errorf("transform did not write %s", absPath)
	}
	if info.IsDir() {
		return stepExecutionResult{}, fmt.Errorf("transform wrote a directory at %s", absPath)
	}
	if err := s.writer.RegisterArtifact(relPath); err != nil {
		return stepExecutionResult{}, err
	}

	s.mu.Lock()
	s.namedArtifacts[assign] = artifacts.NamedArtifact{
		Kind:         "transform",
		Path:         absPath,
		RelativePath: relPath,
	}
	s.mu.Unlock()
	_ = s.writer.AppendEvent(event("artifact.transform", assign, relPath))
	return stepExecutionResult{}, nil
}

// runTransformScript is split out so executeTransform reads as the spec
// semantics and the OS-process plumbing stays testable. The contract is:
//
//	shell:   $GLYPHRUN_INPUT, $GLYPHRUN_OUTPUT, $GLYPHRUN_FIXTURES_JSON
//	node:    argv[2] is the path to a JSON file with the same context
//
// In both cases the script must create the output file at $GLYPHRUN_OUTPUT.
func (s *runState) runTransformScript(parent context.Context, runtime string, scriptPath string, input string, outputPath string, outputDir string, fixtures map[string]string, timeout time.Duration) error {
	runCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	env := os.Environ()
	env = append(env,
		"GLYPHRUN_INPUT="+input,
		"GLYPHRUN_OUTPUT="+outputPath,
	)
	if data, err := json.Marshal(fixtures); err == nil {
		env = append(env, "GLYPHRUN_FIXTURES_JSON="+string(data))
	}

	var cmd *exec.Cmd
	// Ensure the script's working dir and the output's parent dir both
	// exist; the script itself may run from outputDir (matching the
	// convention that the cwd is the artifact dir, so relative paths
	// in the script resolve against the run).
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	switch runtime {
	case "node":
		ctxPath, err := writeTransformContextFile(outputDir, transformContext{
			Input:    input,
			Output:   outputPath,
			Fixtures: fixtures,
			RunDir:   s.writer.RunDir,
		})
		if err != nil {
			return err
		}
		cmd = exec.CommandContext(runCtx, "node", scriptPath, ctxPath)
	default: // "shell"
		cmd = exec.CommandContext(runCtx, "/bin/sh", "-lc", "\""+scriptPath+"\"")
	}
	cmd.Env = env
	cmd.Dir = outputDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			return runTimeoutError{Scope: "transform", Timeout: timeout, Message: fmt.Sprintf("transform timed out after %s", timeout)}
		}
		return fmt.Errorf("transform %s failed: %v %s", scriptPath, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type transformContext struct {
	Input    string            `json:"input"`
	Output   string            `json:"output"`
	Fixtures map[string]string `json:"fixtures"`
	RunDir   string            `json:"runDir"`
}

func writeTransformContextFile(dir string, ctx transformContext) (string, error) {
	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, ".glyphrun-transform-ctx.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// executeBatch queues every press/type/paste/send sub-step into a single
// PTY write so the target app sees them as one input burst — this is the
// mechanism that preserves transient TUI state across keystrokes (a
// command palette, a hover popover, a focused menu). The optional trailing
// `wait` is the only synchronization point inside the batch.
func (s *runState) executeBatch(ctx context.Context, steps []spec.Step) (stepExecutionResult, error) {
	if len(steps) < 2 {
		return stepExecutionResult{}, fmt.Errorf("batch requires at least 2 sub-steps")
	}
	var buf bytes.Buffer
	trailing := (*spec.WaitStep)(nil)
	for i, sub := range steps {
		if sub.Wait != nil {
			if i != len(steps)-1 {
				return stepExecutionResult{}, fmt.Errorf("batch wait must be the final sub-step")
			}
			w := *sub.Wait
			trailing = &w
			continue
		}
		var data []byte
		var err error
		switch {
		case sub.Press != "":
			data, err = input.KeyBytes(sub.Press)
		case sub.Type != "":
			data = []byte(sub.Type)
		case sub.Paste != "":
			data = []byte(sub.Paste)
			if bracketedPasteEnabled(s.emulator) {
				data = input.BracketedPasteBytes(sub.Paste)
			}
		case sub.Send != nil:
			data, err = decodeEscaped(sub.Send.Bytes)
		default:
			return stepExecutionResult{}, fmt.Errorf("batch sub-step %d: unsupported action", i+1)
		}
		if err != nil {
			return stepExecutionResult{}, err
		}
		buf.Write(data)
	}
	if buf.Len() > 0 {
		if err := s.writeInput(buf.Bytes()); err != nil {
			return stepExecutionResult{}, err
		}
	}
	if trailing != nil {
		return stepExecutionResult{}, s.waitFor(ctx, *trailing)
	}
	return stepExecutionResult{}, nil
}

// buildRedactor composes the per-run redactor. The project config
// supplies the base patterns (config.Redaction) and the spec's
// `redaction:` block (when present) adds literal value substitutions
// on top. Both layers are folded into a single redactor so callers
// don't have to know about the layering.
//
// Per-spec patterns are not exposed in the spec schema today (only
// `values` is), but the helper accepts them anyway so a future
// schema bump doesn't require a runner change.
func buildRedactor(cfg config.Redaction, specRedaction *spec.Redaction, secretValues []string) artifacts.Redactor {
	r := artifacts.NewRedactor(cfg)
	if specRedaction != nil && len(specRedaction.Values) > 0 {
		r = r.WithValues(specRedaction.Values)
	}
	if len(secretValues) > 0 {
		r = r.WithValues(secretValues)
	}
	return r
}

// resolveCapturePolicy composes the effective capture policy for a
// run. The project config supplies the base (booleans); the spec's
// `artifacts:` block overrides per channel. An empty CaptureMode
// inherits from the base.
func resolveCapturePolicy(base config.Artifacts, specPolicy *spec.CapturePolicy, status artifacts.RunStatus) spec.CapturePolicy {
	out := spec.CapturePolicy{
		Snapshots:      boolToMode(base.Snapshots),
		Frames:         boolToMode(base.Frames),
		RawLog:         boolToMode(base.RawLog),
		FinalScreen:    boolToMode(base.FinalScreen),
		AgentContext:   boolToMode(base.AgentContext),
		NamedArtifacts: spec.CaptureAlways, // named artifacts are always-on; they're the spec's contract
	}
	if specPolicy == nil {
		return out
	}
	if specPolicy.Snapshots != "" {
		out.Snapshots = specPolicy.Snapshots
	}
	if specPolicy.Frames != "" {
		out.Frames = specPolicy.Frames
	}
	if specPolicy.RawLog != "" {
		out.RawLog = specPolicy.RawLog
	}
	if specPolicy.FinalScreen != "" {
		out.FinalScreen = specPolicy.FinalScreen
	}
	if specPolicy.AgentContext != "" {
		out.AgentContext = specPolicy.AgentContext
	}
	return out
}

func boolToMode(b bool) spec.CaptureMode {
	if b {
		return spec.CaptureAlways
	}
	return spec.CaptureNever
}

// shouldCapture answers "should this artifact channel be written
// for the current run?" with the per-channel capture mode and the
// final run status. An empty mode is treated as "never" so a spec
// that explicitly disables a channel is honored even if the
// project config turns it on.
func shouldCapture(mode spec.CaptureMode, status artifacts.RunStatus) bool {
	switch mode {
	case spec.CaptureAlways:
		return true
	case spec.CaptureOnFailure:
		return status == artifacts.StatusFailed || status == artifacts.StatusErrored
	default:
		return false
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
		if err := s.terminalError(); err != nil {
			return err
		}
		ok, message := s.checkWait(wait)
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return runTimeoutError{Scope: "wait step", Timeout: timeout, Message: fmt.Sprintf("timed out after %s: %s", timeout, message)}
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

// outcomeEval is the result of evaluating a single outcome. It bundles
// the user-facing OutcomeResult with any raw evidence payload the
// verifier returned (e.g. a `script:` verifier's evidence object) so
// evaluateOutcomes can forward it to WriteOutcomeRaw without losing it
// between iterations.
type outcomeEval struct {
	result artifacts.OutcomeResult
	raw    any
}

func (s *runState) evaluateOutcomes(ctx context.Context) []artifacts.OutcomeResult {
	if s.listener != nil {
		s.listener.OnOutcomesStart(len(s.spec.Outcomes))
	}
	results := make([]artifacts.OutcomeResult, 0, len(s.spec.Outcomes))
	for _, outcome := range s.spec.Outcomes {
		eval := s.evaluateOutcome(ctx, outcome)
		results = append(results, eval.result)
		if s.listener != nil {
			s.listener.OnOutcomeFinish(outcome, eval.result)
		}
		_ = s.writer.WriteOutcome(eval.result, map[string]any{
			"id":     outcome.ID,
			"verify": outcome.Verify,
		})
		if eval.result.EvidenceRaw != "" && eval.raw != nil {
			_ = s.writer.WriteOutcomeRaw(outcome.ID, eval.raw)
		}
		if eval.result.Status == artifacts.OutcomePassed {
			_ = s.writer.AppendEvent(event("outcome.passed", outcome.ID, eval.result.Message))
		} else {
			_ = s.writer.AppendEvent(event("outcome.failed", outcome.ID, eval.result.Message))
		}
	}
	return results
}

func (s *runState) evaluateOutcome(ctx context.Context, outcome spec.Outcome) outcomeEval {
	timeout := timeoutFromMS(outcome.TimeoutMS)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(DefaultPollInterval)
	defer tick.Stop()
	var last string
	// Retain the most recent evidence payload so a failing or timed-out
	// outcome still writes outcomes/<id>.raw.json — failure is exactly when
	// that evidence (e.g. a script verifier's {ok:false, evidence} body) is
	// most useful.
	var lastRaw any
	for {
		if err := s.terminalError(); err != nil {
			return failedOutcome(outcome, err.Error(), lastRaw)
		}
		ok, message, raw := s.checkVerifyWithEvidence(ctx, outcome.Verify, outcome.Normalize)
		if raw != nil {
			lastRaw = raw
		}
		if ok {
			result := artifacts.OutcomeResult{ID: outcome.ID, Status: artifacts.OutcomePassed, Message: message, Evidence: "outcomes/" + outcome.ID + ".md"}
			if raw != nil {
				result.EvidenceRaw = "outcomes/" + outcome.ID + ".raw.json"
			}
			return outcomeEval{result: result, raw: raw}
		}
		last = message
		select {
		case <-ctx.Done():
			return failedOutcome(outcome, ctx.Err().Error(), lastRaw)
		case <-deadline.C:
			return failedOutcome(outcome, fmt.Sprintf("timed out after %s: %s", timeout, last), lastRaw)
		case <-tick.C:
		}
	}
}

// failedOutcome builds a failing OutcomeResult, attaching the raw-evidence
// pointer when a verifier produced evidence before the outcome failed.
func failedOutcome(outcome spec.Outcome, message string, raw any) outcomeEval {
	result := artifacts.OutcomeResult{ID: outcome.ID, Status: artifacts.OutcomeFailed, Message: message, Evidence: "outcomes/" + outcome.ID + ".md"}
	if raw != nil {
		result.EvidenceRaw = "outcomes/" + outcome.ID + ".raw.json"
	}
	return outcomeEval{result: result, raw: raw}
}

func (s *runState) checkVerify(ctx context.Context, verify spec.Verify, normalizeOverride *spec.Normalize) (bool, string) {
	ok, message, _ := s.checkVerifyWithEvidence(ctx, verify, normalizeOverride)
	return ok, message
}

// checkVerifyWithEvidence is the full-fidelity variant of checkVerify; it
// also returns the raw evidence payload when a verifier produced one (e.g.
// a `script:` verifier's return object). The caller (evaluateOutcome)
// writes the evidence to outcomes/<id>.raw.json when present.
func (s *runState) checkVerifyWithEvidence(ctx context.Context, verify spec.Verify, normalizeOverride *spec.Normalize) (bool, string, any) {
	screen := s.emulator.Screen()
	normalize := s.normalizeConfigWith(normalizeOverride)
	switch {
	case verify.Screen != nil:
		ok, message := checkScreen(s.screenTextWithNormalize(normalize), *verify.Screen)
		return ok, message, nil
	case verify.Region != nil:
		cond := verify.Region
		ok, message := checkScreen(normalizeText(screen.Region(cond.X, cond.Y, cond.Width, cond.Height).Text(), normalize), spec.ScreenCondition{
			Contains:    cond.Contains,
			NotContains: cond.NotContains,
			Regex:       cond.Regex,
		})
		return ok, message, nil
	case verify.Cell != nil:
		cell := screen.Cell(verify.Cell.X, verify.Cell.Y)
		if verify.Cell.Char != "" && cell.Char != verify.Cell.Char {
			return false, fmt.Sprintf("expected cell %d,%d to be %q, got %q", verify.Cell.X, verify.Cell.Y, verify.Cell.Char, cell.Char), nil
		}
		if verify.Cell.Style != nil {
			if ok, message := checkStyle(cell.Style, *verify.Cell.Style); !ok {
				return false, fmt.Sprintf("cell %d,%d style mismatch: %s", verify.Cell.X, verify.Cell.Y, message), nil
			}
		}
		return true, "cell matched", nil
	case verify.Cursor != nil:
		cursor := screen.Cursor()
		if cursor.X != verify.Cursor.X || cursor.Y != verify.Cursor.Y {
			return false, fmt.Sprintf("expected cursor %d,%d, got %d,%d", verify.Cursor.X, verify.Cursor.Y, cursor.X, cursor.Y), nil
		}
		if verify.Cursor.Visible != nil && cursor.Visible != *verify.Cursor.Visible {
			return false, fmt.Sprintf("expected cursor visible=%v, got %v", *verify.Cursor.Visible, cursor.Visible), nil
		}
		return true, "cursor matched", nil
	case verify.Process != nil:
		ok, message := checkProcess(s.session.ExitState(), *verify.Process)
		return ok, message, nil
	case verify.Snapshot != nil:
		ok, message := s.checkSnapshot(*verify.Snapshot)
		return ok, message, nil
	case verify.Command != nil:
		err := s.runShellCommand(ctx, verify.Command.Run, verify.Command.Cwd, verify.Command.TimeoutMS)
		if err != nil {
			return false, err.Error(), nil
		}
		return true, "command verifier passed", nil
	case verify.File != nil:
		ok, message, evidence := s.checkFile(ctx, *verify.File)
		return ok, message, evidence
	case verify.Script != nil:
		ok, message, evidence := s.checkScript(ctx, *verify.Script)
		return ok, message, evidence
	case verify.Count != nil:
		ok, message, evidence := s.checkCount(screen, *verify.Count)
		return ok, message, evidence
	case verify.Link != nil:
		ok, message, evidence := checkLink(screen, *verify.Link)
		return ok, message, evidence
	case verify.Metrics != nil:
		ok, message := s.checkMetrics(*verify.Metrics)
		return ok, message, nil
	default:
		return false, "unsupported verifier", nil
	}
}

// executeMonitor runs a `monitor:` step: a one-shot capture of the live
// target's process telemetry via the monitor CLI, stored as a named artifact.
// A snapshot (process reading) is always captured; `tree` and `profile` add
// the process subtree and/or a profile. Requires the target PID (Windows
// ConPTY backends have none → clear error) and monitor on $PATH or the run's
// --monitor binary.
func (s *runState) executeMonitor(ctx context.Context, m spec.MonitorStep) (stepExecutionResult, error) {
	if s.session == nil {
		return stepExecutionResult{}, fmt.Errorf("monitor step: target session has not started")
	}
	pid := s.session.PID()
	if pid == 0 {
		return stepExecutionResult{}, fmt.Errorf("monitor step: target PID unavailable (Windows ConPTY does not expose it)")
	}
	client := &procmon.Client{Bin: s.monitorBin}
	info, err := client.Process(pid)
	if err != nil {
		return stepExecutionResult{}, fmt.Errorf("monitor step: %w", err)
	}
	var tree string
	if m.Tree {
		if t, err := client.TreeText(pid); err == nil {
			tree = t
		}
	}
	var prof *procmon.Profile
	if p := strings.TrimSpace(m.Profile); p != "" {
		if pr, err := client.Profile(pid, p); err == nil {
			prof = &pr
		}
	}
	name := strings.TrimSpace(m.SaveAs)
	if name == "" {
		name = "monitor"
	}
	relMD := "monitors/" + artifacts.SafeName(name) + ".md"
	relJSON := "monitors/" + artifacts.SafeName(name) + ".json"
	absMD := s.writer.Resolve(relMD)
	if err := s.writer.WriteArtifactBytes(relMD, []byte(procmon.RenderSnapshotMarkdown(info, tree, prof))); err != nil {
		return stepExecutionResult{}, err
	}
	payload := map[string]any{"snapshot": info}
	if tree != "" {
		payload["tree"] = tree
	}
	if prof != nil {
		payload["profile"] = prof
	}
	if data, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = s.writer.WriteArtifactBytes(relJSON, append(data, '\n'))
	}
	s.mu.Lock()
	s.namedArtifacts[name] = artifacts.NamedArtifact{Kind: "monitor", Path: absMD, RelativePath: relMD}
	s.mu.Unlock()
	_ = s.writer.AppendEvent(event("artifact.monitor", name, relMD))
	return stepExecutionResult{}, nil
}

// checkMetrics asserts process-telemetry perf budgets against the run's
// sampled summary. Each set field is an upper bound (<=). Without telemetry
// (no --monitor, no samples) the outcome fails with a clear, actionable
// message rather than silently passing.
func (s *runState) checkMetrics(c spec.MetricsCondition) (bool, string) {
	return procmon.AssertMetrics(s.procmonSummary(), c.PeakCpuPercent, c.MeanCpuPercent, c.PeakRss, c.MeanRss)
}

// checkFile polls the filesystem for a file matching the verifier's glob.
// The glob is resolved relative to the spec's directory; wildcards are
// supported in the filename portion. When `contains` is set, the matched
// file's text is also required to include that substring.
func (s *runState) checkFile(ctx context.Context, cond spec.FileCondition) (bool, string, any) {
	glob := cond.Glob
	if !filepath.IsAbs(glob) {
		glob = filepath.Join(filepath.Dir(s.runtime.SpecPath), glob)
	}
	timeout := timeoutFromMS(cond.TimeoutMS)
	deadline := time.Now().Add(timeout)
	var lastErr string
	for {
		matches, err := filepath.Glob(glob)
		if err != nil {
			return false, fmt.Sprintf("invalid glob %q: %v", cond.Glob, err), nil
		}
		if len(matches) > 0 {
			match := matches[0]
			if cond.Contains != "" {
				data, err := os.ReadFile(match)
				if err != nil {
					return false, fmt.Sprintf("matched %s but read failed: %v", match, err), nil
				}
				if !strings.Contains(string(data), cond.Contains) {
					return false, fmt.Sprintf("matched %s but text does not contain %q", match, cond.Contains), nil
				}
			}
			return true, fmt.Sprintf("file %q matched (%d candidate(s))", cond.Glob, len(matches)), map[string]any{
				"glob":     cond.Glob,
				"matched":  match,
				"all":      matches,
				"contains": cond.Contains,
			}
		}
		if time.Now().After(deadline) {
			return false, fmt.Sprintf("no file matching %q appeared within %s (last error: %s)", cond.Glob, timeout, lastErr), nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err().Error(), nil
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// checkScript runs the verifier's external script and parses the JSON
// `{ ok, evidence }` response. The runner pre-creates the artifact dir
// and passes the context via env (shell) or argv[2] (Node). The returned
// `evidence` is forwarded to outcomes/<id>.raw.json when non-nil.
func (s *runState) checkScript(ctx context.Context, cond spec.ScriptCondition) (bool, string, any) {
	timeout := timeoutFromMS(cond.TimeoutMS)
	if cond.File == "" && cond.Run == "" {
		return false, "script must set file or run", nil
	}

	specDir := filepath.Dir(s.runtime.SpecPath)
	resolvedFixtures := map[string]string{}
	for k, v := range cond.Fixtures {
		fv, err := s.resolveRuntimePlaceholders(v)
		if err != nil {
			return false, fmt.Sprintf("script fixture %s: %v", k, err), nil
		}
		resolvedFixtures[k] = fv
	}

	runtime := cond.Runtime
	if runtime == "" {
		runtime = "node"
	}
	if cond.Run != "" && runtime == "node" {
		// inline `run` + node is allowed; the body is the script.
	}

	var (
		scriptPath string
		inlineBody string
		cleanup    func()
	)
	if cond.File != "" {
		scriptPath = cond.File
		if !filepath.IsAbs(scriptPath) {
			scriptPath = filepath.Join(specDir, scriptPath)
		}
	} else {
		// inline `run` — write to a temp file the runner owns
		dir, err := os.MkdirTemp(s.writer.RunDir, "script-")
		if err != nil {
			return false, fmt.Sprintf("script: create temp dir: %v", err), nil
		}
		ext := ".js"
		if runtime == "shell" {
			ext = ".sh"
		}
		p := filepath.Join(dir, "inline"+ext)
		inlineBody = cond.Run
		if err := os.WriteFile(p, []byte(inlineBody), 0o644); err != nil {
			return false, fmt.Sprintf("script: write inline body: %v", err), nil
		}
		scriptPath = p
		cleanup = func() { _ = os.RemoveAll(dir) }
	}
	if cleanup != nil {
		defer cleanup()
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	env := append(os.Environ(),
		"GLYPHRUN_RUN_DIR="+s.writer.RunDir,
		"GLYPHRUN=1",
	)
	if data, err := json.Marshal(resolvedFixtures); err == nil {
		env = append(env, "GLYPHRUN_FIXTURES_JSON="+string(data))
	}

	var cmd *exec.Cmd
	switch runtime {
	case "node":
		ctxPath, err := writeVerifierContextFile(s.writer.RunDir, verifierContext{
			Input:    "",
			Fixtures: resolvedFixtures,
			RunDir:   s.writer.RunDir,
			SpecDir:  specDir,
		})
		if err != nil {
			return false, fmt.Sprintf("script: write context: %v", err), nil
		}
		cmd = exec.CommandContext(runCtx, "node", scriptPath, ctxPath)
	default: // "shell"
		cmd = exec.CommandContext(runCtx, "/bin/sh", scriptPath)
	}
	cmd.Env = env
	cmd.Dir = s.writer.RunDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			return false, fmt.Sprintf("script timed out after %s", timeout), nil
		}
		return false, fmt.Sprintf("script %s failed: %v %s", scriptPath, err, strings.TrimSpace(stderr.String())), nil
	}

	// Parse the script's stdout as JSON. We accept the cairn shape
	// (`{ ok, evidence }`); a non-JSON output is treated as a verifier
	// error so a buggy script is loud, not silent.
	rawOut := strings.TrimSpace(stdout.String())
	if rawOut == "" {
		return false, "script returned no output", nil
	}
	var result struct {
		OK       bool        `json:"ok"`
		Evidence interface{} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(rawOut), &result); err != nil {
		return false, fmt.Sprintf("script returned non-JSON output: %s", truncateScriptOutput(rawOut, 200)), nil
	}
	if !result.OK {
		message := "script verifier returned ok=false"
		if result.Evidence != nil {
			if data, err := json.Marshal(result.Evidence); err == nil {
				message += ": " + string(data)
			}
		}
		return false, message, result.Evidence
	}
	return true, "script verifier passed", result.Evidence
}

type verifierContext struct {
	Input    string            `json:"input"`
	Fixtures map[string]string `json:"fixtures"`
	RunDir   string            `json:"runDir"`
	SpecDir  string            `json:"specDir"`
}

func writeVerifierContextFile(runDir string, ctx verifierContext) (string, error) {
	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(runDir, ".glyphrun-verifier-ctx.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func truncateScriptOutput(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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
	normalize := s.normalizeConfigWith(cond.Normalize)
	expectedText := normalizePlainSnapshotText(string(expected), normalize)
	actualText := normalizeSnapshotText(current, normalize)
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
	return s.screenTextWithNormalize(s.normalizeConfig())
}

func (s *runState) screenTextWithNormalize(normalize config.Normalize) string {
	return normalizeSnapshotText(s.emulator.Screen().Snapshot(), normalize)
}

func (s *runState) normalizeSnapshot(snapshot terminal.ScreenSnapshot) terminal.ScreenSnapshot {
	snapshot.Text = normalizeSnapshotText(snapshot, s.normalizeConfig())
	return snapshot
}

func (s *runState) normalizeConfig() config.Normalize {
	return s.normalizeConfigWith(nil)
}

func (s *runState) normalizeConfigWith(overlay *spec.Normalize) config.Normalize {
	out := s.runtime.Config.Terminal.Normalize
	out.Replace = append([]spec.NormalizeReplace(nil), out.Replace...)
	out.IgnoreRegions = append([]spec.NormalizeIgnoreArea(nil), out.IgnoreRegions...)
	out = applyNormalizeOverlay(out, s.spec.Normalize)
	out = applyNormalizeOverlay(out, overlay)
	return out
}

func applyNormalizeOverlay(out config.Normalize, overlay *spec.Normalize) config.Normalize {
	if overlay == nil {
		return out
	}
	if overlay.TrimRight != nil {
		out.TrimRight = *overlay.TrimRight
	}
	if overlay.NormalizeLineEndings != nil {
		out.NormalizeLineEndings = *overlay.NormalizeLineEndings
	}
	if overlay.StripAnsiTitle != nil {
		out.StripAnsiTitle = *overlay.StripAnsiTitle
	}
	out.Replace = append(out.Replace, overlay.Replace...)
	out.IgnoreRegions = append(out.IgnoreRegions, overlay.IgnoreRegions...)
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

func normalizePlainSnapshotText(text string, normalize config.Normalize) string {
	if len(normalize.IgnoreRegions) == 0 {
		return normalizeText(text, normalize)
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for _, region := range normalize.IgnoreRegions {
		for y := region.Y; y < region.Y+region.Height && y < len(lines); y++ {
			if y < 0 {
				continue
			}
			runes := []rune(lines[y])
			for x := region.X; x < region.X+region.Width && x < len(runes); x++ {
				if x >= 0 {
					runes[x] = ' '
				}
			}
			lines[y] = string(runes)
		}
	}
	return normalizeText(strings.Join(lines, "\n"), normalize)
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
	// Start from the process env, layer the active config env, then
	// inject the runner's GLYPHRUN_RUN_DIR/GLYPHRUN flags last so they
	// win over any same-named config value. This matches the precedence
	// in startTarget (the runner's run-dir is authoritative) and lets a
	// `command:` verifier reference $GLYPHRUN_RUN_DIR / ${env.*} /
	// ${vars.*} the same way an outcome can.
	cmd.Env = os.Environ()
	for k, v := range s.runtime.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Env = append(cmd.Env,
		"GLYPHRUN_RUN_DIR="+s.writer.RunDir,
		"GLYPHRUN=1",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			return runTimeoutError{Scope: "command", Timeout: timeout, Message: fmt.Sprintf("%q timed out after %s", command, timeout)}
		}
		return fmt.Errorf("%q failed: %v %s", command, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (s *runState) cleanup() {
	s.cleanupOnce.Do(func() {
		if s.session != nil {
			_ = s.session.Cleanup(CleanupTimeout)
		}
	})
}

// writeTargetPID cooperates with a parent `monitor run <spec>`: when monitor
// launches glyphrun it exports MONITOR=1 (and optionally MONITOR_RUN_DIR).
// Glyphrun writes the spawned target's PID into its run dir (and the parent's
// MONITOR_RUN_DIR when set) so monitor can `monitor process/tree/profile
// <pid>` the exact process the spec exercises — without --monitor opt-in.
// Best-effort: no PID (Windows ConPTY) or no MONITOR env → silent no-op.
func (s *runState) writeTargetPID() {
	if os.Getenv("MONITOR") == "" || s.session == nil {
		return
	}
	pid := s.session.PID()
	if pid == 0 {
		return
	}
	_ = s.writer.WriteText("target.pid", strconv.Itoa(pid)+"\n")
	_ = s.writer.AppendEvent(event("procmon.target_pid", strconv.Itoa(pid), "cooperating with parent monitor"))
	if dir := os.Getenv("MONITOR_RUN_DIR"); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "glyphrun-target.pid"), []byte(strconv.Itoa(pid)+"\n"), 0o644)
	}
}

// procmonRun is the per-run process-telemetry state: a supervised sampling
// goroutine appending to `samples` (guarded by its own mutex) until
// stopProcmonCapture cancels it. `done` is closed when the goroutine exits so
// stopProcmonCapture can stop the reader before reducing the samples.
// hasTelemetry/hasProfile/summary are set by stopProcmonCapture (after the
// goroutine has exited) and read by applyProcmonArtifacts + procmonSummary.
type procmonRun struct {
	client       *procmon.Client
	pid          int
	name         string
	started      time.Time
	interval     time.Duration
	profile      string
	cancel       context.CancelFunc
	done         chan struct{}
	mu           sync.Mutex
	samples      []procmon.Sample
	hasTelemetry bool
	hasProfile   bool
	summary      procmon.Summary
}

// startProcmon launches the sampling goroutine for the spawned target. It is
// a no-op when procmon is disabled or the backend cannot expose a PID (Windows
// ConPTY). The single goroutine is the only new concurrency this feature
// adds; it touches only procmonRun.samples under procmonRun.mu and is
// cancelled by finalizeProcmon before the target is cleaned up.
func (s *runState) startProcmon(ctx context.Context, cfg *ProcmonConfig) {
	if cfg == nil || s.session == nil {
		return
	}
	pid := s.session.PID()
	if pid == 0 {
		return
	}
	interval := cfg.Interval
	if interval < 50*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	pctx, cancel := context.WithCancel(ctx)
	pr := &procmonRun{
		client:   &procmon.Client{Bin: cfg.Bin},
		pid:      pid,
		started:  time.Now().UTC(),
		interval: interval,
		profile:  strings.TrimSpace(cfg.Profile),
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	s.procmon = pr
	go func() {
		defer close(pr.done)
		ticker := time.NewTicker(pr.interval)
		defer ticker.Stop()
		var failures int
		for {
			select {
			case <-pctx.Done():
				return
			case <-ticker.C:
				info, err := pr.client.Process(pr.pid)
				if err != nil {
					failures++
					// monitor missing or target gone: stop after a short
					// run of failures so we don't shell out forever.
					if failures >= 3 {
						return
					}
					continue
				}
				failures = 0
				pr.mu.Lock()
				if pr.name == "" {
					pr.name = info.Name
				}
				pr.samples = append(pr.samples, procmon.Sample{
					At:      time.Now().UTC(),
					CPU:     info.CPUPercent,
					RSS:     info.Memory,
					Threads: info.Threads,
				})
				pr.mu.Unlock()
			}
		}
	}()
}

// finalizeProcmon stops the sampler, captures the process tree (and an
// optional profile) while the target may still be alive, reduces the samples
// to a Summary, and writes `diagnostics/process.md` + `process.json`
// artifacts. It must run BEFORE cleanup() tears the target down. Best-effort:
// a missing monitor or a dead target yields a zero-sample summary with a note,
// never a run failure.
func (s *runState) stopProcmonCapture() {
	pr := s.procmon
	if pr == nil {
		return
	}
	pr.cancel()
	select {
	case <-pr.done:
	case <-time.After(2 * time.Second):
	}
	pr.mu.Lock()
	samples := append([]procmon.Sample(nil), pr.samples...)
	name := pr.name
	pr.mu.Unlock()

	summary := procmon.Summarize(pr.pid, name, pr.started, samples)
	summary.Samples = samples // keep the timeline in the JSON artifact

	// Capture the process tree while the target may still be alive (cleanup
	// runs after this). A dead/exited target returns an empty tree — fine.
	var tree string
	if t, err := pr.client.TreeText(pr.pid); err == nil {
		tree = t
		if strings.TrimSpace(tree) != "" {
			_ = s.writer.WriteArtifactBytes("diagnostics/process.tree.txt", []byte(tree))
			summary.Note = "process tree captured"
		}
	}
	_ = s.writer.WriteDiagnostic("process", procmon.RenderProcessMarkdown(summary, tree))
	if data, err := json.MarshalIndent(summary, "", "  "); err == nil {
		_ = s.writer.WriteArtifactBytes("diagnostics/process.json", append(data, '\n'))
	}
	pr.hasTelemetry = true

	// Optional end-of-run profile (heap/cpu/goroutine/sample), stored as raw
	// monitor JSON. Captured here while the target may still be alive.
	if pr.profile != "" {
		if prof, err := pr.client.Profile(pr.pid, pr.profile); err == nil {
			if data, err := json.MarshalIndent(prof, "", "  "); err == nil {
				_ = s.writer.WriteArtifactBytes("diagnostics/process.profile.json", append(data, '\n'))
				pr.hasProfile = true
			}
		}
	}
	pr.summary = summary
	_ = s.writer.AppendEvent(event("procmon.finalized", name, fmt.Sprintf("%d samples, peak cpu %.1f%%, peak rss %d", summary.SampleCount, summary.PeakCPU, summary.PeakRSS)))
}

// applyProcmonArtifacts surfaces the captured process-telemetry files on the
// run result so `glyph context` / the markdown report list them. Called after
// the result literal is built; the files themselves were written by
// stopProcmonCapture (before cleanup).
func (s *runState) applyProcmonArtifacts(result *artifacts.RunResult) {
	pr := s.procmon
	if pr == nil || !pr.hasTelemetry {
		return
	}
	result.Artifacts["processTelemetry"] = "diagnostics/process.md"
	result.Artifacts["processTelemetryJSON"] = "diagnostics/process.json"
	if pr.hasProfile {
		result.Artifacts["processProfile"] = "diagnostics/process.profile.json"
	}
}

// procmonSummary returns the sampled summary for the metrics verifier, or a
// zero Summary when process telemetry was not enabled. The verifier uses this
// to assert perf budgets (peakRss/peakCpu) without re-sampling.
func (s *runState) procmonSummary() procmon.Summary {
	pr := s.procmon
	if pr == nil {
		return procmon.Summary{}
	}
	pr.mu.Lock()
	samples := append([]procmon.Sample(nil), pr.samples...)
	name := pr.name
	pr.mu.Unlock()
	return procmon.Summarize(pr.pid, name, pr.started, samples)
}

func (s *runState) terminalError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.termErr
}

func (s *runState) terminalPolicyFailure() string {
	mode := strings.ToLower(strings.TrimSpace(s.spec.Terminal.AlternateScreen))
	if mode == "" || mode == "auto" {
		return ""
	}
	_, used := alternateScreenState(s.emulator)
	switch mode {
	case "require":
		if !used {
			return "terminal.alternateScreen=require but target never entered alternate screen mode"
		}
	case "forbid":
		if used {
			return "terminal.alternateScreen=forbid but target entered alternate screen mode"
		}
	}
	return ""
}

// archiveConfigFromConfig translates the config-layer archive block
// into the artifacts-layer ArchiveConfig. The artifacts package must
// not import internal/config, so the runner owns the boundary. A
// timeout string that fails to parse is dropped (default applies).
func archiveConfigFromConfig(c config.ArchiveConfig) artifacts.ArchiveConfig {
	out := artifacts.ArchiveConfig{
		Enabled: c.Enabled,
		Command: c.Command,
		Args:    c.Args,
	}
	if d, err := artifacts.ParseArchiveTimeout(c.Timeout); err == nil {
		out.Timeout = d
	}
	return out
}

func (s *runState) finish(started time.Time, status artifacts.RunStatus, outcomes []artifacts.OutcomeResult, diagnostic string, errorKind artifacts.ErrorKind, exitCodeOverride ...int) artifacts.RunResult {
	if outcomes == nil {
		outcomes = []artifacts.OutcomeResult{}
	}
	s.stopProcmonCapture() // capture tree/profile while target may be alive, before cleanup
	s.cleanup()
	finalSnapshot := s.normalizeSnapshot(s.emulator.Screen().Snapshot())
	ended := time.Now().UTC()
	exitCode := exitPassed
	if status == artifacts.StatusFailed {
		exitCode = exitFailed
	}
	if status == artifacts.StatusErrored {
		exitCode = exitErrored
	}
	if len(exitCodeOverride) > 0 && exitCodeOverride[0] != 0 {
		exitCode = exitCodeOverride[0]
	}
	result := artifacts.RunResult{
		SchemaVersion: 1,
		RunID:         makeRunID(started, s.spec.Name),
		SpecName:      s.spec.Name,
		Intent:        strings.TrimSpace(s.spec.Intent),
		ContractHash:  s.spec.ContractHash,
		Metadata:      s.spec.Metadata,
		CoversSymbol:  s.spec.CoversSymbol,
		Status:        status,
		ErrorKind:     errorKind,
		Diagnostic:    diagnostic,
		StartedAt:     started.Format(time.RFC3339Nano),
		EndedAt:       ended.Format(time.RFC3339Nano),
		DurationMS:    ended.Sub(started).Milliseconds(),
		Target:        s.spec.Target,
		Terminal:      s.spec.Terminal,
		Outcomes:      outcomes,
		RunDir:        s.writer.RunDir,
		ExitCode:      exitCode,
		Artifacts:     map[string]string{"events": "events.ndjson", "environmentDiagnostic": "diagnostics/environment.md"},
	}
	result.NextActions = artifacts.NextActionsFor(errorKind, s.spec.Name, s.spec.ContractHash, "")
	result.Steps = s.stepResults
	s.applyProcmonArtifacts(&result)
	// Use the policy the runner resolved at start (project config +
	// spec override). It's in s.capturePolicy so the finish() function
	// doesn't have to re-resolve.
	policy := s.capturePolicy
	if shouldCapture(policy.AgentContext, status) {
		result.Artifacts["agentContext"] = "agent_context.md"
	}
	if shouldCapture(policy.FinalScreen, status) {
		result.Artifacts["finalScreenText"] = "screens/final.txt"
		result.Artifacts["finalScreenJSON"] = "screens/final.json"
		result.Artifacts["finalScreenSVG"] = "screens/final.svg"
	}
	if shouldCapture(policy.Frames, status) {
		result.Artifacts["frames"] = "frames/frames.ndjson"
	}
	if shouldCapture(policy.RawLog, status) {
		result.Artifacts["rawPtyLog"] = "raw/pty.raw.log"
		result.Artifacts["inputRawLog"] = "raw/input.raw.log"
	}
	if shouldCapture(policy.Snapshots, status) {
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
	// Surface named artifacts in the run result so agents reading run.json
	// can resolve ${artifacts.X.path} / .relativePath without scanning the
	// artifact map. A separate map keeps the artifact index readable.
	s.mu.Lock()
	named := make(map[string]artifacts.NamedArtifact, len(s.namedArtifacts))
	names := make([]string, 0, len(s.namedArtifacts))
	for name, art := range s.namedArtifacts {
		named[name] = art
		names = append(names, name)
	}
	s.mu.Unlock()
	if len(named) > 0 {
		sort.Strings(names)
		result.NamedArtifacts = named
		for _, name := range names {
			art := named[name]
			result.Artifacts["artifact:"+name] = art.RelativePath
		}
	}
	if diagnostic != "" {
		result.Artifacts["failureDiagnostic"] = "diagnostics/failure.md"
		_ = s.writer.WriteDiagnostic("failure", "## Failure\n\n"+diagnostic+"\n\n## Final Screen\n\n```text\n"+finalSnapshot.Text+"\n```\n")
	} else if status == artifacts.StatusFailed {
		result.Artifacts["failureDiagnostic"] = "diagnostics/failure.md"
		_ = s.writer.WriteDiagnostic("failure", renderOutcomeFailureDiagnostic(outcomes, finalSnapshot.Text))
	}
	if shouldCapture(policy.FinalScreen, status) {
		_ = s.writer.WriteFinalScreen(finalSnapshot)
		// A deterministic SVG of the final screen rides alongside the text
		// and JSON forms: it's what humans see in a PR comment and what a
		// multimodal agent can read directly.
		_ = s.writer.WriteScreenSVG("screens/final.svg", render.SnapshotSVG(finalSnapshot, render.DefaultOptions()))
	}
	s.mu.Lock()
	rawPTY := append([]byte(nil), s.rawPTY...)
	inputLog := append([]byte(nil), s.inputLog...)
	frames := append([]terminal.Frame(nil), s.frames...)
	rawPTYTruncated := s.rawPTYTruncated
	s.mu.Unlock()
	if rawPTYTruncated {
		max := s.runtime.Config.Artifacts.MaxRawLogBytes
		marker := fmt.Sprintf("\n[glyphrun: raw PTY log truncated at %d bytes; later output was dropped]\n", max)
		rawPTY = append(rawPTY, []byte(marker)...)
		_ = s.writer.AppendEvent(event("pty.truncated", "", fmt.Sprintf("raw PTY log truncated at %d bytes", max)))
		log.Warn("raw PTY log truncated", "max", max)
	}
	if shouldCapture(policy.RawLog, status) {
		_ = s.writer.WriteRawPTY(rawPTY)
		_ = s.writer.WriteInputLog(inputLog)
	}
	if shouldCapture(policy.Frames, status) {
		_ = s.writer.WriteFrames(frames)
	}
	_ = s.writer.WriteDiagnostic("environment", renderEnvironmentDiagnostic(s.runtime, s.spec, result))
	if status == artifacts.StatusPassed {
		_ = s.writer.AppendEvent(event("run.passed", s.spec.Name, ""))
	} else if status == artifacts.StatusFailed {
		_ = s.writer.AppendEvent(event("run.failed", s.spec.Name, diagnostic))
	} else {
		_ = s.writer.AppendEvent(event("run.errored", s.spec.Name, diagnostic))
	}
	if shouldCapture(policy.AgentContext, status) {
		_ = s.writer.WriteAgentContext(s.spec, result, finalSnapshot.Text, s.writer.RecentEvents(12))
	}
	_ = s.writer.WriteOutcomesIndex(result)
	// Retention: auto-prune older runs when configured. Best-effort —
	// a prune failure is logged as an event so the agent context
	// surfaces it, but never fails the run. (See cairn's retention
	// pattern: a passing run that bombs on disk cleanup is a worse
	// surprise than a passing run that leaves a few extra runs.)
	artifactRoot := ""
	if s.writer != nil {
		artifactRoot = filepath.Dir(s.writer.RunDir)
	}
	if keepRuns := s.runtime.Config.Retention.KeepRuns; keepRuns > 0 && artifactRoot != "" {
		archive := archiveConfigFromConfig(s.runtime.Config.Retention.Archive)
		if report, pruneErr := artifacts.PruneRuns(artifactRoot, keepRuns, archive); pruneErr != nil {
			_ = s.writer.AppendEvent(event("retention.error", "", pruneErr.Error()))
			log.Warn("retention prune failed", "err", pruneErr)
		} else {
			if report.Pruned > 0 {
				_ = s.writer.AppendEvent(event("retention.pruned", "", fmt.Sprintf("pruned %d, kept %d", report.Pruned, report.Kept)))
				log.Debug("retention pruned", "pruned", report.Pruned, "kept", report.Kept)
			}
			if report.Archived > 0 {
				_ = s.writer.AppendEvent(event("retention.archived", "", fmt.Sprintf("archived %d run dir(s) to %s", report.Archived, archive.Command)))
				log.Info("retention archived", "archived", report.Archived, "command", archive.Command)
			}
			for _, m := range report.ArchiveErrors {
				_ = s.writer.AppendEvent(event("retention.archive.error", "", m))
				log.Warn("retention archive error", "message", m)
			}
		}
	}
	// Last-failed tracking: write this spec's name to .last-failed.txt
	// at the artifact root when the run didn't pass. The list is
	// rebuilt (not appended) by the next run, so a spec that
	// subsequently passes drops off the list automatically.
	if artifactRoot != "" {
		existing, _ := artifacts.ReadLastFailed(artifactRoot)
		switch status {
		case artifacts.StatusFailed, artifacts.StatusErrored:
			// Add (or keep) the failing name.
			existing = append(existing, s.spec.Name)
			if err := artifacts.WriteLastFailed(artifactRoot, existing); err != nil {
				_ = s.writer.AppendEvent(event("lastfailed.error", "", err.Error()))
			}
		case artifacts.StatusPassed:
			// Drop the passing name so `--rerun-failed` doesn't keep
			// replaying a now-passing spec.
			filtered := existing[:0]
			for _, n := range existing {
				if n != s.spec.Name {
					filtered = append(filtered, n)
				}
			}
			if err := artifacts.WriteLastFailed(artifactRoot, filtered); err != nil {
				_ = s.writer.AppendEvent(event("lastfailed.error", "", err.Error()))
			}
		}
	}
	// Exact-replay manifest (SPEC §7.3): replay.json captures everything an
	// agent needs to reproduce the run without re-reading the resolved spec.
	// Env values are never included — only key names — and the writer redacts.
	replay := artifacts.BuildReplayManifest(s.spec, s.capturePolicy, s.runtime.Env, s.runtime.SpecPath, result.RunID, version.Version, version.Commit, version.BuildDate)
	replay.GeneratedAt = ended.Format(time.RFC3339Nano)
	if err := s.writer.WriteReplay(replay); err == nil {
		result.Artifacts["replay"] = "replay.json"
	}
	_ = s.writer.FinalizeManifest(&result)
	_ = s.writer.WriteRun(result)
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

func renderEnvironmentDiagnostic(rt config.Runtime, s spec.Spec, result artifacts.RunResult) string {
	var b strings.Builder
	b.WriteString("## Environment\n\n")
	fmt.Fprintf(&b, "- project root: `%s`\n", rt.ProjectRoot)
	if rt.ConfigPath != "" {
		fmt.Fprintf(&b, "- config: `%s`\n", rt.ConfigPath)
	}
	if rt.Environment != "" {
		fmt.Fprintf(&b, "- environment: `%s`\n", rt.Environment)
	}
	fmt.Fprintf(&b, "- artifact root: `%s`\n", rt.Config.ArtifactRoot)
	fmt.Fprintf(&b, "- snapshot root: `%s`\n", rt.Config.SnapshotRoot)
	fmt.Fprintf(&b, "- run dir: `%s`\n", result.RunDir)
	b.WriteString("\n## Target\n\n")
	fmt.Fprintf(&b, "- command: `%s`\n", strings.Join(s.Target.Cmd, " "))
	fmt.Fprintf(&b, "- cwd: `%s`\n", resolveProjectPath(rt.ProjectRoot, s.Target.Cwd))
	if s.Target.TimeoutMS > 0 {
		fmt.Fprintf(&b, "- timeout: %dms\n", s.Target.TimeoutMS)
	}
	if len(s.Target.Env) > 0 {
		fmt.Fprintf(&b, "- target env overrides: %d keys\n", len(s.Target.Env))
	}
	if rt.Secrets != nil {
		source := rt.Secrets.Project
		if rt.Secrets.Group != "" {
			source = rt.Secrets.Group + "/" + rt.Secrets.Env
		}
		fmt.Fprintf(&b, "- secrets: resolved from %s\n", source)
	}
	b.WriteString("\n## Terminal\n\n")
	fmt.Fprintf(&b, "- size: %dx%d\n", s.Terminal.Cols, s.Terminal.Rows)
	fmt.Fprintf(&b, "- profile: `%s`\n", s.Terminal.Profile)
	if s.Terminal.Color != "" {
		fmt.Fprintf(&b, "- color: `%s`\n", s.Terminal.Color)
	}
	if s.Terminal.AlternateScreen != "" {
		fmt.Fprintf(&b, "- alternate screen: `%s`\n", s.Terminal.AlternateScreen)
	}
	b.WriteString("\n## Artifacts\n\n")
	for _, key := range []string{"agentContext", "failureDiagnostic", "finalScreenText", "finalScreenJSON", "events", "frames", "rawPtyLog", "inputRawLog"} {
		if path := result.Artifacts[key]; path != "" {
			fmt.Fprintf(&b, "- %s: `%s`\n", key, path)
		}
	}
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

// checkCount asserts the count of cells in a region. The default
// region is the full screen. The matcher picks which cells to count:
// "nonEmpty" (default) counts non-blank cells, a single rune in
// `matches` counts cells equal to that rune. The comparator is
// exactly one of `equals` / `atLeast` / `atMost` / `between`. The
// terminal-shaped sibling of cairn's `count:` verifier: cairn counts
// DOM nodes by role, glyphrun counts cells by rune. Both feed the
// same insight — "exactly N error rows must be visible after the
// action" — using the model their runner already exposes.
func (s *runState) checkCount(screen terminal.Screen, cond spec.CountCondition) (bool, string, any) {
	var cells []terminal.Cell
	if cond.Region != nil {
		cells = screen.Region(cond.Region.X, cond.Region.Y, cond.Region.Width, cond.Region.Height).Cells()
	} else {
		size := screen.Size()
		cells = screen.Region(0, 0, size.Cols, size.Rows).Cells()
	}
	var matched int
	switch {
	case cond.Matches == "" || cond.Matches == "nonEmpty":
		for _, c := range cells {
			if c.Char != "" && c.Char != " " {
				matched++
			}
		}
	case len(cond.Matches) == 1:
		needle := cond.Matches
		for _, c := range cells {
			if c.Char == needle {
				matched++
			}
		}
	default:
		return false, fmt.Sprintf("count.matches must be a single character or \"nonEmpty\", got %q", cond.Matches), nil
	}
	comps := 0
	if cond.Equals != nil {
		comps++
		if matched != *cond.Equals {
			return false, fmt.Sprintf("expected exactly %d matched cells, got %d", *cond.Equals, matched), map[string]any{"matched": matched, "comparator": "equals", "expected": *cond.Equals}
		}
	}
	if cond.AtLeast != nil {
		comps++
		if matched < *cond.AtLeast {
			return false, fmt.Sprintf("expected at least %d matched cells, got %d", *cond.AtLeast, matched), map[string]any{"matched": matched, "comparator": "atLeast", "expected": *cond.AtLeast}
		}
	}
	if cond.AtMost != nil {
		comps++
		if matched > *cond.AtMost {
			return false, fmt.Sprintf("expected at most %d matched cells, got %d", *cond.AtMost, matched), map[string]any{"matched": matched, "comparator": "atMost", "expected": *cond.AtMost}
		}
	}
	if cond.Between != nil {
		comps++
		if matched < cond.Between[0] || matched > cond.Between[1] {
			return false, fmt.Sprintf("expected between %d and %d matched cells, got %d", cond.Between[0], cond.Between[1], matched), map[string]any{"matched": matched, "comparator": "between", "expected": cond.Between}
		}
	}
	if comps == 0 {
		return false, "count condition has no comparator (equals / atLeast / atMost / between)", nil
	}
	if comps > 1 {
		return false, "count condition has multiple comparators; pick exactly one of equals / atLeast / atMost / between", nil
	}
	return true, fmt.Sprintf("count matched: %d cells", matched), map[string]any{"matched": matched, "comparator": "passed", "expected": matched}
}

// checkLink asserts that an OSC 8 hyperlink is present on the screen. It groups
// contiguous cells sharing a link URI into spans, then matches `url` against the
// URI (substring) and the optional `text` against the linked text (substring).
// With neither set, any hyperlink satisfies the check.
func checkLink(screen terminal.Screen, cond spec.LinkCondition) (bool, string, any) {
	size := screen.Size()
	cells := screen.Region(0, 0, size.Cols, size.Rows).Cells()
	type linkSpan struct {
		url  string
		text string
	}
	var spans []linkSpan
	curIdx := -1
	for _, c := range cells {
		if c.Link == "" {
			curIdx = -1
			continue
		}
		if curIdx < 0 || spans[curIdx].url != c.Link {
			spans = append(spans, linkSpan{url: c.Link})
			curIdx = len(spans) - 1
		}
		ch := c.Char
		if ch == "" {
			ch = " "
		}
		spans[curIdx].text += ch
	}
	if len(spans) == 0 {
		return false, "no hyperlinks found on screen", nil
	}
	for _, sp := range spans {
		text := strings.TrimSpace(sp.text)
		if cond.URL != "" && !strings.Contains(sp.url, cond.URL) {
			continue
		}
		if cond.Text != "" && !strings.Contains(text, cond.Text) {
			continue
		}
		return true, fmt.Sprintf("hyperlink matched: %q -> %q", text, sp.url),
			map[string]any{"url": sp.url, "text": text}
	}
	found := make([]map[string]string, 0, len(spans))
	for _, sp := range spans {
		found = append(found, map[string]string{"url": sp.url, "text": strings.TrimSpace(sp.text)})
	}
	return false, fmt.Sprintf("no hyperlink matched url=%q text=%q (found %d link(s))", cond.URL, cond.Text, len(spans)),
		map[string]any{"found": found}
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

func exitCodeForError(err error) int {
	var timeoutErr runTimeoutError
	if errors.As(err, &timeoutErr) || errors.Is(err, context.DeadlineExceeded) {
		return exitTimedOut
	}
	var terminalErr unsupportedTerminalError
	if errors.As(err, &terminalErr) {
		return exitUnsupportedTerminal
	}
	return exitErrored
}

func targetTimeoutMessage(timeoutMS int) string {
	if timeoutMS <= 0 {
		return context.DeadlineExceeded.Error()
	}
	return fmt.Sprintf("target timed out after %s", time.Duration(timeoutMS)*time.Millisecond)
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

func mouseSGREnabled(emulator terminal.Emulator) bool {
	type modeReporter interface {
		MouseSGRMode() bool
	}
	reporter, ok := emulator.(modeReporter)
	return ok && reporter.MouseSGRMode()
}

func bracketedPasteEnabled(emulator terminal.Emulator) bool {
	type modeReporter interface {
		BracketedPasteMode() bool
	}
	reporter, ok := emulator.(modeReporter)
	return ok && reporter.BracketedPasteMode()
}

func alternateScreenState(emulator terminal.Emulator) (active bool, used bool) {
	type modeReporter interface {
		AlternateScreenMode() bool
		AlternateScreenUsed() bool
	}
	reporter, ok := emulator.(modeReporter)
	if !ok {
		return false, false
	}
	return reporter.AlternateScreenMode(), reporter.AlternateScreenUsed()
}

func event(kind string, name string, info string) artifacts.Event {
	return artifacts.Event{TS: time.Now().UTC().Format(time.RFC3339Nano), Type: kind, Name: name, Info: info}
}

// artifactNameFromPath produces a stable, snake_case-ish assign name for
// unnamed download/transform steps. Empty input falls back to "artifact".
func artifactNameFromPath(p string) string {
	base := filepath.Base(p)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	name = strings.ToLower(name)
	if name == "" || name == "." || name == "/" {
		return "artifact"
	}
	var b strings.Builder
	lastWasUnderscore := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastWasUnderscore = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32) // lowercase without importing strings just for this
			lastWasUnderscore = false
		default:
			if !lastWasUnderscore {
				b.WriteByte('_')
				lastWasUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "artifact"
	}
	// Assign names must start with a letter (see validArtifactAssign).
	if out[0] >= '0' && out[0] <= '9' {
		out = "a_" + out
	}
	return out
}

func describeStep(step spec.Step) string {
	data, err := json.Marshal(step)
	if err != nil {
		return ""
	}
	return string(data)
}

// stepKind returns the type name of a step (wait, type, press, etc.) by
// checking which field is set. Used for the structured StepResult (SPEC §7.3).
func stepKind(step spec.Step) string {
	switch {
	case step.Wait != nil:
		return "wait"
	case step.Press != "":
		return "press"
	case step.Type != "":
		return "type"
	case step.Mouse != nil:
		return "mouse"
	case step.Send != nil:
		return "send"
	case step.Resize != nil:
		return "resize"
	case step.Snapshot != "":
		return "snapshot"
	case step.Use != "":
		return "use"
	case step.Download != nil:
		return "download"
	case step.Transform != nil:
		return "transform"
	case step.Monitor != nil:
		return "monitor"
	case step.Batch != nil:
		return "batch"
	default:
		return "unknown"
	}
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
