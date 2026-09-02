package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// explainSteps and explainVerifiers are the vocabulary `glyph explain`
// advertises. Both the JSON envelope and the markdown render read these
// slices so the two outputs cannot drift apart.
var (
	explainSteps     = []string{"press", "type", "paste", "send", "mouse", "wait", "resize", "snapshot", "use", "when", "download", "transform", "monitor", "batch"}
	explainVerifiers = []string{"screen", "region", "cell", "cursor", "process", "snapshot", "command", "file", "script", "count", "link", "metrics"}
)

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
					"glyph run <spec...> --repeat N",
					"glyph run <spec...> --watch",
					"glyph run <spec...> --monitor <path>",
					"glyph run --rerun-failed",
					"glyph spec verify <spec>",
					"glyph spec scaffold",
					"glyph spec scaffold --kind action",
					"glyph spec scaffold --kind story",
					"glyph spec scaffold --coversSymbol <sym>",
					"glyph diff <runA> <runB>",
					"glyph record -- <command...>",
					"glyph record --scaffold <path> -- <command...>",
					"glyph snapshot update <spec...>",
					"glyph snapshot inventory [run]",
					"glyph replay <run>",
					"glyph replay <run> --html",
					"glyph render <run|latest>",
					"glyph context <run|latest>",
					"glyph repair <spec> [run|latest]",
					"glyph comment [run|latest ...]",
					"glyph docs [topic]",
					"glyph agent",
					"glyph explain",
					"glyph doctor",
					"glyph mcp",
					"glyph list",
					"glyph stories",
					"glyph stories run",
					"glyph stories run --watch",
					"glyph stories run --update",
					"glyph stories run --only <story>",
					"glyph stories serve --watch",
					"glyph stories --html",
					"glyph stories --tui",
					"glyph stories init",
					"glyph render <run|latest> --grid --rulers --spaces",
					"glyph import bats <file>",
					"glyph affected-specs --since <ref>",
					"glyph export bats <spec>",
					"glyph clean",
					"glyph clean --no-archive",
					"glyph version",
				},
				"steps":          explainSteps,
				"stepFields":     []string{"id", "when"},
				"verifiers":      explainVerifiers,
				"screenMatchers": []string{"equals", "contains", "notContains", "matches", "regex"},
				"modes":          []string{"normal", "debug"},
				"formats":        []string{"json", "yaml", "md"},
				"progress":       []string{"auto", "always", "never"},
				"runSchema":      "urn:glyphrun.dev:run:v1",
				"artifacts": []string{
					"run.json",
					"run.yaml",
					"run.md",
					"agent_context.md",
					"events.ndjson",
					"spec.resolved.yml",
					"screens/final.txt",
					"screens/final.json",
					"screens/final.svg",
					"raw/pty.raw.log",
					"frames/frames.ndjson",
					"snapshots/*.txt",
					"outcomes/*.md",
					"diagnostics/*.md",
					".last-failed.json",
				},
			}
			output, err := emitForCLI(cmd, opts, format, value, func() string {
				return `# Glyphrun Explain

- binary: ` + "`glyph`" + `
- agent guide: ` + "`glyph agent --format md`" + `
- docs: ` + "`glyph docs agents --format md`" + `, ` + "`glyph docs authoring --format md`" + `, ` + "`glyph docs snippets --format md`" + `
- init: ` + "`glyph init --cmd ./bin/app --ready ready`" + `
- context: ` + "`glyph context latest --format md`" + `
- steps: ` + strings.Join(explainSteps, ", ") + `
- verifiers: ` + strings.Join(explainVerifiers, ", ") + `
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
