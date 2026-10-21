//go:build windows

package main

import (
	"fmt"
	"os"
)

// The demo needs a Unix PTY (SIGWINCH, stty); the Windows build only keeps
// `go build ./...` green.
func main() {
	fmt.Fprintln(os.Stderr, "resize: this demo needs a Unix PTY")
	os.Exit(1)
}
