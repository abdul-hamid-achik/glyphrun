package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func LoadRuntime(startPath string, explicitPath string, environment string) (Runtime, error) {
	cfg := Defaults()
	configPath, err := resolveConfigPath(startPath, explicitPath)
	if err != nil {
		return Runtime{}, err
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		return Runtime{}, err
	}

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return Runtime{}, err
		}
		if err := ValidateConfigSchema(data, configPath); err != nil {
			return Runtime{}, err
		}
		var loaded Config
		if err := yaml.Unmarshal(data, &loaded); err != nil {
			return Runtime{}, err
		}
		cfg = mergeConfig(cfg, loaded)
		if err := applyExplicitConfigFields(&cfg, data); err != nil {
			return Runtime{}, err
		}
		projectRoot = filepath.Dir(configPath)
	}

	envName := environment
	if envName == "" {
		envName = cfg.DefaultEnvironment
	}
	if envName == "" {
		envName = "local"
	}

	vars := map[string]string{}
	env := defaultEnv()
	var secrets *Secrets
	if selected, ok := cfg.Environments[envName]; ok {
		for k, v := range selected.Vars {
			vars[k] = v
		}
		for k, v := range selected.Env {
			env[k] = v
		}
		secrets = selected.Secrets
	}
	for _, pair := range os.Environ() {
		key, value, ok := splitEnv(pair)
		if ok {
			env[key] = value
		}
	}

	return Runtime{
		Config:      cfg,
		ConfigPath:  configPath,
		ProjectRoot: projectRoot,
		Environment: envName,
		Vars:        vars,
		Env:         env,
		Secrets:     secrets,
	}, nil
}

func applyExplicitConfigFields(cfg *Config, data []byte) error {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	if artifacts, ok := raw["artifacts"].(map[string]any); ok {
		if v, ok := artifacts["rawLog"].(bool); ok {
			cfg.Artifacts.RawLog = v
		}
		if v, ok := artifacts["frames"].(bool); ok {
			cfg.Artifacts.Frames = v
		}
		if v, ok := artifacts["finalScreen"].(bool); ok {
			cfg.Artifacts.FinalScreen = v
		}
		if v, ok := artifacts["snapshots"].(bool); ok {
			cfg.Artifacts.Snapshots = v
		}
		if v, ok := artifacts["agentContext"].(bool); ok {
			cfg.Artifacts.AgentContext = v
		}
	}
	if redaction, ok := raw["redaction"].(map[string]any); ok {
		if v, ok := redaction["enabled"].(bool); ok {
			cfg.Redaction.Enabled = v
		}
	}
	// Retention: actively distinguish an absent keepRuns from an
	// explicit one. Defaults() sets KeepRuns to DefaultKeepRuns; a
	// user who writes retention.keepRuns (including the opt-out 0)
	// must win over that default. We only override when the key is
	// literally present in the raw YAML.
	if retention, ok := raw["retention"].(map[string]any); ok {
		if v, ok := retention["keepRuns"]; ok {
			switch n := v.(type) {
			case int:
				cfg.Retention.KeepRuns = n
			case int64:
				cfg.Retention.KeepRuns = int(n)
			case float64:
				cfg.Retention.KeepRuns = int(n)
			}
		}
		if archive, ok := retention["archive"].(map[string]any); ok {
			if v, ok := archive["enabled"].(bool); ok {
				cfg.Retention.Archive.Enabled = v
			}
			if v, ok := archive["command"].(string); ok {
				cfg.Retention.Archive.Command = v
			}
			if v, ok := archive["timeout"].(string); ok {
				cfg.Retention.Archive.Timeout = v
			}
			if args, ok := archive["args"].([]any); ok {
				parsed := make([]string, 0, len(args))
				for _, a := range args {
					if s, ok := a.(string); ok {
						parsed = append(parsed, s)
					}
				}
				if len(parsed) > 0 {
					cfg.Retention.Archive.Args = parsed
				}
			}
		}
	}
	if terminal, ok := raw["terminal"].(map[string]any); ok {
		if normalize, ok := terminal["normalize"].(map[string]any); ok {
			if v, ok := normalize["trimRight"].(bool); ok {
				cfg.Terminal.Normalize.TrimRight = v
			}
			if v, ok := normalize["normalizeLineEndings"].(bool); ok {
				cfg.Terminal.Normalize.NormalizeLineEndings = v
			}
			if v, ok := normalize["hideCursorInTextSnapshots"].(bool); ok {
				cfg.Terminal.Normalize.HideCursorInTextSnapshots = v
			}
			if v, ok := normalize["stripAnsiTitle"].(bool); ok {
				cfg.Terminal.Normalize.StripAnsiTitle = v
			}
			if v, ok := normalize["ignoreCursorVisibility"].(bool); ok {
				cfg.Terminal.Normalize.IgnoreCursorVisibility = v
			}
		}
	}
	return nil
}

