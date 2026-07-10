package artifacts

import "github.com/abdul-hamid-achik/glyphrun/internal/spec"

// ReplayManifest is the exact-replay artifact (SPEC §7.3) written as
// replay.json alongside run.json. It captures everything an agent or operator
// needs to reproduce a run bit-for-bit WITHOUT re-reading the resolved spec:
// the normalized target argv, the terminal profile/viewport, the resolved
// capture policy, the redacted env KEY NAMES (never values), the glyph
// version, and one exact replay command. Secrets never appear — only env key
// names are listed, and the manifest is redacted like every other artifact.
type ReplayManifest struct {
	SchemaVersion int    `json:"schemaVersion" yaml:"schemaVersion"` // 1
	RunID         string `json:"runId" yaml:"runId"`
	SpecName      string `json:"specName" yaml:"specName"`
	ContractHash  string `json:"contractHash,omitempty" yaml:"contractHash,omitempty"`
	// Replay is the one exact shell command that reproduces this run.
	Replay        string             `json:"replay" yaml:"replay"`
	Argv          []string           `json:"argv" yaml:"argv"` // normalized target cmd
	Cwd           string             `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	Terminal      ReplayTerminal     `json:"terminal" yaml:"terminal"`
	CapturePolicy spec.CapturePolicy `json:"capturePolicy" yaml:"capturePolicy"`
	// EnvKeys are the NAMES of environment variables the target ran with
	// (spec-declared + resolved), sorted. Values are never included.
	EnvKeys     []string       `json:"envKeys,omitempty" yaml:"envKeys,omitempty"`
	Versions    ReplayVersions `json:"versions" yaml:"versions"`
	GeneratedAt string         `json:"generatedAt" yaml:"generatedAt"`
}

// ReplayTerminal is the terminal/viewport snapshot used to reproduce the run.
type ReplayTerminal struct {
	Cols            int    `json:"cols" yaml:"cols"`
	Rows            int    `json:"rows" yaml:"rows"`
	Profile         string `json:"profile,omitempty" yaml:"profile,omitempty"`
	Color           string `json:"color,omitempty" yaml:"color,omitempty"`
	AlternateScreen string `json:"alternateScreen,omitempty" yaml:"alternateScreen,omitempty"`
}

// ReplayVersions records the glyph build that produced the run, so a replay
// can be compared against the same version (or a drift flagged).
type ReplayVersions struct {
	Glyph     string `json:"glyph" yaml:"glyph"`
	Commit    string `json:"commit,omitempty" yaml:"commit,omitempty"`
	BuildDate string `json:"buildDate,omitempty" yaml:"buildDate,omitempty"`
}

// BuildReplayManifest constructs the replay manifest for a run from the
// resolved spec, the effective capture policy, the resolved env (only its key
// names are kept), and the glyph version. `specPath` is the path the operator
// invoked (used in the replay command); `runID` identifies the run.
func BuildReplayManifest(s spec.Spec, policy spec.CapturePolicy, env map[string]string, specPath, runID, glyphVersion, commit, buildDate string) ReplayManifest {
	argv := make([]string, 0, len(s.Target.Cmd))
	argv = append(argv, s.Target.Cmd...)
	envKeys := make([]string, 0, len(env)+len(s.Target.Env))
	for k := range s.Target.Env {
		envKeys = append(envKeys, k)
	}
	for k := range env {
		envKeys = append(envKeys, k)
	}
	envKeys = dedupeSorted(envKeys)
	return ReplayManifest{
		SchemaVersion: 1,
		RunID:         runID,
		SpecName:      s.Name,
		ContractHash:  s.ContractHash,
		Replay:        "glyph run " + specPath + " --format json",
		Argv:          argv,
		Cwd:           s.Target.Cwd,
		Terminal: ReplayTerminal{
			Cols:            s.Terminal.Cols,
			Rows:            s.Terminal.Rows,
			Profile:         s.Terminal.Profile,
			Color:           s.Terminal.Color,
			AlternateScreen: s.Terminal.AlternateScreen,
		},
		CapturePolicy: policy,
		EnvKeys:       envKeys,
		Versions:      ReplayVersions{Glyph: glyphVersion, Commit: commit, BuildDate: buildDate},
		GeneratedAt:   "", // set by the caller (finish) with the run end time
	}
}

func dedupeSorted(xs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	// stable sort
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
