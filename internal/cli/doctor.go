package cli

import (
	"fmt"

	"github.com/abdul-hamid-achik/glyphrun/internal/doctor"
	"github.com/spf13/cobra"
)

func newDoctorCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local Glyphrun prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			result := doctor.Run(doctor.Options{
				ConfigPath:   opts.configPath,
				ArtifactRoot: opts.artifactRoot,
				Environment:  opts.environment,
			})
			value := map[string]any{
				"schemaVersion": result.SchemaVersion,
				"ok":            result.OK,
				"checks":        result.Checks,
			}
			output, err := emitForCLI(cmd, opts, format, value, func() string {
				md := "# Glyphrun Doctor\n\n"
				for _, check := range result.Checks {
					mark := "PASS"
					if check["ok"] != true {
						mark = "FAIL"
					}
					md += fmt.Sprintf("- %s %s: %s\n", mark, check["name"], check["detail"])
				}
				return md
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			if !result.OK {
				return exitError{code: 2}
			}
			return nil
		},
	}
}
