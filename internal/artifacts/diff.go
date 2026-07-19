package artifacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RunDiff struct {
	SchemaVersion       int           `json:"schemaVersion" yaml:"schemaVersion"`
	RunA                string        `json:"runA" yaml:"runA"`
	RunB                string        `json:"runB" yaml:"runB"`
	Changed             bool          `json:"changed" yaml:"changed"`
	StatusA             RunStatus     `json:"statusA" yaml:"statusA"`
	StatusB             RunStatus     `json:"statusB" yaml:"statusB"`
	StatusChanged       bool          `json:"statusChanged" yaml:"statusChanged"`
	ErrorKindA          ErrorKind     `json:"errorKindA,omitempty" yaml:"errorKindA,omitempty"`
	ErrorKindB          ErrorKind     `json:"errorKindB,omitempty" yaml:"errorKindB,omitempty"`
	ErrorKindChanged    bool          `json:"errorKindChanged" yaml:"errorKindChanged"`
	ContractHashA       string        `json:"contractHashA,omitempty" yaml:"contractHashA,omitempty"`
	ContractHashB       string        `json:"contractHashB,omitempty" yaml:"contractHashB,omitempty"`
	ContractHashChanged bool          `json:"contractHashChanged" yaml:"contractHashChanged"`
	DurationMSA         int64         `json:"durationMsA" yaml:"durationMsA"`
	DurationMSB         int64         `json:"durationMsB" yaml:"durationMsB"`
	OutcomeDiffs        []OutcomeDiff `json:"outcomeDiffs" yaml:"outcomeDiffs"`
	StepDiffs           []StepDiff    `json:"stepDiffs,omitempty" yaml:"stepDiffs,omitempty"`
	NamedArtifactDiffs  []string      `json:"namedArtifactDiffs,omitempty" yaml:"namedArtifactDiffs,omitempty"`
	FinalScreenChanged  bool          `json:"finalScreenChanged" yaml:"finalScreenChanged"`
	FinalScreenDiff     string        `json:"finalScreenDiff,omitempty" yaml:"finalScreenDiff,omitempty"`
}

type OutcomeDiff struct {
	ID       string        `json:"id" yaml:"id"`
	Change   string        `json:"change" yaml:"change"`
	StatusA  OutcomeStatus `json:"statusA,omitempty" yaml:"statusA,omitempty"`
	StatusB  OutcomeStatus `json:"statusB,omitempty" yaml:"statusB,omitempty"`
	MessageA string        `json:"messageA,omitempty" yaml:"messageA,omitempty"`
	MessageB string        `json:"messageB,omitempty" yaml:"messageB,omitempty"`
}

