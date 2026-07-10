package artifacts

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultArchiveTimeout is used when ArchiveConfig.Timeout is empty.
const DefaultArchiveTimeout = 5 * time.Minute

// MaxArchiveOutputBytes bounds diagnostic capture from external archive tools.
const MaxArchiveOutputBytes = 64 * 1024

// ArchiveConfig is the artifacts-package view of the config
// retention.archive block. It mirrors config.ArchiveConfig without
// importing internal/config (the artifacts package owns no runner or
// config state). The runner translates config.ArchiveConfig into this
// type before calling PruneRuns.
type ArchiveConfig struct {
	// Enabled gates archival. When false, pruned directories are
	// deleted locally as usual.
	Enabled bool
	// Command is the external binary invoked to archive a run dir.
	// The run directory path is appended as the final positional arg.
	// Required when Enabled is true.
	Command string
	// Args are fixed arguments passed to Command before the run dir.
	Args []string
	// Timeout is the max wall time for the archival command. Empty
	// means DefaultArchiveTimeout. A timeout is treated as archive
	// failure (the local dir is preserved).
	Timeout time.Duration
}

// ArchiveResult captures the outcome of a single archival invocation.
type ArchiveResult struct {
	Path    string `json:"path" yaml:"path"`
	OK      bool   `json:"ok" yaml:"ok"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

type boundedArchiveOutput struct {
	buf       bytes.Buffer
	truncated bool
}

func (w *boundedArchiveOutput) Write(p []byte) (int, error) {
	remaining := MaxArchiveOutputBytes - w.buf.Len()
	if remaining > 0 {
		n := len(p)
		if n > remaining {
			n = remaining
		}
		_, _ = w.buf.Write(p[:n])
	}
	if len(p) > remaining {
		w.truncated = true
	}
	return len(p), nil
}

func (w *boundedArchiveOutput) String() string {
	out := w.buf.String()
	if w.truncated {
		out += "\n[glyphrun: archive command output truncated]"
	}
	return out
}

// ArchiveRun invokes the configured archival command for a single run
// directory. The command is run as:
//
//	<Command> <Args...> <runDir>
//
// with the run directory path appended as the final positional arg.
// Combined stdout+stderr is captured for diagnostics. A non-zero exit
// code, a timeout, or a missing binary is an error; the caller is
// expected to keep the local directory in all those cases (move
// semantics: delete only on success).
//
// ArchiveRun is pure with respect to run state — it does not touch the
// run dir itself, only shells out to the external command. The caller
// owns the delete decision.
func ArchiveRun(cfg ArchiveConfig, runDir string) (ArchiveResult, error) {
	if cfg.Command == "" {
		return ArchiveResult{Path: runDir, OK: false, Message: "archive command not configured"}, nil
	}
	args := append(append([]string{}, cfg.Args...), runDir)
	cmd := exec.Command(cfg.Command, args...)
	cmd.Dir = filepath.Dir(runDir)
	var out boundedArchiveOutput
	cmd.Stdout = &out
	cmd.Stderr = &out

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultArchiveTimeout
	}
	if err := cmd.Start(); err != nil {
		return ArchiveResult{Path: runDir, OK: false, Message: fmt.Sprintf("start %s: %v", cfg.Command, err)}, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		msg := strings.TrimSpace(out.String())
		if err != nil {
			if msg == "" {
				msg = strings.TrimSpace(err.Error())
			}
			return ArchiveResult{Path: runDir, OK: false, Message: fmt.Sprintf("exit: %s", msg)}, err
		}
		return ArchiveResult{Path: runDir, OK: true, Message: msg}, nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done // reap the killed process
		return ArchiveResult{Path: runDir, OK: false, Message: fmt.Sprintf("timeout after %s", timeout)}, fmt.Errorf("archive %s: timeout after %s", runDir, timeout)
	}
}

// archiveEnabled reports whether archival should run for a prune. It
// is a small helper so callers don't repeat the Enabled/Command guard.
func (c ArchiveConfig) archiveEnabled() bool {
	return c.Enabled && c.Command != ""
}

// ParseArchiveTimeout parses a duration string (e.g. "5m", "30s")
// into a time.Duration. Empty returns 0, which the caller maps to the
// default. An invalid string returns an error. This lives here so the
// config-to-artifacts translation stays in the artifacts package.
func ParseArchiveTimeout(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}
