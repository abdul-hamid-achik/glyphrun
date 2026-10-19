package render

import (
	"strconv"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// A wide character is drawn as its own <text> spanning two columns, and the
// text after it starts at the column after its continuation.
func TestSnapshotSVGGivesWideCharactersTheirOwnText(t *testing.T) {
	em := terminal.NewEmulator(10, 1)
	if _, err := em.Feed([]byte("a雪bc")); err != nil {
		t.Fatal(err)
	}
	opts := DefaultOptions()
	svg := SnapshotSVG(em.Screen().Snapshot(), opts)
	cw := opts.CellWidth
	origin := textX(t, svg, "a")
	if x := textX(t, svg, "雪"); x != origin+cw {
		t.Errorf("雪 at x=%d, want %d", x, origin+cw)
	}
	if !strings.Contains(svg, `textLength="`+strconv.Itoa(2*cw)+`"`) {
		t.Errorf("no two-column <text> for the wide character:\n%s", svg)
	}
	if x := textX(t, svg, "bc"); x != origin+3*cw {
		t.Errorf("text after the wide character at x=%d, want %d", x, origin+3*cw)
	}
	if strings.Contains(svg, ">a雪") || strings.Contains(svg, "雪 b") {
		t.Errorf("wide character shares a run:\n%s", svg)
	}
}

// textX returns the x attribute of the <text> element whose content starts
// with s.
func textX(t *testing.T, svg, s string) int {
	t.Helper()
	end := strings.Index(svg, ">"+s)
	if end < 0 {
		t.Fatalf("no <text> with %q in:\n%s", s, svg)
	}
	start := strings.LastIndex(svg[:end], `<text x="`)
	rest := svg[start+len(`<text x="`):]
	x, err := strconv.Atoi(rest[:strings.Index(rest, `"`)])
	if err != nil {
		t.Fatal(err)
	}
	return x
}
