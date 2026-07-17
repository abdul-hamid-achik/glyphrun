// Package ptyrunner launches a target command inside a pseudo-terminal and
// streams its output. The process/PTY mechanics are platform-specific (a Unix
// PTY via creack/pty, a Windows pseudo-console via ConPTY) and live behind the
// `backend` interface in backend_unix.go / backend_windows.go; everything in
// this file — the read loop, exit tracking, and teardown — is platform-neutral.
package ptyrunner

import (
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Options struct {
	Cmd  []string
	Cwd  string
	Env  map[string]string
	Cols int
	Rows int

	OnOutput func([]byte)
	OnError  func(error)
}

type ExitState struct {
	Exited   bool `json:"exited"`
	ExitCode int  `json:"exitCode,omitempty"`
}

// backend abstracts the platform PTY + process. It owns the byte stream
// (Read/Write), resizing, the blocking wait for exit, and teardown. The
// neutral Session drives it.
type backend interface {
	io.Reader
	io.Writer
	resize(cols, rows int) error
	wait() ExitState // blocks until the process exits
	softStop() error // graceful terminate (best-effort; may be a no-op)
	hardStop() error // force kill
	closePTY() error // close the PTY handle so a blocked Read returns
	pid() int        // target process PID, or 0 if unavailable (Windows ConPTY)
}

type Session struct {
	backend      backend
	done         chan ExitState
	outputDone   chan struct{}
	mu           sync.RWMutex
	exit         ExitState
	lastOutputAt time.Time
}

func Start(opts Options) (*Session, error) {
	if len(opts.Cmd) == 0 {
		return nil, errors.New("command is required")
	}
	cwd := opts.Cwd
	if cwd == "" {
		cwd = "."
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	opts.Cwd = absCwd

	b, err := newBackend(opts)
	if err != nil {
		return nil, err
	}
	s := &Session{
		backend:      b,
		done:         make(chan ExitState, 1),
		outputDone:   make(chan struct{}),
		lastOutputAt: time.Now(),
	}
	go s.readLoop(opts.OnOutput, opts.OnError)
	go s.waitLoop()
	return s, nil
}

func (s *Session) Write(data []byte) error {
	_, err := s.backend.Write(data)
	return err
}

func (s *Session) Resize(cols, rows int) error {
	return s.backend.resize(cols, rows)
}

func (s *Session) ExitState() ExitState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.exit
}

func (s *Session) WaitCh() <-chan ExitState {
	return s.done
}

// OutputDoneCh is closed when the PTY read loop has finished (EOF or error).
// After WaitCh signals process exit, callers should wait on this (or use
// WaitForOutput) so final buffered PTY bytes reach the emulator before the
// session is treated as fully drained.
func (s *Session) OutputDoneCh() <-chan struct{} {
	return s.outputDone
}

// WaitForOutput waits until the PTY reader finishes or timeout elapses.
// Returns true if output drained within the timeout. Call this after the
// process has exited so short-lived targets do not lose trailing bytes.
func (s *Session) WaitForOutput(timeout time.Duration) bool {
	return s.waitForOutput(timeout)
}

// PID returns the target process's PID, or 0 if the backend cannot expose it
// (Windows ConPTY). Used by the opt-in `--monitor` process-telemetry capture.
func (s *Session) PID() int {
	return s.backend.pid()
}

func (s *Session) LastOutputAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastOutputAt
}

func (s *Session) Cleanup(timeout time.Duration) ExitState {
	state := s.ExitState()
	if state.Exited {
		if !s.waitForOutput(timeout) {
			_ = s.backend.closePTY()
			_ = s.waitForOutput(time.Second)
		}
		return state
	}
	_ = s.backend.softStop()
	select {
	case state = <-s.done:
		if !s.waitForOutput(timeout) {
			_ = s.backend.closePTY()
			_ = s.waitForOutput(time.Second)
		}
		return state
	case <-time.After(timeout):
	}
	_ = s.backend.hardStop()
	select {
	case state = <-s.done:
	case <-time.After(time.Second):
		state = ExitState{Exited: true, ExitCode: -1}
	}
	_ = s.backend.closePTY()
	_ = s.waitForOutput(time.Second)
	return state
}

func (s *Session) waitForOutput(timeout time.Duration) bool {
	select {
	case <-s.outputDone:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (s *Session) readLoop(onOutput func([]byte), onError func(error)) {
	defer close(s.outputDone)
	buf := make([]byte, 4096)
	for {
		n, err := s.backend.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.mu.Lock()
			s.lastOutputAt = time.Now()
			s.mu.Unlock()
			if onOutput != nil {
				onOutput(chunk)
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && onError != nil {
				onError(err)
			}
			return
		}
	}
}

func (s *Session) waitLoop() {
	state := s.backend.wait()
	s.mu.Lock()
	s.exit = state
	s.mu.Unlock()
	s.done <- state
	close(s.done)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func mergeEnv(base []string, overlay map[string]string) []string {
	values := map[string]string{}
	for _, pair := range base {
		k, v, ok := splitEnv(pair)
		if ok {
			values[k] = v
		}
	}
	for k, v := range overlay {
		values[k] = v
	}
	out := make([]string, 0, len(values))
	for k, v := range values {
		out = append(out, k+"="+v)
	}
	return out
}

func splitEnv(pair string) (string, string, bool) {
	for i := 0; i < len(pair); i++ {
		if pair[i] == '=' {
			return pair[:i], pair[i+1:], true
		}
	}
	return "", "", false
}
