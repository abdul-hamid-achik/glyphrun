package cli

import (
	"encoding/json"
	"fmt"

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
				return err
			}
			switch format {
			case formatJSON:
				payload := map[string]string{
					"version":   version.Version,
					"commit":    version.Commit,
					"buildDate": version.BuildDate,
				}
				data, err := json.MarshalIndent(payload, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			case formatYAML:
				fmt.Fprintf(cmd.OutOrStdout(), "version: %s\ncommit: %s\nbuildDate: %s\n", version.Version, version.Commit, version.BuildDate)
			case formatMD:
				fmt.Fprintln(cmd.OutOrStdout(), version.Full())
			}
			return nil
		},
	}
}
