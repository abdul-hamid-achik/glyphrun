package spec

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContractHashIgnoresSteps(t *testing.T) {
	base := Spec{
		Intent: "user can quit",
		Steps:  []Step{{Press: "q"}},
		Outcomes: []Outcome{{
			ID:          "quit",
			Description: "quits",
			Verify:      Verify{Process: &ProcessCondition{ExitCode: intPtr(0)}},
		}},
	}
	changed := base
	changed.Steps = []Step{{Press: "esc"}}
	a, err := ComputeContractHash(base)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ComputeContractHash(changed)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("hash changed when only steps changed: %s != %s", a, b)
	}
}

func TestContractHashIncludesCoversSymbol(t *testing.T) {
	base := Spec{
		Intent: "user can quit",
		Outcomes: []Outcome{{
			ID:          "quit",
			Description: "quits",
			Verify:      Verify{Process: &ProcessCondition{ExitCode: intPtr(0)}},
		}},
	}
	withCovers := base
	withCovers.CoversSymbol = "github.com/org/repo.Handler.ServeHTTP"
	a, err := ComputeContractHash(base)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ComputeContractHash(withCovers)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("hash should change when coversSymbol is added: both %s", a)
	}
}

func TestContractHashChangesWhenCoversSymbolChanges(t *testing.T) {
	base := Spec{
		Intent:       "user can quit",
		CoversSymbol: "github.com/org/repo.Handler.ServeHTTP",
		Outcomes: []Outcome{{
			ID:          "quit",
			Description: "quits",
			Verify:      Verify{Process: &ProcessCondition{ExitCode: intPtr(0)}},
		}},
	}
	changed := base
	changed.CoversSymbol = "github.com/org/repo.Handler.ServeHTTP2"
	a, err := ComputeContractHash(base)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ComputeContractHash(changed)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("hash should change when coversSymbol value changes: both %s", a)
	}
}

func TestContractHashUnchangedWhenCoversSymbolEmpty(t *testing.T) {
	base := Spec{
		Intent: "user can quit",
		Outcomes: []Outcome{{
			ID:          "quit",
			Description: "quits",
			Verify:      Verify{Process: &ProcessCondition{ExitCode: intPtr(0)}},
		}},
	}
	emptyCovers := base
	emptyCovers.CoversSymbol = ""
	a, err := ComputeContractHash(base)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ComputeContractHash(emptyCovers)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("hash should not change when coversSymbol is empty: %s != %s", a, b)
	}
}

func intPtr(v int) *int {
	return &v
}

// TestContractHashIgnoresUnusedScrollbackFields keeps the scrollback fields out
// of the hash of every spec that does not use them: adding optional fields to
// the model must not invalidate stamped specs.
func TestContractHashIgnoresUnusedScrollbackFields(t *testing.T) {
	outcome := Outcome{
		ID:          "ready",
		Description: "ready is shown",
		Verify:      Verify{Screen: &ScreenCondition{Contains: "ready", IncludeScrollback: false}},
	}
	data, err := json.Marshal(outcome)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "crollback") {
		t.Fatalf("outcome JSON mentions scrollback although it is unused: %s", data)
	}
	base := Spec{Intent: "shows ready", Outcomes: []Outcome{outcome}}
	want := mustHash(t, base)

	// The hash this spec had before the scrollback fields existed, computed by
	// the previous release's `glyph spec verify`.
	const frozen = "sha256:c76f64640319822034a7100290f86dd2377b2a29ded7901dd4e7a2a214520847"
	if want != frozen {
		t.Fatalf("hash = %s, want the pre-scrollback value %s", want, frozen)
	}

	used := base
	used.Outcomes = []Outcome{{
		ID:          "ready",
		Description: "ready is shown",
		Verify:      Verify{Screen: &ScreenCondition{Contains: "ready", IncludeScrollback: true}},
	}}
	if mustHash(t, used) == want {
		t.Fatal("includeScrollback: true is part of the contract and must change the hash")
	}
	withScrollback := base
	withScrollback.Outcomes = []Outcome{{
		ID:          "ready",
		Description: "ready is shown",
		Verify:      Verify{Scrollback: &ScrollbackCondition{Contains: "ready"}},
	}}
	if mustHash(t, withScrollback) == want {
		t.Fatal("a scrollback verifier must change the hash")
	}
}

func mustHash(t *testing.T, s Spec) string {
	t.Helper()
	h, err := ComputeContractHash(s)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