// StepDiff records a change in step execution status between two runs.
type StepDiff struct {
	Index   int    `json:"index" yaml:"index"`
	IDA     string `json:"idA,omitempty" yaml:"idA,omitempty"`
	IDB     string `json:"idB,omitempty" yaml:"idB,omitempty"`
	Kind    string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Change  string `json:"change" yaml:"change"` // added|removed|changed
	StatusA string `json:"statusA,omitempty" yaml:"statusA,omitempty"`
	StatusB string `json:"statusB,omitempty" yaml:"statusB,omitempty"`
	ErrorA  string `json:"errorA,omitempty" yaml:"errorA,omitempty"`
	ErrorB  string `json:"errorB,omitempty" yaml:"errorB,omitempty"`
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
		SchemaVersion:       1,
		RunA:                a.RunID,
		RunB:                b.RunID,
		StatusA:             a.Status,
		StatusB:             b.Status,
		StatusChanged:       a.Status != b.Status,
		ErrorKindA:          a.ErrorKind,
		ErrorKindB:          b.ErrorKind,
		ErrorKindChanged:    a.ErrorKind != b.ErrorKind,
		ContractHashA:       a.ContractHash,
		ContractHashB:       b.ContractHash,
		ContractHashChanged: a.ContractHash != b.ContractHash,
		DurationMSA:         a.DurationMS,
		DurationMSB:         b.DurationMS,
		OutcomeDiffs:        ensureOutcomeDiffs(diffOutcomes(a.Outcomes, b.Outcomes)),
		StepDiffs:           diffSteps(a.Steps, b.Steps),
		NamedArtifactDiffs:  diffNamedArtifacts(a.NamedArtifacts, b.NamedArtifacts),
		FinalScreenChanged:  string(screenA) != string(screenB),
	}
	if diff.FinalScreenChanged {
		diff.FinalScreenDiff = lineDiff(string(screenA), string(screenB))
	}
	diff.Changed = diff.StatusChanged ||
		diff.ErrorKindChanged ||
		diff.ContractHashChanged ||
		len(diff.OutcomeDiffs) > 0 ||
		len(diff.StepDiffs) > 0 ||
		len(diff.NamedArtifactDiffs) > 0 ||
		diff.FinalScreenChanged
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
	if diff.ErrorKindChanged {
		fmt.Fprintf(&b, "- errorKind: %s -> %s\n", diff.ErrorKindA, diff.ErrorKindB)
	}
	if diff.ContractHashChanged {
		fmt.Fprintf(&b, "- contractHash: %s -> %s\n", diff.ContractHashA, diff.ContractHashB)
	}
	if diff.DurationMSA != diff.DurationMSB {
		fmt.Fprintf(&b, "- durationMs: %d -> %d\n", diff.DurationMSA, diff.DurationMSB)
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
	if len(diff.StepDiffs) > 0 {
		b.WriteString("\n## Steps\n\n")
		for _, item := range diff.StepDiffs {
			label := fmt.Sprintf("step %d", item.Index)
			if item.IDA != "" {
				label = item.IDA
			} else if item.IDB != "" {
				label = item.IDB
			}
			fmt.Fprintf(&b, "- %s %s (%s): %s -> %s\n", item.Change, label, item.Kind, item.StatusA, item.StatusB)
		}
	}
	if len(diff.NamedArtifactDiffs) > 0 {
		b.WriteString("\n## Named Artifacts\n\n")
		for _, line := range diff.NamedArtifactDiffs {
			fmt.Fprintf(&b, "- %s\n", line)
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

func diffSteps(a, b []StepResult) []StepDiff {
	max := len(a)
	if len(b) > max {
		max = len(b)
	}
	var diffs []StepDiff
	for i := 0; i < max; i++ {
		var left, right StepResult
		leftOK, rightOK := i < len(a), i < len(b)
		if leftOK {
			left = a[i]
		}
		if rightOK {
			right = b[i]
		}
		switch {
		case !leftOK:
			diffs = append(diffs, StepDiff{
				Index: right.Index, IDB: right.ID, Kind: right.Kind,
				Change: "added", StatusB: right.Status, ErrorB: right.Error,
			})
		case !rightOK:
			diffs = append(diffs, StepDiff{
				Index: left.Index, IDA: left.ID, Kind: left.Kind,
				Change: "removed", StatusA: left.Status, ErrorA: left.Error,
			})
		case left.Status != right.Status || left.Error != right.Error || left.Kind != right.Kind:
			diffs = append(diffs, StepDiff{
				Index: left.Index, IDA: left.ID, IDB: right.ID, Kind: right.Kind,
				Change: "changed", StatusA: left.Status, StatusB: right.Status,
				ErrorA: left.Error, ErrorB: right.Error,
			})
		}
	}
	return diffs
}

func diffNamedArtifacts(a, b map[string]NamedArtifact) []string {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	var lines []string
	for k := range keys {
		left, leftOK := a[k]
		right, rightOK := b[k]
		switch {
		case !leftOK:
			lines = append(lines, fmt.Sprintf("added %s (%s)", k, right.RelativePath))
		case !rightOK:
			lines = append(lines, fmt.Sprintf("removed %s (%s)", k, left.RelativePath))
		case left.RelativePath != right.RelativePath || left.Kind != right.Kind:
			lines = append(lines, fmt.Sprintf("changed %s: %s -> %s", k, left.RelativePath, right.RelativePath))
		}
	}
	return lines
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
