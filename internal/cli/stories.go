package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/scaffold"
	"github.com/abdul-hamid-achik/glyphrun/internal/stories"
	"github.com/abdul-hamid-achik/glyphrun/internal/storyrun"
	"github.com/abdul-hamid-achik/glyphrun/internal/tui"
	"github.com/spf13/cobra"
)

// storyRoots resolves the artifact, snapshot, and stories roots (absolute)
// plus the default terminal for the current project.
type storyRoots struct {
	runtime      config.Runtime
	artifactRoot string
	snapshotRoot string
	storiesRoot  string
}

func resolveStoryRoots(opts *globalOptions, start string) (storyRoots, error) {
	rt, err := config.LoadRuntime(start, opts.configPath, opts.environment)
	if err != nil {
		return storyRoots{}, err
	}
	roots := stories.ResolveRoots(rt, opts.artifactRoot)
	return storyRoots{
		runtime:      rt,
		artifactRoot: roots.ArtifactRoot,
		snapshotRoot: roots.SnapshotRoot,
		storiesRoot:  roots.StoriesRoot,
	}, nil
}

func collectStories(opts *globalOptions, paths []string, feature, tag, owner string, all bool) (stories.Catalog, storyRoots, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	roots, err := resolveStoryRoots(opts, paths[0])
	if err != nil {
		return stories.Catalog{}, roots, err
	}
	collect := stories.ResolveRoots(roots.runtime, opts.artifactRoot).CollectOptions(paths, opts.configPath, opts.environment)
	collect.Feature = feature
	collect.Tag = tag
	collect.Owner = owner
	collect.All = all
	cat, err := stories.Collect(collect)
	return cat, roots, err
}

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
		Short: "Catalog, run, and inspect TUI stories (Storybook for the terminal)",
		Long: `Walk spec paths for stories manifests (stories.yml) and specs tagged
"story", and print a catalog joined to each story's newest result and its
golden status (match / changed / missing).

  glyph stories run [--watch] [--update] [--only list/rows]   run stories
  glyph stories serve [--watch]                              live catalog
  glyph stories --html                                       inspect page
  glyph stories --tui                                        terminal catalog
  glyph stories init                                         scaffold

JSON/YAML output never emits HTML or opens a TUI.`,
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
			cat, roots, err := collectStories(opts, args, feature, tag, owner, all)
			if err != nil {
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
					outPath = filepath.Join(roots.storiesRoot, "stories.html")
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
					"summary":       cat.Summarize(),
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
	cmd.Flags().StringVar(&feature, "feature", "", "filter to stories whose feature matches")
	cmd.Flags().StringVar(&tag, "tag", "", "filter to stories whose tags include the value")
	cmd.Flags().StringVar(&owner, "owner", "", "filter to stories whose owner matches")
	cmd.Flags().BoolVar(&all, "all", false, "include specs that are not tagged story")
	cmd.Flags().BoolVar(&htmlOut, "html", false, "write a self-contained HTML catalog (requires --format md)")
	cmd.Flags().BoolVar(&useTUI, "tui", false, "browse stories in the host terminal (requires a TTY)")
	cmd.Flags().StringVar(&out, "out", "", "HTML output path; '-' writes raw HTML to stdout (default <storiesRoot>/stories.html)")
	cmd.AddCommand(newStoriesRunCommand(opts))
	cmd.AddCommand(newStoriesServeCommand(opts))
	cmd.AddCommand(newStoriesInitCommand(opts))
	return cmd
}

