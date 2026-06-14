//go:build !windows

package ptyrunner

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

// unixBackend runs the target via os/exec attached to a Unix PTY (creack/pty).
type unixBackend struct {
	cmd  *exec.Cmd
	ptmx *os.File
}

func newBackend(opts Options) (backend, error) {
	cmd := exec.Command(opts.Cmd[0], opts.Cmd[1:]...)
	cmd.Dir = opts.Cwd
	cmd.Env = mergeEnv(os.Environ(), opts.Env)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(opts.Cols),
		Rows: uint16(opts.Rows),
	})
	if err != nil {
		return nil, err
	}
	return &unixBackend{cmd: cmd, ptmx: ptmx}, nil
}

func (b *unixBackend) Read(p []byte) (int, error)  { return b.ptmx.Read(p) }
func (b *unixBackend) Write(p []byte) (int, error) { return b.ptmx.Write(p) }

func (b *unixBackend) resize(cols, rows int) error {
	return pty.Setsize(b.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (b *unixBackend) wait() ExitState {
	err := b.cmd.Wait()
	return ExitState{Exited: true, ExitCode: exitCode(err)}
}

func (b *unixBackend) softStop() error {
	if b.cmd.Process == nil {
		return nil
	}
	return b.cmd.Process.Signal(syscall.SIGTERM)
}

func (b *unixBackend) hardStop() error {
	if b.cmd.Process == nil {
		return nil
	}
	return b.cmd.Process.Kill()
}

func (b *unixBackend) closePTY() error {
	return b.ptmx.Close()
}
