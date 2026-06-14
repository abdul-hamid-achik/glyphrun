package spec

import (
	"fmt"
	"regexp"
	"strings"
)

func Validate(s Spec) error {
	if s.Version != 1 {
		return fmt.Errorf("version must be 1")
	}
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	if s.Intent == "" {
		return fmt.Errorf("intent is required")
	}
	if err := validateMetadata(s.Metadata); err != nil {
		return err
	}
	if len(s.Target.Cmd) == 0 || s.Target.Cmd[0] == "" {
		return fmt.Errorf("target.cmd must contain at least one argv item")
	}
	if s.Terminal.Cols <= 0 || s.Terminal.Rows <= 0 {
		return fmt.Errorf("terminal cols and rows must be positive")
	}
	if len(s.Outcomes) == 0 {
		return fmt.Errorf("at least one outcome is required")
	}
	for i, step := range s.Steps {
		if err := validateStep(step); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	for i, outcome := range s.Outcomes {
		if outcome.ID == "" {
			return fmt.Errorf("outcome %d: id is required", i+1)
		}
		if err := validateVerify(outcome.Verify); err != nil {
			return fmt.Errorf("outcome %s: %w", outcome.ID, err)
		}
	}
	return nil
}

// validateMetadata enforces the priority enum. Other fields (feature,
// owner, tags) are free-form so contributors can adapt the metadata
// block to their team's taxonomy without a schema bump.
func validateMetadata(m *Metadata) error {
	if m == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(m.Priority)) {
	case "", "low", "normal", "high", "critical":
	default:
		return fmt.Errorf("metadata.priority must be one of: low, normal, high, critical")
	}
	return nil
}

func validateStep(step Step) error {
	if step.When != nil {
		if err := validateVerify(*step.When); err != nil {
			return fmt.Errorf("when: %w", err)
		}
	}
	count := 0
	if step.Press != "" {
		count++
	}
	if step.Type != "" {
		count++
	}
	if step.Paste != "" {
		count++
	}
	if step.Send != nil {
		count++
		if step.Send.Bytes == "" {
			return fmt.Errorf("send.bytes is required")
		}
	}
	if step.Mouse != nil {
		count++
		if err := validateMouse(*step.Mouse); err != nil {
			return err
		}
	}
	if step.Wait != nil {
		count++
		if err := validateWait(*step.Wait); err != nil {
			return err
		}
	}
	if step.Resize != nil {
		count++
		if step.Resize.Cols <= 0 || step.Resize.Rows <= 0 {
			return fmt.Errorf("resize cols and rows must be positive")
		}
	}
	if step.Snapshot != "" {
		count++
	}
	if step.Use != "" {
		count++
	}
	if step.Download != nil {
		count++
		if err := validateDownload(*step.Download); err != nil {
			return err
		}
	}
	if step.Transform != nil {
		count++
		if err := validateTransform(*step.Transform); err != nil {
			return err
		}
	}
	if len(step.Batch) > 0 {
		count++
		if err := validateBatch(step.Batch); err != nil {
			return err
		}
	}
	if count != 1 {
		return fmt.Errorf("step must contain exactly one action")
	}
	return nil
}

// IsArtifactProducing reports whether the step produces a named artifact
// (download or transform with an assign) or otherwise changes the artifact
// set. Used by the runner to flush artifact placeholders between steps.
func (s Step) IsArtifactProducing() bool {
	if s.Download != nil && s.Download.Assign != "" {
		return true
	}
	if s.Transform != nil && s.Transform.Assign != "" {
		return true
	}
	return false
}

func validateMouse(m MouseStep) error {
	if m.X < 0 || m.Y < 0 {
		return fmt.Errorf("mouse x and y must be non-negative")
	}
	switch m.Button {
	case "", "left", "middle", "right", "wheelUp", "wheelDown":
	default:
		return fmt.Errorf("mouse.button must be one of: left, middle, right, wheelUp, wheelDown")
	}
	switch m.Action {
	case "", "click", "press", "release", "move":
	default:
		return fmt.Errorf("mouse.action must be one of: click, press, release, move")
	}
	return nil
}

