package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

func TestRunSpecShellPTY(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "shell.yml")
	err := os.WriteFile(specPath, []byte(`version: 1
name: shell_quits
intent: a shell target prints ready and exits after q.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\n'; while IFS= read -r line; do if [ \"$line\" = q ]; then exit 0; fi; done"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 2000
  - type: "q"
  - press: "enter"
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: ready_visible
    description: ready is visible
    verify:
      screen:
        contains: "ready"
  - id: clean_exit
    description: process exits
    verify:
      process:
        exitCode: 0
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	if _, err := os.Stat(filepath.Join(result.RunDir, "agent_context.md")); err != nil {
		t.Fatal(err)
	}
	contextData, err := os.ReadFile(filepath.Join(result.RunDir, "agent_context.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contextData), "## Recent Events") {
		t.Fatalf("agent context missing recent events:\n%s", string(contextData))
	}
	if result.Artifacts["environmentDiagnostic"] != "diagnostics/environment.md" {
		t.Fatalf("environment diagnostic artifact missing: %#v", result.Artifacts)
	}
}

func TestRunSpecCapturesFastProcessOutput(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "fast.yml")
	err := os.WriteFile(specPath, []byte(`version: 1
name: fast_output
intent: a very short-lived target prints output before exiting.
target:
  cmd: ["/bin/sh", "-lc", "printf dev"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "dev"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: output_visible
    description: output from a fast process is captured
    verify:
      screen:
        contains: "dev"
  - id: clean_exit
    description: process exits
    verify:
      process:
        exitCode: 0
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	raw, err := os.ReadFile(filepath.Join(result.RunDir, "raw/pty.raw.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "dev") {
		t.Fatalf("raw PTY log missing fast output: %q", string(raw))
	}
}

// TestRunSpecTargetExitDuringScreenWait guards that an already-dead target
// during wait.screen is classified as target_exited (not timeout), finishes
// promptly, and still succeeds when the expected text arrives before exit.
func TestRunSpecTargetExitDuringScreenWait(t *testing.T) {
	type want struct {
		status       artifacts.RunStatus
		errorKind    artifacts.ErrorKind
		exitCode     int // Glyphrun runner exit code
		targetCode   int
		lastPtySub   string
		maxDuration  time.Duration // 0 = no bound
		diagnosticIn string
	}
	cases := []struct {
		name    string
		shell   string
		waitFor string
		timeout int
		redact  string
		want    want
	}{
		{
			name:    "nonzero early exit",
			shell:   `printf 'listen tcp 127.0.0.1:0: bind: operation not permitted\n'; exit 3`,
			waitFor: "ready",
			timeout: 8000,
			want: want{
				status:       artifacts.StatusErrored,
				errorKind:    artifacts.ErrorKindTargetExited,
				exitCode:     2,
				targetCode:   3,
				lastPtySub:   "bind: operation not permitted",
				maxDuration:  3 * time.Second,
				diagnosticIn: "target exited with code 3",
			},
		},
		{
			name:    "output before exit succeeds",
			shell:   `printf 'ready\n'; exit 0`,
			waitFor: "ready",
			timeout: 2000,
			want: want{
				status:   artifacts.StatusPassed,
				exitCode: 0,
			},
		},
		{
			name:    "clean early exit still target_exited",
			shell:   `printf 'unrelated output\n'; exit 0`,
			waitFor: "ready",
			timeout: 8000,
			want: want{
				status:       artifacts.StatusErrored,
				errorKind:    artifacts.ErrorKindTargetExited,
				exitCode:     2,
				targetCode:   0,
				lastPtySub:   "unrelated output",
				maxDuration:  3 * time.Second,
				diagnosticIn: "target exited with code 0",
			},
		},
		{
			name:    "real wait timeout",
			shell:   `printf 'still here\n'; sleep 30`,
			waitFor: "never_appears",
			timeout: 400,
			want: want{
				status:       artifacts.StatusErrored,
				errorKind:    artifacts.ErrorKindTimeout,
				exitCode:     3,
				diagnosticIn: "timed out",
			},
		},
		{
			name:    "lastPtyLine is redacted",
			shell:   `printf 'secret-token-xyz leaked\n'; exit 7`,
			waitFor: "ready",
			timeout: 8000,
			redact:  "secret-token-xyz",
			want: want{
				status:       artifacts.StatusErrored,
				errorKind:    artifacts.ErrorKindTargetExited,
				exitCode:     2,
				targetCode:   7,
				lastPtySub:   "[redacted]",
				maxDuration:  3 * time.Second,
				diagnosticIn: "target exited with code 7",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			specPath := filepath.Join(dir, "spec.yml")
			// Escape shell for YAML double-quoted embedding via JSON string form
			// in the cmd array — write via a small structured builder instead.
			redactionBlock := ""
			if tc.redact != "" {
				redactionBlock = "redaction:\n  values:\n    - " + tc.redact + "\n"
			}
			// Process wait after screen wait only when we expect success, so
			// target-exit cases don't need a second step.
			processStep := ""
			outcome := `  - id: placeholder
    description: placeholder
    verify:
      command:
        run: "true"
`
			if tc.want.status == artifacts.StatusPassed {
				processStep = `
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000`
				outcome = `  - id: ready_visible
    description: ready is visible
    verify:
      screen:
        contains: "ready"
`
			}
			body := fmt.Sprintf(`version: 1
name: target_exit_%s
intent: screen wait observes target exit classification.
%s
target:
  cmd: ["/bin/sh", "-lc", %q]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: %q
      timeoutMs: %d%s
outcomes:
%s`, strings.ReplaceAll(tc.name, " ", "_"), redactionBlock, tc.shell, tc.waitFor, tc.timeout, processStep, outcome)
			if err := os.WriteFile(specPath, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			result, err := RunSpec(context.Background(), Options{
				SpecPath:     specPath,
				ArtifactRoot: filepath.Join(dir, "runs"),
			})
			if err != nil {
				t.Fatal(err)
			}
			elapsed := time.Since(start)
			if result.Status != tc.want.status {
				t.Fatalf("status = %s, errorKind = %s, diagnostic = %q", result.Status, result.ErrorKind, result.Diagnostic)
			}
			if result.ErrorKind != tc.want.errorKind {
				t.Fatalf("errorKind = %q, want %q; diagnostic = %q", result.ErrorKind, tc.want.errorKind, result.Diagnostic)
			}
			if result.ExitCode != tc.want.exitCode {
				t.Fatalf("exitCode = %d, want %d", result.ExitCode, tc.want.exitCode)
			}
			if tc.want.diagnosticIn != "" && !strings.Contains(result.Diagnostic, tc.want.diagnosticIn) {
				t.Fatalf("diagnostic missing %q: %q", tc.want.diagnosticIn, result.Diagnostic)
			}
			if tc.want.errorKind == artifacts.ErrorKindTargetExited {
				if result.TargetExit == nil {
					t.Fatalf("expected targetExit field, got nil; result = %#v", result)
				}
				if result.TargetExit.ExitCode != tc.want.targetCode {
					t.Fatalf("targetExit.exitCode = %d, want %d", result.TargetExit.ExitCode, tc.want.targetCode)
				}
				if tc.want.lastPtySub != "" && !strings.Contains(result.TargetExit.LastPtyLine, tc.want.lastPtySub) {
					t.Fatalf("lastPtyLine = %q, want substring %q", result.TargetExit.LastPtyLine, tc.want.lastPtySub)
				}
				if tc.redact != "" && strings.Contains(result.TargetExit.LastPtyLine, tc.redact) {
					t.Fatalf("lastPtyLine still contains secret %q: %q", tc.redact, result.TargetExit.LastPtyLine)
				}
				if tc.redact != "" && strings.Contains(result.Diagnostic, tc.redact) {
					t.Fatalf("diagnostic still contains secret %q: %q", tc.redact, result.Diagnostic)
				}
				// Next actions must not recommend raising timeouts (timeout errorKind does).
				for _, na := range result.NextActions {
					if strings.Contains(na.Reason, "raise timeoutMs") || strings.Contains(na.Reason, "raise timeout") {
						t.Fatalf("nextActions must not suggest raising timeouts: %+v", result.NextActions)
					}
				}
				if len(result.NextActions) == 0 {
					t.Fatal("expected nextActions for target_exited")
				}
			} else if result.TargetExit != nil {
				t.Fatalf("targetExit should be nil for non-exit classification, got %+v", result.TargetExit)
			}
			if tc.want.maxDuration > 0 && elapsed > tc.want.maxDuration {
				t.Fatalf("elapsed %s exceeds max %s (should finish promptly on target exit)", elapsed, tc.want.maxDuration)
			}
		})
	}
}

func TestRunSpecTargetExitScreenWaitCancellation(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "cancel.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: cancel_during_wait
intent: cancellation during screen wait keeps cancellation semantics.
target:
  cmd: ["/bin/sh", "-lc", "printf 'still here\n'; sleep 30"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "never_appears"
      timeoutMs: 10000
outcomes:
  - id: placeholder
    description: placeholder
    verify:
      command:
        run: "true"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel shortly after start so we hit the wait select on ctx.Done.
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	result, err := RunSpec(ctx, Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Cancellation surfaces as a step failure / errored run, not target_exited.
	if result.ErrorKind == artifacts.ErrorKindTargetExited {
		t.Fatalf("cancellation must not classify as target_exited: %#v", result)
	}
	if result.Status == artifacts.StatusPassed {
		t.Fatalf("expected non-passed status on cancel, got %s", result.Status)
	}
}

func TestRunSpecSkipsConditionalStep(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "conditional.yml")
	err := os.WriteFile(specPath, []byte(`version: 1
name: conditional_skip
intent: a conditional step is skipped when its guard is false.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\n'; sleep 0.1"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 2000
  - when:
      screen:
        contains: "missing"
    type: "SHOULD_NOT_RUN"
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: ready_visible
    description: ready is visible
    verify:
      screen:
        contains: "ready"
  - id: clean_exit
    description: process exits
    verify:
      process:
        exitCode: 0
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	input, err := os.ReadFile(filepath.Join(result.RunDir, "raw/input.raw.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(input) != "" {
		t.Fatalf("conditional input should have been skipped, got %q", string(input))
	}
	events, err := os.ReadFile(filepath.Join(result.RunDir, "events.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(events), `"type":"step.skipped"`) {
		t.Fatalf("events missing step.skipped:\n%s", string(events))
	}
}

func TestRunSpecUpdatesAndComparesSnapshots(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "glyphrun.config.yml")
	if err := os.WriteFile(configPath, []byte(`version: 1
artifactRoot: runs
snapshotRoot: snapshots
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
`), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "snapshot.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: snapshot_demo
intent: a target prints ready and the screen snapshot is stable.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\n'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 2000
  - snapshot: home
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: home_snapshot
    description: the home snapshot matches the committed snapshot
    verify:
      snapshot:
        name: home
        mode: text
`), 0o644); err != nil {
		t.Fatal(err)
	}

	updated, err := RunSpec(context.Background(), Options{
		SpecPath:        specPath,
		ConfigPath:      configPath,
		UpdateSnapshots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != artifacts.StatusPassed {
		t.Fatalf("update status = %s, outcomes = %#v", updated.Status, updated.Outcomes)
	}
	if _, err := os.Stat(filepath.Join(dir, "snapshots", "snapshot_demo", "home.txt")); err != nil {
		t.Fatal(err)
	}

	checked, err := RunSpec(context.Background(), Options{
		SpecPath:   specPath,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked.Status != artifacts.StatusPassed {
		t.Fatalf("compare status = %s, outcomes = %#v", checked.Status, checked.Outcomes)
	}
	if checked.Artifacts["snapshot:home"] != "snapshots/home.txt" {
		t.Fatalf("snapshot artifact missing: %#v", checked.Artifacts)
	}
}

func TestRunSpecHonorsArtifactFlagsAndNormalization(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "glyphrun.config.yml")
	if err := os.WriteFile(configPath, []byte(`version: 1
artifactRoot: runs
snapshotRoot: snapshots
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
  normalize:
    trimRight: true
    normalizeLineEndings: true
    replace:
      - regex: "run-[0-9]+"
        with: "run-<id>"
artifacts:
  rawLog: false
  frames: false
  finalScreen: false
  snapshots: false
  agentContext: false
`), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "normalize.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: normalize_demo
intent: dynamic output can be normalized.
target:
  cmd: ["/bin/sh", "-lc", "printf 'run-123\n'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "run-<id>"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: normalized
    description: normalized output is visible
    verify:
      screen:
        contains: "run-<id>"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{SpecPath: specPath, ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	if _, ok := result.Artifacts["rawPtyLog"]; ok {
		t.Fatalf("raw artifact should be disabled: %#v", result.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(result.RunDir, "raw", "pty.raw.log")); !os.IsNotExist(err) {
		t.Fatalf("raw log should not exist, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(result.RunDir, "agent_context.md")); !os.IsNotExist(err) {
		t.Fatalf("agent context should not exist, err=%v", err)
	}
}

func TestRunSpecOutcomeLevelNormalization(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "outcome-normalize.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: outcome_normalize
intent: outcome-specific normalization can hide volatile terminal text.
target:
  cmd: ["/bin/sh", "-lc", "printf 'build run-123\n'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: normalized
    description: the volatile run id is normalized for this outcome only
    normalize:
      replace:
        - regex: "run-[0-9]+"
          with: "run-<id>"
    verify:
      screen:
        contains: "build run-<id>"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
}

func TestRunSpecTargetTimeoutUsesDocumentedExitCode(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "target-timeout.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: target_timeout
intent: target timeout stops a hung terminal app.
target:
  cmd: ["/bin/sh", "-lc", "sleep 5"]
  timeoutMs: 100
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: clean_exit
    description: the process exits successfully
    verify:
      process:
        exitCode: 0
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusErrored {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	if result.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", result.ExitCode)
	}
}

func TestRunSpecEnforcesAlternateScreenPolicy(t *testing.T) {
	t.Run("require passes when target enters alternate screen", func(t *testing.T) {
		dir := t.TempDir()
		specPath := filepath.Join(dir, "alternate-require.yml")
		if err := os.WriteFile(specPath, []byte(`version: 1
name: alternate_require
intent: target enters alternate screen mode.
target:
  cmd: ["/bin/sh", "-lc", "printf '\\033[?1049hready\\n\\033[?1049l'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
  alternateScreen: require
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: ready_visible
    description: ready is visible
    verify:
      screen:
        contains: "ready"
`), 0o644); err != nil {
			t.Fatal(err)
		}
		result, err := RunSpec(context.Background(), Options{
			SpecPath:     specPath,
			ArtifactRoot: filepath.Join(dir, "runs"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != artifacts.StatusPassed {
			t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
		}
	})

	t.Run("require fails when target does not enter alternate screen", func(t *testing.T) {
		dir := t.TempDir()
		specPath := filepath.Join(dir, "alternate-require-missing.yml")
		if err := os.WriteFile(specPath, []byte(`version: 1
name: alternate_require_missing
intent: target must enter alternate screen mode.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\\n'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
  alternateScreen: require
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: ready_visible
    description: ready is visible
    verify:
      screen:
        contains: "ready"
`), 0o644); err != nil {
			t.Fatal(err)
		}
		result, err := RunSpec(context.Background(), Options{
			SpecPath:     specPath,
			ArtifactRoot: filepath.Join(dir, "runs"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != artifacts.StatusFailed {
			t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
		}
		if result.ExitCode != 1 {
			t.Fatalf("exit code = %d, want 1", result.ExitCode)
		}
		diagnostic, err := os.ReadFile(filepath.Join(result.RunDir, "diagnostics", "failure.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(diagnostic), "alternateScreen=require") {
			t.Fatalf("failure diagnostic missing policy failure:\n%s", string(diagnostic))
		}
	})

	t.Run("forbid fails when target enters alternate screen", func(t *testing.T) {
		dir := t.TempDir()
		specPath := filepath.Join(dir, "alternate-forbid.yml")
		if err := os.WriteFile(specPath, []byte(`version: 1
name: alternate_forbid
intent: target must stay on the main screen.
target:
  cmd: ["/bin/sh", "-lc", "printf '\\033[?1049hready\\n\\033[?1049l'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
  alternateScreen: forbid
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: ready_visible
    description: ready is visible
    verify:
      screen:
        contains: "ready"
`), 0o644); err != nil {
			t.Fatal(err)
		}
		result, err := RunSpec(context.Background(), Options{
			SpecPath:     specPath,
			ArtifactRoot: filepath.Join(dir, "runs"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != artifacts.StatusFailed {
			t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
		}
		events, err := os.ReadFile(filepath.Join(result.RunDir, "events.ndjson"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(events), "terminal.policy.failed") {
			t.Fatalf("events missing terminal policy failure:\n%s", string(events))
		}
	})
}

func TestRunSpecChecksCellStyle(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "style.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: style_demo
intent: styled terminal cells can be asserted.
target:
  cmd: ["/bin/sh", "-lc", "printf '\\033[1m>\\033[0m ready\\n'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "> ready"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: prompt_bold
    description: the prompt marker is bold
    verify:
      cell:
        x: 0
        y: 0
        char: ">"
        style:
          bold: true
  - id: next_cell_plain
    description: the next cell is not bold after reset
    verify:
      cell:
        x: 1
        y: 0
        char: " "
        style:
          bold: false
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
}

func TestRunSpecMarksTruncatedRawLog(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "glyphrun.config.yml")
	cfg := `version: 1
artifactRoot: .glyphrun/runs
artifacts:
  rawLog: true
  maxRawLogBytes: 256
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "big.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: big_output
intent: a target producing more output than the raw log cap should be marked truncated.
target:
  cmd: ["/bin/sh", "-lc", "head -c 10000 < /dev/zero | tr '\\0' 'X'"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      process:
        exitCode: 0
      timeoutMs: 5000
outcomes:
  - id: clean_exit
    description: the target exits cleanly
    verify:
      process:
        exitCode: 0
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ConfigPath:   cfgPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(result.RunDir, "raw/pty.raw.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "[glyphrun: raw PTY log truncated at 256 bytes") {
		t.Fatalf("raw log missing truncation marker, len=%d, tail=%q", len(raw), tailOf(string(raw), 200))
	}
	if int64(len(raw)) > 512 {
		// marker is ~70 bytes; allow a comfortable margin but still well under
		// the cap, proving we stopped accepting data once the cap was hit.
		t.Fatalf("raw log grew past cap+marker: len=%d", len(raw))
	}
	events, err := os.ReadFile(filepath.Join(result.RunDir, "events.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(events), `"pty.truncated"`) {
		t.Fatalf("events missing pty.truncated event:\n%s", string(events))
	}
}

func TestRunSpecWritesReplayManifest(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "replay.yml")
	// A target with a declared env var + a non-default terminal so the manifest
	// has something meaningful to record. The env VALUE must never appear in
	// replay.json — only the key name.
	if err := os.WriteFile(specPath, []byte(`version: 1
name: replay_demo
intent: a run writes an exact-replay manifest.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\n'; sleep 0.1"]
  env:
    REPLAY_TOKEN: s3cret-value
terminal:
  cols: 100
  rows: 30
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: ready_visible
    description: ready is visible
    verify:
      screen:
        contains: "ready"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	if result.Artifacts["replay"] != "replay.json" {
		t.Fatalf("replay artifact key missing: %#v", result.Artifacts)
	}
	data, err := os.ReadFile(filepath.Join(result.RunDir, "replay.json"))
	if err != nil {
		t.Fatalf("replay.json not written: %v", err)
	}
	var m artifacts.ReplayManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("replay.json invalid: %v\n%s", err, string(data))
	}
	if m.SchemaVersion != 1 || m.SpecName != "replay_demo" {
		t.Errorf("replay manifest header wrong: %+v", m)
	}
	if !strings.Contains(m.Replay, "glyph run") || !strings.Contains(m.Replay, specPath) {
		t.Errorf("replay command should reproduce the run, got %q", m.Replay)
	}
	if m.Terminal.Cols != 100 || m.Terminal.Rows != 30 || m.Terminal.Profile != "xterm-256color" {
		t.Errorf("replay terminal wrong: %+v", m.Terminal)
	}
	if len(m.Argv) == 0 || m.Argv[0] != "/bin/sh" {
		t.Errorf("replay argv wrong: %+v", m.Argv)
	}
	// The env key NAME is recorded...
	found := false
	for _, k := range m.EnvKeys {
		if k == "REPLAY_TOKEN" {
			found = true
		}
	}
	if !found {
		t.Errorf("env key REPLAY_TOKEN missing from envKeys: %+v", m.EnvKeys)
	}
	// ...but the secret VALUE must never be present.
	if strings.Contains(string(data), "s3cret-value") {
		t.Errorf("replay.json leaked an env value: %s", string(data))
	}
}

func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func TestRunSpecStepResults(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "steps.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: step_results
intent: a run produces structured per-step results
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\n'; sleep 0.1"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 2000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 2000
outcomes:
  - id: ready_visible
    description: ready is visible
    verify:
      screen:
        contains: "ready"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	// The RunResult must carry structured StepResults (SPEC §7.3).
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results, got %d: %+v", len(result.Steps), result.Steps)
	}
	for i, sr := range result.Steps {
		if sr.Index != i+1 {
			t.Errorf("step %d: expected index %d, got %d", i, i+1, sr.Index)
		}
		if sr.Kind != "wait" {
			t.Errorf("step %d: expected kind 'wait', got %q", i, sr.Kind)
		}
		if sr.Status != "passed" {
			t.Errorf("step %d: expected status 'passed', got %q", i, sr.Status)
		}
		if sr.DurationMS < 0 {
			t.Errorf("step %d: negative duration %d", i, sr.DurationMS)
		}
	}
	// Verify the steps are in run.json too.
	runJson, err := os.ReadFile(filepath.Join(result.RunDir, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(runJson, []byte(`"steps"`)) {
		t.Errorf("run.json missing steps field: %s", string(runJson))
	}
	if !bytes.Contains(runJson, []byte(`"kind": "wait"`)) {
		t.Errorf("run.json missing step kind: %s", string(runJson))
	}
}
