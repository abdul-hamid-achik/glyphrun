package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/runner"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/spf13/cobra"
)

func newRunCommand(opts *globalOptions) *cobra.Command {
	var parallel int
	var updateSnapshots bool
	var progress string
	var junitPath string
	var rerunFailed bool
	var repeat int
	var watch bool
	var watchPaths []string
	var monitorBin string
	var monitorInterval time.Duration
	var monitorProfile string
	cmd := &cobra.Command{
		Use:   "run <spec...>",
		Short: "Run terminal behavior specs",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			listener, err := makeRunProgressListener(cmd, opts, format, parallel, progress)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			// --rerun-failed: read the previous run's failure list
			// (recorded as <artifactRoot>/.last-failed.txt) and replay
			// only those specs. Useful in a CI loop where a flaky
			// test made the run fail and you want to retry just the
			// failing ones without re-running the green ones.
			//
			// The list contains spec *names* (from spec.Name), not
			// file paths, because the runner may have resolved the
			// spec from any path. We surface the names so the
			// contributor can decide which paths to pass next.
			if rerunFailed {
				cp := opts.configPath
				if cp == "" {
					cp, _ = config.FindConfig(".")
				}
				rt, err := config.LoadRuntime(".", cp, opts.environment)
				if err != nil {
					return exitError{code: 2, err: fmt.Errorf("rerun-failed: %w", err)}
				}
				artifactRoot := opts.artifactRoot
				if artifactRoot == "" {
					artifactRoot = rt.Config.ArtifactRoot
				}
				if !filepath.IsAbs(artifactRoot) {
					artifactRoot = filepath.Join(rt.ProjectRoot, artifactRoot)
				}
				failed, err := artifacts.ReadLastFailed(artifactRoot)
				if err != nil {
					return exitError{code: 2, err: fmt.Errorf("rerun-failed: read %s: %w", filepath.Join(artifactRoot, artifacts.LastFailedFile), err)}
				}
				lastFailedPath := filepath.Join(artifactRoot, artifacts.LastFailedFile)
				if failed == nil {
					failed = []string{}
				}
				// The list of failing spec *names* is informational: the
				// contributor copies them into the next `glyph run`
				// invocation, since the runner keeps no name→path index.
				// Route through emitForCLI so --format json/yaml stays
				// machine-parseable instead of emitting Markdown.
				value := map[string]any{
					"schemaVersion":  1,
					"lastFailedFile": lastFailedPath,
					"failed":         failed,
				}
				output, err := emitForCLI(cmd, opts, format, value, func() string {
					var b strings.Builder
					b.WriteString("# Glyphrun Rerun Failed\n\n")
					if len(failed) == 0 {
						b.WriteString("No previous failures recorded in " + lastFailedPath + ".\n")
						return b.String()
					}
					fmt.Fprintf(&b, "Previous failures (from %s):\n\n", lastFailedPath)
					for _, n := range failed {
						b.WriteString("- " + n + "\n")
					}
					b.WriteString("\nRe-run with:\n")
					for _, n := range failed {
						fmt.Fprintf(&b, "  glyph run <path-to-%s> ...\n", n)
					}
					return b.String()
				})
				if err != nil {
					return exitError{code: 2, err: err}
				}
				cmd.Print(output)
				return nil
			}
			// --repeat N runs each spec N times and reports stability, to
			// back up the determinism the tool promises. It's a separate
			// surface from a normal run: the output is a flakiness report,
			// not a run result.
			if repeat > 1 {
				return runFlakinessProbe(cmd, opts, format, args, repeat, parallel, updateSnapshots)
			}
			// --watch re-runs the specs whenever a spec file (or the target
			// command's working tree) changes. It's a human-only,
			// interactive loop, so it refuses non-interactive output modes.
			if watch || len(watchPaths) > 0 {
				return runWatch(cmd, opts, format, args, watchPaths, parallel, updateSnapshots, progress)
			}
			var procmon *runner.ProcmonConfig
			if monitorBin != "" || monitorProfile != "" {
				procmon = &runner.ProcmonConfig{Bin: monitorBin, Interval: monitorInterval, Profile: monitorProfile}
			}
			results, exitCode, err := runSpecs(context.Background(), args, parallel, opts, updateSnapshots, listener, procmon)
			if err != nil {
				return classifyRunError(err)
			}
			var output string
			var outputErr error
			if len(results) == 1 {
				output, outputErr = emitForCLI(cmd, opts, format, results[0], func() string { return artifacts.RenderRunMarkdown(results[0]) })
			} else {
				batch := map[string]any{"schemaVersion": 1, "results": results}
				output, outputErr = emitForCLI(cmd, opts, format, batch, func() string {
					md := "# Glyphrun Batch\n\n## Results\n\n"
					for _, result := range results {
						mark := "PASS"
						if result.Status != artifacts.StatusPassed {
							mark = "FAIL"
						}
						md += "- " + mark + " " + result.SpecName + ": " + string(result.Status) + " `" + result.RunDir + "`\n"
					}
					return md
				})
			}
			if outputErr != nil {
				return exitError{code: 2, err: outputErr}
			}
			if junitPath != "" {
				if err := WriteJUnitReport(junitPath, results); err != nil {
					return exitError{code: 2, err: fmt.Errorf("junit report: %w", err)}
				}
			}
			cmd.Print(output)
			if exitCode != 0 {
				return exitError{code: exitCode}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&parallel, "parallel", 1, "number of specs to run concurrently")
	cmd.Flags().BoolVar(&updateSnapshots, "update-snapshots", false, "update committed snapshots")
	cmd.Flags().StringVar(&progress, "progress", "auto", "live progress: auto, always, never")
	cmd.Flags().StringVar(&junitPath, "junit", "", "write a JUnit XML report to this path (use .xml extension)")
	cmd.Flags().StringVar(&monitorBin, "monitor", "", "opt-in: sample the spawned target's CPU/RSS via the monitor binary at this path and write a diagnostics/process.{md,json} artifact (empty = off)")
	cmd.Flags().DurationVar(&monitorInterval, "monitor-interval", 250*time.Millisecond, "process-telemetry sample interval (use with --monitor)")
	cmd.Flags().StringVar(&monitorProfile, "monitor-profile", "", "capture an end-of-run process profile via monitor: heap|cpu|goroutine|sample (use with --monitor)")
	cmd.Flags().BoolVar(&rerunFailed, "rerun-failed", false, "re-run only the specs that failed in the previous invocation (from .last-failed.txt)")
	cmd.Flags().IntVar(&repeat, "repeat", 1, "run each spec N times and report flakiness/stability instead of a single result")
	cmd.Flags().BoolVar(&watch, "watch", false, "re-run on spec/source changes (interactive; markdown output only)")
	cmd.Flags().StringArrayVar(&watchPaths, "watch-path", nil, "additional file or directory to watch (repeatable); implies --watch")
	return cmd
}

func runSpecs(ctx context.Context, specPaths []string, parallel int, opts *globalOptions, updateSnapshots bool, listener runner.ProgressListener, procmon *runner.ProcmonConfig) ([]artifacts.RunResult, int, error) {
	if parallel < 1 {
		parallel = 1
	}
	if parallel > len(specPaths) {
		parallel = len(specPaths)
	}
	results := make([]artifacts.RunResult, len(specPaths))
	errs := make([]error, len(specPaths))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < parallel; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				result, err := runner.RunSpec(ctx, runner.Options{
					SpecPath:        specPaths[idx],
					ConfigPath:      opts.configPath,
					Environment:     opts.environment,
					ArtifactRoot:    opts.artifactRoot,
					UpdateSnapshots: updateSnapshots,
					Listener:        listener,
					Procmon:         procmon,
				})
				results[idx] = result
				errs[idx] = err
			}
		}()
	}
	for idx := range specPaths {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()

	// Aggregate errors across all specs instead of bailing on the first.
	// Workers have already finished (wg.Wait above), so this collects the
	// full picture for parallel runs. Each error is annotated with the
	// spec path so multi-error output is identifiable.
	exitCode := 0
	var collected []error
	for idx, err := range errs {
		if err != nil {
			collected = append(collected, fmt.Errorf("%s: %w", specPaths[idx], err))
			continue
		}
		if results[idx].ExitCode > exitCode {
			exitCode = results[idx].ExitCode
		}
	}
	if len(collected) > 0 {
		return results, exitCode, errors.Join(collected...)
	}
	return results, exitCode, nil
}

func classifyRunError(err error) error {
	var mismatch spec.ContractHashMismatchError
	if errors.As(err, &mismatch) {
		// Exit 6 is the cairn convention: the spec was parseable but
		// its stamped contract hash doesn't match the recomputed one.
		// This is a behavior contract change, not a parse error (4)
		// or a run failure (1).
		return exitError{code: 6, err: err}
	}
	return exitError{code: 4, err: err}
}
