package spec

import "testing"

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
