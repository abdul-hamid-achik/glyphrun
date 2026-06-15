package terminal

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestReplaySplitRunesFromRealRun is a golden regression against a real glyph
// run that corrupted: replaying tvault's `studio --rw` create/delete flow, the
// PTY chunked a frame mid-"•" (a 3-byte glyph), and the pre-fix emulator
// decoded the fragment as raw bytes — putting rune U+0080 (a C1 control that
// never appears in a real render) into a cell, splitting one glyph into three
// cells, and desyncing every later column on the frame (the visible "32
// Projects" / stale-count corruption).
//
// The fixture (testdata/studio_rw_edit_frames.ndjson) carries, per frame, the
// exact input bytes (rawBytesBase64) AND the pre-fix render (screen.text). We
// replay the EXACT chunk sequence through the current emulator and assert no
// frame contains the corruption rune anymore.
func TestReplaySplitRunesFromRealRun(t *testing.T) {
	type frame struct {
		RawBytesBase64 string `json:"rawBytesBase64"`
		Screen         struct {
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
			Text string `json:"text"`
		} `json:"screen"`
	}

	f, err := os.Open("testdata/studio_rw_edit_frames.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var frames []frame
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for sc.Scan() {
		var fr frame
		if err := json.Unmarshal(sc.Bytes(), &fr); err != nil {
			t.Fatalf("parse frame: %v", err)
		}
		frames = append(frames, fr)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(frames) == 0 {
		t.Fatal("no frames in fixture")
	}

	const corrupt = '' // only arises from a 0x80 byte decoded as a lone rune

	// Sanity: the pre-fix render stored in the fixture MUST contain the
	// corruption, or this fixture would not be exercising the bug.
	oldCorrupt := 0
	for _, fr := range frames {
		if strings.ContainsRune(fr.Screen.Text, corrupt) {
			oldCorrupt++
		}
	}
	if oldCorrupt == 0 {
		t.Fatal("fixture does not exercise the bug — no pre-fix corruption found")
	}
	t.Logf("fixture: %d/%d frames were corrupted before the fix", oldCorrupt, len(frames))

	cols, rows := frames[0].Screen.Cols, frames[0].Screen.Rows
	if cols == 0 || rows == 0 {
		cols, rows = 120, 40
	}
	em := NewEmulator(cols, rows)
	sawCorrectCount := false // the secrets pane count must reach "(3)" after the create
	for i, fr := range frames {
		raw, derr := base64.StdEncoding.DecodeString(fr.RawBytesBase64)
		if derr != nil {
			t.Fatalf("frame %d: decode: %v", i+1, derr)
		}
		if _, ferr := em.Feed(raw); ferr != nil {
			t.Fatalf("frame %d: feed: %v", i+1, ferr)
		}
		text := em.Screen().Text()
		// Bug 1 (split rune across PTY chunks): no stray C1-control cell.
		if strings.ContainsRune(text, corrupt) {
			t.Errorf("frame %d: split-rune corruption (U+0080) still present after the fix", i+1)
		}
		// Bug 2 (bare LF reset the column): the secrets-count write landed at
		// column 0, clobbering the left border into "32 Projects". It must not.
		if strings.Contains(text, "32 Projects") {
			t.Errorf("frame %d: count digit mis-positioned to column 0 (\"32 Projects\") — LF/column bug", i+1)
		}
		if strings.Contains(text, "Secrets (3)") {
			sawCorrectCount = true
		}
	}
	// The pre-fix render never showed "(3)" (the update was dropped). It must now.
	if !sawCorrectCount {
		t.Error("the secrets pane count never reached \"(3)\" — the cell update is still being dropped/mis-applied")
	}
}
