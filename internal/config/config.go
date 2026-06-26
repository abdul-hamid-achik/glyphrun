package config

import "github.com/abdul-hamid-achik/glyphrun/internal/spec"

const (
	DefaultConfigName   = "glyphrun.config.yml"
	DefaultArtifactRoot = ".glyphrun/runs"
	DefaultSnapshotRoot = ".glyphrun/snapshots"
	DefaultSchemaRoot   = "schemas"
)

type Config struct {
	Version            int                    `yaml:"version" json:"version"`
	DefaultEnvironment string                 `yaml:"defaultEnvironment" json:"defaultEnvironment"`
	ArtifactRoot       string                 `yaml:"artifactRoot" json:"artifactRoot"`
	SnapshotRoot       string                 `yaml:"snapshotRoot" json:"snapshotRoot"`
	SchemaRoot         string                 `yaml:"schemaRoot" json:"schemaRoot"`
	Environments       map[string]Environment `yaml:"environments" json:"environments"`
	Terminal           Terminal               `yaml:"terminal" json:"terminal"`
	Artifacts          Artifacts              `yaml:"artifacts" json:"artifacts"`
	Redaction          Redaction              `yaml:"redaction" json:"redaction"`
	Retention          Retention              `yaml:"retention,omitempty" json:"retention,omitempty"`
}

type Environment struct {
	Vars    map[string]string `yaml:"vars" json:"vars"`
	Env     map[string]string `yaml:"env" json:"env"`
	Secrets *Secrets          `yaml:"secrets,omitempty" json:"secrets,omitempty"`
}