func validateDownload(d DownloadStep) error {
	if d.Path == "" {
		return fmt.Errorf("download.path is required")
	}
	if d.Assign != "" && !validArtifactAssign(d.Assign) {
		return fmt.Errorf("download.assign %q must match ^[a-z][A-Za-z0-9_]*$", d.Assign)
	}
	return nil
}

func validateTransform(t TransformStep) error {
	if t.File == "" {
		return fmt.Errorf("transform.file is required")
	}
	if t.SaveAs == "" {
		return fmt.Errorf("transform.saveAs is required")
	}
	if t.Assign != "" && !validArtifactAssign(t.Assign) {
		return fmt.Errorf("transform.assign %q must match ^[a-z][A-Za-z0-9_]*$", t.Assign)
	}
	if t.Runtime != "" && t.Runtime != "node" && t.Runtime != "shell" {
		return fmt.Errorf("transform.runtime must be one of: node, shell")
	}
	return nil
}

func validateBatch(steps []Step) error {
	if len(steps) < 2 {
		return fmt.Errorf("batch requires at least 2 sub-steps")
	}
	trailingWait := false
	for i, sub := range steps {
		// Each sub-step must contain exactly one action; reuse validateStep
		// but inline the count check for clarity (and to skip the "when"
		// outer field that the schema for batchSubStep does not allow).
		count := 0
		if sub.Press != "" {
			count++
		}
		if sub.Type != "" {
			count++
		}
		if sub.Paste != "" {
			count++
		}
		if sub.Send != nil {
			count++
			if sub.Send.Bytes == "" {
				return fmt.Errorf("batch sub-step %d: send.bytes is required", i+1)
			}
		}
		if sub.Wait != nil {
			count++
			if err := validateWait(*sub.Wait); err != nil {
				return fmt.Errorf("batch sub-step %d: %w", i+1, err)
			}
			if i != len(steps)-1 {
				return fmt.Errorf("batch wait is only allowed as the final sub-step (sub-step %d)", i+1)
			}
			trailingWait = true
		}
		if count != 1 {
			return fmt.Errorf("batch sub-step %d must contain exactly one action", i+1)
		}
	}
	_ = trailingWait
	return nil
}

