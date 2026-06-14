package cli

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

// TestWriteJUnitReport_PassedAndFailed runs the JUnit writer against a
// two-result batch (one passed, one failed with two failed outcomes)
// and asserts the XML structure matches what CI dashboards expect.
func TestWriteJUnitReport_PassedAndFailed(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "junit.xml")

	results := []artifacts.RunResult{
		{
			RunID:      "test_run_passed",
			SpecName:   "spec_a",
			Status:     artifacts.StatusPassed,
			DurationMS: 1500,
			Outcomes: []artifacts.OutcomeResult{
				{ID: "ready", Status: artifacts.OutcomePassed, Message: "screen contains ready"},
				{ID: "exit", Status: artifacts.OutcomePassed, Message: "exit 0"},
			},
		},
		{
			RunID:      "test_run_failed",
			SpecName:   "spec_b",
			Status:     artifacts.StatusFailed,
			DurationMS: 2300,
			Outcomes: []artifacts.OutcomeResult{
				{ID: "ready", Status: artifacts.OutcomePassed, Message: "screen contains ready"},
				{ID: "marker", Status: artifacts.OutcomeFailed, Message: "expected screen to contain BATCH-MARKER"},
			},
		},
	}
	if err := WriteJUnitReport(out, results); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// The XML declaration is on the first line so CI tools that sniff
	// for it don't choke.
	if !strings.HasPrefix(string(data), "<?xml") {
		t.Fatalf("expected XML declaration on first line, got:\n%s", string(data))
	}

	var report junitTestSuites
	if err := xml.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal failed: %v\nraw:\n%s", err, string(data))
	}
	if report.Name != "glyphrun" {
		t.Fatalf("unexpected report name: %q", report.Name)
	}
	if report.Tests != 4 {
		t.Fatalf("expected 4 tests total, got %d", report.Tests)
	}
	if report.Failures != 1 {
		t.Fatalf("expected 1 failure, got %d", report.Failures)
	}
	if len(report.Suites) != 2 {
		t.Fatalf("expected 2 suites, got %d", len(report.Suites))
	}
	// The failing spec's marker outcome should have a <failure> element
	// carrying the outcome message.
	var failedSpec *junitSuite
	for i := range report.Suites {
		if report.Suites[i].Name == "spec_b" {
			failedSpec = &report.Suites[i]
		}
	}
	if failedSpec == nil {
		t.Fatal("spec_b missing from report")
	}
	var marker *junitCase
	for i := range failedSpec.Cases {
		if failedSpec.Cases[i].Name == "marker" {
			marker = &failedSpec.Cases[i]
		}
	}
	if marker == nil {
		t.Fatal("marker outcome missing from spec_b")
	}
	if marker.Failure == nil {
		t.Fatalf("expected <failure> on marker, got %+v", marker)
	}
	if !strings.Contains(marker.Failure.Message, "BATCH-MARKER") {
		t.Fatalf("failure message missing detail: %q", marker.Failure.Message)
	}
}

// TestWriteJUnitReport_ErroredRunEmitsSyntheticCase covers a run that
// errored before any outcome could be evaluated (PTY crash). The
// JUnit report should still produce a <testcase> so CI dashboards
// don't silently swallow the error.
func TestWriteJUnitReport_ErroredRunEmitsSyntheticCase(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "junit.xml")
	results := []artifacts.RunResult{
		{
			RunID:    "errored_run",
			SpecName: "exploding",
			Status:   artifacts.StatusErrored,
			RunDir:   dir,
		},
	}
	if err := WriteJUnitReport(out, results); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	var report junitTestSuites
	if err := xml.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if report.Errors != 1 {
		t.Fatalf("expected 1 error, got %d", report.Errors)
	}
	if len(report.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(report.Suites))
	}
	if len(report.Suites[0].Cases) != 1 {
		t.Fatalf("expected synthetic testcase, got %d", len(report.Suites[0].Cases))
	}
	tc := report.Suites[0].Cases[0]
	if tc.Error == nil {
		t.Fatalf("expected <error> on synthetic case, got %+v", tc)
	}
	if tc.Error.Type != "RunError" {
		t.Fatalf("expected error type RunError, got %q", tc.Error.Type)
	}
}

