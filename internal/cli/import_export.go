package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newImportCommand is the dispatcher for `glyph import <format> <file>`.
// The first positional argument selects the importer; subsequent args
// are forwarded to the format-specific implementation. This pattern
// (parent command with format dispatch) keeps the CLI surface flat —
// `glyph import playwright <file>` will slot in here once we add a
// Playwright importer — without growing a tree of nested subcommands.
//
// Flags are bound on the parent command (visible in `glyph import --help`)
// so users can target any importer format with the same flag set.
func newImportCommand(opts *globalOptions) *cobra.Command {
	var outPath string
	var nameOverride string
	cmd := &cobra.Command{
		Use:   "import <format> <file>",
		Short: "Import a test file from another framework as a glyphrun spec",
		Long: `Convert a foreign test format to a glyphrun spec.

Supported formats:
  bats    A .bats file (https://github.com/bats-core/bats-core). One
          @test block per outcome; bodies are replayed through a
          per-spec runner script.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			format := args[0]
			switch format {
			case "bats":
				return runBatsImportWith(cmd, opts, args[1:], outPath, nameOverride)
			default:
				return exitError{code: 2, err: fmt.Errorf("unknown import format %q (supported: bats)", format)}
			}
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "output path (default: <source>.yml next to the source)")
	cmd.Flags().StringVar(&nameOverride, "name", "", "override the spec name (default: derived from the source basename)")
	return cmd
}

// newExportCommand is the mirror of newImportCommand. It dispatches on
// the first positional argument to a format-specific exporter.
func newExportCommand(opts *globalOptions) *cobra.Command {
	var outPath string
	cmd := &cobra.Command{
		Use:   "export <format> <spec>",
		Short: "Export a glyphrun spec to another framework's test format",
		Long: `Convert a glyphrun spec to a foreign test format. The
export is best-effort: steps/outcomes without a clean foreign
mapping are wrapped in TODO comments for a human reviewer.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			format := args[0]
			switch format {
			case "bats":
				return runBatsExportWith(cmd, opts, args[1:], outPath)
			default:
				return exitError{code: 2, err: fmt.Errorf("unknown export format %q (supported: bats)", format)}
			}
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "output path (default: <source>.bats next to the source)")
	return cmd
}
