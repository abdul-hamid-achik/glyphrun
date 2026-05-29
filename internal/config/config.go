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
}

type Environment struct {
	Vars map[string]string `yaml:"vars" json:"vars"`
	Env  map[string]string `yaml:"env" json:"env"`
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

type RedactionPattern struct {
	Name    string `yaml:"name" json:"name"`
	Regex   string `yaml:"regex" json:"regex"`
	Replace string `yaml:"replace" json:"replace"`
}

type Runtime struct {
	Config      Config
	ConfigPath  string
	ProjectRoot string
	Environment string
	Vars        map[string]string
	Env         map[string]string
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
