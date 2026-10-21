package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// fiveLines scrolls "one".."five" through a 20x3 screen: "one" and "two" end
// in the scrollback and the screen shows "three", "four" and "five".
const fiveLines = "one\r\ntwo\r\nthree\r\nfour\r\nfive"

func TestCheckScrollbackCondition(t *testing.T) {
	two, three, nine := 2, 3, 9
	tests := []struct {
		name string
		cond spec.ScrollbackCondition
		want bool
		msg  string // substring of the verdict message
	}{
		{"contains a scrolled line", spec.ScrollbackCondition{Contains: "two"}, true, `scrollback contains "two"`},
		{"does not contain a visible line", spec.ScrollbackCondition{Contains: "four"}, false, `expected scrollback to contain "four" (scrollback has 2 lines)`},
		{"notContains a visible line", spec.ScrollbackCondition{NotContains: "five"}, true, ""},
		{"notContains a scrolled line", spec.ScrollbackCondition{NotContains: "one"}, false, "not to contain"},
		{"matches across lines", spec.ScrollbackCondition{Matches: `(?s)one\ntwo`}, true, ""},
		{"matches anchored per line", spec.ScrollbackCondition{Matches: `(?m)^two$`}, true, ""},
		{"lines equals", spec.ScrollbackCondition{Lines: &spec.LinesCondition{Equals: &two}}, true, "2 lines"},
		{"lines equals fails", spec.ScrollbackCondition{Lines: &spec.LinesCondition{Equals: &three}}, false, "exactly 3 lines, got 2"},
		{"lines atLeast", spec.ScrollbackCondition{Lines: &spec.LinesCondition{AtLeast: &two}}, true, ""},
		{"lines atLeast fails", spec.ScrollbackCondition{Lines: &spec.LinesCondition{AtLeast: &three}}, false, "at least 3"},
		{"lines atMost", spec.ScrollbackCondition{Lines: &spec.LinesCondition{AtMost: &three}}, true, ""},
		{"lines atMost fails", spec.ScrollbackCondition{Lines: &spec.LinesCondition{AtMost: &[]int{1}[0]}}, false, "at most 1"},
		{"lines between", spec.ScrollbackCondition{Lines: &spec.LinesCondition{Between: &[2]int{1, 3}}}, true, ""},
		{"lines between fails", spec.ScrollbackCondition{Lines: &spec.LinesCondition{Between: &[2]int{5, nine}}}, false, "between 5 and 9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			emu := makeScreen(t, 20, 3, fiveLines)
			s := &runState{emulator: emu}
			ok, msg := s.checkScrollbackCondition(tc.cond, config.Normalize{})
			if ok != tc.want {
				t.Fatalf("ok = %v (%s), want %v", ok, msg, tc.want)
			}
			if !strings.Contains(msg, tc.msg) {
				t.Errorf("message %q does not contain %q", msg, tc.msg)
			}
		})
	}
}

func TestCheckScrollbackEmptyAndAlternateScreen(t *testing.T) {
	zero := 0
	t.Run("nothing scrolled", func(t *testing.T) {
		s := &runState{emulator: makeScreen(t, 20, 5, "one\r\ntwo")}
		if ok, msg := s.checkScrollbackCondition(spec.ScrollbackCondition{Lines: &spec.LinesCondition{Equals: &zero}}, config.Normalize{}); !ok {
			t.Fatal(msg)
		}
		if ok, _ := s.checkScrollbackCondition(spec.ScrollbackCondition{Contains: "one"}, config.Normalize{}); ok {
			t.Fatal("a line still on the screen is not in the scrollback")
		}
	})
	t.Run("alternate screen", func(t *testing.T) {
		s := &runState{emulator: makeScreen(t, 20, 2, "\x1b[?1049h"+fiveLines+"\x1b[?1049l")}
		if ok, msg := s.checkScrollbackCondition(spec.ScrollbackCondition{Lines: &spec.LinesCondition{Equals: &zero}}, config.Normalize{}); !ok {
			t.Fatal(msg)
		}
	})
}

