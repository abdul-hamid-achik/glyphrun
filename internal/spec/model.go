package spec

type Spec struct {
	Version      int       `yaml:"version" json:"version"`
	Name         string    `yaml:"name" json:"name"`
	ContractHash string    `yaml:"contractHash,omitempty" json:"contractHash,omitempty"`
	Intent       string    `yaml:"intent" json:"intent"`
	Metadata     *Metadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	CoversSymbol string    `yaml:"coversSymbol,omitempty" json:"coversSymbol,omitempty"`
	// Mode is "normal" (default) or "debug". Debug forces verbose capture
	// (frames, raw log, snapshots, agent context) for flaky diagnosis.
	Mode          string         `yaml:"mode,omitempty" json:"mode,omitempty"`
	Imports       []string       `yaml:"imports,omitempty" json:"imports,omitempty"`
	Target        Target         `yaml:"target" json:"target"`
	Terminal      Terminal       `yaml:"terminal" json:"terminal"`
	Preconditions Preconditions  `yaml:"preconditions,omitempty" json:"preconditions,omitempty"`
	Steps         []Step         `yaml:"steps" json:"steps"`
	Outcomes      []Outcome      `yaml:"outcomes" json:"outcomes"`
	Normalize     *Normalize     `yaml:"normalize,omitempty" json:"normalize,omitempty"`
	Redaction     *Redaction     `yaml:"redaction,omitempty" json:"redaction,omitempty"`
	Artifacts     *CapturePolicy `yaml:"artifacts,omitempty" json:"artifacts,omitempty"`
}

