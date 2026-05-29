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

func intPtr(v int) *int {
	return &v
}
