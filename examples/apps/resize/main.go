package main

// resize is a tiny demo of an inline-mode TUI that survives a terminal resize,
// for the resize example spec. It never enters the alternate screen: it prints
// three finished lines, then a two-line live region (the terminal size and a
// hint) below them, and leaves the cursor on the live region's last line.
//
// On SIGWINCH it redraws the live region using only relative cursor moves: go
// up one line, return to column 0, erase to the end of the screen, and print
// the region again. It never addresses an absolute row, so it only works when
// the terminal kept the screen across the resize, as a real one does. It waits
// for q.

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	raw := exec.Command("stty", "raw", "-echo")
	raw.Stdin = os.Stdin
	_ = raw.Run()
	defer func() {
		sane := exec.Command("stty", "sane")
		sane.Stdin = os.Stdin
		_ = sane.Run()
	}()

	for i := 1; i <= 3; i++ {
		fmt.Printf("history line %d\r\n", i)
	}
	drawRegion()

	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			// Relative moves only: up to the region's first line, column 0,
			// erase from there to the end of the screen, draw it again.
			fmt.Print("\x1b[1A\r\x1b[J")
			drawRegion()
		}
	}()

	reader := bufio.NewReader(os.Stdin)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			os.Exit(1)
		}
		if b == 'q' {
			os.Exit(0)
		}
	}
}

// drawRegion prints the two-line live region and stops at the end of its last
// line, without a trailing newline.
func drawRegion() {
	fmt.Printf("live region: %s\r\npress q to quit", terminalSize())
}

// terminalSize asks the terminal its size with `stty size` and formats it as
// COLSxROWS.
func terminalSize() string {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	var rows, cols int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d %d", &rows, &cols); err != nil {
		return "unknown"
	}
	return fmt.Sprintf("%dx%d", cols, rows)
}
