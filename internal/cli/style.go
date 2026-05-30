package cli

import (
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiGreen   = "\x1b[32m"
	ansiRed     = "\x1b[31m"
	ansiYellow  = "\x1b[33m"
	ansiCyan    = "\x1b[36m"
	ansiBlue    = "\x1b[34m"
	ansiMagenta = "\x1b[35m"
)

func emitForCLI(cmd *cobra.Command, opts *globalOptions, format outputFormat, value any, markdown func() string) (string, error) {
	output, err := emit(format, value, markdown)
	if err != nil {
		return "", err
	}
	if format != formatMD || !colorEnabled(cmd.OutOrStdout(), opts) {
		return output, nil
	}
	return colorizeMarkdown(output), nil
}

func colorEnabled(w io.Writer, opts *globalOptions) bool {
	if opts != nil && opts.noColor {
		return false
	}
	switch strings.ToLower(os.Getenv("GLYPHRUN_COLOR")) {
	case "always", "1", "true", "yes", "on":
		return true
	case "never", "0", "false", "no", "off":
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func colorizeMarkdown(input string) string {
	if input == "" {
		return ""
	}
	var b strings.Builder
	inFence := false
	for _, chunk := range strings.SplitAfter(input, "\n") {
		line := strings.TrimSuffix(chunk, "\n")
		newline := ""
		if strings.HasSuffix(chunk, "\n") {
			newline = "\n"
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			b.WriteString(ansiDim)
			b.WriteString(line)
			b.WriteString(ansiReset)
			b.WriteString(newline)
			continue
		}
		if inFence {
			b.WriteString(line)
			b.WriteString(newline)
			continue
		}
		b.WriteString(colorizeMarkdownLine(line))
		b.WriteString(newline)
	}
	return b.String()
}

func colorizeMarkdownLine(line string) string {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "# "):
		return ansiBold + ansiCyan + line + ansiReset
	case strings.HasPrefix(trimmed, "## "):
		return ansiBold + ansiBlue + line + ansiReset
	case strings.HasPrefix(trimmed, "### "):
		return ansiBold + ansiMagenta + line + ansiReset
	case strings.HasPrefix(trimmed, "- PASS "):
		return strings.Replace(line, "PASS", ansiGreen+ansiBold+"PASS"+ansiReset, 1)
	case strings.HasPrefix(trimmed, "- FAIL "):
		return strings.Replace(line, "FAIL", ansiRed+ansiBold+"FAIL"+ansiReset, 1)
	case strings.Contains(line, "status: passed"):
		return strings.Replace(line, "passed", ansiGreen+ansiBold+"passed"+ansiReset, 1)
	case strings.Contains(line, "status: failed"):
		return strings.Replace(line, "failed", ansiRed+ansiBold+"failed"+ansiReset, 1)
	case strings.Contains(line, "status: errored"):
		return strings.Replace(line, "errored", ansiYellow+ansiBold+"errored"+ansiReset, 1)
	case strings.HasPrefix(trimmed, "- passed:"):
		return strings.Replace(line, "- passed:", ansiGreen+"- passed:"+ansiReset, 1)
	case strings.HasPrefix(trimmed, "- failed:"):
		return strings.Replace(line, "- failed:", ansiRed+"- failed:"+ansiReset, 1)
	case strings.HasPrefix(trimmed, "- artifacts:"):
		return strings.Replace(line, "- artifacts:", ansiMagenta+"- artifacts:"+ansiReset, 1)
	case strings.HasPrefix(trimmed, "- diagnostic:"):
		return strings.Replace(line, "- diagnostic:", ansiYellow+"- diagnostic:"+ansiReset, 1)
	default:
		return line
	}
}
