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
