package stories

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"gopkg.in/yaml.v3"
)

// ManifestKind is the `kind` value that marks a stories manifest.
const ManifestKind = "stories"

// Manifest is the authoring surface for stories: one harness binary plus the
// list of isolated states it can mount. It is the terminal-shaped cousin of a
// Storybook `*.stories.*` file. Expand turns it into ordinary specs so the
// runner, verifiers, goldens, and artifacts stay identical to a hand-written
// spec — the manifest only removes the boilerplate.
type Manifest struct {
	Version  int      `yaml:"version" json:"version"`
	Kind     string   `yaml:"kind" json:"kind"`
	Name     string   `yaml:"name,omitempty" json:"name,omitempty"`
	Harness  Harness  `yaml:"harness" json:"harness"`
	Defaults Defaults `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Stories  []Entry  `yaml:"stories" json:"stories"`
}

// Harness describes the story binary. Build runs once per `glyph stories run`
// (not once per story), Watch lists extra paths that should re-trigger a
// watch loop (typically the harness source directory). Cwd, Build, and
// Watch entries are relative to the project root (where glyphrun.config.yml
// lives), exactly like a spec's target.cwd — not to the manifest file.
type Harness struct {
	Cmd            []string          `yaml:"cmd" json:"cmd"`
	Cwd            string            `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	Env            map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Build          string            `yaml:"build,omitempty" json:"build,omitempty"`
	BuildTimeoutMS int               `yaml:"buildTimeoutMs,omitempty" json:"buildTimeoutMs,omitempty"`
	Watch          []string          `yaml:"watch,omitempty" json:"watch,omitempty"`
}

// Defaults apply to every story unless the story overrides them.
type Defaults struct {
	Terminal       spec.Terminal `yaml:"terminal,omitempty" json:"terminal,omitempty"`
	ReadyTimeoutMS int           `yaml:"readyTimeoutMs,omitempty" json:"readyTimeoutMs,omitempty"`
	Quit           *string       `yaml:"quit,omitempty" json:"quit,omitempty"`
	ExitTimeoutMS  int           `yaml:"exitTimeoutMs,omitempty" json:"exitTimeoutMs,omitempty"`
	Golden         *bool         `yaml:"golden,omitempty" json:"golden,omitempty"`
	// GoldenMode is the snapshot verifier mode: "text" (default; normalized
	// characters, portable across terminals), "cell" (characters and styles,
	// so a color regression fails the run), or "json" (cells plus cursor).
	GoldenMode string   `yaml:"goldenMode,omitempty" json:"goldenMode,omitempty"`
	Tags       []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Owner      string   `yaml:"owner,omitempty" json:"owner,omitempty"`
}

// Entry is one story: an id the harness understands plus optional
// overrides. Ready is the screen condition that marks the state as mounted;
// Steps run after that and before the snapshot; Outcomes are appended to the
// generated ones. Variants re-run the same story with a different terminal
// size, env, or args — the equivalent of Storybook args.
type Entry struct {
	ID             string                `yaml:"id" json:"id"`
	Feature        string                `yaml:"feature,omitempty" json:"feature,omitempty"`
	Intent         string                `yaml:"intent,omitempty" json:"intent,omitempty"`
	Tags           []string              `yaml:"tags,omitempty" json:"tags,omitempty"`
	Args           []string              `yaml:"args,omitempty" json:"args,omitempty"`
	Env            map[string]string     `yaml:"env,omitempty" json:"env,omitempty"`
	Terminal       spec.Terminal         `yaml:"terminal,omitempty" json:"terminal,omitempty"`
	Ready          *spec.ScreenCondition `yaml:"ready,omitempty" json:"ready,omitempty"`
	ReadyTimeoutMS int                   `yaml:"readyTimeoutMs,omitempty" json:"readyTimeoutMs,omitempty"`
	Quit           *string               `yaml:"quit,omitempty" json:"quit,omitempty"`
	Golden         *bool                 `yaml:"golden,omitempty" json:"golden,omitempty"`
	GoldenMode     string                `yaml:"goldenMode,omitempty" json:"goldenMode,omitempty"`
	Steps          []spec.Step           `yaml:"steps,omitempty" json:"steps,omitempty"`
	Outcomes       []spec.Outcome        `yaml:"outcomes,omitempty" json:"outcomes,omitempty"`
	Variants       []Variant             `yaml:"variants,omitempty" json:"variants,omitempty"`
}

// Variant is a named re-run of a story with overrides layered on top.
type Variant struct {
	Name     string            `yaml:"name" json:"name"`
	Terminal spec.Terminal     `yaml:"terminal,omitempty" json:"terminal,omitempty"`
	Env      map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Args     []string          `yaml:"args,omitempty" json:"args,omitempty"`
}