func storiesToTUI(cat stories.Catalog) []tui.Story {
	out := make([]tui.Story, 0, len(cat.Stories))
	for _, s := range cat.Stories {
		label := s.Name
		if s.ID != "" {
			label = s.ID
			if s.Variant != "" {
				label += " @" + s.Variant
			}
		}
		item := tui.Story{Name: label, SpecName: s.Name, Feature: s.Feature, Status: s.Status, RunID: s.RunID, Golden: s.Golden, Diagnostic: s.Diagnostic}
		for _, snap := range s.Snapshots {
			item.Snapshots = append(item.Snapshots, tui.StorySnap{
				Name:    snap.Name,
				Status:  snap.Status,
				Error:   snap.Error,
				Golden:  snap.Golden,
				Screen:  snap.Screen,
				Before:  snap.GoldenScreen,
				Changed: snap.Diff,
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
	sum := cat.Summarize()
	fmt.Fprintf(&b, "- stories: %d · passed %d · failed %d · not run %d · goldens changed %d · goldens missing %d\n\n",
		sum.Stories, sum.Passed, sum.Failed, sum.NotRun, sum.Changed, sum.Missing)
	b.WriteString("| feature | story | spec | status | golden | run | snapshots |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, s := range cat.Stories {
		snaps := make([]string, 0, len(s.Snapshots))
		for _, snap := range s.Snapshots {
			label := snap.Name
			if snap.Golden == stories.GoldenChanged {
				label += fmt.Sprintf(" (±%d)", snap.Changed)
			}
			snaps = append(snaps, label)
		}
		feature := s.Feature
		if feature == "" {
			feature = "—"
		}
		story := s.ID
		if story == "" {
			story = s.Name
		}
		if s.Variant != "" {
			story += "@" + s.Variant
		}
		run := s.RunID
		if run == "" {
			run = "—"
		}
		status := s.Status
		if s.ParseError != "" {
			status = "parse_error: " + s.ParseError
		}
		fmt.Fprintf(&b, "| %s | `%s` | `%s` | %s | %s | `%s` | %s |\n",
			feature, story, s.Name, status, s.Golden, run, strings.Join(snaps, ", "))
	}
	return b.String()
}

func newStoriesRunCommand(opts *globalOptions) *cobra.Command {
	var (
		parallel   int
		update     bool
		strict     bool
		watch      bool
		watchPaths []string
		only       []string
		progress   string
	)
	cmd := &cobra.Command{
		Use:   "run [path...]",
		Short: "Build each harness once, run every story, and refresh the stories index",
		Long: `Discover stories manifests (stories.yml) and specs tagged "story" under
the paths, build each manifest's harness once, run the stories in parallel,
and record the newest result under storiesRoot so the catalog does not depend
on run retention. A story whose golden is missing captures it on this run
(--strict fails instead); --update rewrites every golden.

--watch re-runs on changes to the manifests, harness.watch paths, and story
spec directories (interactive; markdown output only).`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			if (watch || len(watchPaths) > 0) && format != formatMD {
				return exitError{code: 2, err: errors.New("--watch requires --format md (it is an interactive loop, not machine output)")}
			}
			listener, err := makeRunProgressListener(cmd, opts, format, parallel, progress)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			ropts := storyrun.Options{
				Paths:        args,
				Only:         only,
				ConfigPath:   opts.configPath,
				Environment:  opts.environment,
				ArtifactRoot: opts.artifactRoot,
				Parallel:     parallel,
				Update:       update,
				Strict:       strict,
				Listener:     listener,
			}
			if watch || len(watchPaths) > 0 {
				ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
				defer stop()
				stderr := cmd.ErrOrStderr()
				fmt.Fprintln(stderr, "glyph stories watch: press Ctrl-C to stop")
				err := storyrun.Watch(ctx, ropts, watchPaths, func(report storyrun.Report, runErr error) {
					if runErr != nil && ctx.Err() == nil {
						fmt.Fprintln(stderr, runErr)
					}
					out, _ := emitForCLI(cmd, opts, formatMD, report, func() string { return storyrun.RenderReportMarkdown(report) })
					cmd.Print(out)
				})
				if err != nil {
					return exitError{code: 2, err: err}
				}
				fmt.Fprintln(stderr, "\nglyph stories watch: stopped")
				return nil
			}
			plan, err := storyrun.Discover(ropts)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			report, runErr := storyrun.Run(context.Background(), ropts, plan)
			output, err := emitForCLI(cmd, opts, format, report, func() string { return storyrun.RenderReportMarkdown(report) })
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			if runErr != nil {
				return exitError{code: 2, err: runErr}
			}
			if report.ExitCode != 0 {
				return exitError{code: report.ExitCode}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&parallel, "parallel", 4, "number of stories to run concurrently")
	cmd.Flags().BoolVar(&update, "update", false, "rewrite every golden with the captured screen")
	cmd.Flags().BoolVar(&strict, "strict", false, "fail stories whose golden is missing instead of creating it")
	cmd.Flags().BoolVar(&watch, "watch", false, "re-run on manifest/harness/spec changes (interactive; markdown output only)")
	cmd.Flags().StringArrayVar(&watchPaths, "watch-path", nil, "additional file or directory to watch (repeatable); implies --watch")
	cmd.Flags().StringArrayVar(&only, "only", nil, "run only matching stories: key (list/rows, list/rows@wide), spec name, or feature (repeatable)")
	cmd.Flags().StringVar(&progress, "progress", "auto", "live progress: auto, always, never")
	return cmd
}

func newStoriesServeCommand(opts *globalOptions) *cobra.Command {
	var (
		addr       string
		watch      bool
		watchPaths []string
		parallel   int
		noRun      bool
	)
	cmd := &cobra.Command{
		Use:   "serve [path...]",
		Short: "Serve the live stories catalog (rerun, diff, and accept goldens from the browser)",
		Long: `Start a local HTTP server on a loopback address with the inspect page in
live mode: the catalog refreshes over server-sent events after every run, the
page can rerun one or all stories, and "accept golden" rewrites a story's
golden from the browser. --watch re-runs on source changes. Stories run once
on start unless --no-run is set.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			if format != formatMD {
				return exitError{code: 2, err: errors.New("serve requires --format md (it is an interactive server, not machine output)")}
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			stderr := cmd.ErrOrStderr()
			sopts := storyrun.ServeOptions{
				Options: storyrun.Options{
					Paths:        args,
					ConfigPath:   opts.configPath,
					Environment:  opts.environment,
					ArtifactRoot: opts.artifactRoot,
					Parallel:     parallel,
				},
				Addr:       addr,
				Watch:      watch || len(watchPaths) > 0,
				ExtraWatch: watchPaths,
				RunOnStart: !noRun,
				Ready: func(url string) {
					cmd.Printf("# Glyphrun Stories Serve\n\n- url: %s\n- watch: %v\n\nPress Ctrl-C to stop.\n", url, watch || len(watchPaths) > 0)
				},
			}
			if err := storyrun.Serve(ctx, sopts); err != nil {
				return exitError{code: 2, err: err}
			}
			fmt.Fprintln(stderr, "\nglyph stories serve: stopped")
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:4649", "listen address (loopback by default)")
	cmd.Flags().BoolVar(&watch, "watch", false, "re-run stories when manifests, harness sources, or story specs change")
	cmd.Flags().StringArrayVar(&watchPaths, "watch-path", nil, "additional file or directory to watch (repeatable); implies --watch")
	cmd.Flags().IntVar(&parallel, "parallel", 4, "number of stories to run concurrently")
	cmd.Flags().BoolVar(&noRun, "no-run", false, "serve the existing index without running stories on start")
	return cmd
}

func newStoriesInitCommand(opts *globalOptions) *cobra.Command {
	var lang string
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Write a story harness and a stories.yml manifest",
		Long: `Scaffold a stories setup: a harness that mounts one isolated TUI state per
story id, and a stories.yml manifest that lists those ids. --lang go writes a
Bubble Tea v2 harness; --lang sh writes a POSIX shell harness that needs no
toolchain. Existing files are skipped.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			result, err := scaffold.InitStories(dir, lang)
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
				if result.ManifestPath != "" {
					md += "- manifest: `" + result.ManifestPath + "`\n"
				}
				md += "\nNext: `glyph stories run` then `glyph stories serve --watch`.\n"
				return md
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
	cmd.Flags().StringVar(&lang, "lang", "go", "harness language: go (Bubble Tea v2) or sh (POSIX shell)")
	return cmd
}