func resolveConfigPath(startPath string, explicitPath string) (string, error) {
	if explicitPath != "" {
		abs, err := filepath.Abs(explicitPath)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", err
		}
		return abs, nil
	}
	return FindConfig(startPath)
}

func FindConfig(startPath string) (string, error) {
	if startPath == "" {
		startPath = "."
	}
	abs, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	for {
		for _, name := range []string{"glyphrun.config.yml", "glyphrun.config.yaml"} {
			candidate := filepath.Join(abs, name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", nil
		}
		abs = parent
	}
}

func mergeConfig(base Config, overlay Config) Config {
	if overlay.Version != 0 {
		base.Version = overlay.Version
	}
	if overlay.DefaultEnvironment != "" {
		base.DefaultEnvironment = overlay.DefaultEnvironment
	}
	if overlay.ArtifactRoot != "" {
		base.ArtifactRoot = overlay.ArtifactRoot
	}
	if overlay.SnapshotRoot != "" {
		base.SnapshotRoot = overlay.SnapshotRoot
	}
	if overlay.SchemaRoot != "" {
		base.SchemaRoot = overlay.SchemaRoot
	}
	if len(overlay.Environments) > 0 {
		base.Environments = overlay.Environments
	}
	if overlay.Terminal.Profile != "" {
		base.Terminal.Profile = overlay.Terminal.Profile
	}
	if overlay.Terminal.Cols != 0 {
		base.Terminal.Cols = overlay.Terminal.Cols
	}
	if overlay.Terminal.Rows != 0 {
		base.Terminal.Rows = overlay.Terminal.Rows
	}
	// Normalize: do not clobber the struct wholesale — defaults set in
	// Defaults() (TrimRight, NormalizeLineEndings) must survive when the
	// user's config omits a normalize: block or sets only some fields.
	// The bool fields are re-applied by applyExplicitConfigFields from
	// the raw YAML so user-set values still win. Here we only need to
	// merge the slice fields, which have no zero-value ambiguity.
	if len(overlay.Terminal.Normalize.Replace) > 0 {
		base.Terminal.Normalize.Replace = overlay.Terminal.Normalize.Replace
	}
	if len(overlay.Terminal.Normalize.IgnoreRegions) > 0 {
		base.Terminal.Normalize.IgnoreRegions = overlay.Terminal.Normalize.IgnoreRegions
	}
	if overlay.Artifacts.MaxRawLogBytes != 0 {
		base.Artifacts.MaxRawLogBytes = overlay.Artifacts.MaxRawLogBytes
	}
	if overlay.Artifacts.RawLog {
		base.Artifacts.RawLog = true
	}
	if overlay.Artifacts.Frames {
		base.Artifacts.Frames = true
	}
	if overlay.Artifacts.FinalScreen {
		base.Artifacts.FinalScreen = true
	}
	if overlay.Artifacts.Snapshots {
		base.Artifacts.Snapshots = true
	}
	if overlay.Artifacts.AgentContext {
		base.Artifacts.AgentContext = true
	}
	if len(overlay.Redaction.Patterns) > 0 || len(overlay.Redaction.EnvAllowlist) > 0 {
		base.Redaction = overlay.Redaction
	}
	// Retention.KeepRuns is NOT merged here: the zero value from an
	// omitted block would clobber the DefaultKeepRuns default. The
	// explicit value (including the opt-out 0) is applied from the raw
	// YAML in applyExplicitConfigFields, which can distinguish absent
	// from explicit-zero. We only merge the archive sub-block's
	// non-bool fields here; Enabled is handled in
	// applyExplicitConfigFields for the same zero-ambiguity reason.
	if overlay.Retention.Archive.Command != "" {
		base.Retention.Archive.Command = overlay.Retention.Archive.Command
	}
	if len(overlay.Retention.Archive.Args) > 0 {
		base.Retention.Archive.Args = overlay.Retention.Archive.Args
	}
	if overlay.Retention.Archive.Timeout != "" {
		base.Retention.Archive.Timeout = overlay.Retention.Archive.Timeout
	}
	return base
}

func splitEnv(pair string) (string, string, bool) {
	for i := 0; i < len(pair); i++ {
		if pair[i] == '=' {
			return pair[:i], pair[i+1:], true
		}
	}
	return "", "", false
}
