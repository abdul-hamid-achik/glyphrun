package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/spf13/cobra"
)

// ProbeResult is the per-spec stability summary produced by `glyph run
// --repeat N`. A spec is `stable` when every iteration produced the same
// outcome statuses AND the same final screen; it is `flaky` when iterations
// disagree on pass/fail.
type ProbeResult struct {
	Spec       string   `json:"spec" yaml:"spec"`
	Runs       int      `json:"runs" yaml:"runs"`
	Passed     int      `json:"passed" yaml:"passed"`
	Failed     int      `json:"failed" yaml:"failed"`
	Stable     bool     `json:"stable" yaml:"stable"`
	Flaky      bool     `json:"flaky" yaml:"flaky"`
	Divergence string   `json:"divergence,omitempty" yaml:"divergence,omitempty"`
	RunDirs    []string `json:"runDirs" yaml:"runDirs"`
}

// runFlakinessProbe runs each spec `repeat` times and reports stability. It
// exits non-zero if any iteration of any spec failed, so a flaky or
// consistently-failing spec is caught in CI.
func runFlakinessProbe(cmd *cobra.Command, opts *globalOptions, format outputFormat, specPaths []string, repeat, parallel int, updateSnapshots bool) error {
	type aggregate struct {
		spec       string
		passed     int
		failed     int
		signatures []string // one per iteration: outcome vector + final screen
		runDirs    []string
		divergence string
	}
	agg := make([]aggregate, len(specPaths))
	for i := range agg {
		agg[i].spec = specPaths[i]
	}

	anyFailed := false
	for iter := 0; iter < repeat; iter++ {
		// Run iterations without a live listener; the probe prints its own
		// compact per-iteration marker to stderr.
		results, exitCode, err := runSpecs(context.Background(), specPaths, parallel, opts, updateSnapshots, nil)
		if err != nil {
			return classifyRunError(err)
		}
		if exitCode != 0 {
			anyFailed = true
		}
		if !opts.quiet {
			fmt.Fprintf(cmd.ErrOrStderr(), "probe: iteration %d/%d (exit %d)\n", iter+1, repeat, exitCode)
		}
		for i, result := range results {
			if result.Status == artifacts.StatusPassed {
				agg[i].passed++
			} else {
				agg[i].failed++
			}
			agg[i].runDirs = append(agg[i].runDirs, result.RunDir)
			agg[i].signatures = append(agg[i].signatures, runSignature(result))
		}
	}

	report := make([]ProbeResult, 0, len(agg))
	for _, a := range agg {
		stable := allEqual(a.signatures)
		flaky := a.passed > 0 && a.failed > 0
		divergence := ""
		if !stable {
			divergence = describeFirstDivergence(a.signatures)
		}
		report = append(report, ProbeResult{
			Spec:       filepath.Base(a.spec),
			Runs:       repeat,
			Passed:     a.passed,
			Failed:     a.failed,
			Stable:     stable,
			Flaky:      flaky,
			Divergence: divergence,
			RunDirs:    a.runDirs,
		})
	}

	value := map[string]any{
		"schemaVersion": 1,
		"runsPerSpec":   repeat,
		"results":       report,
	}
	output, err := emitForCLI(cmd, opts, format, value, func() string { return renderProbeMarkdown(repeat, report) })
	if err != nil {
		return exitError{code: 2, err: err}
	}
	cmd.Print(output)
	if anyFailed {
		return exitError{code: 1}
	}
	return nil
}

// runSignature is a comparable fingerprint of a run: the ordered outcome
// id=status vector plus the final screen text. Two runs with the same
// signature are considered identical for stability purposes.
func runSignature(result artifacts.RunResult) string {
	var b strings.Builder
	b.WriteString(string(result.Status))
	b.WriteByte('|')
	for _, o := range result.Outcomes {
		b.WriteString(o.ID)
		b.WriteByte('=')
		b.WriteString(string(o.Status))
		b.WriteByte(';')
	}
	b.WriteByte('|')
	if path := result.Artifacts["finalScreenText"]; path != "" {
		if data, err := os.ReadFile(filepath.Join(result.RunDir, path)); err == nil {
			b.Write(data)
		}
	}
	return b.String()
}

func allEqual(sigs []string) bool {
	for i := 1; i < len(sigs); i++ {
		if sigs[i] != sigs[0] {
			return false
		}
	}
	return true
}

// describeFirstDivergence reports the first iteration whose signature differed
// from the first iteration, distinguishing an outcome change from a
// screen-only change.
func describeFirstDivergence(sigs []string) string {
	if len(sigs) == 0 {
		return ""
	}
	baseOutcomes := outcomesField(sigs[0])
	for i := 1; i < len(sigs); i++ {
		if sigs[i] == sigs[0] {
			continue
		}
		iterOutcomes := outcomesField(sigs[i])
		if iterOutcomes != baseOutcomes {
			return fmt.Sprintf("iteration %d differed in outcomes (expected %q, got %q)", i+1, baseOutcomes, iterOutcomes)
		}
		return fmt.Sprintf("iteration %d produced a different final screen", i+1)
	}
	return ""
}

// outcomesField returns the "status|outcomes" portion of a signature (the
// part before the final-screen text), used to tell outcome drift apart from
// screen-only drift.
func outcomesField(sig string) string {
	idx := strings.LastIndexByte(sig, '|')
	if idx < 0 {
		return sig
	}
	return sig[:idx]
}

func renderProbeMarkdown(repeat int, report []ProbeResult) string {
	var b strings.Builder
	b.WriteString("# Glyphrun Flakiness Probe\n\n")
	fmt.Fprintf(&b, "- runs per spec: %d\n\n", repeat)
	// Stable, deterministic ordering of the summary.
	sorted := append([]ProbeResult(nil), report...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Spec < sorted[j].Spec })
	for _, r := range sorted {
		verdict := "stable"
		if r.Flaky {
			verdict = "FLAKY"
		} else if r.Failed > 0 {
			verdict = "consistently failing"
		} else if !r.Stable {
			verdict = "non-deterministic screen"
		}
		fmt.Fprintf(&b, "## %s — %s\n\n", r.Spec, verdict)
		fmt.Fprintf(&b, "- passed: %d/%d\n", r.Passed, r.Runs)
		fmt.Fprintf(&b, "- failed: %d/%d\n", r.Failed, r.Runs)
		fmt.Fprintf(&b, "- stable: %v\n", r.Stable)
		if r.Divergence != "" {
			fmt.Fprintf(&b, "- divergence: %s\n", r.Divergence)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