// Metadata carries org-facing classification on a spec. All fields are
// optional; the block exists so `glyph list` and CI dashboards can group
// and filter specs by feature / owner / priority / tags.
type Metadata struct {
	Feature  string   `yaml:"feature,omitempty" json:"feature,omitempty"`
	Owner    string   `yaml:"owner,omitempty" json:"owner,omitempty"`
	Priority string   `yaml:"priority,omitempty" json:"priority,omitempty"`
	Tags     []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// Redaction declares per-spec values that must be scrubbed from any
// artifact before it lands on disk. Useful when a spec exercises a
// flow that prints a real secret (an auth token, an API key) and
// the contributor wants the runner to redact it without having to
// edit the global config.
//
// The block is additive to the project-level config redaction:
// patterns from both sources are compiled and applied to every
// artifact write.
type Redaction struct {
	// Values is a list of literal strings (>=4 chars) that, if found
	// in any artifact, are replaced with `[redacted]`. The length
	// minimum avoids redacting short common tokens.
	Values []string `yaml:"values,omitempty" json:"values,omitempty"`
	// Headers is a list of header names (case-insensitive) whose
	// values are scrubbed in any captured network/console log.
	// The current runner doesn't capture network headers, so this
	// is reserved for future expansion and is validated-only here.
	Headers []string `yaml:"headers,omitempty" json:"headers,omitempty"`
}

// CapturePolicy is the per-spec override of which artifacts the
// runner writes. Each field is a CaptureMode (always | on-failure
// | never). The runner merges the spec-level policy on top of the
// project-level config; unmentioned fields inherit from the config.
//
// Today glyphrun implements three channels: snapshots, frames, and
// raw logs. The schema is forward-compatible with cairn's larger
// surface (screenshots, console, network, trace, storage) so a
// future runner extension doesn't require a schema bump.
type CapturePolicy struct {
	Snapshots      CaptureMode `yaml:"snapshots,omitempty" json:"snapshots,omitempty"`
	Frames         CaptureMode `yaml:"frames,omitempty" json:"frames,omitempty"`
	RawLog         CaptureMode `yaml:"rawLog,omitempty" json:"rawLog,omitempty"`
	FinalScreen    CaptureMode `yaml:"finalScreen,omitempty" json:"finalScreen,omitempty"`
	AgentContext   CaptureMode `yaml:"agentContext,omitempty" json:"agentContext,omitempty"`
	NamedArtifacts CaptureMode `yaml:"namedArtifacts,omitempty" json:"namedArtifacts,omitempty"`
	// Reserved for future expansion: Screenshots, Console, Network,
	// Trace, Storage. Each is parsed so a future schema bump doesn't
	// reject existing specs.
	Screenshots CaptureMode `yaml:"screenshots,omitempty" json:"screenshots,omitempty"`
	Console     CaptureMode `yaml:"console,omitempty" json:"console,omitempty"`
	Network     CaptureMode `yaml:"network,omitempty" json:"network,omitempty"`
	Trace       CaptureMode `yaml:"trace,omitempty" json:"trace,omitempty"`
	Storage     CaptureMode `yaml:"storage,omitempty" json:"storage,omitempty"`
}

// CaptureMode is the per-channel capture policy.
type CaptureMode string

const (
	CaptureAlways    CaptureMode = "always"
	CaptureOnFailure CaptureMode = "on-failure"
	CaptureNever     CaptureMode = "never"
)

func (m CaptureMode) Valid() bool {
	switch m {
	case "", CaptureAlways, CaptureOnFailure, CaptureNever:
		return true
	}
	return false
}

type ReusableAction struct {
	Version int    `yaml:"version,omitempty" json:"version,omitempty"`
	Name    string `yaml:"name" json:"name"`
	Steps   []Step `yaml:"steps" json:"steps"`
}

type Target struct {
	Cmd       []string          `yaml:"cmd" json:"cmd"`
	Cwd       string            `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	Env       map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	TimeoutMS int               `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

type Terminal struct {
	Cols            int    `yaml:"cols,omitempty" json:"cols,omitempty"`
	Rows            int    `yaml:"rows,omitempty" json:"rows,omitempty"`
	Profile         string `yaml:"profile,omitempty" json:"profile,omitempty"`
	Color           string `yaml:"color,omitempty" json:"color,omitempty"`
	AlternateScreen string `yaml:"alternateScreen,omitempty" json:"alternateScreen,omitempty"`
}

type Preconditions struct {
	Commands []Command `yaml:"commands,omitempty" json:"commands,omitempty"`
}

type Command struct {
	Run       string `yaml:"run" json:"run"`
	Cwd       string `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	TimeoutMS int    `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

type Step struct {
	// ID is an optional stable label for failure messages, events, and StepResult.
	// It is not part of the contract hash.
	ID string `yaml:"id,omitempty" json:"id,omitempty"`
	// When is a guard: full Verify object or shorthand string (see Conditional).
	When      *Conditional   `yaml:"when,omitempty" json:"when,omitempty"`
	Press     string         `yaml:"press,omitempty" json:"press,omitempty"`
	Type      string         `yaml:"type,omitempty" json:"type,omitempty"`
	Paste     string         `yaml:"paste,omitempty" json:"paste,omitempty"`
	Send      *SendStep      `yaml:"send,omitempty" json:"send,omitempty"`
	Mouse     *MouseStep     `yaml:"mouse,omitempty" json:"mouse,omitempty"`
	Wait      *WaitStep      `yaml:"wait,omitempty" json:"wait,omitempty"`
	Resize    *ResizeStep    `yaml:"resize,omitempty" json:"resize,omitempty"`
	Snapshot  string         `yaml:"snapshot,omitempty" json:"snapshot,omitempty"`
	Use       string         `yaml:"use,omitempty" json:"use,omitempty"`
	Download  *DownloadStep  `yaml:"download,omitempty" json:"download,omitempty"`
	Transform *TransformStep `yaml:"transform,omitempty" json:"transform,omitempty"`
	Monitor   *MonitorStep   `yaml:"monitor,omitempty" json:"monitor,omitempty"`
	Batch     []Step         `yaml:"batch,omitempty" json:"batch,omitempty"`
}

// DownloadStep captures a file from a known filesystem path into the run
// artifact directory under artifacts/<assign>/<saveAs>. The path may use
// ${vars.*} and ${env.*} placeholders (resolved at parse time) and
// ${artifacts.<name>.path} placeholders (resolved at run time, after
// earlier steps have populated their artifacts).
type DownloadStep struct {
	Path      string `yaml:"path" json:"path"`
	SaveAs    string `yaml:"saveAs,omitempty" json:"saveAs,omitempty"`
	Assign    string `yaml:"assign,omitempty" json:"assign,omitempty"`
	WaitFor   bool   `yaml:"waitFor,omitempty" json:"waitFor,omitempty"`
	TimeoutMS int    `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

// TransformStep runs an external script that produces a new named artifact.
// Supported runtimes: "node" (default: shell). The script receives a JSON
// context on its argv (Node) or via env vars (shell) and writes its output
// to the path advertised as `output.path`.
type TransformStep struct {
	Runtime   string            `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	File      string            `yaml:"file" json:"file"`
	Input     string            `yaml:"input,omitempty" json:"input,omitempty"`
	SaveAs    string            `yaml:"saveAs" json:"saveAs"`
	Assign    string            `yaml:"assign,omitempty" json:"assign,omitempty"`
	Fixtures  map[string]string `yaml:"fixtures,omitempty" json:"fixtures,omitempty"`
	TimeoutMS int               `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

// MonitorStep captures process telemetry of the live target at a point in the
// flow via the `monitor` CLI and stores it as a named artifact — evidence a
// spec author can keep or assert on later. It is the step-level sibling of
// `glyph run --monitor` (run-level sampling): a one-shot reading (always),
// optionally a process tree and/or a profile. Requires monitor on $PATH (or
// the run's --monitor binary); a missing monitor or an unavailable PID
// (Windows ConPTY) fails the step with a clear message.
type MonitorStep struct {
	SaveAs    string `yaml:"saveAs,omitempty" json:"saveAs,omitempty"`   // named artifact name (default: "monitor")
	Tree      bool   `yaml:"tree,omitempty" json:"tree,omitempty"`       // capture the process subtree
	Profile   string `yaml:"profile,omitempty" json:"profile,omitempty"` // heap|cpu|goroutine|sample
	TimeoutMS int    `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

type SendStep struct {
	Bytes string `yaml:"bytes" json:"bytes"`
}

// MouseStep sends a mouse event to the target at the 0-based cell (X, Y). The
// runner encodes it as SGR (1006) or legacy X10 depending on the mode the
// target enabled. Button defaults to "left"; Action defaults to "click".
type MouseStep struct {
	X      int    `yaml:"x" json:"x"`
	Y      int    `yaml:"y" json:"y"`
	Button string `yaml:"button,omitempty" json:"button,omitempty"`
	Action string `yaml:"action,omitempty" json:"action,omitempty"`
}

type WaitStep struct {
	Screen    *ScreenCondition  `yaml:"screen,omitempty" json:"screen,omitempty"`
	Process   *ProcessCondition `yaml:"process,omitempty" json:"process,omitempty"`
	Idle      *IdleCondition    `yaml:"idle,omitempty" json:"idle,omitempty"`
	TimeoutMS int               `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

type ResizeStep struct {
	Cols int `yaml:"cols" json:"cols"`
	Rows int `yaml:"rows" json:"rows"`
}

type Outcome struct {
	ID          string     `yaml:"id" json:"id"`
	Description string     `yaml:"description" json:"description"`
	Verify      Verify     `yaml:"verify" json:"verify"`
	TimeoutMS   int        `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
	Normalize   *Normalize `yaml:"normalize,omitempty" json:"normalize,omitempty"`
}

type Verify struct {
	Screen   *ScreenCondition   `yaml:"screen,omitempty" json:"screen,omitempty"`
	Region   *RegionCondition   `yaml:"region,omitempty" json:"region,omitempty"`
	Cell     *CellCondition     `yaml:"cell,omitempty" json:"cell,omitempty"`
	Cursor   *CursorCondition   `yaml:"cursor,omitempty" json:"cursor,omitempty"`
	Process  *ProcessCondition  `yaml:"process,omitempty" json:"process,omitempty"`
	Snapshot *SnapshotCondition `yaml:"snapshot,omitempty" json:"snapshot,omitempty"`
	Command  *CommandCondition  `yaml:"command,omitempty" json:"command,omitempty"`
	File     *FileCondition     `yaml:"file,omitempty" json:"file,omitempty"`
	Script   *ScriptCondition   `yaml:"script,omitempty" json:"script,omitempty"`
	Count    *CountCondition    `yaml:"count,omitempty" json:"count,omitempty"`
	Link     *LinkCondition     `yaml:"link,omitempty" json:"link,omitempty"`
	Metrics  *MetricsCondition  `yaml:"metrics,omitempty" json:"metrics,omitempty"`
}

// LinkCondition asserts that an OSC 8 hyperlink is present on the screen. `url`
// matches a substring of the link's URI; the optional `text` matches a
// substring of the linked text (the visible characters carrying that link).
type LinkCondition struct {
	URL  string `yaml:"url,omitempty" json:"url,omitempty"`
	Text string `yaml:"text,omitempty" json:"text,omitempty"`
}

// MetricsCondition asserts process-telemetry perf budgets against the run's
// sampled summary (collected by `glyph run --monitor`). Each set field is an
// upper bound (<=): the run passes only if the observed peak/mean stays at or
// below the budget. Requires process telemetry — run with `--monitor` (or add
// a `monitor:` step); without samples the outcome fails with a clear message.
type MetricsCondition struct {
	PeakCpuPercent *float64 `yaml:"peakCpuPercent,omitempty" json:"peakCpuPercent,omitempty"`
	PeakRss        *int64   `yaml:"peakRss,omitempty" json:"peakRss,omitempty"` // bytes
	MeanCpuPercent *float64 `yaml:"meanCpuPercent,omitempty" json:"meanCpuPercent,omitempty"`
	MeanRss        *int64   `yaml:"meanRss,omitempty" json:"meanRss,omitempty"` // bytes
}

// CountCondition asserts a count of cells in a region. The matcher
// selects which cells to count (`equals` matches the exact cell
// character; `nonEmpty` counts non-blank cells). The comparator is
// exactly one of `equals` / `atLeast` / `atMost` / `between`. This
// is the terminal-shaped sibling of cairn's `count` verifier; a
// future `count: { role: "row" }` shape can be added without a
// schema bump since `role` is reserved.
type CountCondition struct {
	Region  *RegionCondition `yaml:"region,omitempty" json:"region,omitempty"`
	Matches string           `yaml:"matches,omitempty" json:"matches,omitempty"`
	Equals  *int             `yaml:"equals,omitempty" json:"equals,omitempty"`
	AtLeast *int             `yaml:"atLeast,omitempty" json:"atLeast,omitempty"`
	AtMost  *int             `yaml:"atMost,omitempty" json:"atMost,omitempty"`
	Between *[2]int          `yaml:"between,omitempty" json:"between,omitempty"`
}

// FileCondition polls the filesystem for a file matching a glob, optionally
// requiring its body to contain a needle. The glob is resolved relative to
// the spec file's directory; wildcards (`*`, `?`) are supported in the
// filename portion. The literal `*` is treated as a wildcard, so a path
// with literal `*` in a directory component is not supported (matches
// cairn's `file:` verifier).
type FileCondition struct {
	Glob      string `yaml:"glob" json:"glob"`
	Contains  string `yaml:"contains,omitempty" json:"contains,omitempty"`
	TimeoutMS int    `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

// ScriptCondition runs an external Node module (or inline script) that
// returns `{ ok, evidence }`. The script receives a JSON context via
// argv[2] (Node) or env vars (shell). The returned `evidence` is written
// alongside the outcome's markdown as `outcomes/<id>.raw.json` so a
// long payload survives the markdown budget.
type ScriptCondition struct {
	Runtime   string            `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Run       string            `yaml:"run,omitempty" json:"run,omitempty"`
	File      string            `yaml:"file,omitempty" json:"file,omitempty"`
	Fixtures  map[string]string `yaml:"fixtures,omitempty" json:"fixtures,omitempty"`
	TimeoutMS int               `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

// ScreenCondition asserts against full-screen text. Matchers are additive
// when multiple are set (all must pass). Preferred modern forms:
//
//	equals | contains | matches (alias of regex) | notContains
type ScreenCondition struct {
	Equals      string `yaml:"equals,omitempty" json:"equals,omitempty"`
	Contains    string `yaml:"contains,omitempty" json:"contains,omitempty"`
	NotContains string `yaml:"notContains,omitempty" json:"notContains,omitempty"`
	// Matches is the preferred alias for Regex (cairn-style matcher name).
	Matches string `yaml:"matches,omitempty" json:"matches,omitempty"`
	// Regex is kept for backward compatibility; treated the same as Matches.
	Regex string `yaml:"regex,omitempty" json:"regex,omitempty"`
}

type RegionCondition struct {
	X           int    `yaml:"x" json:"x"`
	Y           int    `yaml:"y" json:"y"`
	Width       int    `yaml:"width" json:"width"`
	Height      int    `yaml:"height" json:"height"`
	Equals      string `yaml:"equals,omitempty" json:"equals,omitempty"`
	Contains    string `yaml:"contains,omitempty" json:"contains,omitempty"`
	NotContains string `yaml:"notContains,omitempty" json:"notContains,omitempty"`
	Matches     string `yaml:"matches,omitempty" json:"matches,omitempty"`
	Regex       string `yaml:"regex,omitempty" json:"regex,omitempty"`
}

type CellCondition struct {
	X     int    `yaml:"x" json:"x"`
	Y     int    `yaml:"y" json:"y"`
	Char  string `yaml:"char,omitempty" json:"char,omitempty"`
	Style *Style `yaml:"style,omitempty" json:"style,omitempty"`
}

type Style struct {
	Fg        string `yaml:"fg,omitempty" json:"fg,omitempty"`
	Bg        string `yaml:"bg,omitempty" json:"bg,omitempty"`
	Bold      *bool  `yaml:"bold,omitempty" json:"bold,omitempty"`
	Dim       *bool  `yaml:"dim,omitempty" json:"dim,omitempty"`
	Italic    *bool  `yaml:"italic,omitempty" json:"italic,omitempty"`
	Underline *bool  `yaml:"underline,omitempty" json:"underline,omitempty"`
	Reverse   *bool  `yaml:"reverse,omitempty" json:"reverse,omitempty"`
}

type CursorCondition struct {
	X       int   `yaml:"x,omitempty" json:"x,omitempty"`
	Y       int   `yaml:"y,omitempty" json:"y,omitempty"`
	Visible *bool `yaml:"visible,omitempty" json:"visible,omitempty"`
}

type ProcessCondition struct {
	ExitCode *int  `yaml:"exitCode,omitempty" json:"exitCode,omitempty"`
	Exited   *bool `yaml:"exited,omitempty" json:"exited,omitempty"`
}

type IdleCondition struct {
	QuietForMS int `yaml:"quietForMs" json:"quietForMs"`
}

type SnapshotCondition struct {
	Name      string     `yaml:"name" json:"name"`
	Mode      string     `yaml:"mode,omitempty" json:"mode,omitempty"`
	Normalize *Normalize `yaml:"normalize,omitempty" json:"normalize,omitempty"`
}

type CommandCondition struct {
	Run       string `yaml:"run" json:"run"`
	Cwd       string `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	TimeoutMS int    `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

type Normalize struct {
	TrimRight            *bool                 `yaml:"trimRight,omitempty" json:"trimRight,omitempty"`
	NormalizeLineEndings *bool                 `yaml:"normalizeLineEndings,omitempty" json:"normalizeLineEndings,omitempty"`
	StripAnsiTitle       *bool                 `yaml:"stripAnsiTitle,omitempty" json:"stripAnsiTitle,omitempty"`
	Replace              []NormalizeReplace    `yaml:"replace,omitempty" json:"replace,omitempty"`
	IgnoreRegions        []NormalizeIgnoreArea `yaml:"ignoreRegions,omitempty" json:"ignoreRegions,omitempty"`
}

type NormalizeReplace struct {
	Regex string `yaml:"regex" json:"regex"`
	With  string `yaml:"with" json:"with"`
}

type NormalizeIgnoreArea struct {
	X      int    `yaml:"x" json:"x"`
	Y      int    `yaml:"y" json:"y"`
	Width  int    `yaml:"width" json:"width"`
	Height int    `yaml:"height" json:"height"`
	Reason string `yaml:"reason,omitempty" json:"reason,omitempty"`
}
