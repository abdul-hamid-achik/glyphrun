package cli

import (
	"context"
	"errors"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/runner"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/spf13/cobra"
)

func newSnapshotCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Manage committed terminal snapshots",
	}
	cmd.AddCommand(newSnapshotUpdateCommand(opts))
	return cmd
}

func newSnapshotUpdateCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "update <spec...>",
		Short: "Run specs and update committed terminal snapshots",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			results := make([]artifacts.RunResult, 0, len(args))
			exitCode := 0
			for _, specPath := range args {
				result, err := runner.RunSpec(context.Background(), runner.Options{
					SpecPath:        specPath,
					ConfigPath:      opts.configPath,
					Environment:     opts.environment,
					ArtifactRoot:    opts.artifactRoot,
					UpdateSnapshots: true,
				})
				if err != nil {
					var mismatch spec.ContractHashMismatchError
					if errors.As(err, &mismatch) {
						return exitError{code: 5, err: err}
					}
					return exitError{code: 4, err: err}
				}
				results = append(results, result)
				if result.ExitCode > exitCode {
					exitCode = result.ExitCode
				}
			}
			value := map[string]any{"schemaVersion": 1, "updated": true, "results": results}
			output, err := emitForCLI(cmd, opts, format, value, func() string {
				md := "# Snapshot Update\n\n"
				for _, result := range results {
					md += "- " + string(result.Status) + " " + result.SpecName + " " + result.RunDir + "\n"
				}
				return md
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			if exitCode != 0 {
				return exitError{code: exitCode}
			}
			return nil
		},
	}
}
