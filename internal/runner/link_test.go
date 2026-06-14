package runner

import (
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

// osc8 wraps text in an OSC 8 hyperlink pointing at uri.
func osc8(uri, text string) string {
	return "\x1b]8;;" + uri + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

func TestCheckLink_MatchesURLAndText(t *testing.T) {
	emu := makeScreen(t, 40, 1, "see "+osc8("https://glyphrun.dev", "docs"))
	ok, msg, raw := checkLink(emu.Screen(), spec.LinkCondition{URL: "glyphrun.dev", Text: "docs"})
	if !ok {
		t.Fatalf("expected link match, got fail: %s", msg)
	}
	ev, _ := raw.(map[string]any)
	if ev["url"] != "https://glyphrun.dev" {
		t.Errorf("evidence url = %v, want the full URI", ev["url"])
	}
}

func TestCheckLink_URLOnly(t *testing.T) {
	emu := makeScreen(t, 40, 1, osc8("https://example.com/path", "click here"))
	if ok, msg, _ := checkLink(emu.Screen(), spec.LinkCondition{URL: "example.com"}); !ok {
		t.Fatalf("expected url-only match, got fail: %s", msg)
	}
}

func TestCheckLink_TextMismatchFails(t *testing.T) {
	emu := makeScreen(t, 40, 1, osc8("https://glyphrun.dev", "docs"))
	if ok, _, _ := checkLink(emu.Screen(), spec.LinkCondition{URL: "glyphrun.dev", Text: "home"}); ok {
		t.Fatalf("expected fail when link text does not match")
	}
}

func TestCheckLink_NoLinksFails(t *testing.T) {
	emu := makeScreen(t, 20, 1, "plain text no link")
	if ok, _, _ := checkLink(emu.Screen(), spec.LinkCondition{URL: "anything"}); ok {
		t.Fatalf("expected fail when no hyperlinks present")
	}
}
