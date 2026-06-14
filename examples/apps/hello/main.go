package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
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

	fmt.Print("\x1b[2J\x1b[H")
	fmt.Println("hello from glyphrun")
	fmt.Println("press q to quit")
	// An OSC 8 hyperlink: the text "docs" links to the project URL. The
	// link.yml example asserts this with the `link` verifier.
	fmt.Print("\x1b]8;;https://glyphrun.dev\x1b\\docs\x1b]8;;\x1b\\\n")

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
