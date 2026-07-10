package artifacts

// NextAction is one actionable next step an agent can take after a run, derived
// from the run's errorKind/diagnostic so a weak model gets a concrete command
// instead of treating an error as ambiguous (SPEC §7.1 verification contracts).
// SafeToAutoRun is false for every action glyph emits — none are safe to run
// without the operator: even re-stamping snapshots changes files.
type NextAction struct {
	Tool          string         `json:"tool,omitempty" yaml:"tool,omitempty"`
	Command       string         `json:"command,omitempty" yaml:"command,omitempty"`
	Arguments     map[string]any `json:"arguments,omitempty" yaml:"arguments,omitempty"`
	Reason        string         `json:"reason" yaml:"reason"`
	SafeToAutoRun bool           `json:"safeToAutoRun" yaml:"safeToAutoRun"`
}

// NextActionsFor maps an errorKind to the actionable next steps an agent should
// consider. Returns nil for a non-error run (empty kind) so passed/failed runs
// stay byte-identical — nextActions is omitempty on RunResult.
func NextActionsFor(kind ErrorKind, specName, contractHash, expectedHash string) []NextAction {
	if kind == "" {
		return nil
	}
	rerun := "glyph run " + specName + " --format json"
	switch kind {
	case ErrorKindTargetStart:
		return []NextAction{{Command: rerun,
			Reason: "the target command could not start — fix the spec's cmd/cwd or ensure the target binary exists, then rerun"}}
	case ErrorKindTimeout:
		return []NextAction{{Command: rerun,
			Reason: "the target or a step exceeded its timeout — raise timeoutMs in the spec/step, then rerun"}}
	case ErrorKindContractHashMismatch:
		return []NextAction{{Command: "glyph run " + specName + " --update-snapshots",
			Reason: "the contract hash changed after an intentional snapshot change — re-stamp it (writes files; not auto-safe)"}}
	case ErrorKindUnsupportedTerminal:
		return []NextAction{{Command: rerun,
			Reason: "the terminal profile is unavailable — switch the spec's terminal.profile (e.g. xterm-256color) or install it, then rerun"}}
	case ErrorKindStepFailure:
		return []NextAction{{Command: rerun,
			Reason: "a step errored — inspect the diagnostic and the run artifacts, fix the spec or target, then rerun"}}
	case ErrorKindPrecondition:
		return []NextAction{{Command: rerun,
			Reason: "a precondition failed (missing secret/binary) — resolve what the diagnostic names, then rerun"}}
	case ErrorKindSpecParse:
		return []NextAction{{Command: rerun,
			Reason: "the spec failed to parse — fix the schema/version error the diagnostic names, then rerun"}}
	default:
		return nil
	}
}