// Expanded is one runnable story: the spec the runner executes plus the
// identity the catalog shows (id, variant, snapshot name).
type Expanded struct {
	ID           string
	Variant      string
	Feature      string
	SnapshotName string
	Golden       bool
	ManifestPath string
	Spec         spec.Spec
	Parsed       spec.ParseResult
}

// GoldenOutcomeID is the id of the generated snapshot-verifier outcome, or
// "" when the story keeps no golden. Consumers match outcomes by this id,
// never by message text.
const GoldenOutcomeID = "golden"

// GoldenOutcome returns the name and outcome id of the first snapshot
// verifier in a spec ("" when there is none). It is the single place that
// decides which outcome a story's golden state is read from.
func GoldenOutcome(s spec.Spec) (snapshotName, outcomeID string) {
	name, id, _ := GoldenOutcomeMode(s)
	return name, id
}

// GoldenOutcomeMode is GoldenOutcome plus the verifier mode ("text" when
// unset). The catalog uses the mode to decide whether a style-only change
// counts against the golden or is only reported.
func GoldenOutcomeMode(s spec.Spec) (snapshotName, outcomeID, mode string) {
	for _, o := range s.Outcomes {
		if o.Verify.Snapshot != nil {
			mode := strings.ToLower(strings.TrimSpace(o.Verify.Snapshot.Mode))
			if mode == "" {
				mode = "text"
			}
			return o.Verify.Snapshot.Name, o.ID, mode
		}
	}
	return "", "", ""
}

// Key is the catalog identity: `list/rows` or `list/rows@wide`.
func (e Expanded) Key() string {
	if e.Variant == "" {
		return e.ID
	}
	return e.ID + "@" + e.Variant
}

const (
	defaultReadyTimeoutMS = 5000
	defaultExitTimeoutMS  = 3000
	defaultQuitKey        = "q"
	defaultIdleQuietMS    = 300
)

// IsManifestPath reports whether a file name follows the manifest convention:
// `stories.yml`, `stories.yaml`, or `*.stories.yml` / `*.stories.yaml`.
func IsManifestPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "stories.yml", "stories.yaml":
		return true
	}
	return strings.HasSuffix(base, ".stories.yml") || strings.HasSuffix(base, ".stories.yaml")
}

// IsManifestSource reports whether a document declares `kind: stories`.
func IsManifestSource(source []byte) bool {
	return spec.DocumentKind(source) == ManifestKind
}

