package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var docsByTopic = map[string]string{
	"overview": `# Glyphrun Overview

Glyphrun runs YAML or JSON terminal behavior specs against a target command in a real PTY. Assertions read a virtual terminal screen, not raw ANSI bytes.
`,
	"authoring": `# Authoring

Separate behavior contracts from repairable steps. Keep user intent in ` + "`intent`" + `, stable expectations in ` + "`outcomes`" + `, and navigation/input hints in ` + "`steps`" + `.
`,
	"steps": `# Steps

Supported v1 steps: ` + "`press`" + `, ` + "`type`" + `, ` + "`paste`" + `, ` + "`send`" + `, ` + "`wait`" + `, ` + "`resize`" + `, ` + "`snapshot`" + `, and imported ` + "`use`" + ` actions.
`,
	"verifiers": `# Verifiers

Supported v1 verifiers: ` + "`screen`" + `, ` + "`region`" + `, ` + "`cell`" + `, ` + "`cursor`" + `, ` + "`process`" + `, ` + "`snapshot`" + `, and trusted ` + "`command`" + `.
`,
	"artifacts": `# Artifacts

Each run writes ` + "`run.json`" + `, ` + "`run.yaml`" + `, ` + "`run.md`" + `, ` + "`agent_context.md`" + `, ` + "`events.ndjson`" + `, ` + "`spec.resolved.yml`" + `, final screens, frames, raw logs, snapshots, outcomes, and diagnostics.
`,
	"agents": `# Agents

Call ` + "`glyph explain --format json`" + ` first, then ` + "`glyph spec verify`" + `, ` + "`glyph run`" + `, and ` + "`glyph context latest`" + `. Do not edit ` + "`intent`" + ` or ` + "`outcomes`" + ` without surfacing the contract change.
`,
	"mcp": `# MCP

MCP is reserved for a later phase. Tools should mirror the CLI and call the same internal command handlers.
`,
	"configuration": `# Configuration

Glyphrun reads ` + "`glyphrun.config.yml`" + ` by walking up from the spec path. Defaults include ` + "`.glyphrun/runs`" + ` artifacts and an xterm-256color terminal profile.
`,
	"troubleshooting": `# Troubleshooting

Use ` + "`glyph context latest --format md`" + ` after a failure. Inspect ` + "`screens/final.txt`" + `, ` + "`raw/pty.raw.log`" + `, ` + "`frames/frames.ndjson`" + `, and ` + "`diagnostics/failure.md`" + `.
`,
}

func newDocsCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "docs [topic]",
		Short: "Show focused documentation",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			topic := "overview"
			if len(args) > 0 {
				topic = args[0]
			}
			content, ok := docsByTopic[topic]
			if !ok {
				return exitError{code: 2, err: fmt.Errorf("unknown docs topic %q", topic)}
			}
			value := map[string]any{"schemaVersion": 1, "topic": topic, "content": content}
			output, err := emit(format, value, func() string { return content })
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
}
