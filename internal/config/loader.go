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
	if selected, ok := cfg.Environments[envName]; ok {
		for k, v := range selected.Vars {
			vars[k] = v
		}
		for k, v := range selected.Env {
			env[k] = v
		}
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
	base.Terminal.Normalize = overlay.Terminal.Normalize
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
