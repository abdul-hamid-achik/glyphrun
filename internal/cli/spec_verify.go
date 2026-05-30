package cli

import (
	"errors"
	"fmt"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/spf13/cobra"
)

func newSpecCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "spec", Short: "Work with specs"}
	cmd.AddCommand(newSpecVerifyCommand(opts))
	cmd.AddCommand(newSpecScaffoldCommand())
	return cmd
}

func newSpecVerifyCommand(opts *globalOptions) *cobra.Command {
	var stamp bool
	cmd := &cobra.Command{
		Use:   "verify <spec>",
		Short: "Validate a spec and its contract hash",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			rt, err := config.LoadRuntime(args[0], opts.configPath, opts.environment)
			if err != nil {
				return exitError{code: 4, err: err}
			}
			parseOpts := rt.SpecParseOptions()
			parseOpts.AllowHashMismatch = stamp
			parsed, err := spec.ParseFile(args[0], parseOpts)
			if err != nil {
				var mismatch spec.ContractHashMismatchError
				if errors.As(err, &mismatch) {
					return exitError{code: 5, err: err}
				}
				return exitError{code: 4, err: err}
			}
			if stamp {
				if err := spec.StampContractHash(parsed.Path, parsed.ContractHash); err != nil {
					return exitError{code: 4, err: err}
				}
				parsed.Spec.ContractHash = parsed.ContractHash
			}
			value := map[string]any{
				"schemaVersion":     1,
				"valid":             true,
				"name":              parsed.Spec.Name,
				"path":              parsed.Path,
				"contractHash":      parsed.ContractHash,
				"contractHashValid": parsed.ContractHashValid || stamp,
				"steps":             len(parsed.Resolved.Steps),
				"outcomes":          len(parsed.Resolved.Outcomes),
				"stamped":           stamp,
			}
			output, err := emitForCLI(cmd, opts, format, value, func() string {
				return fmt.Sprintf("# Spec Valid\n\n- name: %s\n- contractHash: `%s`\n- steps: %d\n- outcomes: %d\n", parsed.Spec.Name, parsed.ContractHash, len(parsed.Resolved.Steps), len(parsed.Resolved.Outcomes))
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
	cmd.Flags().BoolVar(&stamp, "stamp", false, "write the computed contractHash into the spec")
	return cmd
}

func newSpecScaffoldCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "scaffold",
		Short: "Print a starter spec",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Print(`version: 1
name: hello_quits

intent: |
  a user can open the app and quit cleanly.

target:
  cmd: ["./bin/app"]
  cwd: "."

terminal:
  cols: 80
  rows: 24
  profile: xterm-256color

steps:
  - wait:
      screen:
        contains: "ready"
  - press: "q"
  - wait:
      process:
        exitCode: 0

outcomes:
  - id: ready_visible
    description: the app renders its ready state
    verify:
      screen:
        contains: "ready"
`)
		},
	}
}