// validArtifactAssign matches cairn's `^[a-z][A-Za-z0-9_]*$` pattern.
var artifactAssignPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9_]*$`)

func validArtifactAssign(name string) bool {
	return artifactAssignPattern.MatchString(name)
}

func validateWait(wait WaitStep) error {
	count := 0
	if wait.Screen != nil {
		count++
		if err := validateScreenCondition(*wait.Screen); err != nil {
			return err
		}
	}
	if wait.Process != nil {
		count++
	}
	if wait.Idle != nil {
		count++
		if wait.Idle.QuietForMS <= 0 {
			return fmt.Errorf("idle.quietForMs must be positive")
		}
	}
	if count != 1 {
		return fmt.Errorf("wait must contain exactly one target")
	}
	return nil
}

func validateVerify(v Verify) error {
	count := 0
	if v.Screen != nil {
		count++
		if err := validateScreenCondition(*v.Screen); err != nil {
			return err
		}
	}
	if v.Region != nil {
		count++
		if v.Region.Width <= 0 || v.Region.Height <= 0 {
			return fmt.Errorf("region width and height must be positive")
		}
		if err := validateRegionCondition(*v.Region); err != nil {
			return err
		}
	}
	if v.Cell != nil {
		count++
		if v.Cell.Char == "" && v.Cell.Style == nil {
			return fmt.Errorf("cell verifier must contain char or style")
		}
		if v.Cell.Style != nil && !v.Cell.Style.hasAssertions() {
			return fmt.Errorf("cell.style must contain at least one assertion")
		}
	}
	if v.Cursor != nil {
		count++
	}
	if v.Process != nil {
		count++
	}
	if v.Snapshot != nil {
		count++
		if v.Snapshot.Name == "" {
			return fmt.Errorf("snapshot.name is required")
		}
	}
	if v.Command != nil {
		count++
		if v.Command.Run == "" {
			return fmt.Errorf("command.run is required")
		}
	}
	if v.File != nil {
		count++
		if v.File.Glob == "" {
			return fmt.Errorf("file.glob is required")
		}
	}
	if v.Script != nil {
		count++
		if err := validateScriptCondition(*v.Script); err != nil {
			return err
		}
	}
	if v.Count != nil {
		count++
		if err := validateCountCondition(*v.Count); err != nil {
			return err
		}
	}
	if v.Link != nil {
		count++
		if v.Link.URL == "" && v.Link.Text == "" {
			return fmt.Errorf("link verifier must contain url or text")
		}
	}
	if count != 1 {
		return fmt.Errorf("verify must contain exactly one verifier")
	}
	return nil
}

func validateCountCondition(c CountCondition) error {
	comps := 0
	if c.Equals != nil {
		comps++
	}
	if c.AtLeast != nil {
		comps++
	}
	if c.AtMost != nil {
		comps++
	}
	if c.Between != nil {
		comps++
	}
	if comps == 0 {
		return fmt.Errorf("count must include exactly one of equals / atLeast / atMost / between")
	}
	if comps > 1 {
		return fmt.Errorf("count must include exactly one comparator; got %d", comps)
	}
	if c.Between != nil && (c.Between[0] > c.Between[1] || c.Between[0] < 0) {
		return fmt.Errorf("count.between must be [min, max] with 0 <= min <= max")
	}
	if c.Region != nil {
		if c.Region.Width <= 0 || c.Region.Height <= 0 {
			return fmt.Errorf("count.region width and height must be positive")
		}
	}
	if c.Matches != "" && c.Matches != "nonEmpty" {
		if len([]rune(c.Matches)) != 1 {
			return fmt.Errorf("count.matches must be a single character or \"nonEmpty\", got %q", c.Matches)
		}
	}
	return nil
}

func validateScriptCondition(s ScriptCondition) error {
	// exactly one of run / file
	has := 0
	if s.Run != "" {
		has++
	}
	if s.File != "" {
		has++
	}
	if has != 1 {
		return fmt.Errorf("script must contain exactly one of: run, file")
	}
	if s.Runtime != "" && s.Runtime != "node" && s.Runtime != "shell" {
		return fmt.Errorf("script.runtime must be one of: node, shell")
	}
	return nil
}

func validateRegionCondition(cond RegionCondition) error {
	count := 0
	if cond.Contains != "" {
		count++
	}
	if cond.NotContains != "" {
		count++
	}
	if cond.Regex != "" {
		count++
		if _, err := regexp.Compile(cond.Regex); err != nil {
			return fmt.Errorf("region.regex is invalid: %w", err)
		}
	}
	if count != 1 {
		return fmt.Errorf("region condition must contain exactly one assertion")
	}
	return nil
}

func validateScreenCondition(cond ScreenCondition) error {
	count := 0
	if cond.Contains != "" {
		count++
	}
	if cond.NotContains != "" {
		count++
	}
	if cond.Regex != "" {
		count++
		if _, err := regexp.Compile(cond.Regex); err != nil {
			return fmt.Errorf("screen.regex is invalid: %w", err)
		}
	}
	if count != 1 {
		return fmt.Errorf("screen condition must contain exactly one assertion")
	}
	return nil
}

func (s Style) hasAssertions() bool {
	return s.Fg != "" ||
		s.Bg != "" ||
		s.Bold != nil ||
		s.Dim != nil ||
		s.Italic != nil ||
		s.Underline != nil ||
		s.Reverse != nil
}

type ContractHashMismatchError struct {
	Expected string
	Actual   string
}

func (e ContractHashMismatchError) Error() string {
	return fmt.Sprintf("contractHash mismatch: stamped %s, computed %s", e.Expected, e.Actual)
}
