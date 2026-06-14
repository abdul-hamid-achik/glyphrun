package cli

import (
	"github.com/abdul-hamid-achik/glyphrun/internal/version"
	"github.com/spf13/cobra"
)

// newVersionCommand prints the build-time version, commit, and
// build date. It mirrors `glyph --version` (which cobra wires from
// the root's Version field) so the subcommand is useful in
// non-interactive contexts where flag parsing may be limited.
func newVersionCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the binary's version, commit, and build date",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			value := struct {
				Version   string `json:"version" yaml:"version"`
				Commit    string `json:"commit" yaml:"commit"`
				BuildDate string `json:"buildDate" yaml:"buildDate"`
			}{version.Version, version.Commit, version.BuildDate}
			output, err := emitForCLI(cmd, opts, format, value, version.Full)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
}
