package cli

import (
	"os"

	"github.com/abdul-hamid-achik/glyphrun/internal/log"
	"github.com/abdul-hamid-achik/glyphrun/internal/version"
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
				log.Error("glyph failed", "err", ee.err)
			}
			return ee.code
		}
		if !opts.quiet {
			log.Error("glyph failed", "err", err)
		}
		return 2
	}
	return 0
}

// configureLogger installs the diagnostic logger from the parsed
// global flags. It is wired as the root PersistentPreRunE so it runs
// after cobra has populated opts (quiet/verbose/no-color/format) but
// before any subcommand's RunE. All diagnostics go to stderr; stdout
// stays reserved for the command result. JSON/YAML output formats
// switch the logger to JSON lines so stderr stays machine-parseable.
func configureLogger(opts *globalOptions) {
	log.Configure(log.Options{
		Writer:  os.Stderr,
		Quiet:   opts.quiet,
		Verbose: opts.verbose,
		NoColor: opts.noColor,
		JSON:    opts.format == "json" || opts.format == "yaml",
	})
}

func newRootCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "glyph",
		Short:         "Run terminal behavior specs in a real PTY",
		Version:       version.Full(),
		SilenceUsage:  true,
		SilenceErrors: true,
		// PersistentPreRunE runs after flag parsing, so opts.quiet/
		// verbose/no-color/format are populated. Subcommands inherit
		// this unless they define their own PersistentPreRunE (none do).
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			configureLogger(opts)
			return nil
		},
	}
	// Cobra's Print/Printf default to stderr (OutOrStderr). Glyphrun's
	// contract is that command output — including --format json/yaml — goes to
	// stdout so it stays machine-readable, while progress and diagnostics go to
	// stderr. Pin both streams explicitly. Tests override SetOut with a buffer.
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
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
	cmd.AddCommand(newRenderCommand(opts))
	cmd.AddCommand(newContextCommand(opts))
	cmd.AddCommand(newRepairCommand(opts))
	cmd.AddCommand(newCommentCommand(opts))
	cmd.AddCommand(newDocsCommand(opts))
	cmd.AddCommand(newAgentCommand(opts))
	cmd.AddCommand(newExplainCommand(opts))
	cmd.AddCommand(newDoctorCommand(opts))
	cmd.AddCommand(newMCPCommand(opts))
	cmd.AddCommand(newListCommand(opts))
	cmd.AddCommand(newAffectedSpecsCommand(opts))
	cmd.AddCommand(newImportCommand(opts))
	cmd.AddCommand(newExportCommand(opts))
	cmd.AddCommand(newCleanCommand(opts))
	cmd.AddCommand(newVersionCommand(opts))
	return cmd
}
