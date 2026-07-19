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
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/runner"
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

// Analyze reads a run's events/step results and final screen and proposes step
// repairs. The steps argument is the spec's resolved steps (imports/use already
// expanded), so indices line up with the run's step events / StepResult[].
func Analyze(runDir string, steps []spec.Step) []Proposal {
	failures := readStepFailures(runDir)
	if len(failures) == 0 {
		failures = readStepFailuresFromRunJSON(runDir)
	}
	finalScreen := readRunFile(runDir, "screens/final.txt")
	return propose(steps, failures, finalScreen)
}

// Apply rewrites the proposed step field in the spec file via surgical YAML
// node editing, leaving every other node — including the contract-hashed
// intent/outcomes — untouched. Supported kinds: wait.screen.contains,
// wait.timeoutMs.
func Apply(specPath string, p Proposal) error {
	if p.Proposed == "" || p.Current == "" {
		return fmt.Errorf("proposal for step %d has nothing to apply", p.StepIndex)
	}
	switch p.Kind {
	case "wait.timeoutMs":
		return editStepWaitTimeoutMS(specPath, p.StepIndex, p.Proposed)
	case "wait.screen.contains", "":
		return editStepWaitContains(specPath, p.StepIndex, p.Proposed)
	default:
		return fmt.Errorf("proposal kind %q for step %d is not auto-applicable", p.Kind, p.StepIndex)
	}
}

// VerifyOptions configures a verified repair (SPEC §7.2).
type VerifyOptions struct {
	ConfigPath   string
	Environment  string
	ArtifactRoot string
}

// VerifyResult is the outcome of a verified transactional repair (SPEC §7.2).
// The repair is applied to a TEMP copy of the spec, the temp is cold-start
// rerun, and the repair is accepted only if the previously-failed step
// advances and all contract outcomes pass; otherwise the original spec is
// left untouched (rollback). The result carries the before/after run IDs,
// retained evidence (the after run dir), and the exact replay action.
type VerifyResult struct {
	SchemaVersion int        `json:"schemaVersion" yaml:"schemaVersion"` // 1
	Spec          string     `json:"spec" yaml:"spec"`
	BeforeRun     string     `json:"beforeRun" yaml:"beforeRun"`                   // runId of the failed run
	AfterRun      string     `json:"afterRun,omitempty" yaml:"afterRun,omitempty"` // runId of the verification rerun
	Proposals     []Proposal `json:"proposals" yaml:"proposals"`
	Verified      bool       `json:"verified" yaml:"verified"`
	Confidence    string     `json:"confidence" yaml:"confidence"` // high | low
	Reason        string     `json:"reason,omitempty" yaml:"reason,omitempty"`
	Evidence      string     `json:"evidence,omitempty" yaml:"evidence,omitempty"` // after run dir (retained)
	Replay        string     `json:"replay,omitempty" yaml:"replay,omitempty"`     // exact replay command
}

// Verify performs a transactional repair (SPEC §7.2): it applies the
// applicable proposals to a temp copy of the spec (in the spec's own dir so
// config/relative-file resolution is identical), cold-start reruns it, and
// accepts the repair only if the rerun passes (the failed step advances AND
// all contract outcomes pass). The original spec is never modified by Verify;
// the caller writes the accepted repair separately. The temp copy is always
// removed.
func Verify(ctx context.Context, specPath string, runDir string, proposals []Proposal, opts VerifyOptions) (VerifyResult, error) {
	before, _ := artifacts.LoadRunResult(runDir)
	result := VerifyResult{
		SchemaVersion: 1,
		BeforeRun:     before.RunID,
		Proposals:     proposals,
		Replay:        "glyph run " + specPath + " --format json",
	}
	// Only proposals with a concrete proposed value can be verified.
	applicable := make([]Proposal, 0, len(proposals))
	for _, p := range proposals {
		if p.Proposed != "" && p.Current != "" {
			applicable = append(applicable, p)
		}
	}
	if len(applicable) == 0 {
		result.Reason = "no applicable step repairs to verify (proposals are diagnostic only)"
		result.Confidence = "low"
		return result, nil
	}
	// Copy the spec to a sibling temp file so config/spec-dir resolution is
	// identical to the original run. Hidden name to avoid globs picking it up.
	data, err := os.ReadFile(specPath)
	if err != nil {
		return result, fmt.Errorf("read spec for verify: %w", err)
	}
	randSuffix := make([]byte, 4)
	_, _ = rand.Read(randSuffix)
	tempPath := filepath.Join(filepath.Dir(specPath), "."+strings.TrimSuffix(filepath.Base(specPath), filepath.Ext(specPath))+".repair-"+hex.EncodeToString(randSuffix)+filepath.Ext(specPath))
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return result, fmt.Errorf("write temp spec for verify: %w", err)
	}
	defer os.Remove(tempPath)
	for _, p := range applicable {
		if err := Apply(tempPath, p); err != nil {
			return result, fmt.Errorf("apply repair to temp spec step %d: %w", p.StepIndex, err)
		}
	}
	// Cold-start rerun the repaired temp spec. RunSpec always starts a fresh
	// target process, so this is a true cold start.
	after, runErr := runner.RunSpec(ctx, runner.Options{
		SpecPath:     tempPath,
		ConfigPath:   opts.ConfigPath,
		Environment:  opts.Environment,
		ArtifactRoot: opts.ArtifactRoot,
	})
	if runErr != nil {
		result.Confidence = "low"
		result.Reason = fmt.Sprintf("verification rerun errored: %v", runErr)
		return result, nil
	}
	result.AfterRun = after.RunID
	result.Evidence = after.RunDir
	if after.Status == artifacts.StatusPassed {
		result.Verified = true
		result.Confidence = "high"
	} else {
		result.Confidence = "low"
		reason := fmt.Sprintf("verification rerun %s", after.Status)
		if after.Diagnostic != "" {
			reason += ": " + after.Diagnostic
		}
		result.Reason = reason
	}
	return result, nil
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

