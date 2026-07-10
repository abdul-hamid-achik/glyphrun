package artifacts

import "github.com/abdul-hamid-achik/glyphrun/internal/spec"

type RunStatus string

const (
	StatusPassed  RunStatus = "passed"
	StatusFailed  RunStatus = "failed"
	StatusErrored RunStatus = "errored"
)

// ErrorKind classifies the cause of an errored or runner-level-failed run so
// agents can choose an actionable next step (re-stamp, raise timeout, switch
// profile, fix precondition) instead of treating every error as ambiguous.
type ErrorKind string

const (
	ErrorKindTargetStart          ErrorKind = "target_start"
	ErrorKindTimeout              ErrorKind = "timeout"
	ErrorKindContractHashMismatch ErrorKind = "contract_hash_mismatch"
	ErrorKindUnsupportedTerminal  ErrorKind = "unsupported_terminal"
	ErrorKindStepFailure          ErrorKind = "step_failure"
	ErrorKindPrecondition         ErrorKind = "precondition"
	ErrorKindSpecParse            ErrorKind = "spec_parse"
)

type OutcomeStatus string

const (
	OutcomePassed OutcomeStatus = "passed"
	OutcomeFailed OutcomeStatus = "failed"
)

type RunResult struct {
	SchemaVersion  int                      `json:"schemaVersion" yaml:"schemaVersion"`
	RunID          string                   `json:"runId" yaml:"runId"`
	SpecName       string                   `json:"specName" yaml:"specName"`
	Intent         string                   `json:"intent,omitempty" yaml:"intent,omitempty"`
	ContractHash   string                   `json:"contractHash,omitempty" yaml:"contractHash,omitempty"`
	ExpectedHash   string                   `json:"expectedHash,omitempty" yaml:"expectedHash,omitempty"`
	Metadata       *spec.Metadata           `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CoversSymbol   string                   `json:"coversSymbol,omitempty" yaml:"coversSymbol,omitempty"`
	Status         RunStatus                `json:"status" yaml:"status"`
	ErrorKind      ErrorKind                `json:"errorKind,omitempty" yaml:"errorKind,omitempty"`
	Diagnostic     string                   `json:"diagnostic,omitempty" yaml:"diagnostic,omitempty"`
	StartedAt      string                   `json:"startedAt" yaml:"startedAt"`
	EndedAt        string                   `json:"endedAt" yaml:"endedAt"`
	DurationMS     int64                    `json:"durationMs" yaml:"durationMs"`
	Target         spec.Target              `json:"target" yaml:"target"`
	Terminal       spec.Terminal            `json:"terminal" yaml:"terminal"`
	Outcomes       []OutcomeResult          `json:"outcomes" yaml:"outcomes"`
	Artifacts      map[string]string        `json:"artifacts" yaml:"artifacts"`
	NamedArtifacts map[string]NamedArtifact `json:"namedArtifacts,omitempty" yaml:"namedArtifacts,omitempty"`
	Manifest       []ArtifactManifestEntry  `json:"manifest,omitempty" yaml:"manifest,omitempty"`
	RunDir         string                   `json:"runDir" yaml:"runDir"`
	ExitCode       int                      `json:"exitCode" yaml:"exitCode"`
	NextActions    []NextAction             `json:"nextActions,omitempty" yaml:"nextActions,omitempty"`
	Steps          []StepResult             `json:"steps,omitempty" yaml:"steps,omitempty"`
}

// StepResult is the per-step execution record (SPEC §7.3 structured StepResult[]).
// Duration, kind, normalized error, and status let an agent see exactly which
// step failed and how long each took without scanning events.ndjson.
type StepResult struct {
	Index      int    `json:"index" yaml:"index"`   // 1-based
	Kind       string `json:"kind" yaml:"kind"`     // wait|type|press|click|hover|fill|...
	Status     string `json:"status" yaml:"status"` // passed|failed|skipped
	DurationMS int64  `json:"durationMs" yaml:"durationMs"`
	Error      string `json:"error,omitempty" yaml:"error,omitempty"`
}

// NamedArtifact describes a file produced by a download or transform step.
// `Path` is the absolute filesystem path; `RelativePath` is the path
// relative to the run dir (also the value of `${artifacts.<name>.relativePath}`).
type NamedArtifact struct {
	Kind         string `json:"kind" yaml:"kind"`
	Path         string `json:"path" yaml:"path"`
	RelativePath string `json:"relativePath" yaml:"relativePath"`
}

type OutcomeResult struct {
	ID          string        `json:"id" yaml:"id"`
	Status      OutcomeStatus `json:"status" yaml:"status"`
	Message     string        `json:"message,omitempty" yaml:"message,omitempty"`
	Evidence    string        `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	EvidenceRaw string        `json:"evidenceRaw,omitempty" yaml:"evidenceRaw,omitempty"`
}

type Event struct {
	TS   string `json:"ts" yaml:"ts"`
	Type string `json:"type" yaml:"type"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Info string `json:"info,omitempty" yaml:"info,omitempty"`
}
