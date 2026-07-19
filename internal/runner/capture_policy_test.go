package runner

import (
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

// TestShouldCapture_Stable locks the per-channel capture policy
// truth table. A regression here would mean a passing run leaks a
// huge raw log to disk, or a failing run loses the artifacts a
// contributor needs to debug.
func TestShouldCapture_Stable(t *testing.T) {
	cases := []struct {
		name   string
		mode   spec.CaptureMode
		status artifacts.RunStatus
		want   bool
	}{
		{"empty mode never captures", "", artifacts.StatusPassed, false},
		{"always captures on pass", spec.CaptureAlways, artifacts.StatusPassed, true},
		{"always captures on fail", spec.CaptureAlways, artifacts.StatusFailed, true},
		{"always captures on errored", spec.CaptureAlways, artifacts.StatusErrored, true},
		{"on-failure skips pass", spec.CaptureOnFailure, artifacts.StatusPassed, false},
		{"on-failure captures fail", spec.CaptureOnFailure, artifacts.StatusFailed, true},
		{"on-failure captures errored", spec.CaptureOnFailure, artifacts.StatusErrored, true},
		{"never always skips", spec.CaptureNever, artifacts.StatusPassed, false},
		{"never skips on fail", spec.CaptureNever, artifacts.StatusFailed, false},
		{"never skips on errored", spec.CaptureNever, artifacts.StatusErrored, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldCapture(tc.mode, tc.status); got != tc.want {
				t.Errorf("shouldCapture(%q, %s) = %v, want %v", tc.mode, tc.status, got, tc.want)
			}
		})
	}
}

// TestResolveCapturePolicy_ProjectConfigOnly covers the no-spec-
// override case: the project config's booleans translate to
// CaptureAlways / CaptureNever, and named-artifacts are forced on
// (they're the spec's contract).
func TestResolveCapturePolicy_ProjectConfigOnly(t *testing.T) {
	p := resolveCapturePolicy(config.Artifacts{
		Snapshots:    true,
		Frames:       false,
		RawLog:       true,
		FinalScreen:  true,
		AgentContext: false,
	}, nil, "")
	if p.Snapshots != spec.CaptureAlways {
		t.Errorf("Snapshots: got %q, want always", p.Snapshots)
	}
	if p.Frames != spec.CaptureNever {
		t.Errorf("Frames: got %q, want never", p.Frames)
	}
	if p.NamedArtifacts != spec.CaptureAlways {
		t.Errorf("NamedArtifacts should always be on, got %q", p.NamedArtifacts)
	}
}

// TestResolveCapturePolicy_SpecOverrideWins confirms a per-spec
// block overrides the project config — useful for one-off specs
// that are known to be expensive (turn off rawLog) or critical
// (force agentContext always).
func TestResolveCapturePolicy_SpecOverrideWins(t *testing.T) {
	base := config.Artifacts{
		Snapshots:    true,
		RawLog:       true,
		AgentContext: true,
	}
	specPol := &spec.CapturePolicy{
		// Spec turns off snapshots and raw log, but insists on
		// agentContext even on pass.
		Snapshots:    spec.CaptureNever,
		RawLog:       spec.CaptureOnFailure,
		AgentContext: spec.CaptureAlways,
	}
	p := resolveCapturePolicy(base, specPol, "")
	if p.Snapshots != spec.CaptureNever {
		t.Errorf("Snapshots: got %q, want never (spec override)", p.Snapshots)
	}
	if p.RawLog != spec.CaptureOnFailure {
		t.Errorf("RawLog: got %q, want on-failure (spec override)", p.RawLog)
	}
	if p.AgentContext != spec.CaptureAlways {
		t.Errorf("AgentContext: got %q, want always (spec override)", p.AgentContext)
	}
}

// TestResolveCapturePolicy_DebugForcesAlways ensures mode: debug turns on
// the expensive channels regardless of config defaults.
func TestResolveCapturePolicy_DebugForcesAlways(t *testing.T) {
	base := config.Artifacts{
		Snapshots:    false,
		Frames:       false,
		RawLog:       false,
		FinalScreen:  false,
		AgentContext: false,
	}
	p := resolveCapturePolicy(base, nil, "debug")
	if p.Snapshots != spec.CaptureAlways || p.Frames != spec.CaptureAlways || p.RawLog != spec.CaptureAlways {
		t.Errorf("debug mode should force always capture, got %+v", p)
	}
}

// TestResolveCapturePolicy_SpecLeavesInherit confirms that an
// unmentioned per-spec field inherits from the project config —
// a spec that only overrides snapshots doesn't accidentally turn
// off everything else.
func TestResolveCapturePolicy_SpecLeavesInherit(t *testing.T) {
	base := config.Artifacts{
		Snapshots:    true,
		Frames:       true,
		AgentContext: true,
	}
	specPol := &spec.CapturePolicy{
		// Only override snapshots.
		Snapshots: spec.CaptureNever,
	}
	p := resolveCapturePolicy(base, specPol, "")
	if p.Snapshots != spec.CaptureNever {
		t.Errorf("Snapshots: got %q, want never (overridden)", p.Snapshots)
	}
	if p.Frames != spec.CaptureAlways {
		t.Errorf("Frames should inherit from config, got %q", p.Frames)
	}
	if p.AgentContext != spec.CaptureAlways {
		t.Errorf("AgentContext should inherit from config, got %q", p.AgentContext)
	}
}

// TestBoolToMode_RoundTrip locks the boolean-to-mode mapping so the
// config layer keeps behaving the same after the policy refactor.
func TestBoolToMode_RoundTrip(t *testing.T) {
	if boolToMode(true) != spec.CaptureAlways {
		t.Error("true should map to always")
	}
	if boolToMode(false) != spec.CaptureNever {
		t.Error("false should map to never")
	}
}
