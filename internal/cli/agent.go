package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newAgentCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "agent",
		Short: "Show the agent-facing Glyphrun workflow guide",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			value := map[string]any{
				"schemaVersion": 1,
				"purpose":       "bootstrap instructions for agents using Glyphrun",
				"commands": []string{
					"glyph agent --format md",
					"glyph explain --format json",
					"glyph docs agents --format md",
					"glyph docs authoring --format md",
					"glyph docs snippets --format md",
					"glyph spec verify <spec> --format json",
					"glyph run <spec> --format json",
					"glyph context latest --format md",
					"glyph diff <runA> <runB> --format md",
				},
				"rules": []string{
					"Treat intent and outcomes as the behavior contract.",
					"Treat steps as the repairable path to reach the contract.",
					"Run spec verification before running a spec.",
					"Use context latest after failures before editing code.",
					"Prefer json or yaml for machine parsing and md for human reports.",
				},
				"topics": docsTopics(),
			}
			output, err := emitForCLI(cmd, opts, format, value, renderAgentGuideMarkdown)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
}

func renderAgentGuideMarkdown() string {
	var b strings.Builder
	b.WriteString("# Glyphrun Agent Guide\n\n")
	b.WriteString("Glyphrun runs terminal behavior specs in a real PTY and writes artifact packs that are useful to both humans and coding agents.\n\n")
	b.WriteString("## First Commands\n\n")
	b.WriteString("- `glyph explain --format json`\n")
	b.WriteString("- `glyph docs agents --format md`\n")
	b.WriteString("- `glyph docs authoring --format md`\n")
	b.WriteString("- `glyph docs snippets --format md`\n")
	b.WriteString("- `glyph doctor --format json`\n\n")
	b.WriteString("## Spec Workflow\n\n")
	b.WriteString("- `glyph spec verify <spec> --format json`\n")
	b.WriteString("- `glyph run <spec> --format json`\n")
	b.WriteString("- `glyph run <spec> --format md`\n")
	b.WriteString("- `glyph context latest --format md`\n\n")
	b.WriteString("## Failure Workflow\n\n")
	b.WriteString("- inspect `glyph context latest --format md`\n")
	b.WriteString("- inspect `diagnostics/failure.md`\n")
	b.WriteString("- inspect `screens/final.txt`\n")
	b.WriteString("- inspect `frames/frames.ndjson` when timing or transitions matter\n")
	b.WriteString("- compare runs with `glyph diff <runA> <runB> --format md`\n\n")
	b.WriteString("## Contract Rules\n\n")
	b.WriteString("- keep `intent` and `outcomes` stable unless the expected behavior truly changed\n")
	b.WriteString("- adjust `steps` when the route through the UI changed\n")
	b.WriteString("- use `glyph spec verify --stamp` when a deliberate contract change needs a fresh hash\n")
	b.WriteString("- use `--format json` or `--format yaml` for machine-readable output\n")
	b.WriteString("- use `--format md` for terminal reports, PR notes, and debugging\n\n")
	b.WriteString("## Docs Topics\n\n")
	for _, topic := range docsTopics() {
		b.WriteString("- `glyph docs ")
		b.WriteString(topic)
		b.WriteString(" --format md`\n")
	}
	return b.String()
}
