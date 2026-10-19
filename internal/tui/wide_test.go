package tui

import (
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

func TestPaintScreenDoesNotPadWideCharacters(t *testing.T) {
	em := terminal.NewEmulator(6, 1)
	if _, err := em.Feed([]byte("a雪b")); err != nil {
		t.Fatal(err)
	}
	snap := em.Screen().Snapshot()
	if got := paintScreen(&snap, false, false); got != "a雪b  " {
		t.Fatalf("painted %q, want %q", got, "a雪b  ")
	}
}
