package main

// mouseapp is a tiny demo for the `mouse:` step. It enables SGR mouse tracking
// (private modes 1000 + 1006), waits for a click, and prints the 1-based cell
// it received — so the mouse_click.yml example can assert the click landed
// where the step said.

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

	fmt.Print("\x1b[?1049h")
	defer fmt.Print("\x1b[?1049l")
	// Enable mouse reporting + the SGR (1006) encoding.
	fmt.Print("\x1b[?1000h\x1b[?1006h")
	defer fmt.Print("\x1b[?1006l\x1b[?1000l")

	fmt.Print("\x1b[2J\x1b[H")
	fmt.Print("click somewhere\r\n")

	reader := bufio.NewReader(os.Stdin)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			os.Exit(1)
		}
		if b == 'q' {
			os.Exit(0)
		}
		if b != 0x1b {
			continue
		}
		// Expect an SGR mouse report: ESC [ < b ; x ; y (M|m).
		if c, _ := reader.ReadByte(); c != '[' {
			continue
		}
		if c, _ := reader.ReadByte(); c != '<' {
			continue
		}
		var sb strings.Builder
		var final byte
		for {
			c, err := reader.ReadByte()
			if err != nil {
				os.Exit(1)
			}
			if c == 'M' || c == 'm' {
				final = c
				break
			}
			sb.WriteByte(c)
		}
		// Report only the press ('M') of a left click (button 0).
		if final != 'M' {
			continue
		}
		parts := strings.Split(sb.String(), ";")
		if len(parts) == 3 && parts[0] == "0" {
			fmt.Printf("clicked at %s,%s\r\n", parts[1], parts[2])
		}
	}
}
