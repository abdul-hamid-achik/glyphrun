package artifacts

import "github.com/abdul-hamid-achik/glyphrun/internal/spec"

type RunStatus string

const (
	StatusPassed  RunStatus = "passed"
	StatusFailed  RunStatus = "failed"
	StatusErrored RunStatus = "errored"
)

type OutcomeStatus string

const (
	OutcomePassed OutcomeStatus = "passed"
	OutcomeFailed OutcomeStatus = "failed"
)

type RunResult struct {
	SchemaVersion int               `json:"schemaVersion" yaml:"schemaVersion"`
	RunID         string            `json:"runId" yaml:"runId"`
	SpecName      string            `json:"specName" yaml:"specName"`
	Status        RunStatus         `json:"status" yaml:"status"`
	StartedAt     string            `json:"startedAt" yaml:"startedAt"`
	EndedAt       string            `json:"endedAt" yaml:"endedAt"`
	DurationMS    int64             `json:"durationMs" yaml:"durationMs"`
	Target        spec.Target       `json:"target" yaml:"target"`
	Terminal      spec.Terminal     `json:"terminal" yaml:"terminal"`
	Outcomes      []OutcomeResult   `json:"outcomes" yaml:"outcomes"`
	Artifacts     map[string]string `json:"artifacts" yaml:"artifacts"`
	RunDir        string            `json:"runDir" yaml:"runDir"`
	ExitCode      int               `json:"exitCode" yaml:"exitCode"`
}

type OutcomeResult struct {
	ID       string        `json:"id" yaml:"id"`
	Status   OutcomeStatus `json:"status" yaml:"status"`
	Message  string        `json:"message,omitempty" yaml:"message,omitempty"`
	Evidence string        `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

type Event struct {
	TS   string `json:"ts" yaml:"ts"`
	Type string `json:"type" yaml:"type"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Info string `json:"info,omitempty" yaml:"info,omitempty"`
}
