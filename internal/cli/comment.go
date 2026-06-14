package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/spf13/cobra"
)

func newCommentCommand(opts *globalOptions) *cobra.Command {
	var out string
	var last int
	cmd := &cobra.Command{
		Use:   "comment [run|latest ...]",
		Short: "Render a PR-comment-ready Markdown summary of one or more runs",
		Long: "Produce GitHub-flavored Markdown summarizing run results — status, " +
			"outcome counts, failure focus, the final screen, and artifact pointers " +
			"(including the deterministic SVG screenshot). Writes to stdout by " +
			"default so it can be piped to `gh pr comment -F -`, or to a file with " +
			"--out. Use --last N to summarize the N most recent runs.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			var runDirs []string
			explicit := false
			for _, arg := range args {
				if arg != "latest" {
					explicit = true
				}
			}
			switch {
			case explicit:
				for _, arg := range args {
					dir, err := resolveRunDir(root, arg)
					if err != nil {
						return exitError{code: 2, err: err}
					}
					runDirs = append(runDirs, dir)
				}
			default:
				// No run given (or just "latest"): take the most recent runs by
				// modification time, which is correct even when `record-*`
				// dirs (non-timestamped names) are present.
				if last < 1 {
					last = 1
				}
				runDirs, err = recentRunDirs(root, last)
				if err != nil {
					return exitError{code: 2, err: fmt.Errorf("list runs: %w", err)}
				}
			}

			results := make([]artifacts.RunResult, 0, len(runDirs))
			for _, dir := range runDirs {
				result, err := artifacts.LoadRunResult(dir)
				if err != nil {
					continue
				}
				results = append(results, result)
			}
			markdown := renderPRComment(results)

			// Write to stdout (not cmd.Print, which cobra routes to stderr) so
			// the comment can be piped directly into `gh pr comment -F -`.
			if out == "" {
				_, _ = cmd.OutOrStdout().Write([]byte(markdown))
				return nil
			}
			if dir := filepath.Dir(out); dir != "" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return exitError{code: 2, err: err}
				}
			}
			if err := os.WriteFile(out, []byte(markdown), 0o644); err != nil {
				return exitError{code: 2, err: err}
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "wrote PR comment to %s\n", out)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "write the comment Markdown to this file (default: stdout)")
	cmd.Flags().IntVar(&last, "last", 1, "summarize the N most recent runs when no run is given")
	return cmd
}

// recentRunDirs returns the n newest run directories under root, newest first.
// Ordering is by modification time rather than name, because `record-*` run
// dirs do not share the timestamped naming of `glyph run` dirs.
func recentRunDirs(root string, n int) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	type runDir struct {
		path string
		mod  int64
	}
	var dirs []runDir
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		dirs = append(dirs, runDir{path: filepath.Join(root, e.Name()), mod: info.ModTime().UnixNano()})
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].mod > dirs[j].mod })
	if n < len(dirs) {
		dirs = dirs[:n]
	}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, d.path)
	}
	return out, nil
}

// renderPRComment builds GitHub-flavored Markdown summarizing the given runs.
func renderPRComment(results []artifacts.RunResult) string {
	var b strings.Builder
	allPassed := true
	for _, r := range results {
		if r.Status != artifacts.StatusPassed {
			allPassed = false
		}
	}
	badge := "✅ passed"
	if !allPassed {
		badge = "❌ failed"
	}
	if len(results) == 0 {
		return "## Glyphrun\n\nNo runs found.\n"
	}

	fmt.Fprintf(&b, "## Glyphrun — %s\n\n", badge)

	// Summary table across runs.
	b.WriteString("| Spec | Status | Outcomes | Exit | Duration |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, r := range results {
		passed, failed := 0, 0
		for _, o := range r.Outcomes {
			if o.Status == artifacts.OutcomePassed {
				passed++
			} else {
				failed++
			}
		}
		mark := "✅"
		if r.Status != artifacts.StatusPassed {
			mark = "❌"
		}
		fmt.Fprintf(&b, "| `%s` | %s %s | %d✓ %d✗ | %d | %dms |\n",
			r.SpecName, mark, r.Status, passed, failed, r.ExitCode, r.DurationMS)
	}
	b.WriteByte('\n')

	// Per-failure detail with the final screen folded into a <details> block.
	for _, r := range results {
		if r.Status == artifacts.StatusPassed {
			continue
		}
		fmt.Fprintf(&b, "### ❌ %s\n\n", r.SpecName)
		for _, o := range r.Outcomes {
			if o.Status == artifacts.OutcomePassed {
				continue
			}
			fmt.Fprintf(&b, "- `%s`", o.ID)
			if o.Message != "" {
				fmt.Fprintf(&b, ": %s", o.Message)
			}
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
		if screen := readRunScreen(r); screen != "" {
			b.WriteString("<details><summary>Final screen</summary>\n\n```text\n")
			b.WriteString(screen)
			b.WriteString("\n```\n\n</details>\n\n")
		}
		// Point at the deterministic SVG and agent context in the uploaded
		// artifact pack.
		if r.Artifacts["finalScreenSVG"] != "" {
			fmt.Fprintf(&b, "Screenshot: `%s/%s` · ", filepath.Base(r.RunDir), r.Artifacts["finalScreenSVG"])
		}
		if r.Artifacts["agentContext"] != "" {
			fmt.Fprintf(&b, "Context: `%s/%s`", filepath.Base(r.RunDir), r.Artifacts["agentContext"])
		}
		b.WriteString("\n\n")
	}

	b.WriteString("<sub>Generated by `glyph comment`.</sub>\n")
	return b.String()
}

func readRunScreen(r artifacts.RunResult) string {
	rel := r.Artifacts["finalScreenText"]
	if rel == "" {
		rel = "screens/final.txt"
	}
	data, err := os.ReadFile(filepath.Join(r.RunDir, rel))
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\n")
}
