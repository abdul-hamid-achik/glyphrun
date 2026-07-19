package artifacts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LastFailedFile is the legacy line-oriented name list kept for humans and
// older tooling. Prefer LastFailedJSON for path-aware reruns.
const LastFailedFile = ".last-failed.txt"

// LastFailedJSON is the structured failure index written next to LastFailedFile.
// Each entry records the spec name and the filesystem path so
// `glyph run --rerun-failed` can re-execute without a name→path lookup.
const LastFailedJSON = ".last-failed.json"

// FailedSpec is one non-passing spec recorded for --rerun-failed.
type FailedSpec struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

type lastFailedDocument struct {
	SchemaVersion int          `json:"schemaVersion"`
	Failed        []FailedSpec `json:"failed"`
}

// WriteLastFailed records failed specs at the artifact root. The previous
// list is replaced wholesale so a re-run of a previously-passing spec clears
// it. Both the JSON index (path-aware) and the legacy text file (names only)
// are written for compatibility.
func WriteLastFailed(artifactRoot string, entries []FailedSpec) error {
	if artifactRoot == "" {
		return nil
	}
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		return err
	}
	cleaned := normalizeFailedSpecs(entries)
	doc := lastFailedDocument{SchemaVersion: 1, Failed: cleaned}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(artifactRoot, LastFailedJSON), data, 0o644); err != nil {
		return err
	}
	// Legacy text: names only, sorted, one per line.
	names := make([]string, 0, len(cleaned))
	for _, e := range cleaned {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	contents := strings.Join(names, "\n")
	if contents != "" {
		contents += "\n"
	}
	return os.WriteFile(filepath.Join(artifactRoot, LastFailedFile), []byte(contents), 0o644)
}

// ReadLastFailed returns failed specs from the artifact root. Prefers the
// JSON index; falls back to the legacy text file (names only, empty Path).
func ReadLastFailed(artifactRoot string) ([]FailedSpec, error) {
	if artifactRoot == "" {
		return nil, nil
	}
	jsonPath := filepath.Join(artifactRoot, LastFailedJSON)
	if data, err := os.ReadFile(jsonPath); err == nil {
		var doc lastFailedDocument
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		return normalizeFailedSpecs(doc.Failed), nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Legacy fallback.
	data, err := os.ReadFile(filepath.Join(artifactRoot, LastFailedFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var out []FailedSpec
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Support optional "name\tpath" lines if a human edited the text file.
		name, path, _ := strings.Cut(line, "\t")
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)
		if name == "" {
			continue
		}
		out = append(out, FailedSpec{Name: name, Path: path})
	}
	return normalizeFailedSpecs(out), nil
}

func normalizeFailedSpecs(entries []FailedSpec) []FailedSpec {
	// Dedupe by path when present, else by name. Prefer the entry that has a path.
	byKey := map[string]FailedSpec{}
	order := make([]string, 0, len(entries))
	for _, e := range entries {
		name := strings.TrimSpace(e.Name)
		path := strings.TrimSpace(e.Path)
		if name == "" && path == "" {
			continue
		}
		if name == "" {
			name = filepath.Base(path)
		}
		key := path
		if key == "" {
			key = "name:" + name
		}
		if prev, ok := byKey[key]; ok {
			if prev.Path == "" && path != "" {
				byKey[key] = FailedSpec{Name: name, Path: path}
			}
			continue
		}
		byKey[key] = FailedSpec{Name: name, Path: path}
		order = append(order, key)
	}
	// Stable sort by name then path for readable diffs.
	sort.SliceStable(order, func(i, j int) bool {
		a, b := byKey[order[i]], byKey[order[j]]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Path < b.Path
	})
	out := make([]FailedSpec, 0, len(order))
	for _, k := range order {
		out = append(out, byKey[k])
	}
	return out
}

// FailedPaths returns filesystem paths suitable for `glyph run`, skipping
// entries that only have a name (legacy records without path).
func FailedPaths(entries []FailedSpec) []string {
	var paths []string
	seen := map[string]bool{}
	for _, e := range entries {
		p := strings.TrimSpace(e.Path)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	return paths
}
