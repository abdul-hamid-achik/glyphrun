package cli

import (
	"context"
	"fmt"
	"hash"
	"hash/fnv"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/spf13/cobra"
)

const watchPollInterval = 400 * time.Millisecond

// watchExcludedDirs are directory names skipped when fingerprinting a watch
// root, so the watcher does not trip on its own artifact output or VCS churn.
var watchExcludedDirs = map[string]bool{
	".glyphrun":    true,
	".git":         true,
	"node_modules": true,
	"vendor":       true,
}

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
		results, _, runErr := runSpecs(ctx, specPaths, parallel, opts, updateSnapshots, listener)
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
	seen := map[string]bool{}
	var roots []string
	add := func(p string) {
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			return
		}
		seen[abs] = true
		roots = append(roots, abs)
	}
	for _, s := range specPaths {
		add(filepath.Dir(s))
	}
	for _, p := range extraPaths {
		add(p)
	}
	sort.Strings(roots)
	return roots
}

// fingerprint folds the size and modification time of every non-excluded file
// under the watch roots into a single hash. A change to any watched file
// changes the hash, which is the signal to re-run.
func fingerprint(roots []string) uint64 {
	h := fnv.New64a()
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			writeFileFingerprint(h, root, info)
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if watchExcludedDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if fi, err := d.Info(); err == nil {
				writeFileFingerprint(h, path, fi)
			}
			return nil
		})
	}
	return h.Sum64()
}

func writeFileFingerprint(h hash.Hash64, path string, info os.FileInfo) {
	_, _ = h.Write([]byte(path))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.FormatInt(info.Size(), 10)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.FormatInt(info.ModTime().UnixNano(), 10)))
	_, _ = h.Write([]byte{0})
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