// TestWriteJUnitReport_CreatesParentDir asserts the writer creates the
// destination directory (CI workflows often point --junit at a path
// inside reports/ that doesn't exist yet).
func TestWriteJUnitReport_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "nested", "deeper", "junit.xml")
	if err := WriteJUnitReport(out, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected file at %s, got: %v", out, err)
	}
}

// TestBuildJUnitReport_TableDriven covers the dispatch logic for the
// common input shapes. Each row is one run + outcome combination; the
// test asserts the resulting JUnit counts.
func TestBuildJUnitReport_TableDriven(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		results        []artifacts.RunResult
		wantTests      int
		wantFailures   int
		wantErrors     int
		wantSuites     int
		wantFirstClass string
	}{
		{
			name:           "empty",
			results:        nil,
			wantTests:      0,
			wantFailures:   0,
			wantErrors:     0,
			wantSuites:     0,
			wantFirstClass: "",
		},
		{
			name: "all-passed",
			results: []artifacts.RunResult{{
				SpecName:   "p",
				Status:     artifacts.StatusPassed,
				DurationMS: 100,
				Outcomes: []artifacts.OutcomeResult{
					{ID: "a", Status: artifacts.OutcomePassed},
					{ID: "b", Status: artifacts.OutcomePassed},
				},
			}},
			wantTests: 2, wantFailures: 0, wantErrors: 0, wantSuites: 1, wantFirstClass: "p",
		},
		{
			name: "all-failed",
			results: []artifacts.RunResult{{
				SpecName:   "f",
				Status:     artifacts.StatusFailed,
				DurationMS: 100,
				Outcomes: []artifacts.OutcomeResult{
					{ID: "a", Status: artifacts.OutcomeFailed, Message: "x"},
				},
			}},
			wantTests: 1, wantFailures: 1, wantErrors: 0, wantSuites: 1, wantFirstClass: "f",
		},
		{
			name: "mixed",
			results: []artifacts.RunResult{
				{
					SpecName: "m1", Status: artifacts.StatusPassed, DurationMS: 50,
					Outcomes: []artifacts.OutcomeResult{{ID: "ok", Status: artifacts.OutcomePassed}},
				},
				{
					SpecName: "m2", Status: artifacts.StatusPassed, DurationMS: 50,
					Outcomes: []artifacts.OutcomeResult{
						{ID: "ok", Status: artifacts.OutcomePassed},
						{ID: "no", Status: artifacts.OutcomeFailed, Message: "nope"},
					},
				},
			},
			wantTests: 3, wantFailures: 1, wantErrors: 0, wantSuites: 2, wantFirstClass: "m1",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			report := buildJUnitReport(tc.results)
			if report.Tests != tc.wantTests {
				t.Errorf("Tests = %d, want %d", report.Tests, tc.wantTests)
			}
			if report.Failures != tc.wantFailures {
				t.Errorf("Failures = %d, want %d", report.Failures, tc.wantFailures)
			}
			if report.Errors != tc.wantErrors {
				t.Errorf("Errors = %d, want %d", report.Errors, tc.wantErrors)
			}
			if len(report.Suites) != tc.wantSuites {
				t.Errorf("Suites = %d, want %d", len(report.Suites), tc.wantSuites)
			}
			if tc.wantFirstClass != "" && len(report.Suites) > 0 && report.Suites[0].Name != tc.wantFirstClass {
				t.Errorf("first suite name = %q, want %q", report.Suites[0].Name, tc.wantFirstClass)
			}
		})
	}
}

// TestFormatJunitDuration_Stable is a tiny sanity test for the
// duration formatter. The function is small but it's the shape CI
// dashboards parse, so a regression here would silently break reports.
func TestFormatJunitDuration_Stable(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"0s":    "0s",
		"1ms":   "0.001s",
		"1s":    "1.000s",
		"250ms": "0.250s",
		"2m":    "120.000s",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			// We don't have direct access to time.Duration literals; use
			// time.ParseDuration for the test.
			d, err := time.ParseDuration(in)
			if err != nil {
				t.Fatal(err)
			}
			if got := formatJunitDuration(d); got != want {
				t.Errorf("formatJunitDuration(%s) = %q, want %q", in, got, want)
			}
		})
	}
}
