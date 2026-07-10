package artifacts

import "testing"

func TestNextActionsForEveryErrorKind(t *testing.T) {
	// Every declared errorKind maps to exactly one actionable next step that is
	// NOT safe to auto-run (even re-stamping changes files).
	kinds := []ErrorKind{
		ErrorKindTargetStart, ErrorKindTimeout, ErrorKindContractHashMismatch,
		ErrorKindUnsupportedTerminal, ErrorKindStepFailure, ErrorKindPrecondition,
		ErrorKindSpecParse,
	}
	for _, k := range kinds {
		acts := NextActionsFor(k, "spec_x", "", "")
		if len(acts) != 1 {
			t.Errorf("errorKind %s: expected 1 action, got %d", k, len(acts))
			continue
		}
		if acts[0].SafeToAutoRun {
			t.Errorf("errorKind %s: action must not be safeToAutoRun", k)
		}
		if acts[0].Reason == "" {
			t.Errorf("errorKind %s: action must carry a reason", k)
		}
	}
}

func TestNextActionsForContractHashMismatchSuggestsRestamp(t *testing.T) {
	acts := NextActionsFor(ErrorKindContractHashMismatch, "spec_x", "sha256:abc", "sha256:def")
	if len(acts) != 1 || acts[0].Command == "" {
		t.Fatalf("expected one command action, got %+v", acts)
	}
	if !contains(acts[0].Command, "--update-snapshots") {
		t.Errorf("contract hash mismatch should suggest re-stamping, got %q", acts[0].Command)
	}
}

func TestNextActionsForEmptyKindIsNil(t *testing.T) {
	// A non-errored run (empty kind) must produce no nextActions so passed/
	// failed runs stay byte-identical (nextActions is omitempty).
	if got := NextActionsFor("", "spec_x", "", ""); got != nil {
		t.Errorf("empty errorKind should yield nil nextActions, got %+v", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
