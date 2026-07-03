package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

func TestMergeConfigPreservesNormalizeDefaults(t *testing.T) {
	base := Defaults()
	if !base.Terminal.Normalize.TrimRight {
		t.Fatal("default TrimRight should be true")
	}
	if !base.Terminal.Normalize.NormalizeLineEndings {
		t.Fatal("default NormalizeLineEndings should be true")
	}

	overlay := Config{}
	merged := mergeConfig(base, overlay)

	if !merged.Terminal.Normalize.TrimRight {
		t.Fatal("merged TrimRight lost default after empty overlay")
	}
	if !merged.Terminal.Normalize.NormalizeLineEndings {
		t.Fatal("merged NormalizeLineEndings lost default after empty overlay")
	}
}

func TestMergeConfigMergesNormalizeReplaceAndIgnoreRegions(t *testing.T) {
	base := Defaults()
	overlay := Config{}
	overlay.Terminal.Normalize.Replace = []spec.NormalizeReplace{
		{Regex: "foo", With: "bar"},
	}
	overlay.Terminal.Normalize.IgnoreRegions = []spec.NormalizeIgnoreArea{
		{X: 1, Y: 1, Width: 2, Height: 2},
	}

	merged := mergeConfig(base, overlay)

	if len(merged.Terminal.Normalize.Replace) != 1 {
		t.Fatalf("Replace not merged: %+v", merged.Terminal.Normalize.Replace)
	}
	if merged.Terminal.Normalize.Replace[0].Regex != "foo" {
		t.Fatalf("Replace[0].Regex = %q", merged.Terminal.Normalize.Replace[0].Regex)
	}
	if len(merged.Terminal.Normalize.IgnoreRegions) != 1 {
		t.Fatalf("IgnoreRegions not merged: %+v", merged.Terminal.Normalize.IgnoreRegions)
	}
	if merged.Terminal.Normalize.IgnoreRegions[0].X != 1 {
		t.Fatalf("IgnoreRegions[0].X = %d", merged.Terminal.Normalize.IgnoreRegions[0].X)
	}
}

func TestLoadRuntimePreservesNormalizeDefaultsWhenOmitted(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "glyphrun.config.yml")
	yaml := `version: 1
artifactRoot: .glyphrun/runs
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(dir, cfgPath, "")
	if err != nil {
		t.Fatal(err)
	}

	if !rt.Config.Terminal.Normalize.TrimRight {
		t.Fatalf("TrimRight default lost: %+v", rt.Config.Terminal.Normalize)
	}
	if !rt.Config.Terminal.Normalize.NormalizeLineEndings {
		t.Fatalf("NormalizeLineEndings default lost: %+v", rt.Config.Terminal.Normalize)
	}
}

func TestLoadRuntimeOverridesOnlyUserSetNormalizeFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "glyphrun.config.yml")
	yaml := `version: 1
terminal:
  cols: 80
  rows: 24
  normalize:
    trimRight: false
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(dir, cfgPath, "")
	if err != nil {
		t.Fatal(err)
	}

	if rt.Config.Terminal.Normalize.TrimRight {
		t.Fatal("user-set trimRight: false should override default true")
	}
	if !rt.Config.Terminal.Normalize.NormalizeLineEndings {
		t.Fatal("normalizeLineEndings default should survive when only trimRight is set")
	}
}

func TestLoadRuntimeSecretsBlock(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "glyphrun.config.yml")
	yaml := `version: 1
environments:
  local:
    secrets:
      group: liftclub
      env: preview
      only:
        - DATABASE_URL
        - STRIPE_SECRET_KEY
    env:
      TVAULT_DIR: .glyphrun/tmp/vault
      TVAULT_PASSPHRASE: glyphpass
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(dir, cfgPath, "")
	if err != nil {
		t.Fatal(err)
	}

	if rt.Secrets == nil {
		t.Fatal("expected Secrets to be populated")
	}
	if rt.Secrets.Group != "liftclub" {
		t.Fatalf("Group = %q, want %q", rt.Secrets.Group, "liftclub")
	}
	if rt.Secrets.Env != "preview" {
		t.Fatalf("Env = %q, want %q", rt.Secrets.Env, "preview")
	}
	if len(rt.Secrets.Only) != 2 {
		t.Fatalf("Only = %v, want 2 entries", rt.Secrets.Only)
	}
	if rt.Secrets.Only[0] != "DATABASE_URL" {
		t.Fatalf("Only[0] = %q, want %q", rt.Secrets.Only[0], "DATABASE_URL")
	}
}

func TestLoadRuntimeSecretsProjectMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "glyphrun.config.yml")
	yaml := `version: 1
