//go:build windows

package ptyrunner

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/UserExistsError/conpty"
)

// windowsBackend runs the target inside a Windows pseudo-console (ConPTY).
// Unlike the Unix path there is no separate child handle to signal: conpty
// owns the process, and Close() both terminates it and frees the handles.
type windowsBackend struct {
	cpty      *conpty.ConPty
	closeOnce sync.Once
}

func newBackend(opts Options) (backend, error) {
	if !conpty.IsConPtyAvailable() {
		return nil, fmt.Errorf("ConPTY is not available on this Windows version (requires Windows 10 1809+)")
	}
	cpty, err := conpty.Start(
		buildCommandLine(opts.Cmd),
		conpty.ConPtyDimensions(opts.Cols, opts.Rows),
		conpty.ConPtyWorkDir(opts.Cwd),
		conpty.ConPtyEnv(mergeEnv(os.Environ(), opts.Env)),
	)
	if err != nil {
		return nil, err
	}
	return &windowsBackend{cpty: cpty}, nil
}

func (b *windowsBackend) Read(p []byte) (int, error)  { return b.cpty.Read(p) }
func (b *windowsBackend) Write(p []byte) (int, error) { return b.cpty.Write(p) }

func (b *windowsBackend) resize(cols, rows int) error {
	return b.cpty.Resize(cols, rows)
}

func (b *windowsBackend) wait() ExitState {
	code, err := b.cpty.Wait(context.Background())
	if err != nil {
		return ExitState{Exited: true, ExitCode: -1}
	}
	return ExitState{Exited: true, ExitCode: int(code)}
}

// softStop has no graceful equivalent on Windows consoles (there is no
// SIGTERM); the neutral Cleanup falls through to hardStop after its timeout.
func (b *windowsBackend) softStop() error { return nil }

func (b *windowsBackend) hardStop() error { return b.close() }

func (b *windowsBackend) closePTY() error { return b.close() }

// close is idempotent: conpty.Close terminates the process and frees handles,
// and both hardStop and closePTY may call it.
func (b *windowsBackend) close() error {
	var err error
	b.closeOnce.Do(func() { err = b.cpty.Close() })
	return err
}

// pid returns 0 on Windows: ConPTY owns the child process and does not expose
// its PID through the conpty wrapper, so process telemetry (monitor sampling)
// is unavailable on this backend. The procmon feature degrades gracefully.
func (b *windowsBackend) pid() int { return 0 }

// buildCommandLine renders argv into a single Windows command-line string with
// each argument escaped per the CommandLineToArgvW rules (via syscall).
func buildCommandLine(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = syscall.EscapeArg(a)
	}
	return joinSpace(parts)
}

func joinSpace(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}
