package artifacts

import "testing"

func TestNextActionsForEveryErrorKind(t *testing.T) {
	// Every declared errorKind maps to at least one actionable next step that
	// is NOT safe to auto-run (even re-stamping changes files).
	kinds := []ErrorKind{
		ErrorKindTargetStart, ErrorKindTimeout, ErrorKindTargetExited,
		ErrorKindContractHashMismatch, ErrorKindUnsupportedTerminal,
		ErrorKindStepFailure, ErrorKindPrecondition, ErrorKindSpecParse,
	}
	for _, k := range kinds {
		acts := NextActionsFor(k, "spec_x", "", "")
		if len(acts) < 1 {
			t.Errorf("errorKind %s: expected at least 1 action, got %d", k, len(acts))
			continue
		}
		for i, a := range acts {
			if a.SafeToAutoRun {
				t.Errorf("errorKind %s action %d: must not be safeToAutoRun", k, i)
			}
			if a.Reason == "" {
				t.Errorf("errorKind %s action %d: must carry a reason", k, i)
			}
			if a.Command == "" {
				t.Errorf("errorKind %s action %d: must carry a command", k, i)
			}
		}
	}
}

func TestNextActionsForContractHashMismatchSuggestsStamp(t *testing.T) {
	acts := NextActionsForOpts(ErrorKindContractHashMismatch, NextActionsOptions{
		SpecPath:     "specs/foo.yml",
		SpecName:     "foo",
		ContractHash: "sha256:abc",
		ExpectedHash: "sha256:def",
	})
	if len(acts) < 1 || acts[0].Command == "" {
		t.Fatalf("expected stamp action, got %+v", acts)
	}
	if !contains(acts[0].Command, "spec verify") || !contains(acts[0].Command, "--stamp") {
		t.Errorf("contract hash mismatch should suggest spec verify --stamp, got %q", acts[0].Command)
	}
	if contains(acts[0].Command, "--update-snapshots") {
		t.Errorf("contract hash mismatch must not suggest --update-snapshots, got %q", acts[0].Command)
	}
	if !contains(acts[0].Command, "specs/foo.yml") {
		t.Errorf("stamp command should use the spec path, got %q", acts[0].Command)
	}
}

func TestNextActionsForTargetExitedDoesNotSuggestTimeout(t *testing.T) {
	acts := NextActionsFor(ErrorKindTargetExited, "spec_x", "", "")
	if len(acts) < 1 {
		t.Fatalf("expected actions, got %+v", acts)
	}
	for _, a := range acts {
		if contains(a.Reason, "raise timeoutMs") || contains(a.Reason, "raise timeout") {
			t.Errorf("target_exited must not recommend raising timeouts, got %q", a.Reason)
		}
	}
	joined := ""
	for _, a := range acts {
		joined += a.Reason + " "
	}
	if !contains(joined, "inspect") {
		t.Errorf("target_exited should recommend inspecting diagnostics, got %q", joined)
	}
	if !contains(joined, "fixing the target") && !contains(joined, "fix the target") {
		t.Errorf("target_exited should recommend fixing the target, got %q", joined)
	}
}

func TestNextActionsForStepFailureIncludesRepair(t *testing.T) {
	acts := NextActionsForOpts(ErrorKindStepFailure, NextActionsOptions{
		SpecPath: "examples/specs/hello.yml",
		RunDir:   "/tmp/run-x",
	})
	foundRepair := false
	foundContext := false
	for _, a := range acts {
		if contains(a.Command, "repair") {
			foundRepair = true
		}
		if contains(a.Command, "context") && contains(a.Command, "/tmp/run-x") {
			foundContext = true
		}
	}
	if !foundRepair {
		t.Errorf("step_failure should suggest glyph repair, got %+v", acts)
	}
	if !foundContext {
		t.Errorf("step_failure should suggest context with run dir, got %+v", acts)
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
