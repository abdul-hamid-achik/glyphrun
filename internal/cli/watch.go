package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/watchfs"
	"github.com/spf13/cobra"
)

const watchPollInterval = watchfs.PollInterval

// runWatch re-runs the given specs whenever a watched file changes. It is an
// interactive, human-facing loop: it requires markdown output (the structured
// formats are for one-shot automation) and polls the filesystem so it pulls in
// no third-party file-notification dependency.
func runWatch(cmd *cobra.Command, opts *globalOptions, format outputFormat, specPaths, extraPaths []string, parallel int, updateSnapshots bool, progress string) error {
	if format != formatMD {
		return exitError{code: 2, err: fmt.Errorf("--watch requires --format md (it is an interactive loop, not machine output)")}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	roots := watchRoots(specPaths, extraPaths)
	stderr := cmd.ErrOrStderr()

	runOnce := func() {
		listener, err := makeRunProgressListener(cmd, opts, format, parallel, progress)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return
		}
		results, _, runErr := runSpecs(ctx, specPaths, parallel, opts, updateSnapshots, listener, nil)
		if runErr != nil && ctx.Err() == nil {
			fmt.Fprintln(stderr, runErr)
		}
		cmd.Print(renderWatchResults(cmd, opts, results))
	}

	fmt.Fprintf(stderr, "glyph watch: %d spec(s), %d watched path(s); press Ctrl-C to stop\n", len(specPaths), len(roots))
	runOnce()
	last := fingerprint(roots)

	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(stderr, "\nglyph watch: stopped")
			return nil
		case <-ticker.C:
			fp := fingerprint(roots)
			if fp == last {
				continue
			}
			fmt.Fprintln(stderr, "glyph watch: change detected, re-running…")
			runOnce()
			// Recompute after the run so artifact writes (which are excluded
			// anyway) or editor save races don't immediately re-trigger.
			last = fingerprint(roots)
		}
	}
}

// watchRoots is the deduplicated set of directories to watch: the directory of
// each spec file plus any explicit --watch-path entries.
func watchRoots(specPaths, extraPaths []string) []string {
	paths := make([]string, 0, len(specPaths)+len(extraPaths))
	for _, s := range specPaths {
		paths = append(paths, filepath.Dir(s))
	}
	paths = append(paths, extraPaths...)
	return watchfs.Roots(paths...)
}

// fingerprint is the shared polling change detector (see internal/watchfs).
func fingerprint(roots []string) uint64 {
	return watchfs.Fingerprint(roots)
}

// renderWatchResults formats a watch iteration's results the same way a normal
// `glyph run` does (single result or batch summary), with color applied.
func renderWatchResults(cmd *cobra.Command, opts *globalOptions, results []artifacts.RunResult) string {
	var value any
	var markdown func() string
	if len(results) == 1 {
		value = results[0]
		markdown = func() string { return artifacts.RenderRunMarkdown(results[0]) }
	} else {
		value = map[string]any{"schemaVersion": 1, "results": results}
		markdown = func() string {
			md := "# Glyphrun Batch\n\n## Results\n\n"
			for _, result := range results {
				mark := "PASS"
				if result.Status != artifacts.StatusPassed {
					mark = "FAIL"
				}
				md += "- " + mark + " " + result.SpecName + ": " + string(result.Status) + " `" + result.RunDir + "`\n"
			}
			return md
		}
	}
	out, err := emitForCLI(cmd, opts, formatMD, value, markdown)
	if err != nil {
		return err.Error() + "\n"
	}
	return out
}
