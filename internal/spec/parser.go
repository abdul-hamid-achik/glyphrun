package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type ParseOptions struct {
	ProjectRoot       string
	Vars              map[string]string
	Env               map[string]string
	ConfigValues      map[string]string
	DefaultTerminal   Terminal
	AllowHashMismatch bool
}

type ParseResult struct {
	Spec              Spec
	Resolved          Spec
	Path              string
	ContractHash      string
	ContractHashValid bool
}

func ParseFile(path string, opts ParseOptions) (ParseResult, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ParseResult{}, err
	}
	source, err := os.ReadFile(abs)
	if err != nil {
		return ParseResult{}, err
	}
	if err := ValidateSourceSchema(source, abs, opts); err != nil {
		return ParseResult{}, err
	}

	specValue, err := parseSpecSource(source, abs, opts)
	if err != nil {
		return ParseResult{}, err
	}
	applyDefaults(&specValue, opts.DefaultTerminal)
	if err := Validate(specValue); err != nil {
		return ParseResult{}, err
	}

	resolved, err := expandImports(specValue, abs, opts)
	if err != nil {
		return ParseResult{}, err
	}
	hash, err := ComputeContractHash(specValue)
	if err != nil {
		return ParseResult{}, err
	}
	valid := specValue.ContractHash != "" && specValue.ContractHash == hash
	if specValue.ContractHash != "" && !valid && !opts.AllowHashMismatch {
		return ParseResult{}, ContractHashMismatchError{Expected: specValue.ContractHash, Actual: hash}
	}

	return ParseResult{
		Spec:              specValue,
		Resolved:          resolved,
		Path:              abs,
		ContractHash:      hash,
		ContractHashValid: valid,
	}, nil
}

func ParseActionFile(path string, opts ParseOptions) (ReusableAction, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return ReusableAction{}, err
	}
	substituted, err := SubstitutePlaceholders(string(source), path, opts)
	if err != nil {
		return ReusableAction{}, err
	}
	var action ReusableAction
	if err := decodeKnown([]byte(substituted), &action); err != nil {
		return ReusableAction{}, err
	}
	if action.Name == "" {
		return ReusableAction{}, fmt.Errorf("action %s is missing name", path)
	}
	if len(action.Steps) == 0 {
		return ReusableAction{}, fmt.Errorf("action %s has no steps", path)
	}
	for i, step := range action.Steps {
		if err := validateStep(step); err != nil {
			return ReusableAction{}, fmt.Errorf("action %s step %d: %w", action.Name, i+1, err)
		}
	}
	return action, nil
}

func parseSpecSource(source []byte, path string, opts ParseOptions) (Spec, error) {
	substituted, err := SubstitutePlaceholders(string(source), path, opts)
	if err != nil {
		return Spec{}, err
	}
	var out Spec
	if strings.HasSuffix(path, ".json") {
		if err := json.Unmarshal([]byte(substituted), &out); err != nil {
			return Spec{}, err
		}
		return out, nil
	}
	if err := decodeKnown([]byte(substituted), &out); err != nil {
		return Spec{}, err
	}
	return out, nil
}

func decodeKnown(data []byte, target any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(target); err != nil {
		return err
	}
	return nil
}

func applyDefaults(s *Spec, terminal Terminal) {
	if s.Target.Cwd == "" {
		s.Target.Cwd = "."
	}
	if s.Terminal.Cols == 0 {
		s.Terminal.Cols = terminal.Cols
	}
	if s.Terminal.Rows == 0 {
		s.Terminal.Rows = terminal.Rows
	}
	if s.Terminal.Profile == "" {
		s.Terminal.Profile = terminal.Profile
	}
	if s.Terminal.Color == "" {
		s.Terminal.Color = "auto"
	}
	if s.Terminal.AlternateScreen == "" {
		s.Terminal.AlternateScreen = "auto"
	}
}

func expandImports(s Spec, specPath string, opts ParseOptions) (Spec, error) {
	if len(s.Imports) == 0 {
		return s, nil
	}
	actions := map[string]ReusableAction{}
	for _, importPath := range s.Imports {
		resolvedPath := importPath
		if !filepath.IsAbs(resolvedPath) {
			resolvedPath = filepath.Join(filepath.Dir(specPath), importPath)
		}
		action, err := ParseActionFile(resolvedPath, opts)
		if err != nil {
			return Spec{}, err
		}
		actions[action.Name] = action
	}
	var steps []Step
	for _, step := range s.Steps {
		if step.Use == "" {
			steps = append(steps, step)
			continue
		}
		action, ok := actions[step.Use]
		if !ok {
			return Spec{}, fmt.Errorf("unresolved action %q", step.Use)
		}
		steps = append(steps, action.Steps...)
	}
	s.Steps = steps
	return s, nil
}

var placeholderPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// IsRuntimePlaceholder reports whether a placeholder key refers to a
// runtime-resolved value (currently only the artifact namespace) that the
// parser must leave intact and the runner resolves just before each step.
func IsRuntimePlaceholder(key string) bool {
	return strings.HasPrefix(key, "artifacts.")
}

func SubstitutePlaceholders(text string, filePath string, opts ParseOptions) (string, error) {
	projectRoot := opts.ProjectRoot
	if projectRoot == "" {
		projectRoot = filepath.Dir(filePath)
	}
	var missing []string
	out := placeholderPattern.ReplaceAllStringFunc(text, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		// Artifact placeholders are resolved at run time, not parse time —
		// an earlier step's download/transform may populate them between
		// the parse step and the runner dispatch.
		if IsRuntimePlaceholder(key) {
			return match
		}
		switch {
		case key == "projectRoot":
			return projectRoot
		case strings.HasPrefix(key, "vars."):
			name := strings.TrimPrefix(key, "vars.")
			if value, ok := opts.Vars[name]; ok {
				return value
			}
		case strings.HasPrefix(key, "env."):
			name := strings.TrimPrefix(key, "env.")
			if value, ok := opts.Env[name]; ok {
				return value
			}
		case strings.HasPrefix(key, "config."):
			name := strings.TrimPrefix(key, "config.")
			if value, ok := opts.ConfigValues[name]; ok {
				return value
			}
		}
		missing = append(missing, key)
		return match
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("unresolved placeholder(s) in %s: %s", filePath, strings.Join(missing, ", "))
	}
	return out, nil
}
