package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runVersionSubcommand executes `glyph version` against a fresh
// root command and returns stdout. We don't reuse Execute() — that
// exits the test process on a bad flag, and tests need to inspect
// output directly.
func runVersionSubcommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	opts := &globalOptions{}
	root := newRootCommand(opts)
	root.SilenceUsage = true
	root.SilenceErrors = true
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	// SetArgs bypasses os.Args so tests don't pick up the test
	// runner's command line.
	root.SetArgs(append([]string{"version"}, args...))
	if err := root.Execute(); err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

// TestVersionSubcommand_DefaultFormat: the human-readable form is
// "version (commit buildDate)". This is the shape `glyph --version`
// also prints (with a "glyph version " prefix that cobra adds);
// the subcommand output is the bare version.Full() string. Tests
// pin the paren-pair wire format so a careless refactor doesn't
// break scripts that grep for it.
func TestVersionSubcommand_DefaultFormat(t *testing.T) {
	out, err := runVersionSubcommand(t)
	if err != nil {
		t.Fatalf("version subcommand failed: %v", err)
	}
	out = strings.TrimSpace(out)
	if !strings.Contains(out, "(") || !strings.Contains(out, ")") {
		t.Errorf("version output should contain '(...)' wire format, got %q", out)
	}
}

// TestVersionSubcommand_JSON: the JSON output is a 3-key object
// the test pins so a CI dashboard that depends on
// .version / .commit / .buildDate stays parseable.
func TestVersionSubcommand_JSON(t *testing.T) {
	out, err := runVersionSubcommand(t, "--format", "json")
	if err != nil {
		t.Fatalf("version --format json failed: %v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	for _, key := range []string{"version", "commit", "buildDate"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("JSON output missing key %q (got %v)", key, payload)
		}
	}
}

// TestVersionSubcommand_YAML: same shape as JSON but in YAML so a
// release-notes script can pipe it through `yq`.
func TestVersionSubcommand_YAML(t *testing.T) {
	out, err := runVersionSubcommand(t, "--format", "yaml")
	if err != nil {
		t.Fatalf("version --format yaml failed: %v", err)
	}
	for _, want := range []string{"version:", "commit:", "buildDate:"} {
		if !strings.Contains(out, want) {
			t.Errorf("YAML output missing key %q (got %q)", want, out)
		}
	}
}

// TestVersionSubcommand_RejectsBadFormat: the format flag goes
// through resolveFormat, so an unsupported value returns an error
// rather than silently emitting the default.
func TestVersionSubcommand_RejectsBadFormat(t *testing.T) {
	_, err := runVersionSubcommand(t, "--format", "xml")
	if err == nil {
		t.Fatal("expected error for unsupported --format xml, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported --format") {
		t.Errorf("error should mention \"unsupported --format\", got: %v", err)
	}
}

// TestRoot_HasVersionField: the root command's Version field is
// what makes `glyph --version` work via cobra's built-in handler.
// A regression here would silently disable the flag.
func TestRoot_HasVersionField(t *testing.T) {
	opts := &globalOptions{}
	root := newRootCommand(opts)
	if root.Version == "" {
		t.Fatal("root command's Version field is empty; --version would print nothing")
	}
	// Sanity: also confirm the subcommand is registered.
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("version subcommand is not registered on the root command")
	}
	// Compile-time guard so cobra's import isn't dropped.
	var _ = cobra.Command{}
}
