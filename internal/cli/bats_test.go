package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestImportBats_HappyPath exercises the importer end-to-end on a small
// .bats file with three @test blocks. It asserts:
//   - the spec file is written next to the source
//   - the runner script is written next to the spec
//   - the spec has one outcome per @test
//   - the runner script correctly dispatches to each test body
func TestImportBats_HappyPath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "sample.bats")
	body := `#!/usr/bin/env bats

@test "addition" {
  result="$((1 + 1))"
  [ "$result" -eq 2 ]
}

@test "greeting" {
  result="$(printf hello)"
  [ "$result" = "hello" ]
}

@test "third" {
  echo "ok"
}
`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := importBats(src, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Tests != 3 {
		t.Fatalf("expected 3 tests, got %d", res.Tests)
	}
	// Spec file exists, next to the source.
	if _, err := os.Stat(res.SpecPath); err != nil {
		t.Fatalf("spec file not written: %v", err)
	}
	// Runner script exists.
	runnerPath := strings.TrimSuffix(res.SpecPath, filepath.Ext(res.SpecPath)) + ".runner.sh"
	if _, err := os.Stat(runnerPath); err != nil {
		t.Fatalf("runner script not written: %v", err)
	}
	// Spec content includes one outcome per test.
	for _, name := range []string{"addition", "greeting", "third"} {
		if !strings.Contains(res.SpecYAML, "id: "+name) {
			t.Errorf("spec missing outcome for %q", name)
		}
	}
	// Runner script dispatches on each test's index.
	runnerBytes, err := os.ReadFile(runnerPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := string(runnerBytes)
	for _, marker := range []string{
		`if [ "$idx" = "1" ]`,
		`if [ "$idx" = "2" ]`,
		`if [ "$idx" = "3" ]`,
	} {
		if !strings.Contains(runner, marker) {
			t.Errorf("runner missing %q", marker)
		}
	}
}

// TestImportBats_NameOverride confirms the --name flag overrides the
// default (which is derived from the source filename).
func TestImportBats_NameOverride(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "ignored.bats")
	if err := os.WriteFile(src, []byte(`@test "a" { true; }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := importBats(src, "", "custom_name")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.SpecYAML, `name: "custom_name"`) {
		t.Errorf("name override not applied; spec:\n%s", res.SpecYAML)
	}
}

// TestImportBats_NoTests is the error-path: a .bats file with no
// @test blocks must surface a clear error, not a silent success.
func TestImportBats_NoTests(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "empty.bats")
	if err := os.WriteFile(src, []byte(`#!/usr/bin/env bats
# no tests here
`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := importBats(src, "", "")
	if err == nil {
		t.Fatal("expected error on empty .bats, got nil")
	}
	if !strings.Contains(err.Error(), "no @test blocks") {
		t.Errorf("error message should mention the missing @test blocks, got %q", err.Error())
	}
}

// TestImportBats_MalformedBats covers the orphan @test case — a
// header that never opens a `{`. The parser must warn, drop the
// orphan, and not panic.
func TestImportBats_MalformedBats(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "broken.bats")
	body := `#!/usr/bin/env bats

@test "complete" {
  [ "x" = "x" ]
}

@test "unterminated"
  [ "y" = "y" ]
`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := importBats(src, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Tests != 1 {
		t.Errorf("expected 1 valid test, got %d", res.Tests)
	}
	hasWarn := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "orphan") {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected orphan warning, got %v", res.Warnings)
	}
}

// TestExportBats_HappyPath confirms the exporter emits a .bats file
// with one @test per outcome, matching the spec's `intent` and
// `metadata` blocks in comment headers.
func TestExportBats_HappyPath(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "demo.yml")
	body := `version: 1
name: demo
metadata:
  feature: smoke
  priority: high
  tags: [s, t]
intent: a demo spec
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: first
    description: first outcome
    verify:
      command:
        run: "true"
  - id: second
    description: second outcome
    verify:
      command:
        run: "true"
`
	if err := os.WriteFile(specPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := exportBats(specPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Tests != 2 {
		t.Errorf("expected 2 tests, got %d", res.Tests)
	}
	batsBytes, err := os.ReadFile(res.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	bats := string(batsBytes)
	for _, want := range []string{
		`@test "first"`,
		`@test "second"`,
		`# Intent: a demo spec`,
		`# Feature: smoke`,
		`# Tag: s`,
		`# Tag: t`,
	} {
		if !strings.Contains(bats, want) {
			t.Errorf("exported bats missing %q", want)
		}
	}
}

// TestSlugify_Stable locks the slug mapping so a spec name like
// `"Login with OAuth (PKCE)"` always becomes `login_with_oauth_pkce`.
// This matters because the importer uses the slug as the spec
// outcome ID, and a regression here would rename outcomes silently.
func TestSlugify_Stable(t *testing.T) {
	cases := map[string]string{
		"Login with OAuth (PKCE)": "login_with_oauth_pkce",
		"plain":                   "plain",
		"123-numbers":             "t_123_numbers",
		"!!!":                     "",
		"":                        "",
		"a---b":                   "a_b",
		"Already_Snake":           "already_snake",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
