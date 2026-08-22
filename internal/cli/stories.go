package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/scaffold"
	"github.com/abdul-hamid-achik/glyphrun/internal/stories"
	"github.com/abdul-hamid-achik/glyphrun/internal/tui"
	"github.com/spf13/cobra"
)

func newStoriesCommand(opts *globalOptions) *cobra.Command {
	var (
		feature string
		tag     string
		owner   string
		all     bool
		htmlOut bool
		out     string
		useTUI  bool
	)
	cmd := &cobra.Command{
		Use:   "stories [path...]",
		Short: "Catalog TUI stories and inspect captured screens",
		Long: `Walk spec paths and print a catalog of stories (specs, usually tagged
"story") joined to their newest run's snapshots.

--html writes a self-contained inspect page (grid / rulers / spaces overlays
and a cell hover inspector). --tui browses the same snapshots in the host
terminal. JSON/YAML output never emits HTML or opens a TUI.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			if htmlOut && useTUI {
				return exitError{code: 2, err: errors.New("--html and --tui cannot be combined")}
			}
			if htmlOut && format != formatMD {
				return exitError{code: 2, err: errors.New("--html requires --format md")}
			}
			if useTUI && format != formatMD {
				return exitError{code: 2, err: errors.New("--tui requires --format md")}
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
			paths := args
			if len(paths) == 0 {
				paths = []string{"."}
			}
			cat, err := stories.Collect(stories.CollectOptions{
				Paths:        paths,
				ArtifactRoot: root,
				ConfigPath:   opts.configPath,
				Environment:  opts.environment,
				Feature:      feature,
				Tag:          tag,
				Owner:        owner,
				All:          all,
			})
			if err != nil {
				if errors.Is(err, stories.ErrNoSpecs) {
					return exitError{code: 2, err: err}
				}
				return exitError{code: 2, err: err}
			}
			if useTUI {
				if !isTerminalWriter(cmd.OutOrStdout()) {
					return exitError{code: 2, err: errNotATTY}
				}
				if err := tui.RunStories(storiesToTUI(cat)); err != nil {
					return exitError{code: 2, err: err}
				}
				return nil
			}
			if htmlOut {
				page := stories.RenderHTML(cat)
				if out == "-" {
					_, _ = cmd.OutOrStdout().Write([]byte(page))
					return nil
				}
				outPath := out
				if outPath == "" {
					outPath = filepath.Join(root, "stories.html")
				}
				if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
					return exitError{code: 2, err: err}
				}
				if err := os.WriteFile(outPath, []byte(page), 0o644); err != nil {
					return exitError{code: 2, err: err}
				}
				value := map[string]any{
					"schemaVersion": 1,
					"path":          outPath,
					"bytes":         len(page),
					"stories":       len(cat.Stories),
				}
				output, err := emitForCLI(cmd, opts, format, value, func() string {
					return fmt.Sprintf("# Glyphrun Stories\n\n- html: `%s` (%d bytes)\n- stories: %d\n", outPath, len(page), len(cat.Stories))
				})
				if err != nil {
					return exitError{code: 2, err: err}
				}
				cmd.Print(output)
				return nil
			}
			output, err := emitForCLI(cmd, opts, format, cat, func() string { return renderStoriesMarkdown(cat) })
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
	cmd.Flags().StringVar(&feature, "feature", "", "filter to specs whose metadata.feature matches")
	cmd.Flags().StringVar(&tag, "tag", "", "filter to specs whose metadata.tags includes the value")
	cmd.Flags().StringVar(&owner, "owner", "", "filter to specs whose metadata.owner matches")
	cmd.Flags().BoolVar(&all, "all", false, "include specs that are not tagged story")
	cmd.Flags().BoolVar(&htmlOut, "html", false, "write a self-contained HTML catalog (requires --format md)")
	cmd.Flags().BoolVar(&useTUI, "tui", false, "browse stories in the host terminal (requires a TTY)")
	cmd.Flags().StringVar(&out, "out", "", "HTML output path; '-' writes raw HTML to stdout")
	cmd.AddCommand(newStoriesInitCommand(opts))
	return cmd
}

func storiesToTUI(cat stories.Catalog) []tui.Story {
	out := make([]tui.Story, 0, len(cat.Stories))
	for _, s := range cat.Stories {
		item := tui.Story{Name: s.Name, Feature: s.Feature, Status: s.Status, RunID: s.RunID}
		for _, snap := range s.Snapshots {
			item.Snapshots = append(item.Snapshots, tui.StorySnap{
				Name:   snap.Name,
				Status: snap.Status,
				Error:  snap.Error,
				Screen: snap.Screen,
			})
		}
		out = append(out, item)
	}
	return out
}

func renderStoriesMarkdown(cat stories.Catalog) string {
	var b strings.Builder
	b.WriteString("# Glyphrun Stories\n\n")
	if len(cat.Stories) == 0 {
		b.WriteString("No stories found.\n")
		return b.String()
	}
	b.WriteString("| feature | name | status | run | snapshots |\n| --- | --- | --- | --- | --- |\n")
	for _, s := range cat.Stories {
		snaps := make([]string, 0, len(s.Snapshots))
		for _, snap := range s.Snapshots {
			snaps = append(snaps, snap.Name)
		}
		feature := s.Feature
		if feature == "" {
			feature = "—"
		}
		run := s.RunID
		if run == "" {
			run = "—"
		}
		fmt.Fprintf(&b, "| %s | `%s` | %s | `%s` | %s |\n",
			feature, s.Name, s.Status, run, strings.Join(snaps, ", "))
	}
	return b.String()
}

func newStoriesInitCommand(opts *globalOptions) *cobra.Command {
	var lang string
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Write a Go Bubble Tea story harness and a starter spec",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			if lang != "go" {
				return exitError{code: 2, err: fmt.Errorf("unsupported --lang %q (v1 supports go)", lang)}
			}
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			result, err := scaffold.InitGo(dir)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			output, err := emitForCLI(cmd, opts, format, result, func() string {
				md := "# Stories Init\n\n"
				md += "- lang: `" + result.Lang + "`\n"
				for _, w := range result.Written {
					md += "- written: `" + w + "`\n"
				}
				for _, s := range result.Skipped {
					md += "- skipped: `" + s + "`\n"
				}
				if result.SpecPath != "" {
					md += "- spec: `" + result.SpecPath + "`\n"
				}
				return md
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
	cmd.Flags().StringVar(&lang, "lang", "go", "harness language (v1: go)")
	return cmd
}
