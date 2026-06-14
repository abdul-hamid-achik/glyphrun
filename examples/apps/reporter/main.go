package main

// reporter is a tiny TUI app used by the download / transform / batch
// example specs. It demonstrates the three capabilities that the new
// glyphrun step kinds target:
//
//   - download: the app writes a report file to a known path and prints
//     that path; the spec captures it into the run's artifacts dir.
//   - transform: a Node transform reads the captured report, uppercases
//     the body, and writes a new named artifact.
//   - batch: pressing "w" + then "enter" + then a one-char "tag" must be
//     delivered as a single PTY write so the prompt state survives.

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

	// Alternate-screen: the spec asserts we entered it.
	fmt.Print("\x1b[2J\x1b[H")
	fmt.Println("reporter ready")
	fmt.Println("press w to write a report, q to quit")

	reader := bufio.NewReader(os.Stdin)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return
			}
			os.Exit(1)
		}
		switch b {
		case 'q':
			return
		case 'w':
			// Read a one-character tag (the spec sends it as part of a
			// batch) and the trailing newline. We accept both
			// line-buffered and raw inputs.
			tagBytes := make([]byte, 0, 4)
			for {
				c, err := reader.ReadByte()
				if err != nil {
					os.Exit(1)
				}
				tagBytes = append(tagBytes, c)
				if c == '\n' || c == '\r' {
					break
				}
				if len(tagBytes) >= 4 {
					break
				}
			}
			tag := strings.TrimSpace(string(tagBytes))
			if tag == "" {
				tag = "default"
			}
			path, err := writeReport(tag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "writeReport failed: %v\n", err)
				continue
			}
			// The spec greps the screen for this exact line.
			fmt.Printf("\x1b[2K\rreport=%s\n", path)
		}
	}
}

func writeReport(tag string) (string, error) {
	dir := os.Getenv("GLYPHRUN_REPORTER_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// When the spec sets GLYPHRUN_REPORTER_NAME, write to that exact
	// filename so the spec can capture the path deterministically with
	// a `download:` step. Otherwise fall back to a randomized filename
	// (the original behavior).
	name := os.Getenv("GLYPHRUN_REPORTER_NAME")
	if name == "" {
		suffix, err := randomSuffix()
		if err != nil {
			return "", err
		}
		name = fmt.Sprintf("report-%s-%s.txt", tag, suffix)
	}
	path := filepath.Join(dir, name)
	body := fmt.Sprintf("tag=%s\ntimestamp=%s\nstatus=ok\n", tag, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func randomSuffix() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