environments:
  ci:
    secrets:
      project: liftclub-preview
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(dir, cfgPath, "ci")
	if err != nil {
		t.Fatal(err)
	}

	if rt.Secrets == nil {
		t.Fatal("expected Secrets to be populated")
	}
	if rt.Secrets.Project != "liftclub-preview" {
		t.Fatalf("Project = %q, want %q", rt.Secrets.Project, "liftclub-preview")
	}
	if rt.Secrets.Group != "" {
		t.Fatalf("Group = %q, want empty", rt.Secrets.Group)
	}
}

func TestLoadRuntimeNoSecretsBlockIsNil(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "glyphrun.config.yml")
	yaml := `version: 1
environments:
  local:
    env:
      TERM: xterm-256color
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := LoadRuntime(dir, cfgPath, "")
	if err != nil {
		t.Fatal(err)
	}

	if rt.Secrets != nil {
		t.Fatalf("expected nil Secrets, got %+v", rt.Secrets)
	}
}

// TestLoadRuntimeRetentionDefaultIsThree confirms that a config file
// with no retention block leaves the Defaults() KeepRuns (3) in
// place. The loader deliberately does NOT merge KeepRuns in
// mergeConfig so an omitted block cannot clobber the default.
func TestLoadRuntimeRetentionDefaultIsThree(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "glyphrun.config.yml")
	yaml := `version: 1
artifactRoot: .glyphrun/runs
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := LoadRuntime(dir, cfgPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if rt.Config.Retention.KeepRuns != 3 {
		t.Fatalf("KeepRuns = %d, want default 3", rt.Config.Retention.KeepRuns)
	}
}

// TestLoadRuntimeRetentionExplicitZeroOptsOut covers the opt-out: an
// explicit retention.keepRuns: 0 must win over the default 3 and
// disables auto-prune. applyExplicitConfigFields distinguishes absent
// from explicit-zero via the raw YAML.
func TestLoadRuntimeRetentionExplicitZeroOptsOut(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "glyphrun.config.yml")
	yaml := `version: 1
retention:
  keepRuns: 0
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := LoadRuntime(dir, cfgPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if rt.Config.Retention.KeepRuns != 0 {
		t.Fatalf("KeepRuns = %d, want 0 (opt-out)", rt.Config.Retention.KeepRuns)
	}
}

// TestLoadRuntimeRetentionExplicitKeep covers a non-default keep
// count: retention.keepRuns: 7 must override the default 3.
func TestLoadRuntimeRetentionExplicitKeep(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "glyphrun.config.yml")
	yaml := `version: 1
retention:
  keepRuns: 7
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := LoadRuntime(dir, cfgPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if rt.Config.Retention.KeepRuns != 7 {
		t.Fatalf("KeepRuns = %d, want 7", rt.Config.Retention.KeepRuns)
	}
}

// TestLoadRuntimeArchiveBlock covers the retention.archive sub-block:
// enabled, command, args, and timeout are read from the raw YAML.
// Timeout is stored as a string (parsed to duration later by
// artifacts.ParseArchiveTimeout in the runner), so we assert the
// string form.
func TestLoadRuntimeArchiveBlock(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "glyphrun.config.yml")
	yaml := `version: 1
retention:
  keepRuns: 3
  archive:
    enabled: true
    command: fcheap
    args:
      - store
    timeout: 90s
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := LoadRuntime(dir, cfgPath, "")
	if err != nil {
		t.Fatal(err)
	}
	a := rt.Config.Retention.Archive
	if !a.Enabled {
		t.Errorf("Archive.Enabled = false, want true")
	}
	if a.Command != "fcheap" {
		t.Errorf("Archive.Command = %q, want %q", a.Command, "fcheap")
	}
	if len(a.Args) != 1 || a.Args[0] != "store" {
		t.Errorf("Archive.Args = %v, want [store]", a.Args)
	}
	if a.Timeout != "90s" {
		t.Errorf("Archive.Timeout = %q, want %q", a.Timeout, "90s")
	}
}
