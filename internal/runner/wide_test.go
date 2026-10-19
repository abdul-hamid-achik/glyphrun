package runner

import (
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

// The cell path (the live screen) and the text path (a .txt golden) of the
// ignore-region normalization must agree on a screen with wide characters, or a
// golden can never match its own capture.
func TestNormalizationAgreesOnWideCharacters(t *testing.T) {
	const body = "a雪b🔩c éx\r\n日本語 tail"
	emu := makeScreen(t, 20, 2, body)
	snap := emu.Screen().Snapshot()
	for _, tt := range []struct {
		name   string
		region spec.NormalizeIgnoreArea
		want   string
	}{
		{"away from the wide characters", spec.NormalizeIgnoreArea{X: 15, Y: 0, Width: 5, Height: 2}, "a雪b🔩c éx\n日本語 tail"},
		{"over the left half", spec.NormalizeIgnoreArea{X: 1, Y: 0, Width: 1, Height: 1}, "a  b🔩c éx\n日本語 tail"},
		{"over the right half", spec.NormalizeIgnoreArea{X: 2, Y: 0, Width: 1, Height: 1}, "a  b🔩c éx\n日本語 tail"},
		{"after a wide character", spec.NormalizeIgnoreArea{X: 7, Y: 1, Width: 4, Height: 1}, "a雪b🔩c éx\n日本語"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			normalize := config.Normalize{TrimRight: true, IgnoreRegions: []spec.NormalizeIgnoreArea{tt.region}}
			fromCells := normalizeSnapshotText(snap, normalize)
			fromText := normalizePlainSnapshotText(snap.Text, normalize)
			if fromCells != tt.want {
				t.Errorf("cell path = %q, want %q", fromCells, tt.want)
			}
			if fromText != fromCells {
				t.Errorf("text path = %q, cell path = %q", fromText, fromCells)
			}
		})
	}
}

func TestCheckLinkWideText(t *testing.T) {
	emu := makeScreen(t, 20, 1, osc8("https://example.com", "雪 docs"))
	ok, msg, raw := checkLink(emu.Screen(), spec.LinkCondition{Text: "雪 docs"})
	if !ok {
		t.Fatalf("expected the link text to read %q: %s (%v)", "雪 docs", msg, raw)
	}
}

func TestCheckCountMatchesAWideCharacter(t *testing.T) {
	emu := makeScreen(t, 20, 1, "雪 a 雪 b 雪")
	s := &runState{emulator: emu}
	ok, msg, _ := s.checkCount(emu.Screen(), spec.CountCondition{Matches: "雪", Equals: intPtr(3)})
	if !ok {
		t.Fatalf("expected 3 cells holding 雪: %s", msg)
	}
}

func TestCheckCountMatchesAClusteredCell(t *testing.T) {
	emu := makeScreen(t, 20, 1, "🇲🇽 é 🇲🇽 👨‍👩‍👧")
	s := &runState{emulator: emu}
	for needle, want := range map[string]int{"🇲🇽": 2, "é": 1, "👨‍👩‍👧": 1} {
		ok, msg, _ := s.checkCount(emu.Screen(), spec.CountCondition{Matches: needle, Equals: intPtr(want)})
		if !ok {
			t.Errorf("matches %q: %s", needle, msg)
		}
	}
}