func TestScreenConditionIncludeScrollback(t *testing.T) {
	norm := config.Normalize{TrimRight: true, NormalizeLineEndings: true}
	tests := []struct {
		name string
		cond spec.ScreenCondition
		want bool
	}{
		{"screen alone misses a scrolled line", spec.ScreenCondition{Contains: "one"}, false},
		{"with scrollback finds it", spec.ScreenCondition{Contains: "one", IncludeScrollback: true}, true},
		{"with scrollback still finds the screen", spec.ScreenCondition{Contains: "five", IncludeScrollback: true}, true},
		{"scrollback comes first", spec.ScreenCondition{Matches: `(?s)one\ntwo\nthree\nfour\nfive$`, IncludeScrollback: true}, true},
		{"equals is the whole transcript", spec.ScreenCondition{Equals: "one\ntwo\nthree\nfour\nfive", IncludeScrollback: true}, true},
		{"notContains sees the scrollback", spec.ScreenCondition{NotContains: "one", IncludeScrollback: true}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &runState{emulator: makeScreen(t, 20, 3, fiveLines)}
			ok, msg := s.checkScreenCondition(tc.cond, norm)
			if ok != tc.want {
				t.Fatalf("ok = %v (%s), want %v", ok, msg, tc.want)
			}
		})
	}
	t.Run("without scrollback the screen text is unchanged", func(t *testing.T) {
		s := &runState{emulator: makeScreen(t, 20, 3, "one\r\ntwo")}
		ok, msg := s.checkScreenCondition(spec.ScreenCondition{Equals: "one\ntwo", IncludeScrollback: true}, norm)
		if !ok {
			t.Fatal(msg)
		}
	})
}

func TestScrollbackTextNormalization(t *testing.T) {
	// Blank rows survive normalization (they are scrollback rows), and the
	// replace rules apply as they do to the screen. Ignore regions are screen
	// coordinates and leave the scrollback alone.
	s := &runState{emulator: makeScreen(t, 20, 2, "id-42   \r\n\r\n\r\nlast\r\nmore")}
	norm := config.Normalize{
		TrimRight: true,
		Replace:   []spec.NormalizeReplace{{Regex: `id-\d+`, With: "id-<n>"}},
		IgnoreRegions: []spec.NormalizeIgnoreArea{
			{X: 0, Y: 0, Width: 20, Height: 1},
		},
	}
	got := s.scrollbackText(norm)
	if want := "id-<n>\n\n"; got != want {
		t.Fatalf("scrollbackText = %q, want %q", got, want)
	}
	if lines := s.scrollbackInfo().Lines; lines != 3 {
		t.Fatalf("lines = %d, want 3 (a blank row counts)", lines)
	}
}

func TestCheckLinesReportsDroppedRows(t *testing.T) {
	five := 5
	ok, msg := checkLines(termInfo(3, 7), spec.LinesCondition{Equals: &five})
	if ok || !strings.Contains(msg, "7 older lines were dropped") {
		t.Fatalf("ok=%v msg=%q, want a dropped-rows hint", ok, msg)
	}
}

// --- end to end ---------------------------------------------------------

const hundredLines = `i=1; while [ $i -le 100 ]; do echo "line $i"; i=$((i+1)); done`

func writeScrollbackFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A target that prints 100 lines through a 10-row screen leaves 91 of them in
// the scrollback (the screen keeps the last 9 and the cursor's blank row).
func TestRunSpecScrollbackEndToEnd(t *testing.T) {
	dir := t.TempDir()
	specPath := writeScrollbackFile(t, dir, "scrollback.yml", fmt.Sprintf(`version: 1
name: scrollback_e2e
intent: a hundred printed lines reach the scrollback whole, in order.
target:
  cmd: ["/bin/sh", "-c", %q]
terminal:
  cols: 40
  rows: 10
  profile: xterm-256color
steps:
  - wait:
      scrollback:
        contains: "line 50"
      timeoutMs: 3000
  - wait:
      screen:
        contains: "line 100"
      timeoutMs: 3000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 3000
  - snapshot: done
outcomes:
  - id: first_line_committed
    description: the first line scrolled into the scrollback
    verify:
      scrollback:
        matches: '(?m)^line 1$'
  - id: scrolled_in_order
    description: the lines arrived whole and in order
    verify:
      scrollback:
        matches: '(?s)^line 1\nline 2\nline 3\n.*\nline 90\nline 91$'
  - id: line_count
    description: ninety-one lines scrolled off
    verify:
      scrollback:
        lines: {equals: 91}
  - id: last_lines_still_visible
    description: the newest lines are on the screen, not in the scrollback
    verify:
      scrollback:
        notContains: "line 100"
  - id: screen_alone_lacks_the_first_line
    description: the visible screen no longer shows line 1
    verify:
      screen:
        notContains: "line 1\n"
  - id: whole_transcript
    description: scrollback plus screen holds every line
    verify:
      screen:
        includeScrollback: true
        matches: '(?s)^line 1\n.*line 100'
`, hundredLines))
	result, err := RunSpec(context.Background(), Options{SpecPath: specPath, ArtifactRoot: filepath.Join(dir, "runs")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	if result.ScrollbackLines != 91 || result.ScrollbackDropped != 0 {
		t.Fatalf("scrollbackLines = %d, scrollbackDropped = %d, want 91 and 0", result.ScrollbackLines, result.ScrollbackDropped)
	}
	if got := result.Artifacts["scrollback"]; got != "scrollback.txt" {
		t.Fatalf("artifacts[scrollback] = %q", got)
	}
	final := readText(t, filepath.Join(result.RunDir, "scrollback.txt"))
	lines := strings.Split(strings.TrimRight(final, "\n"), "\n")
	if len(lines) != 91 || lines[0] != "line 1" || lines[90] != "line 91" {
		t.Fatalf("scrollback.txt has %d lines, first %q, last %q", len(lines), lines[0], lines[len(lines)-1])
	}
	snap := readText(t, filepath.Join(result.RunDir, "snapshots", "done.scrollback.txt"))
	if !strings.HasPrefix(snap, "line 1\n") || !strings.HasSuffix(snap, "line 91\n") {
		t.Fatalf("snapshots/done.scrollback.txt = %q ... %q", snap[:min(20, len(snap))], snap[max(0, len(snap)-20):])
	}
	if got := result.Artifacts["snapshotScrollback:done"]; got != "snapshots/done.scrollback.txt" {
		t.Fatalf("artifacts[snapshotScrollback:done] = %q", got)
	}

	// run.json records the counts, and the reports mention the scrollback.
	var onDisk map[string]any
	if err := json.Unmarshal([]byte(readText(t, filepath.Join(result.RunDir, "run.json"))), &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk["scrollbackLines"] != float64(91) {
		t.Fatalf("run.json scrollbackLines = %v", onDisk["scrollbackLines"])
	}
	if _, present := onDisk["scrollbackDropped"]; present {
		t.Fatalf("run.json scrollbackDropped should be omitted when zero: %v", onDisk["scrollbackDropped"])
	}
	for _, name := range []string{"run.md", "agent_context.md"} {
		if text := readText(t, filepath.Join(result.RunDir, name)); !strings.Contains(text, "scrollback: 91 lines") || !strings.Contains(text, "scrollback.txt") {
			t.Errorf("%s does not mention the scrollback:\n%s", name, text)
		}
	}
	// A screen snapshot is not polluted by the scrollback.
	if text := readText(t, filepath.Join(result.RunDir, "snapshots", "done.txt")); strings.Contains(text, "line 1\n") || !strings.Contains(text, "line 100") {
		t.Fatalf("snapshots/done.txt = %q", text)
	}
}

// terminal.scrollbackLines caps the buffer: the oldest rows go, the counter
// says how many, and a failing check points at the cap.
func TestRunSpecScrollbackCapAndDroppedCounter(t *testing.T) {
	dir := t.TempDir()
	configPath := writeScrollbackFile(t, dir, "glyphrun.config.yml", `version: 1
artifactRoot: runs
terminal:
  cols: 40
  rows: 10
  profile: xterm-256color
  scrollbackLines: 10
`)
	specPath := writeScrollbackFile(t, dir, "cap.yml", fmt.Sprintf(`version: 1
name: scrollback_cap
intent: only the newest scrollback rows are kept.
target:
  cmd: ["/bin/sh", "-c", %q]
steps:
  - wait:
      screen:
        contains: "line 100"
      timeoutMs: 3000
  - wait:
      process:
        exitCode: 0
      timeoutMs: 3000
outcomes:
  - id: capped
    description: ten rows kept
    verify:
      scrollback:
        lines: {equals: 10}
  - id: newest_kept
    description: the newest scrolled rows are the ones kept
    verify:
      scrollback:
        matches: '^line 82\n(.*\n)*line 91$'
  - id: oldest_gone
    description: the first line was dropped
    timeoutMs: 300
    verify:
      scrollback:
        contains: "line 1"
`, hundredLines))
	result, err := RunSpec(context.Background(), Options{SpecPath: specPath, ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusFailed {
		t.Fatalf("status = %s, want failed (the last outcome expects a dropped line)", result.Status)
	}
	byID := map[string]artifacts.OutcomeResult{}
	for _, o := range result.Outcomes {
		byID[o.ID] = o
	}
	for _, id := range []string{"capped", "newest_kept"} {
		if byID[id].Status != artifacts.OutcomePassed {
			t.Errorf("outcome %s: %s %s", id, byID[id].Status, byID[id].Message)
		}
	}
	if o := byID["oldest_gone"]; o.Status != artifacts.OutcomeFailed || !strings.Contains(o.Message, "81 older lines were dropped") {
		t.Errorf("outcome oldest_gone = %s %q, want a failure that names the 81 dropped lines", o.Status, o.Message)
	}
	if result.ScrollbackLines != 10 || result.ScrollbackDropped != 81 {
		t.Fatalf("scrollbackLines = %d, scrollbackDropped = %d, want 10 and 81", result.ScrollbackLines, result.ScrollbackDropped)
	}
	if text := readText(t, filepath.Join(result.RunDir, "run.md")); !strings.Contains(text, "81 older lines dropped") {
		t.Errorf("run.md does not mention the dropped rows:\n%s", text)
	}
}

// An app on the alternate screen never adds to the scrollback, and a run with
// no scrollback writes no scrollback artifacts.
func TestRunSpecScrollbackAlternateScreenAddsNothing(t *testing.T) {
	dir := t.TempDir()
	specPath := writeScrollbackFile(t, dir, "alt.yml", `version: 1
name: scrollback_alt
intent: the alternate screen does not feed the scrollback.
target:
  cmd: ["/bin/sh", "-c", "printf '\\033[?1049h'; i=1; while [ $i -le 60 ]; do echo \"alt $i\"; i=$((i+1)); done; printf '\\033[?1049l'"]
terminal:
  cols: 40
  rows: 10
  profile: xterm-256color
steps:
  - wait:
      process:
        exitCode: 0
      timeoutMs: 3000
outcomes:
  - id: nothing_scrolled
    description: the scrollback is empty
    verify:
      scrollback:
        lines: {equals: 0}
`)
	result, err := RunSpec(context.Background(), Options{SpecPath: specPath, ArtifactRoot: filepath.Join(dir, "runs")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	if _, ok := result.Artifacts["scrollback"]; ok {
		t.Fatalf("no scrollback.txt expected: %#v", result.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(result.RunDir, "scrollback.txt")); !os.IsNotExist(err) {
		t.Fatalf("scrollback.txt should not exist, err=%v", err)
	}
	if strings.Contains(readText(t, filepath.Join(result.RunDir, "run.md")), "scrollback:") {
		t.Fatal("run.md should not mention an empty scrollback")
	}
}

// A scrollback wait that never matches times out with the scrollback verdict.
func TestRunSpecScrollbackWaitTimesOut(t *testing.T) {
	dir := t.TempDir()
	specPath := writeScrollbackFile(t, dir, "wait.yml", `version: 1
name: scrollback_wait_timeout
intent: a scrollback wait reports what it waited for.
target:
  cmd: ["/bin/sh", "-c", "echo one; echo two; sleep 3"]
terminal:
  cols: 40
  rows: 10
  profile: xterm-256color
steps:
  - wait:
      scrollback:
        contains: "never"
      timeoutMs: 300
outcomes:
  - id: unreachable
    description: unreachable
    verify:
      process: {exited: true}
`)
	result, err := RunSpec(context.Background(), Options{SpecPath: specPath, ArtifactRoot: filepath.Join(dir, "runs")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status == artifacts.StatusPassed {
		t.Fatal("the wait should have failed")
	}
	if !strings.Contains(result.Diagnostic, `expected scrollback to contain "never"`) {
		t.Fatalf("diagnostic = %q", result.Diagnostic)
	}
}

func termInfo(lines, dropped int) terminal.ScrollbackInfo {
	return terminal.ScrollbackInfo{Lines: lines, Dropped: dropped, Limit: lines}
}
