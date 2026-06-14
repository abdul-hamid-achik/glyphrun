// Package repair analyzes a failed run and proposes fixes to a spec's steps.
//
// It exists as its own package so both the CLI and the MCP server can reuse it
// without business logic leaking into the thin command handlers. The cardinal
// rule: repair only ever touches `steps` (the repairable navigation hints),
// never `intent` or `outcomes` (the contract). Applying a repair therefore
// keeps a spec's stamped contract hash valid.
package repair

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"gopkg.in/yaml.v3"
)

// Proposal is a single suggested fix to a spec's steps.
type Proposal struct {
	StepIndex int    `json:"stepIndex" yaml:"stepIndex"` // 1-based, matches run events
	Kind      string `json:"kind" yaml:"kind"`
	Current   string `json:"current,omitempty" yaml:"current,omitempty"`
	Proposed  string `json:"proposed,omitempty" yaml:"proposed,omitempty"`
	Rationale string `json:"rationale" yaml:"rationale"`
	Applied   bool   `json:"applied" yaml:"applied"`
}

// Analyze reads a run's events and final screen and proposes step repairs. The
// steps argument is the spec's resolved steps (imports/use already expanded),
// so indices line up with the run's step events.
func Analyze(runDir string, steps []spec.Step) []Proposal {
	failures := readStepFailures(runDir)
	finalScreen := readRunFile(runDir, "screens/final.txt")
	return propose(steps, failures, finalScreen)
}

// Apply rewrites steps[p.StepIndex-1].wait.screen.contains in the spec file via
// surgical YAML node editing, leaving every other node — including the
// contract-hashed intent/outcomes — untouched.
func Apply(specPath string, p Proposal) error {
	if p.Proposed == "" || p.Current == "" {
		return fmt.Errorf("proposal for step %d has nothing to apply", p.StepIndex)
	}
	return editStepWaitContains(specPath, p.StepIndex, p.Proposed)
}

// LatestRunDirForSpec returns the newest run directory whose run.json names the
// given spec, so repair defaults to that spec's last run rather than the newest
// run of any spec.
func LatestRunDirForSpec(root, specName string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	// Run dir names are timestamp-prefixed, so reverse-lexical is newest-first.
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names {
		dir := filepath.Join(root, name)
		result, err := artifacts.LoadRunResult(dir)
		if err != nil {
			continue
		}
		if result.SpecName == specName {
			return dir, nil
		}
	}
	return "", os.ErrNotExist
}

// stepFailure is a failed step pulled from a run's events.
type stepFailure struct {
	index   int // 1-based
	message string
}

func readStepFailures(runDir string) []stepFailure {
	f, err := os.Open(filepath.Join(runDir, "events.ndjson"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []stepFailure
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e artifacts.Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if e.Type != "step.failed" {
			continue
		}
		if idx := stepIndexFromName(e.Name); idx > 0 {
			out = append(out, stepFailure{index: idx, message: e.Info})
		}
	}
	return out
}

// stepIndexFromName parses the 1-based index from an event name like "step.3".
func stepIndexFromName(name string) int {
	_, num, ok := strings.Cut(name, ".")
	if !ok {
		return 0
	}
	idx, err := strconv.Atoi(num)
	if err != nil {
		return 0
	}
	return idx
}

// propose builds repair suggestions for each failed `wait: screen: contains:`
// step whose needle is missing from the final screen. The fix is the closest
// line actually on screen — the common "the ready string changed" case.
func propose(steps []spec.Step, failures []stepFailure, finalScreen string) []Proposal {
	var proposals []Proposal
	for _, fail := range failures {
		if fail.index < 1 || fail.index > len(steps) {
			continue
		}
		step := steps[fail.index-1]
		if step.Wait == nil || step.Wait.Screen == nil {
			if step.Wait != nil && step.Wait.Process != nil {
				proposals = append(proposals, Proposal{
					StepIndex: fail.index,
					Kind:      "wait.process",
					Rationale: "the process did not reach this exit state in time; you may be missing an interaction step (e.g. a quit key) before this wait",
				})
			}
			continue
		}
		needle := step.Wait.Screen.Contains
		if needle == "" {
			continue
		}
		if strings.Contains(finalScreen, needle) {
			proposals = append(proposals, Proposal{
				StepIndex: fail.index,
				Kind:      "wait.screen.contains",
				Current:   needle,
				Rationale: "the expected text is present on the final screen; the wait may need a longer timeoutMs or an earlier step is out of order",
			})
			continue
		}
		best := closestScreenLine(needle, finalScreen)
		proposals = append(proposals, Proposal{
			StepIndex: fail.index,
			Kind:      "wait.screen.contains",
			Current:   needle,
			Proposed:  best,
			Rationale: fmt.Sprintf("%q is not on the final screen; the closest on-screen text is %q", needle, best),
		})
	}
	return proposals
}

// closestScreenLine returns the on-screen line most similar to needle (scored
// by longest common substring), falling back to the most prominent line.
func closestScreenLine(needle, finalScreen string) string {
	needleLower := strings.ToLower(needle)
	best := ""
	bestScore := 0
	for _, raw := range strings.Split(finalScreen, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if score := longestCommonSubstring(needleLower, strings.ToLower(line)); score > bestScore {
			bestScore = score
			best = line
		}
	}
	if bestScore >= 3 {
		return capLine(best)
	}
	return prominentLine(finalScreen)
}

// prominentLine returns the first non-trivial, letter-bearing line of a screen.
func prominentLine(finalScreen string) string {
	for _, raw := range strings.Split(finalScreen, "\n") {
		line := strings.TrimSpace(raw)
		if len([]rune(line)) < 3 {
			continue
		}
		if strings.ContainsFunc(line, unicode.IsLetter) {
			return capLine(line)
		}
	}
	return ""
}

func capLine(line string) string {
	runes := []rune(line)
	if len(runes) > 50 {
		return strings.TrimSpace(string(runes[:50]))
	}
	return line
}

// longestCommonSubstring returns the length of the longest substring common to
// a and b. Lines are short, so the O(n*m) table is fine.
func longestCommonSubstring(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	best := 0
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			if ra[i-1] == rb[j-1] {
				curr[j] = prev[j-1] + 1
				if curr[j] > best {
					best = curr[j]
				}
			} else {
				curr[j] = 0
			}
		}
		prev, curr = curr, prev
		for j := range curr {
			curr[j] = 0
		}
	}
	return best
}

func readRunFile(runDir, rel string) string {
	data, err := os.ReadFile(filepath.Join(runDir, rel))
	if err != nil {
		return ""
	}
	return string(data)
}

func editStepWaitContains(specPath string, index int, newValue string) error {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("spec is not a mapping")
	}
	steps := mappingValue(doc.Content[0], "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return fmt.Errorf("spec has no steps sequence")
	}
	if index < 1 || index > len(steps.Content) {
		return fmt.Errorf("step index %d out of range", index)
	}
	wait := mappingValue(steps.Content[index-1], "wait")
	if wait == nil {
		return fmt.Errorf("step %d is not a wait", index)
	}
	screen := mappingValue(wait, "screen")
	if screen == nil {
		return fmt.Errorf("step %d wait has no screen condition", index)
	}
	contains := mappingValue(screen, "contains")
	if contains == nil {
		return fmt.Errorf("step %d wait.screen has no contains", index)
	}
	contains.Value = newValue
	contains.Tag = "!!str"
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(specPath, out, 0o644)
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
