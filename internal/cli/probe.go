package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/flaky"
	"github.com/spf13/cobra"
)

// runFlakinessProbe runs each spec `repeat` times and reports stability. It
// exits non-zero if any iteration of any spec failed, so a flaky or
// consistently-failing spec is caught in CI.
func runFlakinessProbe(cmd *cobra.Command, opts *globalOptions, format outputFormat, specPaths []string, repeat, parallel int, updateSnapshots bool) error {
	// perSpec[i] accumulates the run results for specPaths[i] across iterations.
	perSpec := make([][]artifacts.RunResult, len(specPaths))
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
			perSpec[i] = append(perSpec[i], result)
		}
	}

	report := make([]flaky.Result, 0, len(specPaths))
	for i, path := range specPaths {
		report = append(report, flaky.Summarize(filepath.Base(path), repeat, perSpec[i]))
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

func renderProbeMarkdown(repeat int, report []flaky.Result) string {
	var b strings.Builder
	b.WriteString("# Glyphrun Flakiness Probe\n\n")
	fmt.Fprintf(&b, "- runs per spec: %d\n\n", repeat)
	sorted := append([]flaky.Result(nil), report...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Spec < sorted[j].Spec })
	for _, r := range sorted {
		verdict := "stable"
		switch {
		case r.Flaky:
			verdict = "FLAKY"
		case r.Failed > 0:
			verdict = "consistently failing"
		case !r.Stable:
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
