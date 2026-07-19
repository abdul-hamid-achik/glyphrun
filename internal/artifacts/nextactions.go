package artifacts

import "strings"

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

// NextActionsOptions configures path-aware next-action commands.
type NextActionsOptions struct {
	// SpecPath is the filesystem path to the spec (preferred in commands).
	// When empty, SpecName is used as a placeholder.
	SpecPath string
	// SpecName is the logical name (spec.name).
	SpecName string
	// ContractHash / ExpectedHash populate stamp diagnostics for hash mismatch.
	ContractHash string
	ExpectedHash string
	// RunDir is the absolute path of the failed run pack when known.
	RunDir string
}

// NextActionsFor maps an errorKind to the actionable next steps an agent should
// consider. Returns nil for a non-error run (empty kind) so passed/failed runs
// stay byte-identical — nextActions is omitempty on RunResult.
//
// Prefer NextActionsForOpts when a filesystem path is known so agents get
// copy-pasteable commands instead of bare spec names.
func NextActionsFor(kind ErrorKind, specName, contractHash, expectedHash string) []NextAction {
	return NextActionsForOpts(kind, NextActionsOptions{
		SpecName:     specName,
		ContractHash: contractHash,
		ExpectedHash: expectedHash,
	})
}

// NextActionsForOpts is the full next-actions mapper with path-aware commands.
func NextActionsForOpts(kind ErrorKind, opts NextActionsOptions) []NextAction {
	if kind == "" {
		return nil
	}
	target := opts.SpecPath
	if target == "" {
		target = opts.SpecName
	}
	if target == "" {
		target = "<spec>"
	}
	rerun := "glyph run " + shellQuoteArg(target) + " --format json"
	contextCmd := "glyph context latest --format md"
	if opts.RunDir != "" {
		contextCmd = "glyph context " + shellQuoteArg(opts.RunDir) + " --format md"
	}
	repairCmd := "glyph repair " + shellQuoteArg(target) + " latest --format json"
	stampCmd := "glyph spec verify " + shellQuoteArg(target) + " --stamp --format json"

	switch kind {
	case ErrorKindTargetStart:
		return []NextAction{
			{Command: contextCmd, Reason: "inspect agent_context / diagnostics for the start failure"},
			{Command: rerun, Reason: "after fixing cmd/cwd or the missing binary, rerun the spec"},
		}
	case ErrorKindTimeout:
		return []NextAction{
			{Command: contextCmd, Reason: "inspect the wait that timed out and the final screen"},
			{Command: rerun, Reason: "after raising timeoutMs on the wait/target (or fixing a hung app), rerun"},
		}
	case ErrorKindTargetExited:
		return []NextAction{
			{Command: contextCmd, Reason: "inspect the diagnostic and raw/pty.raw.log — the target died before the screen wait was satisfied"},
			{Command: rerun, Reason: "after fixing the target startup/environment, rerun"},
		}
	case ErrorKindContractHashMismatch:
		return []NextAction{
			{Command: stampCmd, Reason: "intent/outcomes (the contract) changed — re-stamp contractHash after confirming the behavior change is intentional (writes the spec file; not auto-safe)"},
			{Command: rerun, Reason: "after stamping, rerun to confirm the new contract holds"},
		}
	case ErrorKindUnsupportedTerminal:
		return []NextAction{
			{Command: rerun, Reason: "switch terminal.profile (e.g. xterm-256color) or install the profile, then rerun"},
		}
	case ErrorKindStepFailure:
		return []NextAction{
			{Command: contextCmd, Reason: "inspect diagnostic, final screen, and step results"},
			{Command: repairCmd, Reason: "propose step-only repairs (never touches the contract)"},
			{Command: rerun, Reason: "after fixing the spec or target, rerun"},
		}
	case ErrorKindPrecondition:
		return []NextAction{
			{Command: contextCmd, Reason: "read the precondition failure detail"},
			{Command: rerun, Reason: "after resolving the missing secret/binary/setup, rerun"},
		}
	case ErrorKindSpecParse:
		return []NextAction{
			{Command: "glyph spec verify " + shellQuoteArg(target) + " --format json", Reason: "see the schema/validation error for the spec"},
			{Command: rerun, Reason: "after fixing the parse/schema error, rerun"},
		}
	default:
		return nil
	}
}

func shellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if !needsShellQuote(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func needsShellQuote(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/', r == '.', r == '_', r == '-', r == '+', r == '=', r == ':', r == '@', r == '%':
		default:
			return true
		}
	}
	return false
}
