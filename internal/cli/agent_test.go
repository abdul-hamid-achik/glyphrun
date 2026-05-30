package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestAgentCommandPrintsWorkflowGuide(t *testing.T) {
	opts := &globalOptions{format: "md"}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"agent", "--format", "md"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agent failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"# Glyphrun Agent Guide",
		"glyph explain --format json",
		"glyph spec verify <spec> --format json",
		"glyph context latest --format md",
		"glyph docs agents --format md",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent output missing %q:\n%s", want, out)
		}
	}
}

func TestAgentCommandSupportsJSON(t *testing.T) {
	opts := &globalOptions{format: "json"}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"agent", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agent json failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`"schemaVersion": 1`,
		`"purpose": "bootstrap instructions for agents using Glyphrun"`,
		`"glyph context latest --format md"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent json missing %q:\n%s", want, out)
		}
	}
}

func TestMarkdownColorCanBeForcedAndDisabled(t *testing.T) {
	t.Setenv("GLYPHRUN_COLOR", "always")
	opts := &globalOptions{format: "md"}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"agent", "--format", "md"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agent failed: %v", err)
	}
	if !strings.Contains(stdout.String(), ansiCyan) {
		t.Fatalf("expected forced color output, got %q", stdout.String())
	}

	opts = &globalOptions{format: "md", noColor: true}
	cmd = newRootCommand(opts)
	stdout.Reset()
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"agent", "--format", "md", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agent no-color failed: %v", err)
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("expected no ANSI output, got %q", stdout.String())
	}
}

func TestSpecScaffoldAction(t *testing.T) {
	opts := &globalOptions{format: "md"}
	cmd := newRootCommand(opts)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"spec", "scaffold", "--kind", "action"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scaffold action failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"name: wait_for_ready_and_quit",
		"steps:",
		"snapshot: ready",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("action scaffold missing %q:\n%s", want, out)
		}
	}
}
