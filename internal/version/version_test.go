package version

import "testing"

// TestVersion_Defaults locks the unlinked-build fallback. A
// regression here would surface as `go run ./cmd/glyph` printing
// the empty string, which scripts that parse `--version` would
// silently miss.
func TestVersion_Defaults(t *testing.T) {
	if Version == "" {
		t.Error("Version must default to a non-empty value (e.g. \"dev\")")
	}
	if Commit == "" {
		t.Error("Commit must default to a non-empty value (e.g. \"unknown\")")
	}
	if BuildDate == "" {
		t.Error("BuildDate must default to a non-empty value (e.g. \"unknown\")")
	}
}

// TestFull_Shape locks the wire format of `glyph --version`. A
// CI script that greps for `(` / `)` depends on this staying
// stable.
func TestFull_Shape(t *testing.T) {
	got := Full()
	if got == "" {
		t.Fatal("Full() returned an empty string")
	}
	want := Version + " (" + Commit + " " + BuildDate + ")"
	if got != want {
		t.Errorf("Full() = %q, want %q", got, want)
	}
}
