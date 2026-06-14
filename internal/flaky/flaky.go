// Package flaky summarizes the stability of repeated runs of a spec.
//
// It lives outside internal/cli so the comparison logic — what counts as a
// stable run, how divergence is described — is testable without driving the
// runner, keeping `glyph run --repeat` a thin loop over RunSpec.
package flaky

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

// Result is the per-spec stability summary. A spec is Stable when every
// iteration produced the same outcome statuses AND the same final screen; it is
// Flaky when iterations disagree on pass/fail.
type Result struct {
	Spec       string   `json:"spec" yaml:"spec"`
	Runs       int      `json:"runs" yaml:"runs"`
	Passed     int      `json:"passed" yaml:"passed"`
	Failed     int      `json:"failed" yaml:"failed"`
	Stable     bool     `json:"stable" yaml:"stable"`
	Flaky      bool     `json:"flaky" yaml:"flaky"`
	Divergence string   `json:"divergence,omitempty" yaml:"divergence,omitempty"`
	RunDirs    []string `json:"runDirs" yaml:"runDirs"`
}

// Summarize folds one spec's per-iteration run results into a stability Result.
func Summarize(spec string, runs int, results []artifacts.RunResult) Result {
	r := Result{Spec: spec, Runs: runs}
	signatures := make([]string, 0, len(results))
	for _, res := range results {
		if res.Status == artifacts.StatusPassed {
			r.Passed++
		} else {
			r.Failed++
		}
		r.RunDirs = append(r.RunDirs, res.RunDir)
		signatures = append(signatures, Signature(res))
	}
	r.Stable = allEqual(signatures)
	r.Flaky = r.Passed > 0 && r.Failed > 0
	if !r.Stable {
		r.Divergence = describeFirstDivergence(signatures)
	}
	return r
}

// Signature is a comparable fingerprint of a run: the ordered outcome
// id=status vector plus the final screen text. Two runs with the same
// signature are identical for stability purposes.
func Signature(result artifacts.RunResult) string {
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
// from the first, distinguishing an outcome change from a screen-only change.
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

// outcomesField returns the "status|outcomes" portion of a signature (the part
// before the final-screen text), used to tell outcome drift apart from
// screen-only drift.
func outcomesField(sig string) string {
	idx := strings.LastIndexByte(sig, '|')
	if idx < 0 {
		return sig
	}
	return sig[:idx]
}
