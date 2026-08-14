package inventory

import (
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

func TestFromSnapshot(t *testing.T) {
	rep := FromSnapshot(terminal.ScreenSnapshot{
		Cols: 80,
		Rows: 24,
		Text: "Welcome\n^X exit\nprompt>",
	})
	if len(rep.Items) != 3 {
		t.Fatalf("items = %#v", rep.Items)
	}
	if rep.Items[1].Hotkey != "ctrl+x" {
		t.Errorf("hotkey = %q", rep.Items[1].Hotkey)
	}
	if rep.Items[2].Kind != "prompt" {
		t.Errorf("prompt kind = %q", rep.Items[2].Kind)
	}
}
