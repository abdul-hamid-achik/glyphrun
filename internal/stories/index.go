package stories

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

// IndexEntry is the newest result of one story, persisted under the stories
// root together with a copy of its captured screens. The index is what
// `glyph stories` joins against first: run directories are subject to
// retention pruning, the index is not, so a catalog of fifty stories does not
// collapse to "not_run" after `retention.keepRuns` kicks in.
type IndexEntry struct {
	SchemaVersion int      `json:"schemaVersion"`
	Key           string   `json:"key"`
	ID            string   `json:"id,omitempty"`
	Variant       string   `json:"variant,omitempty"`
	Feature       string   `json:"feature,omitempty"`
	SpecName      string   `json:"specName"`
	Source        string   `json:"source"`
	SourcePath    string   `json:"sourcePath,omitempty"`
	Intent        string   `json:"intent,omitempty"`
	Owner         string   `json:"owner,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	RunID         string   `json:"runId"`
	RunDir        string   `json:"runDir,omitempty"`
	Status        string   `json:"status"`
	ErrorKind     string   `json:"errorKind,omitempty"`
	Diagnostic    string   `json:"diagnostic,omitempty"`
	StartedAt     string   `json:"startedAt,omitempty"`
	EndedAt       string   `json:"endedAt,omitempty"`
	DurationMS    int64    `json:"durationMs"`
	// Terminal is the size the story ran at (variants change it).
	Terminal spec.Terminal `json:"terminal"`
	// GoldenName is the snapshot the `snapshot` verifier compared (if any).
	GoldenName string                    `json:"goldenName,omitempty"`
	Outcomes   []artifacts.OutcomeResult `json:"outcomes,omitempty"`
	// Screens lists the screen files copied into the entry directory:
	// "final" plus every snapshot captured during the run.
	Screens []string `json:"screens"`
}

// IndexFile is the per-story metadata file name inside the stories root.
const IndexFile = "latest.json"

// IndexDir is where a story's entry lives under the stories root.
func IndexDir(root, specName string) string {
	return filepath.Join(root, artifacts.SanitizeRunName(specName))
}

// WriteIndexEntry stores the entry and copies the run's final screen and
// named snapshots (JSON cell grids) next to it. It replaces any previous
// entry for the same story atomically enough for a local tool: metadata is
// written last, so a crash mid-copy leaves the old latest.json in place.
func WriteIndexEntry(root string, entry IndexEntry, runDir string) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("stories root is empty")
	}
	dir := IndexDir(root, entry.SpecName)
	if err := os.MkdirAll(filepath.Join(dir, "screens"), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(dir, "snapshots")); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "snapshots"), 0o755); err != nil {
		return err
	}
	entry.SchemaVersion = 1
	entry.Screens = nil
	if runDir != "" {
		if copyFile(filepath.Join(runDir, "screens", "final.json"), filepath.Join(dir, "screens", "final.json")) == nil {
			entry.Screens = append(entry.Screens, "final")
		}
		snaps, _ := os.ReadDir(filepath.Join(runDir, "snapshots"))
		names := make([]string, 0, len(snaps))
		for _, s := range snaps {
			if s.IsDir() || !strings.HasSuffix(s.Name(), ".json") {
				continue
			}
			names = append(names, s.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			if copyFile(filepath.Join(runDir, "snapshots", name), filepath.Join(dir, "snapshots", name)) == nil {
				entry.Screens = append(entry.Screens, strings.TrimSuffix(name, ".json"))
			}
		}
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, IndexFile+".tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, IndexFile))
}

// ReadIndex loads every entry under the stories root keyed by spec name.
// A missing root is an empty index, not an error.
func ReadIndex(root string) map[string]IndexEntry {
	out := map[string]IndexEntry{}
	if strings.TrimSpace(root) == "" {
		return out
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, e.Name(), IndexFile))
		if err != nil {
			continue
		}
		var entry IndexEntry
		if err := json.Unmarshal(data, &entry); err != nil || entry.SpecName == "" {
			continue
		}
		out[entry.SpecName] = entry
	}
	return out
}

// EntryFromRun builds the index entry for a finished run of a story.
func EntryFromRun(key, id, variant, feature, source, sourcePath, goldenName string, result artifacts.RunResult) IndexEntry {
	entry := IndexEntry{
		Key:        key,
		ID:         id,
		Variant:    variant,
		Feature:    feature,
		SpecName:   result.SpecName,
		Source:     source,
		SourcePath: sourcePath,
		Intent:     strings.TrimSpace(result.Intent),
		RunID:      result.RunID,
		RunDir:     result.RunDir,
		Status:     string(result.Status),
		ErrorKind:  string(result.ErrorKind),
		Diagnostic: result.Diagnostic,
		StartedAt:  result.StartedAt,
		EndedAt:    result.EndedAt,
		DurationMS: result.DurationMS,
		Terminal:   result.Terminal,
		GoldenName: goldenName,
		Outcomes:   result.Outcomes,
	}
	if result.Metadata != nil {
		entry.Owner = result.Metadata.Owner
		entry.Tags = append([]string(nil), result.Metadata.Tags...)
		if entry.Feature == "" {
			entry.Feature = result.Metadata.Feature
		}
	}
	return entry
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
