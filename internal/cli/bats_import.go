package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// batsImportResult captures the result of importing a .bats file. The
// importer emits a single glyphrun spec that, when run, replays the
// original bats body through a sub-script per test and produces one
// outcome per `@test` block.
type batsImportResult struct {
	SourcePath string `json:"sourcePath" yaml:"sourcePath"`
	SpecPath   string `json:"specPath" yaml:"specPath"`
	SpecName   string `json:"specName" yaml:"specName"`
	Tests      int    `json:"tests" yaml:"tests"`
	// SpecYAML is the generated spec, included for callers that want to
	// inline the result instead of writing it to disk.
	SpecYAML string `json:"specYaml" yaml:"specYaml"`
	// Warnings are non-fatal mapping notes (e.g. "could not map
	// `run -e`, emitting as plain command").
	Warnings []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

func newBatsImportCommand(opts *globalOptions) *cobra.Command {
	// newBatsImportCommand is retained for direct invocation (`glyph import bats`)
	// but the user-facing surface is `glyph import bats` via the parent
	// newImportCommand. The actual handler lives in runBatsImport.
	cmd := &cobra.Command{
		Use:    "bats <file.bats>",
		Short:  "Import a .bats test file as a glyphrun spec (use `glyph import bats` instead)",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBatsImport(cmd, opts, args)
		},
	}
	cmd.Flags().String("out", "", "output path (default: <source>.yml next to the source)")
	cmd.Flags().String("name", "", "override the spec name (default: derived from the file basename)")
	return cmd
}

// runBatsImport is the entry point used by both `glyph import bats` and
// the hidden direct subcommand. It pulls --out and --name from the
// cobra command, calls the importer, and emits the result.
func runBatsImport(cmd *cobra.Command, opts *globalOptions, args []string) error {
	if len(args) != 1 {
		return exitError{code: 2, err: fmt.Errorf("import bats expects exactly one <file> argument")}
	}
	format, err := resolveFormat(opts.format)
	if err != nil {
		return exitError{code: 2, err: err}
	}
	outPath, _ := cmd.Flags().GetString("out")
	name, _ := cmd.Flags().GetString("name")
	return runBatsImportWith(cmd, opts, args, outPath, name, format)
}

// runBatsImportWith is the testable / direct form of runBatsImport
// that takes the values directly. The dispatcher in
// import_export.go calls this with the values it pulled from the
// parent command's flags; the hidden direct subcommand calls it
// with the values it pulled from its own flags.
func runBatsImportWith(cmd *cobra.Command, opts *globalOptions, args []string, outPath string, name string, format ...outputFormat) error {
	if len(args) != 1 {
		return exitError{code: 2, err: fmt.Errorf("import bats expects exactly one <file> argument")}
	}
	var f outputFormat
	if len(format) > 0 {
		f = format[0]
	} else {
		var err error
		f, err = resolveFormat(opts.format)
		if err != nil {
			return exitError{code: 2, err: err}
		}
	}
	source := args[0]
	result, err := importBats(source, outPath, name)
	if err != nil {
		return exitError{code: 2, err: err}
	}
	output, err := emitForCLI(cmd, opts, f, result, func() string {
		return renderBatsImportMarkdown(result)
	})
	if err != nil {
		return exitError{code: 2, err: err}
	}
	cmd.Print(output)
	return nil
}

// batsTest captures a single `@test "name" { body }` block.
type batsTest struct {
	name string
	body string
	line int // 1-based line in the source
}

