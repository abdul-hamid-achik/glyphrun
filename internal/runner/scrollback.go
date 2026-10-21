package runner

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// scrollbackOf returns the emulator's scrollback reader. An emulator without
// one has no scrollback, which reads as empty.
func scrollbackOf(emulator terminal.Emulator) (terminal.ScrollbackReader, bool) {
	reader, ok := emulator.(terminal.ScrollbackReader)
	return reader, ok
}

// scrollbackInfo summarizes the run's scrollback; the zero value when the
// emulator keeps none.
func (s *runState) scrollbackInfo() terminal.ScrollbackInfo {
	if reader, ok := scrollbackOf(s.emulator); ok {
		return reader.ScrollbackInfo()
	}
	return terminal.ScrollbackInfo{}
}

// scrollbackText returns the scrollback as text with the same normalization the
// screen text gets, one row per line, oldest first. Trailing blank rows are
// kept: they are rows of the scrollback, and `lines` counts them.
func (s *runState) scrollbackText(normalize config.Normalize) string {
	reader, ok := scrollbackOf(s.emulator)
	if !ok {
		return ""
	}
	return normalizeScrollbackText(reader.ScrollbackText(), normalize)
}

// normalizeScrollbackText normalizes scrollback text like normalizeText does for
// the screen, except that it keeps the blank rows at the end, which
// normalizeText trims. Ignore regions are screen coordinates and do not apply.
func normalizeScrollbackText(raw string, normalize config.Normalize) string {
	trailing := len(raw) - len(strings.TrimRight(raw, "\n"))
	normalize.IgnoreRegions = nil
	return normalizeText(raw, normalize) + strings.Repeat("\n", trailing)
}

// screenWithScrollbackText is the scrollback followed by the visible screen,
// each normalized as it is on its own, joined by a line break.
func (s *runState) screenWithScrollbackText(normalize config.Normalize) string {
	screen := s.screenTextWithNormalize(normalize)
	if s.scrollbackInfo().Lines == 0 {
		return screen
	}
	back := s.scrollbackText(normalize)
	if screen == "" {
		return back
	}
	return back + "\n" + screen
}

// scrollbackArtifactText returns the normalized scrollback and its row count
// for the run's artifacts. Both are empty when nothing scrolled off.
func (s *runState) scrollbackArtifactText() (string, int) {
	info := s.scrollbackInfo()
	if info.Lines == 0 {
		return "", 0
	}
	return s.scrollbackText(s.normalizeConfig()), info.Lines
}

// checkScreenCondition evaluates a screen condition, over the screen plus the
// scrollback when the condition asks for it.
func (s *runState) checkScreenCondition(cond spec.ScreenCondition, normalize config.Normalize) (bool, string) {
	if !cond.IncludeScrollback {
		return checkScreen(s.screenTextWithNormalize(normalize), cond)
	}
	ok, message := checkText("screen and scrollback", s.screenWithScrollbackText(normalize), cond)
	if !ok {
		message += s.scrollbackHint()
	}
	return ok, message
}

// checkScrollbackCondition evaluates a scrollback condition.
func (s *runState) checkScrollbackCondition(cond spec.ScrollbackCondition, normalize config.Normalize) (bool, string) {
	info := s.scrollbackInfo()
	if cond.Lines != nil {
		return checkLines(info, *cond.Lines)
	}
	text := s.scrollbackText(normalize)
	var ok bool
	var message string
	switch {
	case cond.Contains != "":
		ok, message = checkText("scrollback", text, spec.ScreenCondition{Contains: cond.Contains})
	case cond.NotContains != "":
		ok, message = checkText("scrollback", text, spec.ScreenCondition{NotContains: cond.NotContains})
	case cond.Matches != "":
		if _, err := regexp.Compile(cond.Matches); err != nil {
			return false, err.Error()
		}
		ok, message = checkText("scrollback", text, spec.ScreenCondition{Matches: cond.Matches})
	default:
		return false, "scrollback condition is empty"
	}
	if !ok {
		message += s.scrollbackHint()
	}
	return ok, message
}

// scrollbackHint says how much scrollback there was when a text check failed,
// and warns when rows were dropped, since the missing text may be among them.
func (s *runState) scrollbackHint() string {
	info := s.scrollbackInfo()
	hint := fmt.Sprintf(" (scrollback has %d lines", info.Lines)
	if info.Dropped > 0 {
		hint += fmt.Sprintf(", %d older lines were dropped: raise terminal.scrollbackLines", info.Dropped)
	}
	return hint + ")"
}

// checkLines compares the scrollback's row count with exactly one comparator.
func checkLines(info terminal.ScrollbackInfo, cond spec.LinesCondition) (bool, string) {
	got := info.Lines
	extra := ""
	if info.Dropped > 0 {
		extra = fmt.Sprintf(" (%d older lines were dropped: raise terminal.scrollbackLines)", info.Dropped)
	}
	switch {
	case cond.Equals != nil:
		if got == *cond.Equals {
			return true, fmt.Sprintf("scrollback has %d lines", got)
		}
		return false, fmt.Sprintf("expected scrollback to have exactly %d lines, got %d%s", *cond.Equals, got, extra)
	case cond.AtLeast != nil:
		if got >= *cond.AtLeast {
			return true, fmt.Sprintf("scrollback has %d lines (at least %d)", got, *cond.AtLeast)
		}
		return false, fmt.Sprintf("expected scrollback to have at least %d lines, got %d%s", *cond.AtLeast, got, extra)
	case cond.AtMost != nil:
		if got <= *cond.AtMost {
			return true, fmt.Sprintf("scrollback has %d lines (at most %d)", got, *cond.AtMost)
		}
		return false, fmt.Sprintf("expected scrollback to have at most %d lines, got %d%s", *cond.AtMost, got, extra)
	case cond.Between != nil:
		if got >= cond.Between[0] && got <= cond.Between[1] {
			return true, fmt.Sprintf("scrollback has %d lines (between %d and %d)", got, cond.Between[0], cond.Between[1])
		}
		return false, fmt.Sprintf("expected scrollback to have between %d and %d lines, got %d%s", cond.Between[0], cond.Between[1], got, extra)
	default:
		return false, "scrollback.lines has no comparator (equals / atLeast / atMost / between)"
	}
}
