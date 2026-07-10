package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
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
				emitSpecErrorEnvelope(cmd, opts, format, args[0], err)
				return exitError{code: 4, err: err}
			}
			parseOpts := rt.SpecParseOptions()
			parseOpts.AllowHashMismatch = stamp
			parsed, err := spec.ParseFile(args[0], parseOpts)
			if err != nil {
				emitSpecErrorEnvelope(cmd, opts, format, args[0], err)
				var mismatch spec.ContractHashMismatchError
				if errors.As(err, &mismatch) {
					return exitError{code: 6, err: err}
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
				"intent":            strings.TrimSpace(parsed.Spec.Intent),
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
	var kind string
	var coversSymbol string
	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "Print a starter spec",
		Long: "Print a starter spec (or reusable action) to seed a new spec file.\n\n" +
			"--coversSymbol <sym> binds the starter spec to the code symbol it exercises,\n" +
			"so `glyph affected-specs` can select it when that symbol's blast radius is hit.\n" +
			"An uncovered symbol (e.g. from `codemap orphans`) can scaffold a stub with this\n" +
			"binding in one call. Only the `spec` kind carries coversSymbol — actions are\n" +
			"reusable step libraries with no contract.",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch kind {
			case "spec":
				cmd.Print(starterSpecTemplate(coversSymbol))
				return nil
			case "action":
				if strings.TrimSpace(coversSymbol) != "" {
					return exitError{code: 2, err: fmt.Errorf("--coversSymbol applies to --kind spec only; actions have no contract")}
				}
				cmd.Print(starterActionTemplate())
				return nil
			default:
				return exitError{code: 2, err: fmt.Errorf("unsupported --kind %q", kind)}
			}
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "spec", "starter kind: spec, action")
	cmd.Flags().StringVar(&coversSymbol, "coversSymbol", "", "bind the starter spec to the code symbol it exercises (kind=spec only)")
	return cmd
}

// starterSpecTemplate returns the starter spec template. When coversSymbol is
// non-empty it is written as a top-level field so the stub is immediately
// selectable by `glyph affected-specs` without a manual edit.
func starterSpecTemplate(coversSymbol string) string {
	cs := ""
	if c := strings.TrimSpace(coversSymbol); c != "" {
		cs = "coversSymbol: " + c + "\n"
	}
	return "version: 1\n" +
		"name: hello_quits\n" +
		cs +
		"\nintent: |\n" +
		"  a user can open the app and quit cleanly.\n" +
		"\ntarget:\n" +
		"  cmd: [\"./bin/app\"]\n" +
		"  cwd: \".\"\n" +
		"\nterminal:\n" +
		"  cols: 80\n" +
		"  rows: 24\n" +
		"  profile: xterm-256color\n" +
		"\nsteps:\n" +
		"  - wait:\n" +
		"      screen:\n" +
		"        contains: \"ready\"\n" +
		"  - press: \"q\"\n" +
		"  - wait:\n" +
		"      process:\n" +
		"        exitCode: 0\n" +
		"\noutcomes:\n" +
		"  - id: ready_visible\n" +
		"    description: the app renders its ready state\n" +
		"    verify:\n" +
		"      screen:\n" +
		"        contains: \"ready\"\n"
}

// starterActionTemplate returns the reusable-action template (no contract, no
// coversSymbol — actions are step libraries imported by specs).
func starterActionTemplate() string {
	return "version: 1\n" +
		"name: wait_for_ready_and_quit\n" +
		"\nsteps:\n" +
		"  - wait:\n" +
		"      screen:\n" +
		"        contains: \"ready\"\n" +
		"      timeoutMs: 5000\n" +
		"  - snapshot: ready\n" +
		"  - press: \"q\"\n" +
		"  - wait:\n" +
		"      process:\n" +
		"        exitCode: 0\n" +
		"      timeoutMs: 3000\n"
}

// emitSpecErrorEnvelope writes a structured error envelope to stdout for the
// spec verify command's exit 4 (parse) and exit 6 (contract-hash mismatch)
// paths. Without this, stdout is empty and agents can only see a stderr log
// line — losing the actionable detail (re-stamp, fix schema) that the
// errorKind + diagnostic carry.
func emitSpecErrorEnvelope(cmd *cobra.Command, opts *globalOptions, format outputFormat, specPath string, err error) {
	name := strings.TrimSuffix(filepath.Base(specPath), filepath.Ext(specPath))
	kind := "spec_parse"
	diagnostic := err.Error()
	contractHash := ""
	expectedHash := ""
	var mismatch spec.ContractHashMismatchError
	if errors.As(err, &mismatch) {
		kind = "contract_hash_mismatch"
		contractHash = mismatch.Actual
		expectedHash = mismatch.Expected
		if mismatch.SpecName != "" {
			name = mismatch.SpecName
		}
	}
	value := map[string]any{
		"schemaVersion": 1,
		"status":        "errored",
		"errorKind":     kind,
		"specName":      name,
		"diagnostic":    diagnostic,
	}
	if contractHash != "" {
		value["contractHash"] = contractHash
	}
	if expectedHash != "" {
		value["expectedHash"] = expectedHash
	}
	if na := artifacts.NextActionsFor(artifacts.ErrorKind(kind), name, contractHash, expectedHash); len(na) > 0 {
		value["nextActions"] = na
	}
	output, emitErr := emitForCLI(cmd, opts, format, value, func() string {
		var b strings.Builder
		fmt.Fprintf(&b, "# Spec Error: %s\n\n", name)
		fmt.Fprintf(&b, "- errorKind: `%s`\n", kind)
		fmt.Fprintf(&b, "- spec: `%s`\n", name)
		if contractHash != "" {
			fmt.Fprintf(&b, "- contractHash: `%s`\n", contractHash)
			fmt.Fprintf(&b, "- expectedHash: `%s`\n", expectedHash)
		}
		for _, na := range artifacts.NextActionsFor(artifacts.ErrorKind(kind), name, contractHash, expectedHash) {
			if na.Command != "" {
				fmt.Fprintf(&b, "- next action: `%s` — %s\n", na.Command, na.Reason)
			} else {
				fmt.Fprintf(&b, "- next action: %s\n", na.Reason)
			}
		}
		return b.String()
	})
	if emitErr == nil {
		cmd.Print(output)
	}
}
