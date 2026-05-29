package cli

import (
	"context"
	"os"

	"github.com/abdul-hamid-achik/glyphrun/internal/mcp"
	"github.com/spf13/cobra"
)

func newMCPCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run the Glyphrun MCP stdio server",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := mcp.Serve(context.Background(), os.Stdin, os.Stdout, mcp.ServerOptions{
				ConfigPath:   opts.configPath,
				ArtifactRoot: opts.artifactRoot,
				Environment:  opts.environment,
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			return nil
		},
	}
}
