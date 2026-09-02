package artifacts

import (
	"path/filepath"
	"strings"
)

// SanitizeRunName maps a spec or snapshot name to the lowercase, filesystem
// safe form used for run ids, committed golden directories, and the stories
// index. Every consumer of those paths must go through this one function so
// the runner and the catalog agree on where a golden lives.
func SanitizeRunName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "run"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// CommittedSnapshotPath is the golden text file for a spec's named snapshot:
// <snapshotRoot>/<spec>/<name>.txt, with the JSON cell grid as the .json
// sibling. A relative snapshotRoot resolves under projectRoot.
func CommittedSnapshotPath(projectRoot, snapshotRoot, specName, snapshotName string) string {
	root := snapshotRoot
	if root == "" {
		root = ".glyphrun/snapshots"
	}
	if !filepath.IsAbs(root) && projectRoot != "" {
		root = filepath.Join(projectRoot, root)
	}
	return filepath.Join(root, SanitizeRunName(specName), SanitizeRunName(snapshotName)+".txt")
}
