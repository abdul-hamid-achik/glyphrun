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
	switch kind := DocumentKind(source); kind {
	case "":
	case "stories":
		return ParseResult{}, fmt.Errorf("%s is a stories manifest, not a spec; run it with `glyph stories run`", path)
	default:
		return ParseResult{}, fmt.Errorf("%s: unsupported document kind %q (specs carry no kind field)", path, kind)
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
		return ParseResult{}, ContractHashMismatchError{Expected: specValue.ContractHash, Actual: hash, SpecName: specValue.Name}
	}

	return ParseResult{
		Spec:              specValue,
		Resolved:          resolved,
		Path:              abs,
		ContractHash:      hash,
		ContractHashValid: valid,
	}, nil
}

// DocumentKind peeks at a YAML/JSON document's top-level `kind` field without
// decoding the rest. Specs omit it (or say "spec"); a stories manifest says
// "stories". An unreadable document yields "" so the normal parser reports
// the real error.
func DocumentKind(source []byte) string {
	var head struct {
		Kind string `yaml:"kind" json:"kind"`
	}
	if err := yaml.Unmarshal(source, &head); err != nil {
		return ""
	}
	return strings.TrimSpace(head.Kind)
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

// ApplyDefaults fills the target cwd and terminal fields a spec left unset
// from the project default terminal. ParseFile applies it to every file;
// callers that build specs in memory (stories) apply it before Validate.
func ApplyDefaults(s *Spec, terminal Terminal) { applyDefaults(s, terminal) }

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

// PlaceholderPattern matches one `${...}` span, or `$${...}`, its escape. A span
// holds no '$', '{' or '}', so in `${a:-${vars.b}}` the inner span is the one
// found and substituted. The
// runner shares it so the parse-time and run-time passes agree on what a span
// is. Whether a span is a Glyphrun placeholder is decided by IsPlaceholderKey,
// not by this pattern: scripts are full of `${...}` that belong to the shell.
var PlaceholderPattern = regexp.MustCompile(`\$\$\{[^{}$]+\}|\$\{[^{}$]+\}`)

// placeholderKeyPattern is the shape of a Glyphrun placeholder key:
//
//	projectRoot
//	vars.NAME   env.NAME   config.NAME
//	artifacts.NAME.FIELD   (resolved at run time)
//
// NAME starts with a letter, digit or underscore and continues with those plus
// '.' and '-', which covers every var, environment variable and config value
// name. None of this is valid shell parameter syntax (a '.' ends a parameter
// name), so a real key never collides with a shell expansion.
//
// A bare identifier is deliberately not a key, except projectRoot, the one
// documented bare key. ${HOME}, ${PWD} and ${IFS} are shell variables and
// pass through untouched; treating every identifier as a key would force an
// escape on each of them and break every existing script that uses one.
var placeholderKeyPattern = regexp.MustCompile(`^(?:projectRoot|(?:vars|env|config)\.[A-Za-z0-9_][A-Za-z0-9_.-]*|artifacts\.[A-Za-z0-9_][A-Za-z0-9_.-]*)$`)

// IsPlaceholderKey reports whether the text between `${` and `}` is shaped like
// a Glyphrun placeholder key. Anything else (`${HOME}`, `${var#*:}`,
// `${x:-y}`, `${#arr}`, a JavaScript `${a + b}`) belongs to the script and is
// left untouched. A key-shaped span that does not resolve is an error, so a
// typo such as `${vars.nmae}` is still caught.
func IsPlaceholderKey(key string) bool {
	return placeholderKeyPattern.MatchString(key)
}

// IsRuntimePlaceholder reports whether a placeholder key refers to a
// runtime-resolved value (currently only the artifact namespace) that the
// parser must leave intact and the runner resolves just before each step.
func IsRuntimePlaceholder(key string) bool {
	return strings.HasPrefix(key, "artifacts.")
}

// SubstitutePlaceholders resolves the parse-time placeholders in a spec or
// action source:
//
//   - `${projectRoot}`, `${vars.NAME}`, `${env.NAME}` and `${config.NAME}` are
//     replaced; one that is not defined is an error.
//   - `${artifacts.NAME.FIELD}` is left for the runner, which resolves it just
//     before each step.
//   - `$${...}` is an escape: it becomes a literal `${...}`, whatever is inside,
//     for text that is key-shaped but belongs to the script (`$${vars.x}` in a
//     JavaScript template, or `$${projectRoot}` for a shell variable of that
//     name). This is the only pass that reads the escape. An escaped artifact
//     key comes out as `${artifacts...}`, which the runner resolves in the
//     fields it resolves, so an artifact key cannot be escaped there.
//   - any other `${...}` is not a placeholder and is left as written.
func SubstitutePlaceholders(text string, filePath string, opts ParseOptions) (string, error) {
	projectRoot := opts.ProjectRoot
	if projectRoot == "" {
		projectRoot = filepath.Dir(filePath)
	}
	var missing []string
	out := PlaceholderPattern.ReplaceAllStringFunc(text, func(match string) string {
		if strings.HasPrefix(match, "$$") {
			return match[1:] // drop one '$': "$${x}" -> "${x}"
		}
		key := match[2 : len(match)-1]
		if !IsPlaceholderKey(key) {
			return match
		}
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
		return "", fmt.Errorf("unresolved placeholder(s) in %s: %s (write $${...} for a literal ${...})", filePath, strings.Join(missing, ", "))
	}
	return out, nil
}
