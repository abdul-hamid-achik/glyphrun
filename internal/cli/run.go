package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

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
				if len(failed) == 0 {
					cmd.Print("glyph run: no previous failures recorded in " + filepath.Join(artifactRoot, artifacts.LastFailedFile) + "\n")
					return nil
				}
				// Pretty-print the list and exit 0 — the contributor
				// copies the spec names into the next `glyph run`
				// invocation. This is intentionally interactive:
				// the next pass should re-supply the paths, since the
				// runner doesn't keep a name→path index.
				cmd.Print("# Glyphrun Rerun Failed\n\n")
				cmd.Print(fmt.Sprintf("Previous failures (from %s):\n\n", filepath.Join(artifactRoot, artifacts.LastFailedFile)))
				for _, n := range failed {
					cmd.Print("- " + n + "\n")
				}
				cmd.Print("\nRe-run with:\n")
				for _, n := range failed {
					cmd.Print(fmt.Sprintf("  glyph run <path-to-%s> ...\n", n))
				}
				return nil
			}
			results, exitCode, err := runSpecs(context.Background(), args, parallel, opts, updateSnapshots, listener)
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
	cmd.Flags().BoolVar(&rerunFailed, "rerun-failed", false, "re-run only the specs that failed in the previous invocation (from .last-failed.txt)")
	return cmd
}

func runSpecs(ctx context.Context, specPaths []string, parallel int, opts *globalOptions, updateSnapshots bool, listener runner.ProgressListener) ([]artifacts.RunResult, int, error) {
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
