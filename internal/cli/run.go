package cli

import (
	"context"
	"errors"
	"sync"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/runner"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/spf13/cobra"
)

func newRunCommand(opts *globalOptions) *cobra.Command {
	var parallel int
	var updateSnapshots bool
	var progress string
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

	exitCode := 0
	for idx, err := range errs {
		if err != nil {
			return nil, 0, err
		}
		if results[idx].ExitCode > exitCode {
			exitCode = results[idx].ExitCode
		}
	}
	return results, exitCode, nil
}

func classifyRunError(err error) error {
	var mismatch spec.ContractHashMismatchError
	if errors.As(err, &mismatch) {
		return exitError{code: 5, err: err}
	}
	return exitError{code: 4, err: err}
}