// propose builds repair suggestions for failed wait steps. Heuristics:
//   - wait.screen.contains present at end → raise timeoutMs
//   - wait.screen.contains missing → replace with closest on-screen line
//   - wait.process failed → suggest a quit key if the screen shows one
//   - timed-out wait message → suggest higher timeoutMs
func propose(steps []spec.Step, failures []stepFailure, finalScreen string) []Proposal {
	var proposals []Proposal
	for _, fail := range failures {
		if fail.index < 1 || fail.index > len(steps) {
			continue
		}
		step := steps[fail.index-1]
		if step.Wait == nil || step.Wait.Screen == nil {
			if step.Wait != nil && step.Wait.Process != nil {
				quitKey := detectQuitKey(finalScreen)
				rationale := "the process did not reach this exit state in time; you may be missing an interaction step (e.g. a quit key) before this wait"
				if quitKey != "" {
					rationale = fmt.Sprintf("%s; final screen suggests pressing %q before this wait", rationale, quitKey)
				}
				proposals = append(proposals, Proposal{
					StepIndex: fail.index,
					Kind:      "wait.process",
					Rationale: rationale,
				})
			}
			continue
		}
		needle := step.Wait.Screen.Contains
		if needle == "" {
			// equals/matches forms: still suggest timeout if the failure was a timeout.
			if strings.Contains(strings.ToLower(fail.message), "timed out") {
				cur := step.Wait.TimeoutMS
				if cur <= 0 {
					cur = 5000
				}
				proposals = append(proposals, Proposal{
					StepIndex: fail.index,
					Kind:      "wait.timeoutMs",
					Current:   strconv.Itoa(cur),
					Proposed:  strconv.Itoa(cur * 2),
					Rationale: "wait timed out; consider raising wait.timeoutMs (proposed 2x current)",
				})
			}
			continue
		}
		if strings.Contains(finalScreen, needle) {
			cur := step.Wait.TimeoutMS
			if cur <= 0 {
				cur = 5000
			}
			proposals = append(proposals, Proposal{
				StepIndex: fail.index,
				Kind:      "wait.timeoutMs",
				Current:   strconv.Itoa(cur),
				Proposed:  strconv.Itoa(cur * 2),
				Rationale: "the expected text is present on the final screen; the wait likely needs a longer timeoutMs (or an earlier step is out of order)",
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

// detectQuitKey looks for common quit affordances on the final screen.
func detectQuitKey(finalScreen string) string {
	lower := strings.ToLower(finalScreen)
	patterns := []struct {
		needle string
		key    string
	}{
		{"press q", "q"},
		{"press 'q'", "q"},
		{"(q)uit", "q"},
		{"[q]uit", "q"},
		{"quit with q", "q"},
		{"ctrl+c", "ctrl+c"},
		{"ctrl-c", "ctrl+c"},
		{"^c", "ctrl+c"},
	}
	for _, p := range patterns {
		if strings.Contains(lower, p.needle) {
			return p.key
		}
	}
	return ""
}

func readStepFailuresFromRunJSON(runDir string) []stepFailure {
	result, err := artifacts.LoadRunResult(runDir)
	if err != nil {
		return nil
	}
	var out []stepFailure
	for _, s := range result.Steps {
		if s.Status != "failed" {
			continue
		}
		out = append(out, stepFailure{index: s.Index, message: s.Error})
	}
	return out
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

func editStepWaitTimeoutMS(specPath string, index int, newValue string) error {
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
	timeout := mappingValue(wait, "timeoutMs")
	if timeout == nil {
		// Insert timeoutMs key into the wait mapping.
		wait.Content = append(wait.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "timeoutMs"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: newValue},
		)
	} else {
		timeout.Value = newValue
		timeout.Tag = "!!int"
	}
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
