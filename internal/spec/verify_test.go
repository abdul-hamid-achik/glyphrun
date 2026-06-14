package spec

import (
	"strings"
	"testing"
)

// TestValidateStep_AcceptsNewStepKinds covers the four new step kinds
// (download, transform, batch) and a few of their validation rules. Each
// row is one invalid or valid spec; tests must remain table-driven and
// self-explanatory so future contributors can extend them.
func TestValidateStep_AcceptsNewStepKinds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		step    Step
		wantErr string
	}{
		{
			name: "valid download",
			step: Step{Download: &DownloadStep{Path: "/tmp/x", SaveAs: "x.txt", Assign: "x"}},
		},
		{
			name:    "download missing path",
			step:    Step{Download: &DownloadStep{}},
			wantErr: "download.path is required",
		},
		{
			name:    "download invalid assign",
			step:    Step{Download: &DownloadStep{Path: "/tmp/x", Assign: "1bad"}},
			wantErr: "download.assign",
		},
		{
			name: "valid transform",
			step: Step{Transform: &TransformStep{File: "./t.sh", SaveAs: "out.txt", Assign: "out"}},
		},
		{
			name:    "transform missing file",
			step:    Step{Transform: &TransformStep{SaveAs: "out.txt"}},
			wantErr: "transform.file is required",
		},
		{
			name:    "transform missing saveAs",
			step:    Step{Transform: &TransformStep{File: "./t.sh"}},
			wantErr: "transform.saveAs is required",
		},
		{
			name:    "transform invalid runtime",
			step:    Step{Transform: &TransformStep{File: "./t.sh", SaveAs: "x.txt", Runtime: "lua"}},
			wantErr: "transform.runtime must be one of",
		},
		{
			name: "valid batch with trailing wait",
			step: Step{Batch: []Step{
				{Press: "w"},
				{Type: "abc"},
				{Wait: &WaitStep{Screen: &ScreenCondition{Contains: "ok"}, TimeoutMS: 1000}},
			}},
		},
		{
			name:    "batch requires at least 2 sub-steps",
			step:    Step{Batch: []Step{{Press: "w"}}},
			wantErr: "batch requires at least 2 sub-steps",
		},
		{
			name: "batch wait must be last",
			step: Step{Batch: []Step{
				{Press: "w"},
				{Wait: &WaitStep{Screen: &ScreenCondition{Contains: "ok"}, TimeoutMS: 1000}},
				{Press: "enter"},
			}},
			wantErr: "batch wait is only allowed as the final",
		},
		{
			name: "batch sub-step cannot have two actions",
			step: Step{Batch: []Step{
				{Press: "w", Type: "oops"},
				{Press: "enter"},
			}},
			wantErr: "must contain exactly one action",
		},
		{
			name: "step cannot mix press and download",
			step: Step{
				Press:    "q",
				Download: &DownloadStep{Path: "/tmp/x"},
			},
			wantErr: "step must contain exactly one action",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateStep(tc.step)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestIsArtifactProducing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		step Step
		want bool
	}{
		{"plain press", Step{Press: "q"}, false},
		{"download without assign", Step{Download: &DownloadStep{Path: "/x"}}, false},
		{"download with assign", Step{Download: &DownloadStep{Path: "/x", Assign: "x"}}, true},
		{"transform without assign", Step{Transform: &TransformStep{File: "t.sh", SaveAs: "o"}}, false},
		{"transform with assign", Step{Transform: &TransformStep{File: "t.sh", SaveAs: "o", Assign: "o"}}, true},
		{"batch is not artifact-producing", Step{Batch: []Step{{Press: "w"}, {Press: "enter"}}}, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.step.IsArtifactProducing(); got != tc.want {
				t.Fatalf("IsArtifactProducing() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidArtifactAssign(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"x", true},
		{"report1", true},
		{"report_v2", true},
		{"Report", false},  // must start lowercase
		{"1report", false}, // must start with a letter
		{"report-v2", false},
		{"", false},
		{"with space", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := validArtifactAssign(tc.in); got != tc.want {
				t.Fatalf("validArtifactAssign(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidateVerify_AcceptsFileAndScriptVerifiers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		verify  Verify
		wantErr string
	}{
		{
			name:   "file with glob only",
			verify: Verify{File: &FileCondition{Glob: "*.json"}},
		},
		{
			name:   "file with glob and contains",
			verify: Verify{File: &FileCondition{Glob: "*.json", Contains: "status"}},
		},
		{
			name:    "file missing glob",
			verify:  Verify{File: &FileCondition{}},
			wantErr: "file.glob is required",
		},
		{
			name:   "script with run",
			verify: Verify{Script: &ScriptCondition{Run: "return { ok: true }"}},
		},
		{
			name:   "script with file",
			verify: Verify{Script: &ScriptCondition{File: "./verifier.ts"}},
		},
		{
			name:    "script with both run and file",
			verify:  Verify{Script: &ScriptCondition{Run: "x", File: "y"}},
			wantErr: "exactly one of: run, file",
		},
		{
			name:    "script with neither run nor file",
			verify:  Verify{Script: &ScriptCondition{}},
			wantErr: "exactly one of: run, file",
		},
		{
			name:    "script with invalid runtime",
			verify:  Verify{Script: &ScriptCondition{Run: "x", Runtime: "lua"}},
			wantErr: "script.runtime must be one of",
		},
		{
			name:    "screen and file together is rejected",
			verify:  Verify{Screen: &ScreenCondition{Contains: "x"}, File: &FileCondition{Glob: "*"}},
			wantErr: "verify must contain exactly one verifier",
		},
		{
			name:   "count equals with rune",
			verify: Verify{Count: &CountCondition{Matches: "x", Equals: intPtr(3)}},
		},
		{
			name:   "count between with region",
			verify: Verify{Count: &CountCondition{Region: &RegionCondition{X: 0, Y: 0, Width: 10, Height: 1}, Between: &[2]int{1, 5}}},
		},
		{
			name:    "count without comparator",
			verify:  Verify{Count: &CountCondition{}},
			wantErr: "count must include exactly one",
		},
		{
			name:    "count with two comparators",
			verify:  Verify{Count: &CountCondition{Equals: intPtr(1), AtLeast: intPtr(0)}},
			wantErr: "exactly one comparator",
		},
		{
			name:    "count with bad between range",
			verify:  Verify{Count: &CountCondition{Between: &[2]int{5, 2}}},
			wantErr: "count.between",
		},
		{
			name:    "count with multi-char matches",
			verify:  Verify{Count: &CountCondition{Matches: "ab", Equals: intPtr(1)}},
			wantErr: "single character",
		},
		{
			name:    "count with bad region dims",
			verify:  Verify{Count: &CountCondition{Region: &RegionCondition{X: 0, Y: 0, Width: 0, Height: 1}, Equals: intPtr(1)}},
			wantErr: "width and height must be positive",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateVerify(tc.verify)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestValidateMetadata_AcceptsPriorities(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		priority string
		set      bool // whether to build a non-nil Metadata
		wantErr  string
	}{
		{"nil metadata", "", false, ""},
		{"low", "low", true, ""},
		{"normal", "normal", true, ""},
		{"high", "high", true, ""},
		{"critical", "critical", true, ""},
		{"uppercase is normalized", "CRITICAL", true, ""},
		{"whitespace is trimmed", "  high  ", true, ""},
		{"unknown priority is rejected", "blocker", true, "metadata.priority must be one of"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var m *Metadata
			if tc.set {
				m = &Metadata{Priority: tc.priority}
			}
			err := validateMetadata(m)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}