// FindManifests walks the given files/directories and returns manifest paths
// (by file name convention, or by `kind: stories` for explicit files).
func FindManifests(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if !info.IsDir() {
			if IsManifestPath(abs) {
				add(abs)
				continue
			}
			if data, err := os.ReadFile(abs); err == nil && IsManifestSource(data) {
				add(abs)
			}
			continue
		}
		err = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				switch d.Name() {
				case ".git", "node_modules", ".glyphrun", "vendor":
					return filepath.SkipDir
				}
				return nil
			}
			if IsManifestPath(path) {
				add(path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// LoadManifest reads, schema-validates, and strictly decodes a manifest. The
// parse options carry the project root and schemaRoot so a project-level
// schema override applies to manifests the same way it applies to specs.
func LoadManifest(path string, opts spec.ParseOptions) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	return ParseManifest(data, path, opts)
}

// ParseManifest validates a manifest document against the stories JSON
// schema (project schemaRoot first, embedded fallback), then decodes it with
// unknown fields rejected.
func ParseManifest(data []byte, path string, opts spec.ParseOptions) (Manifest, error) {
	if err := validateManifestSchema(data, path, opts); err != nil {
		return Manifest{}, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", path, err)
	}
	if m.Kind != ManifestKind {
		return Manifest{}, fmt.Errorf("%s: kind must be %q", path, ManifestKind)
	}
	if m.Name == "" {
		m.Name = filepath.Base(filepath.Dir(path))
	}
	ids := map[string]bool{}
	for i, st := range m.Stories {
		if strings.TrimSpace(st.ID) == "" {
			return Manifest{}, fmt.Errorf("%s: stories[%d]: id is required", path, i)
		}
		if ids[st.ID] {
			return Manifest{}, fmt.Errorf("%s: duplicate story id %q", path, st.ID)
		}
		ids[st.ID] = true
		names := map[string]bool{}
		for j, v := range st.Variants {
			if strings.TrimSpace(v.Name) == "" {
				return Manifest{}, fmt.Errorf("%s: story %q variants[%d]: name is required", path, st.ID, j)
			}
			if names[v.Name] {
				return Manifest{}, fmt.Errorf("%s: story %q: duplicate variant %q", path, st.ID, v.Name)
			}
			names[v.Name] = true
		}
	}
	return m, nil
}

func validateManifestSchema(data []byte, path string, opts spec.ParseOptions) error {
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := spec.ValidateDocumentSchema(spec.ToJSONValue(document), path, "glyphrun.stories.v1.schema.json", opts); err != nil {
		return fmt.Errorf("%s: stories manifest schema: %w", path, err)
	}
	return nil
}

// Expand turns every story (times its variants) into a validated spec. The
// defaultTerminal fills cols/rows/profile the manifest left unset (the same
// fallback ParseFile applies to a spec file).
func Expand(m Manifest, manifestPath string, defaultTerminal spec.Terminal) ([]Expanded, error) {
	abs, err := filepath.Abs(manifestPath)
	if err != nil {
		return nil, err
	}
	if len(m.Harness.Cmd) == 0 || m.Harness.Cmd[0] == "" {
		return nil, fmt.Errorf("%s: harness.cmd is required", manifestPath)
	}
	var out []Expanded
	// Spec names key run ids, goldens, and the index, so two ids that
	// sanitize to the same name (list/rows vs list_rows) must be rejected
	// rather than silently sharing a golden.
	names := map[string]string{}
	for _, entry := range m.Stories {
		base, err := expandOne(m, entry, Variant{}, abs, defaultTerminal)
		if err != nil {
			return nil, err
		}
		if prev, dup := names[base.Spec.Name]; dup {
			return nil, fmt.Errorf("%s: stories %q and %q both map to spec name %q", manifestPath, prev, base.Key(), base.Spec.Name)
		}
		names[base.Spec.Name] = base.Key()
		out = append(out, base)
		for _, v := range entry.Variants {
			ex, err := expandOne(m, entry, v, abs, defaultTerminal)
			if err != nil {
				return nil, err
			}
			if prev, dup := names[ex.Spec.Name]; dup {
				return nil, fmt.Errorf("%s: stories %q and %q both map to spec name %q", manifestPath, prev, ex.Key(), ex.Spec.Name)
			}
			names[ex.Spec.Name] = ex.Key()
			out = append(out, ex)
		}
	}
	return out, nil
}

func expandOne(m Manifest, entry Entry, v Variant, manifestPath string, defaultTerminal spec.Terminal) (Expanded, error) {
	id := strings.TrimSpace(entry.ID)
	feature := entry.Feature
	if feature == "" {
		if i := strings.Index(id, "/"); i > 0 {
			feature = id[:i]
		} else {
			feature = m.Name
		}
	}
	snapName := artifacts.SanitizeRunName(strings.ReplaceAll(id, "/", "_"))
	if i := strings.LastIndex(id, "/"); i >= 0 && i+1 < len(id) {
		snapName = artifacts.SanitizeRunName(id[i+1:])
	}
	specName := SpecNameForStory(id, v.Name)

	golden := true
	if m.Defaults.Golden != nil {
		golden = *m.Defaults.Golden
	}
	if entry.Golden != nil {
		golden = *entry.Golden
	}
	quit := defaultQuitKey
	if m.Defaults.Quit != nil {
		quit = *m.Defaults.Quit
	}
	if entry.Quit != nil {
		quit = *entry.Quit
	}
	readyTimeout := m.Defaults.ReadyTimeoutMS
	if entry.ReadyTimeoutMS > 0 {
		readyTimeout = entry.ReadyTimeoutMS
	}
	if readyTimeout <= 0 {
		readyTimeout = defaultReadyTimeoutMS
	}
	exitTimeout := m.Defaults.ExitTimeoutMS
	if exitTimeout <= 0 {
		exitTimeout = defaultExitTimeoutMS
	}

	args := []string{id}
	if len(entry.Args) > 0 {
		args = append([]string(nil), entry.Args...)
	}
	if len(v.Args) > 0 {
		args = append([]string(nil), v.Args...)
	}
	cmd := append(append([]string(nil), m.Harness.Cmd...), args...)

	env := map[string]string{}
	for _, layer := range []map[string]string{m.Harness.Env, entry.Env, v.Env} {
		for k, val := range layer {
			env[k] = val
		}
	}
	if len(env) == 0 {
		env = nil
	}

	term := mergeTerminal(mergeTerminal(m.Defaults.Terminal, entry.Terminal), v.Terminal)

	tags := dedupTags(append(append(append([]string{"story"}, m.Defaults.Tags...), entry.Tags...), variantTag(v.Name)...))

	intent := strings.TrimSpace(entry.Intent)
	if intent == "" {
		intent = fmt.Sprintf("story %s renders its isolated state", id)
	}
	if v.Name != "" {
		intent += fmt.Sprintf(" (variant %s)", v.Name)
	}

	var steps []spec.Step
	if entry.Ready != nil {
		steps = append(steps, spec.Step{ID: "ready", Wait: &spec.WaitStep{Screen: entry.Ready, TimeoutMS: readyTimeout}})
	} else {
		steps = append(steps, spec.Step{ID: "ready", Wait: &spec.WaitStep{Idle: &spec.IdleCondition{QuietForMS: defaultIdleQuietMS}, TimeoutMS: readyTimeout}})
	}
	steps = append(steps, entry.Steps...)
	steps = append(steps, spec.Step{ID: "capture", Snapshot: snapName})
	if quit != "" {
		zero := 0
		steps = append(steps,
			spec.Step{ID: "quit", Press: quit},
			spec.Step{ID: "exit", Wait: &spec.WaitStep{Process: &spec.ProcessCondition{ExitCode: &zero}, TimeoutMS: exitTimeout}},
		)
	}

	var outcomes []spec.Outcome
	if golden {
		goldenMode := strings.ToLower(strings.TrimSpace(entry.GoldenMode))
		if goldenMode == "" {
			goldenMode = strings.ToLower(strings.TrimSpace(m.Defaults.GoldenMode))
		}
		desc := "the screen matches the committed golden " + snapName
		if goldenMode == "cell" || goldenMode == "json" {
			desc += " (" + goldenMode + " mode: characters and styles)"
		}
		outcomes = append(outcomes, spec.Outcome{
			ID:          GoldenOutcomeID,
			Description: desc,
			Verify:      spec.Verify{Snapshot: &spec.SnapshotCondition{Name: snapName, Mode: goldenMode}},
		})
	}
	if entry.Ready != nil {
		outcomes = append(outcomes, spec.Outcome{
			ID:          "ready",
			Description: "the story state is on screen",
			Verify:      spec.Verify{Screen: entry.Ready},
		})
	}
	outcomes = append(outcomes, entry.Outcomes...)
	if len(outcomes) == 0 {
		outcomes = append(outcomes, spec.Outcome{
			ID:          "mounted",
			Description: "the story mounted without the target exiting early",
			Verify:      spec.Verify{Process: &spec.ProcessCondition{Exited: boolPtr(quit != "")}},
		})
	}

	s := spec.Spec{
		Version: 1,
		Name:    specName,
		Intent:  intent,
		Metadata: &spec.Metadata{
			Feature: feature,
			Owner:   m.Defaults.Owner,
			Tags:    tags,
		},
		Target: spec.Target{
			Cmd: cmd,
			Cwd: m.Harness.Cwd,
			Env: env,
		},
		Terminal: term,
		Steps:    steps,
		Outcomes: outcomes,
	}
	spec.ApplyDefaults(&s, defaultTerminal)
	if err := spec.Validate(s); err != nil {
		return Expanded{}, fmt.Errorf("%s: story %q: %w", manifestPath, expandedKey(id, v.Name), err)
	}
	hash, err := spec.ComputeContractHash(s)
	if err != nil {
		return Expanded{}, err
	}
	s.ContractHash = hash
	return Expanded{
		ID:           id,
		Variant:      v.Name,
		Feature:      feature,
		SnapshotName: snapName,
		Golden:       golden,
		ManifestPath: manifestPath,
		Spec:         s,
		Parsed: spec.ParseResult{
			Spec:              s,
			Resolved:          s,
			Path:              manifestPath,
			ContractHash:      hash,
			ContractHashValid: true,
		},
	}, nil
}

func expandedKey(id, variant string) string {
	if variant == "" {
		return id
	}
	return id + "@" + variant
}

// SpecNameForStory maps a story id (and optional variant) to the spec name
// used for run ids and golden directories: `list/rows` → `story_list_rows`,
// `list/rows@wide` → `story_list_rows__wide`. It uses the runner's own name
// sanitizer so the golden directory the catalog reads is the one the runner
// writes.
func SpecNameForStory(id, variant string) string {
	name := "story_" + artifacts.SanitizeRunName(strings.ReplaceAll(strings.TrimSpace(id), "/", "_"))
	if variant != "" {
		name += "__" + artifacts.SanitizeRunName(variant)
	}
	return name
}

func mergeTerminal(base, over spec.Terminal) spec.Terminal {
	if over.Cols > 0 {
		base.Cols = over.Cols
	}
	if over.Rows > 0 {
		base.Rows = over.Rows
	}
	if over.Profile != "" {
		base.Profile = over.Profile
	}
	if over.Color != "" {
		base.Color = over.Color
	}
	if over.AlternateScreen != "" {
		base.AlternateScreen = over.AlternateScreen
	}
	return base
}

func variantTag(name string) []string {
	if name == "" {
		return nil
	}
	return []string{"variant:" + name}
}

func dedupTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func boolPtr(b bool) *bool { return &b }
