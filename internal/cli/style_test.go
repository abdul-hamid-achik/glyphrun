package cli

import (
	"strings"
	"testing"
)

func TestColorizeMarkdownHighlightsImportantLines(t *testing.T) {
	out := colorizeMarkdown("# Glyphrun Run: passed\n\n- status: passed\n- PASS ready\n- FAIL quit\n")
	for _, want := range []string{
		ansiCyan,
		ansiGreen,
		ansiRed,
		ansiReset,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("colorized output missing %q: %q", want, out)
		}
	}
}

func TestColorizeMarkdownLeavesFencedTextAlone(t *testing.T) {
	out := colorizeMarkdown("```text\n- PASS literal\n```\n")
	if strings.Contains(out, ansiGreen+ansiBold+"PASS") {
		t.Fatalf("fenced text should not be status-colored: %q", out)
	}
}
