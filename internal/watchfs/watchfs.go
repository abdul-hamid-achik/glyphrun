// Package watchfs is the polling change detector shared by `glyph run
// --watch` and `glyph stories run --watch`. It folds file sizes and
// modification times under a set of roots into one hash; a changed hash is the
// signal to re-run. Polling keeps the binary free of a file-notification
// dependency and behaves the same on every platform.
package watchfs

import (
	"hash"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// PollInterval is how often callers should re-fingerprint.
const PollInterval = 400 * time.Millisecond

// ExcludedDirs are directory names skipped while fingerprinting so the
// watcher does not trip on its own artifact output or VCS churn.
var ExcludedDirs = map[string]bool{
	".glyphrun":    true,
	".git":         true,
	"node_modules": true,
	"vendor":       true,
}

// Roots returns the deduplicated, sorted absolute paths to watch. Files are
// watched as-is; a spec file's parent directory is a common choice.
func Roots(paths ...string) []string {
	seen := map[string]bool{}
	var roots []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		roots = append(roots, abs)
	}
	sort.Strings(roots)
	return roots
}

// Fingerprint folds the size and modification time of every non-excluded
// file under the roots into a single hash.
func Fingerprint(roots []string) uint64 {
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
				if ExcludedDirs[d.Name()] {
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
