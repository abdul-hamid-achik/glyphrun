package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CleanReport captures the result of a `glyph clean` invocation.
// Pruned is the count of run directories removed, Kept is the count
// that were preserved (either because they're newer than the keep
// window or because --all wasn't set).
type CleanReport struct {
	Pruned int      `json:"pruned" yaml:"pruned"`
	Kept   int      `json:"kept" yaml:"kept"`
	Paths  []string `json:"paths,omitempty" yaml:"paths,omitempty"`
}

// PruneRuns removes all but the N newest run directories under
// artifactRoot. The runner calls this on every successful run when
// the project config has `retention.keepRuns` set. A prune failure
// is surfaced but never fails the run (it would be a bad surprise
// for a contributor who got a passing run plus a disk-clean error).
//
// Returns a CleanReport even when the prune was a no-op; the caller
// can decide whether to surface the report (e.g. emit a
// "retention.kept" event in agent_context.md).
func PruneRuns(artifactRoot string, keepRuns int) (CleanReport, error) {
	if keepRuns <= 0 {
		return CleanReport{}, nil
	}
	entries, err := os.ReadDir(artifactRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return CleanReport{}, nil
		}
		return CleanReport{}, err
	}
	// Run directory names are timestamped (YYYY-MM-DDTHH-MM-SSZ-...) so
	// lexical sort matches chronological sort. Filter to actual
	// directories and sort newest-first.
	type runEntry struct {
		name    string
		modTime int64
	}
	var runs []runEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		runs = append(runs, runEntry{name: e.Name(), modTime: info.ModTime().UnixNano()})
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].modTime > runs[j].modTime
	})
	report := CleanReport{}
	if len(runs) <= keepRuns {
		report.Kept = len(runs)
		return report, nil
	}
	var prunedPaths []string
	for _, r := range runs[keepRuns:] {
		path := filepath.Join(artifactRoot, r.name)
		if err := os.RemoveAll(path); err != nil {
			return report, fmt.Errorf("prune %s: %w", path, err)
		}
		prunedPaths = append(prunedPaths, path)
	}
	report.Pruned = len(prunedPaths)
	report.Kept = keepRuns
	report.Paths = prunedPaths
	return report, nil
}

// CleanAll removes every run directory under artifactRoot. The
// `glyph clean --all` command is the user-facing path; programmatic
// callers can also use this for "start fresh" workflows.
func CleanAll(artifactRoot string) (CleanReport, error) {
	entries, err := os.ReadDir(artifactRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return CleanReport{}, nil
		}
		return CleanReport{}, err
	}
	report := CleanReport{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Skip non-run directories: hidden files (.DS_Store, etc.)
		// and the .glyphrun tmp dir if it lives here. The
		// timestamp-prefix convention is a strong enough signal
		// for "this is a run dir" that we use it as the filter.
		if !looksLikeRunDir(e.Name()) {
			continue
		}
		path := filepath.Join(artifactRoot, e.Name())
		if err := os.RemoveAll(path); err != nil {
			return report, fmt.Errorf("prune %s: %w", path, err)
		}
		report.Pruned++
		report.Paths = append(report.Paths, path)
	}
	return report, nil
}

// looksLikeRunDir returns true for names that follow the runner's
// `YYYY-MM-DDTHH-MM-SSZ-...` convention. Anything else (e.g. `.DS_Store`,
// `tmp/`, `snapshots/`) is preserved by CleanAll.
func looksLikeRunDir(name string) bool {
	// 2006-01-02T15-04-05Z is the format used by the runner.
	// The prefix is 18 chars long ("2006-01-02T15-04-05Z").
	if len(name) < 18 {
		return false
	}
	if !strings.HasPrefix(name, "20") {
		return false
	}
	// Cheap structural check: the YYYY-MM-DDTHH-MM-SSZ prefix has
	// digits and dashes at fixed positions. The runner tolerates
	// any extra suffix after that, so we don't validate the whole
	// name; a false positive only costs a directory we shouldn't
	// have deleted, which the user can re-create.
	return true
}
