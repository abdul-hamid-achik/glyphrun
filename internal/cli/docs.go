package cli

import (
	"fmt"

	glyphdocs "github.com/abdul-hamid-achik/glyphrun/internal/docs"
	"github.com/spf13/cobra"
)

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
			content, ok := glyphdocs.Content(topic)
			if !ok {
				return exitError{code: 2, err: fmt.Errorf("unknown docs topic %q; run `glyph docs topics --format md`", topic)}
			}
			value := map[string]any{"schemaVersion": 1, "topic": topic, "content": content}
			output, err := emitForCLI(cmd, opts, format, value, func() string { return content })
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
}

func docsTopics() []string {
	return glyphdocs.Topics()
}
