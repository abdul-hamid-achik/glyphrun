package artifacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RunDiff struct {
	SchemaVersion      int           `json:"schemaVersion" yaml:"schemaVersion"`
	RunA               string        `json:"runA" yaml:"runA"`
	RunB               string        `json:"runB" yaml:"runB"`
	Changed            bool          `json:"changed" yaml:"changed"`
	StatusA            RunStatus     `json:"statusA" yaml:"statusA"`
	StatusB            RunStatus     `json:"statusB" yaml:"statusB"`
	StatusChanged      bool          `json:"statusChanged" yaml:"statusChanged"`
	OutcomeDiffs       []OutcomeDiff `json:"outcomeDiffs" yaml:"outcomeDiffs"`
	FinalScreenChanged bool          `json:"finalScreenChanged" yaml:"finalScreenChanged"`
	FinalScreenDiff    string        `json:"finalScreenDiff,omitempty" yaml:"finalScreenDiff,omitempty"`
}

type OutcomeDiff struct {
	ID       string        `json:"id" yaml:"id"`
	Change   string        `json:"change" yaml:"change"`
	StatusA  OutcomeStatus `json:"statusA,omitempty" yaml:"statusA,omitempty"`
	StatusB  OutcomeStatus `json:"statusB,omitempty" yaml:"statusB,omitempty"`
	MessageA string        `json:"messageA,omitempty" yaml:"messageA,omitempty"`
	MessageB string        `json:"messageB,omitempty" yaml:"messageB,omitempty"`
}

func LoadRunResult(runDir string) (RunResult, error) {
	data, err := os.ReadFile(filepath.Join(runDir, "run.json"))
	if err != nil {
		return RunResult{}, err
	}
	var result RunResult
	if err := json.Unmarshal(data, &result); err != nil {
		return RunResult{}, err
	}
	return result, nil
}

func DiffRunDirs(runADir string, runBDir string) (RunDiff, error) {
	a, err := LoadRunResult(runADir)
	if err != nil {
		return RunDiff{}, fmt.Errorf("load run A: %w", err)
	}
	b, err := LoadRunResult(runBDir)
	if err != nil {
		return RunDiff{}, fmt.Errorf("load run B: %w", err)
	}
	screenA, _ := os.ReadFile(filepath.Join(runADir, artifactPath(a, "finalScreenText", "screens/final.txt")))
	screenB, _ := os.ReadFile(filepath.Join(runBDir, artifactPath(b, "finalScreenText", "screens/final.txt")))
	diff := RunDiff{
		SchemaVersion:      1,
		RunA:               a.RunID,
		RunB:               b.RunID,
		StatusA:            a.Status,
		StatusB:            b.Status,
		StatusChanged:      a.Status != b.Status,
		OutcomeDiffs:       ensureOutcomeDiffs(diffOutcomes(a.Outcomes, b.Outcomes)),
		FinalScreenChanged: string(screenA) != string(screenB),
	}
	if diff.FinalScreenChanged {
		diff.FinalScreenDiff = lineDiff(string(screenA), string(screenB))
	}
	diff.Changed = diff.StatusChanged || len(diff.OutcomeDiffs) > 0 || diff.FinalScreenChanged
	return diff, nil
}

func ensureOutcomeDiffs(diffs []OutcomeDiff) []OutcomeDiff {
	if diffs == nil {
		return []OutcomeDiff{}
	}
	return diffs
}

func RenderRunDiffMarkdown(diff RunDiff) string {
	var b strings.Builder
	b.WriteString("# Glyphrun Diff\n\n")
	fmt.Fprintf(&b, "- run A: %s\n", diff.RunA)
	fmt.Fprintf(&b, "- run B: %s\n", diff.RunB)
	fmt.Fprintf(&b, "- changed: %v\n", diff.Changed)
	if diff.StatusChanged {
		fmt.Fprintf(&b, "- status: %s -> %s\n", diff.StatusA, diff.StatusB)
	}
	if len(diff.OutcomeDiffs) > 0 {
		b.WriteString("\n## Outcomes\n\n")
		for _, item := range diff.OutcomeDiffs {
			fmt.Fprintf(&b, "- %s %s", item.Change, item.ID)
			if item.StatusA != "" || item.StatusB != "" {
				fmt.Fprintf(&b, ": %s -> %s", item.StatusA, item.StatusB)
			}
			if item.MessageA != item.MessageB {
				fmt.Fprintf(&b, " (%q -> %q)", item.MessageA, item.MessageB)
			}
			b.WriteByte('\n')
		}
	}
	if diff.FinalScreenChanged {
		b.WriteString("\n## Final Screen\n\n```diff\n")
		b.WriteString(diff.FinalScreenDiff)
		b.WriteString("\n```\n")
	}
	return b.String()
}

func artifactPath(result RunResult, key string, fallback string) string {
	if result.Artifacts != nil && result.Artifacts[key] != "" {
		return result.Artifacts[key]
	}
	return fallback
}

func diffOutcomes(a []OutcomeResult, b []OutcomeResult) []OutcomeDiff {
	aByID := map[string]OutcomeResult{}
	bByID := map[string]OutcomeResult{}
	ids := map[string]bool{}
	for _, outcome := range a {
		aByID[outcome.ID] = outcome
		ids[outcome.ID] = true
	}
	for _, outcome := range b {
		bByID[outcome.ID] = outcome
		ids[outcome.ID] = true
	}
	var diffs []OutcomeDiff
	for id := range ids {
		left, leftOK := aByID[id]
		right, rightOK := bByID[id]
		switch {
		case !leftOK:
			diffs = append(diffs, OutcomeDiff{ID: id, Change: "added", StatusB: right.Status, MessageB: right.Message})
		case !rightOK:
			diffs = append(diffs, OutcomeDiff{ID: id, Change: "removed", StatusA: left.Status, MessageA: left.Message})
		case left.Status != right.Status || left.Message != right.Message:
			diffs = append(diffs, OutcomeDiff{
				ID:       id,
				Change:   "changed",
				StatusA:  left.Status,
				StatusB:  right.Status,
				MessageA: left.Message,
				MessageB: right.Message,
			})
		}
	}
	return diffs
}

func lineDiff(a string, b string) string {
	aLines := strings.Split(strings.TrimRight(a, "\n"), "\n")
	bLines := strings.Split(strings.TrimRight(b, "\n"), "\n")
	max := len(aLines)
	if len(bLines) > max {
		max = len(bLines)
	}
	var out strings.Builder
	for i := 0; i < max; i++ {
		var left, right string
		if i < len(aLines) {
			left = aLines[i]
		}
		if i < len(bLines) {
			right = bLines[i]
		}
		if left == right {
			out.WriteString("  ")
			out.WriteString(left)
			out.WriteByte('\n')
			continue
		}
		if i < len(aLines) {
			out.WriteString("- ")
			out.WriteString(left)
			out.WriteByte('\n')
		}
		if i < len(bLines) {
			out.WriteString("+ ")
			out.WriteString(right)
			out.WriteByte('\n')
		}
	}
	return strings.TrimRight(out.String(), "\n")
}
