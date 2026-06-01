package ptyrunner

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
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

type Session struct {
	cmd          *exec.Cmd
	ptmx         *os.File
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
	cmd := exec.Command(opts.Cmd[0], opts.Cmd[1:]...)
	cmd.Dir = absCwd
	cmd.Env = mergeEnv(os.Environ(), opts.Env)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(opts.Cols),
		Rows: uint16(opts.Rows),
	})
	if err != nil {
		return nil, err
	}
	s := &Session{
		cmd:          cmd,
		ptmx:         ptmx,
		done:         make(chan ExitState, 1),
		outputDone:   make(chan struct{}),
		lastOutputAt: time.Now(),
	}
	go s.readLoop(opts.OnOutput, opts.OnError)
	go s.waitLoop()
	return s, nil
}

func (s *Session) Write(data []byte) error {
	_, err := s.ptmx.Write(data)
	return err
}

func (s *Session) Resize(cols, rows int) error {
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (s *Session) ExitState() ExitState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.exit
}

func (s *Session) WaitCh() <-chan ExitState {
	return s.done
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
			_ = s.ptmx.Close()
			_ = s.waitForOutput(time.Second)
		}
		return state
	}
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
	}
	select {
	case state = <-s.done:
		if !s.waitForOutput(timeout) {
			_ = s.ptmx.Close()
			_ = s.waitForOutput(time.Second)
		}
		return state
	case <-time.After(timeout):
	}
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	select {
	case state = <-s.done:
	case <-time.After(time.Second):
		state = ExitState{Exited: true, ExitCode: -1}
	}
	_ = s.ptmx.Close()
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
		n, err := s.ptmx.Read(buf)
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
	err := s.cmd.Wait()
	state := ExitState{Exited: true, ExitCode: exitCode(err)}
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
