//go:build windows

package ptyrunner

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// TestConPtyEchoes drives a real command through the ConPTY backend and checks
// that output streams back and the exit state is captured. It runs only on
// Windows CI, where a pseudo-console is available.
func TestConPtyEchoes(t *testing.T) {
	var mu sync.Mutex
	var out []byte

	s, err := Start(Options{
		Cmd:  []string{"cmd.exe", "/c", "echo glyphrun-conpty"},
		Cols: 80,
		Rows: 24,
		OnOutput: func(b []byte) {
			mu.Lock()
			out = append(out, b...)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-s.WaitCh():
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for cmd.exe to exit")
	}

	state := s.Cleanup(2 * time.Second)
	if !state.Exited {
		t.Errorf("expected process to be marked exited, got %+v", state)
	}

	mu.Lock()
	got := string(out)
	mu.Unlock()
	if !strings.Contains(got, "glyphrun-conpty") {
		t.Errorf("expected echoed text in output, got %q", got)
	}
}

func TestBuildCommandLineEscapes(t *testing.T) {
	got := buildCommandLine([]string{"app.exe", "hello world", "plain"})
	if !strings.Contains(got, "app.exe") || !strings.Contains(got, "plain") {
		t.Errorf("command line missing args: %q", got)
	}
	if !strings.Contains(got, "\"hello world\"") {
		t.Errorf("expected the spaced arg to be quoted: %q", got)
	}
}