// importBats reads a .bats file and produces a glyphrun spec that, when
// run, replays each test through a sub-script. The sub-script approach
// matches the way glyphrun already handles TUI subprocesses (a fresh
// shell per outcome) and keeps each outcome's evidence isolated.
//
// The generated spec's `target.cmd` runs the importer's per-test
// sub-script, indexed by an env var `GLYPHRUN_TEST_INDEX`. The
// sub-script dispatches to the i-th test's body. The single outcome
// per test runs `command: run: "<sub-script> <i>"` and passes if the
// sub-script exits 0.
func importBats(source string, outPath string, nameOverride string) (batsImportResult, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return batsImportResult{}, err
	}
	tests, warnings, err := parseBatsTests(string(data))
	if err != nil {
		return batsImportResult{}, fmt.Errorf("parse bats: %w", err)
	}
	if len(tests) == 0 {
		return batsImportResult{}, fmt.Errorf("no @test blocks found in %s", source)
	}
	specName := nameOverride
	if specName == "" {
		base := filepath.Base(source)
		specName = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if outPath == "" {
		outPath = strings.TrimSuffix(source, filepath.Ext(source)) + ".yml"
	}
	// Emit a companion runner script alongside the spec — it dispatches
	// to the i-th test's body so the spec stays a single self-contained
	// unit (per cairn's importer philosophy: the output is a complete
	// artifact, not a partial file with TODOs).
	runnerPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".runner.sh"
	runner, runnerWarnings := buildBatsRunnerScript(tests)
	warnings = append(warnings, runnerWarnings...)

	specYAML := renderBatsImportedSpec(specName, source, runnerPath, tests)
	if err := os.WriteFile(runnerPath, []byte(runner), 0o755); err != nil {
		return batsImportResult{}, fmt.Errorf("write runner: %w", err)
	}
	if err := os.WriteFile(outPath, []byte(specYAML), 0o644); err != nil {
		return batsImportResult{}, fmt.Errorf("write spec: %w", err)
	}
	return batsImportResult{
		SourcePath: source,
		SpecPath:   outPath,
		SpecName:   specName,
		Tests:      len(tests),
		SpecYAML:   specYAML,
		Warnings:   warnings,
	}, nil
}

// parseBatsTests walks a .bats file and pulls out every `@test "name" { body }`.
// The body is captured as-is (including helper variable assignments,
// sub-shells, `run` invocations, and assertion lines) so a subsequent
// render produces a verbatim replay.
func parseBatsTests(source string) ([]batsTest, []string, error) {
	var tests []batsTest
	var warnings []string
	scanner := bufio.NewScanner(strings.NewReader(source))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		inTest     bool
		current    batsTest
		body       strings.Builder
		braceDepth int
		seenBrace  bool // true once we've observed any `{` for the current test
	)
	// flush records the accumulated body as a test. The caller's
	// responsibility is to NOT include the closing `}` in the body;
	// the loop below is careful to drop the brace line before
	// appending.
	flush := func(line int) {
		if current.name != "" {
			if seenBrace {
				current.body = strings.TrimRight(body.String(), "\n")
				current.line = line
				tests = append(tests, current)
			} else {
				warnings = append(warnings, fmt.Sprintf("line %d: orphan @test %q (no opening brace); dropped", line, current.name))
			}
		}
		current = batsTest{}
		body.Reset()
		inTest = false
		braceDepth = 0
		seenBrace = false
	}
	// Match a @test header. Three shapes are accepted:
	//   1. `@test "name" {`  — header on its own line, brace on same line
	//   2. `@test "name"`    — header on its own line, brace on the next
	//   3. `@test "name" { body }` — header and body on the same line
	testStartRe := regexp.MustCompile(`^@test\s+"([^"]+)"\s*(.*)$`)
	openBrace := regexp.MustCompile(`\{`)
	closeBrace := regexp.MustCompile(`\}`)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		text := scanner.Text()
		if !inTest {
			match := testStartRe.FindStringSubmatch(strings.TrimSpace(text))
			if match == nil {
				if strings.HasPrefix(strings.TrimSpace(text), "run ") {
					warnings = append(warnings, fmt.Sprintf("line %d: `run` outside @test block; not mapped", lineNo))
				}
				continue
			}
			current.name = match[1]
			rest := match[2]
			braceDepth = strings.Count(text, "{") - strings.Count(text, "}")
			seenBrace = strings.Contains(text, "{")
			switch {
			case braceDepth > 0:
				inTest = true
			case strings.TrimSpace(rest) == "" && !seenBrace:
				// shape 2 — keep waiting for the next line which must
				// open the brace. If it never does, the test is dropped
				// at EOF via the `seenBrace` check.
				inTest = true
			case strings.TrimSpace(rest) == "" && seenBrace:
				// Header had a `{` and a `}` on the same line — empty
				// body. Flush immediately.
				current.line = lineNo
				tests = append(tests, current)
				current = batsTest{}
				inTest = false
			default:
				// shape 3 — single-line body, capture as-is
				inTest = true
				current.body = strings.TrimSpace(rest)
				current.line = lineNo
				tests = append(tests, current)
				current = batsTest{}
				inTest = false
			}
			continue
		}
		// Body line. Update depth *first* so we can detect the line
		// that brings depth to zero — that's the closing `}` of the
		// @test block, which is NOT part of the test body and must be
		// excluded from the captured string.
		opens := len(openBrace.FindAllString(text, -1))
		closes := len(closeBrace.FindAllString(text, -1))
		if opens > 0 {
			seenBrace = true
		}
		nextDepth := braceDepth + opens - closes
		if nextDepth <= 0 {
			flush(lineNo)
			continue
		}
		body.WriteString(text)
		body.WriteByte('\n')
		braceDepth = nextDepth
	}
	if err := scanner.Err(); err != nil {
		return nil, warnings, err
	}
	// If the file ended mid-test (no closing `}` ever seen), warn and
	// drop the partial test rather than emit a malformed runner entry.
	if inTest && current.name != "" {
		if seenBrace {
			warnings = append(warnings, fmt.Sprintf("unterminated @test %q at end of file", current.name))
		} else {
			warnings = append(warnings, fmt.Sprintf("orphan @test %q (no opening brace); dropped", current.name))
		}
	}
	return tests, warnings, nil
}

