package spec

import (
	"fmt"
	"regexp"
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
	if count != 1 {
		return fmt.Errorf("step must contain exactly one action")
	}
	return nil
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
	if count != 1 {
		return fmt.Errorf("verify must contain exactly one verifier")
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
