package cli

import (
	"strings"
	"testing"
)

func TestAllEqual(t *testing.T) {
	tests := []struct {
		name string
		sigs []string
		want bool
	}{
		{name: "empty", sigs: nil, want: true},
		{name: "single", sigs: []string{"a"}, want: true},
		{name: "all same", sigs: []string{"a", "a", "a"}, want: true},
		{name: "one differs", sigs: []string{"a", "b", "a"}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := allEqual(tc.sigs); got != tc.want {
				t.Errorf("allEqual(%v) = %v, want %v", tc.sigs, got, tc.want)
			}
		})
	}
}

func TestOutcomesField(t *testing.T) {
	tests := []struct {
		name string
		sig  string
		want string
	}{
		{name: "with screen", sig: "passed|a=passed;|screen text", want: "passed|a=passed;"},
		{name: "no pipe", sig: "passed", want: "passed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := outcomesField(tc.sig); got != tc.want {
				t.Errorf("outcomesField(%q) = %q, want %q", tc.sig, got, tc.want)
			}
		})
	}
}

func TestDescribeFirstDivergence(t *testing.T) {
	tests := []struct {
		name     string
		sigs     []string
		contains string
	}{
		{
			name:     "stable",
			sigs:     []string{"passed|a=passed;|s", "passed|a=passed;|s"},
			contains: "",
		},
		{
			name:     "outcome drift",
			sigs:     []string{"passed|a=passed;|s", "failed|a=failed;|s"},
			contains: "differed in outcomes",
		},
		{
			name:     "screen drift",
			sigs:     []string{"passed|a=passed;|one", "passed|a=passed;|two"},
			contains: "different final screen",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := describeFirstDivergence(tc.sigs)
			if tc.contains == "" {
				if got != "" {
					t.Errorf("expected no divergence, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.contains) {
				t.Errorf("describeFirstDivergence = %q, want substring %q", got, tc.contains)
			}
		})
	}
}