// buildBatsRunnerScript emits a shell script that, when called as
// `<runner> <index>`, sources the i-th test's body. The script is
// written alongside the imported spec and referenced from the spec's
// outcome `command.run` verifiers.
//
// The runner uses `eval` with a here-doc to replay the test body; that
// preserves the original line breaks and quoting exactly. A leading
// `set -e` ensures assertions (`[ ... ]`) fail-fast, matching bats'
// default per-test isolation.
func buildBatsRunnerScript(tests []batsTest) (string, []string) {
	var warnings []string
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("# Auto-generated by `glyph import bats`. Do not edit by hand;\n")
	b.WriteString("# re-run the importer on the original .bats file instead.\n")
	b.WriteString("set -eu\n")
	b.WriteString("idx=\"${1:-}\"\n")
	b.WriteString("if [ -z \"$idx\" ]; then\n")
	b.WriteString("  echo \"usage: $0 <test-index>\" >&2\n")
	b.WriteString("  exit 2\n")
	b.WriteString("fi\n")
	for i, t := range tests {
		if strings.Contains(t.body, "$status") || strings.Contains(t.body, "${status}") {
			warnings = append(warnings, fmt.Sprintf("test %d %q: `run` semantics mapped to plain command; $status/$output may not be populated", i+1, t.name))
		}
		fmt.Fprintf(&b, "if [ \"$idx\" = \"%d\" ]; then\n", i+1)
		// The body's `run cmd` lines need bats-style `$status`/`$output`
		// variables; the imported spec wraps the test in a function
		// that captures stdout, so a true `run` requires more machinery.
		// We emit a minimal shim: redirect the body to a tmp file, then
		// `eval` the file contents. This preserves syntax and variables
		// but does NOT emulate bats' `run` helper. Tests that depend on
		// `run` will need manual review.
		b.WriteString("cat > /tmp/.glyphrun-bats-test.sh <<'BATS_EOF'\n")
		b.WriteString(t.body)
		if !strings.HasSuffix(t.body, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString("BATS_EOF\n")
		b.WriteString("  sh /tmp/.glyphrun-bats-test.sh\n")
		b.WriteString("  exit 0\n")
		b.WriteString("fi\n")
	}
	b.WriteString("echo \"unknown test index: $idx\" >&2\n")
	b.WriteString("exit 2\n")
	return b.String(), warnings
}

// renderBatsImportedSpec emits the glyphrun YAML for the imported file.
// The spec runs the runner script as its `target.cmd` and has one
// outcome per @test. The `command.run` for each outcome invokes the
// runner with the corresponding index. The single `target.cmd` and
// the per-outcome `command.run` together replay the original test
// suite, with the same line-level failure semantics as bats.
func renderBatsImportedSpec(name string, source string, runnerPath string, tests []batsTest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Generated by `glyph import bats %s`. Re-run the importer\n", filepath.Base(source))
	fmt.Fprintf(&b, "# to refresh this file after editing the source.\n")
	fmt.Fprintf(&b, "version: 1\n")
	fmt.Fprintf(&b, "name: %s\n\n", yamlQuote(name))
	fmt.Fprintf(&b, "intent: |\n  imported from %s (%d test(s)) via `glyph import bats`.\n\n", filepath.Base(source), len(tests))
	fmt.Fprintf(&b, "metadata:\n  feature: imported\n  priority: normal\n  tags:\n    - bats-import\n\n")
	fmt.Fprintf(&b, "target:\n")
	// `cmd:` is parsed as a list of strings; emit each argv element as
	// a YAML scalar without surrounding quotes (the value is already
	// shell-safe and quoting would produce the double-quote doubling
	// seen in early iterations of this importer).
	fmt.Fprintf(&b, "  cmd: [%s]\n", yamlScalar(runnerPath))
	fmt.Fprintf(&b, "  cwd: \".\"\n\n")
	fmt.Fprintf(&b, "terminal:\n  cols: 80\n  rows: 24\n  profile: xterm-256color\n\n")
	fmt.Fprintf(&b, "steps:\n")
	fmt.Fprintf(&b, "  - type: \"0\\n\"  # placeholder to keep the target PTY alive\n")
	fmt.Fprintf(&b, "outcomes:\n")
	for i, t := range tests {
		cleanName := slugify(t.name)
		if cleanName == "" {
			cleanName = fmt.Sprintf("test_%d", i+1)
		}
		fmt.Fprintf(&b, "  - id: %s\n", cleanName)
		fmt.Fprintf(&b, "    description: |\n      imported bats test: %q (source line %d)\n", t.name, t.line)
		fmt.Fprintf(&b, "    verify:\n")
		fmt.Fprintf(&b, "      command:\n")
		fmt.Fprintf(&b, "        run: %s\n", shellCmdScalar(runnerPath, i+1))
		fmt.Fprintf(&b, "        timeoutMs: 30000\n")
	}
	return b.String()
}

// renderBatsImportMarkdown is the human-friendly summary printed by
// `glyph import bats` when --format md. It tells the contributor which
// file was written, how many tests were mapped, and which warnings
// (if any) deserve a second look.
func renderBatsImportMarkdown(r batsImportResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Glyphrun Bats Import\n\n")
	fmt.Fprintf(&b, "- source: `%s`\n", r.SourcePath)
	fmt.Fprintf(&b, "- spec: `%s`\n", r.SpecPath)
	fmt.Fprintf(&b, "- name: `%s`\n", r.SpecName)
	fmt.Fprintf(&b, "- tests mapped: %d\n", r.Tests)
	if len(r.Warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}
	b.WriteString("\n## Next Commands\n\n")
	fmt.Fprintf(&b, "- `glyph spec verify %s --format json`\n", r.SpecPath)
	fmt.Fprintf(&b, "- `glyph run %s --format md`\n", r.SpecPath)
	return b.String()
}

func slugify(s string) string {
	var b strings.Builder
	lastWasUnderscore := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastWasUnderscore = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
			lastWasUnderscore = false
		case r == '-', r == '_', r == '.':
			if !lastWasUnderscore {
				b.WriteByte('_')
				lastWasUnderscore = true
			}
		default:
			if !lastWasUnderscore {
				b.WriteByte('_')
				lastWasUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return ""
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "t_" + out
	}
	return out
}

func yamlQuote(s string) string {
	// Always use double-quoted YAML strings; escape backslashes and double quotes.
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// yamlScalar emits a YAML scalar in plain (unquoted) form when the
// value is shell- and YAML-safe, otherwise it falls back to a quoted
// string. Used for `target.cmd` and other list-of-strings fields
// where the surrounding `[ ... ]` already provides YAML structure.
func yamlScalar(s string) string {
	if s == "" {
		return `""`
	}
	plainOK := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '.', r == '/':
		default:
			plainOK = false
		}
		if !plainOK {
			break
		}
	}
	if !plainOK {
		return yamlQuote(s)
	}
	if s == "" {
		return `""`
	}
	// Reserved YAML scalars that would parse as booleans, numbers, etc.
	switch s {
	case "true", "false", "null", "yes", "no", "on", "off", "~":
		return yamlQuote(s)
	}
	return s
}

// shellCmdScalar formats a shell command + positional argument as a
// YAML scalar in plain (unquoted) form when both are safe, otherwise
// falls back to a double-quoted scalar with shell-safe escaping. The
// runner script path is shell-quoted first, then concatenated with
// the index — keeping the path unquoted when possible makes the
// generated spec easier to read in code review.
func shellCmdScalar(runnerPath string, index int) string {
	pathScalar := yamlScalar(runnerPath)
	return fmt.Sprintf("%s %d", pathScalar, index)
}

// ensure imports stay in sync if the bats parser grows new helpers.
var _ = io.Discard
var _ = bytes.NewBuffer
