package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/spf13/cobra"
)

func newContextCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "context <run|latest>",
		Short: "Print an agent context artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			rt, err := config.LoadRuntime(".", opts.configPath, opts.environment)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			root := opts.artifactRoot
			if root == "" {
				root = rt.Config.ArtifactRoot
			}
			if !filepath.IsAbs(root) {
				root = filepath.Join(rt.ProjectRoot, root)
			}
			runDir, err := resolveContextRunDir(root, args[0])
			if err != nil {
				return exitError{code: 2, err: err}
			}
			contextPath := filepath.Join(runDir, "agent_context.md")
			content, err := os.ReadFile(contextPath)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			value := map[string]any{
				"schemaVersion": 1,
				"run":           filepath.Base(runDir),
				"path":          contextPath,
				"content":       string(content),
			}
			output, err := emitForCLI(cmd, opts, format, value, func() string { return string(content) })
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
}

func resolveContextRunDir(root string, arg string) (string, error) {
	if arg != "latest" {
		return resolveRunDir(root, arg)
	}
	// `glyph context latest` resolves to the newest run that actually
	// produced an agent_context.md, skipping runs that died before
	// writing one.
	return latestRunDir(root, "agent_context.md")
}

func resolveRunDir(root string, arg string) (string, error) {
	if arg == "latest" {
		return latestRunDir(root, "")
	}
	if filepath.IsAbs(arg) {
		return arg, nil
	}
	if strings.ContainsRune(arg, filepath.Separator) {
		return filepath.Abs(arg)
	}
	return filepath.Join(root, arg), nil
}

// latestRunDir returns the newest run directory under root. Run
// directory names are timestamped, so a reverse lexical sort matches
// chronological order. When requireFile is non-empty, only directories
// containing that file are considered.
func latestRunDir(root string, requireFile string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	type candidate struct {
		path string
		name string
	}
	var dirs []candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if requireFile != "" {
			if _, err := os.Stat(filepath.Join(path, requireFile)); err != nil {
				continue
			}
		}
		dirs = append(dirs, candidate{path: path, name: entry.Name()})
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name > dirs[j].name })
	if len(dirs) == 0 {
		return "", os.ErrNotExist
	}
	return dirs[0].path, nil
}
