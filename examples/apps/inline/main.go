package main

// inline is a tiny demo of an inline-mode TUI, for the scrollback example
// spec. It never enters the alternate screen. It keeps a two-line live region
// (a rule and a status line) at the bottom of the terminal, and commits 100
// finished lines above it: each one is written at the bottom of a scroll
// region that starts at the top row, so the oldest lines scroll off the top
// into the terminal's scrollback while the live region stays put. It waits for
// q.

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const committed = 100

func main() {
	rows := terminalRows()
	region := rows - 2 // the scroll region holds everything above the live region

	raw := exec.Command("stty", "raw", "-echo")
	raw.Stdin = os.Stdin
	_ = raw.Run()
	defer func() {
		sane := exec.Command("stty", "sane")
		sane.Stdin = os.Stdin
		_ = sane.Run()
	}()

	fmt.Print("\x1b[2J\x1b[H")
	// Confine scrolling to rows 1..region. Because the region starts at row 1,
	// a line that scrolls out of it goes into the scrollback.
	fmt.Printf("\x1b[1;%dr", region)
	live(rows, 0, false)
	for i := 1; i <= committed; i++ {
		if i <= region {
			// The region is still filling up: write the line on its own row.
			fmt.Printf("\x1b[%d;1Hcommitted line %d", i, i)
		} else {
			// Full: move to the last row of the region, scroll it up one line
			// (the top line enters the scrollback), and write the new line there.
			fmt.Printf("\x1b[%d;1H\n\x1b[%d;1Hcommitted line %d", region, region, i)
		}
		live(rows, i, false)
	}
	live(rows, committed, true)

	reader := bufio.NewReader(os.Stdin)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			os.Exit(1)
		}
		if b == 'q' {
			// Release the scroll region and park the cursor on the last row.
			// No newline: that would scroll one more line into the scrollback.
			fmt.Printf("\x1b[r\x1b[%d;1H", rows)
			os.Exit(0)
		}
	}
}

// live redraws the two-line live region on the last two rows.
func live(rows, n int, done bool) {
	status := fmt.Sprintf("status: %d committed", n)
	if done {
		status += " - press q to quit"
	}
	fmt.Printf("\x1b7\x1b[%d;1H\x1b[2K%s\x1b[%d;1H\x1b[2K%s\x1b8", rows-1, strings.Repeat("-", 20), rows, status)
}

// terminalRows asks the terminal its height with `stty size` ("rows cols").
func terminalRows() int {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 24
	}
	var rows, cols int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d %d", &rows, &cols); err != nil || rows < 4 {
		return 24
	}
	return rows
}