// Secrets declares a tvault env-group whose resolved values are injected
// into the run environment at start time. The config file carries only the
// group/env names (or a direct project) — never secret values. At run time
// glyphrun calls `tvault env --group <g> --env <e> --format json`, parses the
// output, and merges the key/value pairs into the process environment. All
// resolved values are also added to the per-run redactor so they are scrubbed
// from every artifact before it lands on disk.
//
// Either (Group + Env) or Project must be set. If neither is set, the block is
// a no-op (useful for sharing a config across environments where only some
// have a tvault backend).
//
// Only and Prefix are optional least-privilege filters applied client-side
// after resolution. A key is kept if it matches either filter.
type Secrets struct {
	// Provider is the secrets backend. Currently only "tvault" is supported.
	// Defaults to "tvault" when empty.
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty"`

	// Binary is the path to the tvault executable. Defaults to "tvault"
	// (looked up on PATH).
	Binary string `yaml:"binary,omitempty" json:"binary,omitempty"`

	// Group is the tvault environment group name (e.g. "liftclub").
	// Requires Env to also be set.
	Group string `yaml:"group,omitempty" json:"group,omitempty"`

	// Env is the environment name within the group (e.g. "preview").
	// Requires Group to also be set.
	Env string `yaml:"env,omitempty" json:"env,omitempty"`

	// Project is a direct tvault project name, used when the project is not
	// part of an environment group. Mutually exclusive with Group+Env.
	Project string `yaml:"project,omitempty" json:"project,omitempty"`

	// Only is an explicit allowlist of secret keys to inject. Keys not in
	// this list are dropped after resolution.
	Only []string `yaml:"only,omitempty" json:"only,omitempty"`

	// Prefix injects only secret keys that start with this prefix.
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`
}

type Terminal struct {
	Profile   string    `yaml:"profile" json:"profile"`
	Cols      int       `yaml:"cols" json:"cols"`
	Rows      int       `yaml:"rows" json:"rows"`
	Normalize Normalize `yaml:"normalize" json:"normalize"`
}

type Normalize struct {
	TrimRight                 bool                       `yaml:"trimRight" json:"trimRight"`
	NormalizeLineEndings      bool                       `yaml:"normalizeLineEndings" json:"normalizeLineEndings"`
	HideCursorInTextSnapshots bool                       `yaml:"hideCursorInTextSnapshots" json:"hideCursorInTextSnapshots"`
	StripAnsiTitle            bool                       `yaml:"stripAnsiTitle" json:"stripAnsiTitle"`
	IgnoreCursorVisibility    bool                       `yaml:"ignoreCursorVisibility" json:"ignoreCursorVisibility"`
	Replace                   []spec.NormalizeReplace    `yaml:"replace" json:"replace"`
	IgnoreRegions             []spec.NormalizeIgnoreArea `yaml:"ignoreRegions" json:"ignoreRegions"`
}

type Artifacts struct {
	RawLog         bool  `yaml:"rawLog" json:"rawLog"`
	Frames         bool  `yaml:"frames" json:"frames"`
	FinalScreen    bool  `yaml:"finalScreen" json:"finalScreen"`
	Snapshots      bool  `yaml:"snapshots" json:"snapshots"`
	AgentContext   bool  `yaml:"agentContext" json:"agentContext"`
	MaxRawLogBytes int64 `yaml:"maxRawLogBytes" json:"maxRawLogBytes"`
}

type Redaction struct {
	Enabled      bool               `yaml:"enabled" json:"enabled"`
	EnvAllowlist []string           `yaml:"envAllowlist" json:"envAllowlist"`
	Patterns     []RedactionPattern `yaml:"patterns" json:"patterns"`
}

// Retention governs disk usage of the artifact root. The runner
// auto-prunes after every successful run when KeepRuns is set;
// `glyph clean` does the same on demand and supports `--all` to
// wipe the artifact root.
type Retention struct {
	// KeepRuns is the number of newest run directories to keep per
	// artifact root. Older runs are pruned after each successful
	// run. 0 (the default) means "no auto-prune".
	KeepRuns int `yaml:"keepRuns,omitempty" json:"keepRuns,omitempty"`
}

type RedactionPattern struct {
	Name    string `yaml:"name" json:"name"`
	Regex   string `yaml:"regex" json:"regex"`
	Replace string `yaml:"replace" json:"replace"`
}

type Runtime struct {
	Config      Config
	ConfigPath  string
	ProjectRoot string
	SpecPath    string
	Environment string
	Vars        map[string]string
	Env         map[string]string
	Secrets     *Secrets
}

func Defaults() Config {
	return Config{
		Version:            1,
		DefaultEnvironment: "local",
		ArtifactRoot:       DefaultArtifactRoot,
		SnapshotRoot:       DefaultSnapshotRoot,
		SchemaRoot:         DefaultSchemaRoot,
		Environments: map[string]Environment{
			"local": {
				Vars: map[string]string{
					"helloBin":    "./examples/apps/hello/hello",
					"defaultCols": "100",
					"defaultRows": "30",
				},
				Env: defaultEnv(),
			},
		},
		Terminal: Terminal{
			Profile: "xterm-256color",
			Cols:    100,
			Rows:    30,
			Normalize: Normalize{
				TrimRight:                 true,
				NormalizeLineEndings:      true,
				HideCursorInTextSnapshots: false,
			},
		},
		Artifacts: Artifacts{
			RawLog:         true,
			Frames:         true,
			FinalScreen:    true,
			Snapshots:      true,
			AgentContext:   true,
			MaxRawLogBytes: 10 * 1024 * 1024,
		},
		Redaction: Redaction{
			Enabled: true,
			EnvAllowlist: []string{
				"TERM",
				"COLORTERM",
				"LANG",
				"LC_ALL",
				"CI",
				"GLYPHRUN",
			},
			Patterns: []RedactionPattern{
				{Name: "authorization-header", Regex: `(?i)authorization:\s*[^\r\n]+`, Replace: "Authorization: <redacted>"},
				{Name: "cookie-header", Regex: `(?i)(set-cookie|cookie):\s*[^\r\n]+`, Replace: "$1: <redacted>"},
				{Name: "token", Regex: `(?i)(access_token|refresh_token|password|secret)=([^&\s]+)`, Replace: "$1=<redacted>"},
				{Name: "bearer-token", Regex: `(?i)bearer\s+[a-z0-9._~+/-]+`, Replace: "bearer <redacted>"},
				{Name: "private-key", Regex: `-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`, Replace: "<redacted private key>"},
			},
		},
	}
}

func defaultEnv() map[string]string {
	return map[string]string{
		"TERM":      "xterm-256color",
		"COLORTERM": "truecolor",
		"LANG":      "en_US.UTF-8",
		"LC_ALL":    "en_US.UTF-8",
		"CI":        "1",
		"GLYPHRUN":  "1",
	}
}

func (rt Runtime) SpecParseOptions() spec.ParseOptions {
	return spec.ParseOptions{
		ProjectRoot: rt.ProjectRoot,
		Vars:        rt.Vars,
		Env:         rt.Env,
		ConfigValues: map[string]string{
			"artifactRoot": rt.Config.ArtifactRoot,
			"snapshotRoot": rt.Config.SnapshotRoot,
			"schemaRoot":   rt.Config.SchemaRoot,
		},
		DefaultTerminal: spec.Terminal{
			Cols:    rt.Config.Terminal.Cols,
			Rows:    rt.Config.Terminal.Rows,
			Profile: rt.Config.Terminal.Profile,
		},
	}
}
