package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type globalOptions struct {
	configPath   string
	artifactRoot string
	format       string
	quiet        bool
	verbose      bool
	noColor      bool
	environment  string
}

type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func Execute() int {
	opts := &globalOptions{}
	root := newRootCommand(opts)
	if err := root.Execute(); err != nil {
		if ee, ok := err.(exitError); ok {
			if ee.err != nil && !opts.quiet {
				fmt.Fprintln(root.ErrOrStderr(), ee.err)
			}
			return ee.code
		}
		if !opts.quiet {
			fmt.Fprintln(root.ErrOrStderr(), err)
		}
		return 2
	}
	return 0
}

func newRootCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "glyph",
		Short:         "Run terminal behavior specs in a real PTY",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "config file path")
	cmd.PersistentFlags().StringVar(&opts.artifactRoot, "artifact-root", "", "artifact root directory")
	cmd.PersistentFlags().StringVar(&opts.format, "format", "md", "output format: json, yaml, md")
	cmd.PersistentFlags().BoolVar(&opts.quiet, "quiet", false, "suppress non-structured diagnostics")
	cmd.PersistentFlags().BoolVar(&opts.verbose, "verbose", false, "enable verbose diagnostics")
	cmd.PersistentFlags().BoolVar(&opts.noColor, "no-color", false, "disable color")
	cmd.PersistentFlags().StringVar(&opts.environment, "env", "", "config environment")

	cmd.AddCommand(newRunCommand(opts))
	cmd.AddCommand(newInitCommand(opts))
	cmd.AddCommand(newSpecCommand(opts))
	cmd.AddCommand(newSnapshotCommand(opts))
	cmd.AddCommand(newDiffCommand(opts))
	cmd.AddCommand(newRecordCommand(opts))
	cmd.AddCommand(newReplayCommand(opts))
	cmd.AddCommand(newContextCommand(opts))
	cmd.AddCommand(newDocsCommand(opts))
	cmd.AddCommand(newAgentCommand(opts))
	cmd.AddCommand(newExplainCommand(opts))
	cmd.AddCommand(newDoctorCommand(opts))
	cmd.AddCommand(newMCPCommand(opts))
	return cmd
}
