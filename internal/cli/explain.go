package cli

import "github.com/spf13/cobra"

func newExplainCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "explain",
		Short: "Describe the current CLI/spec/artifact vocabulary",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			value := map[string]any{
				"schemaVersion": 1,
				"project":       "glyphrun",
				"binary":        "glyph",
				"commands": []string{
					"glyph init [dir]",
					"glyph run <spec...>",
					"glyph spec verify <spec>",
					"glyph spec scaffold",
					"glyph spec scaffold --kind action",
					"glyph snapshot update <spec...>",
					"glyph diff <runA> <runB>",
					"glyph record -- <command...>",
					"glyph replay <run>",
					"glyph context <run|latest>",
					"glyph docs [topic]",
					"glyph agent",
					"glyph explain",
					"glyph doctor",
					"glyph mcp",
					"glyph list",
					"glyph import bats <file>",
					"glyph export bats <spec>",
					"glyph clean",
					"glyph version",
				},
				"steps":     []string{"press", "type", "paste", "send", "wait", "resize", "snapshot", "use", "when", "download", "transform", "batch"},
				"verifiers": []string{"screen", "region", "cell", "cursor", "process", "snapshot", "command", "file", "script", "count"},
				"formats":   []string{"json", "yaml", "md"},
				"progress":  []string{"auto", "always", "never"},
				"artifacts": []string{
					"run.json",
					"run.yaml",
					"run.md",
					"agent_context.md",
					"events.ndjson",
					"spec.resolved.yml",
					"screens/final.txt",
					"screens/final.json",
					"raw/pty.raw.log",
					"frames/frames.ndjson",
					"snapshots/*.txt",
					"outcomes/*.md",
					"diagnostics/*.md",
				},
			}
			output, err := emitForCLI(cmd, opts, format, value, func() string {
				return `# Glyphrun Explain

- binary: ` + "`glyph`" + `
- agent guide: ` + "`glyph agent --format md`" + `
- docs: ` + "`glyph docs agents --format md`" + `, ` + "`glyph docs authoring --format md`" + `, ` + "`glyph docs snippets --format md`" + `
- init: ` + "`glyph init --cmd ./bin/app --ready ready`" + `
- context: ` + "`glyph context latest --format md`" + `
- steps: press, type, paste, send, wait, resize, snapshot, use, when guards, download, transform, batch
- verifiers: screen, region, cell, cursor, process, snapshot, command, file, script, count
- formats: json, yaml, md
- progress: ` + "`glyph run <spec> --progress auto|always|never`" + `
- artifacts: run summaries, agent context, events, final screen, raw PTY log, frames, snapshots, outcomes, diagnostics
`
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
}
