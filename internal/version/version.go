// Package version exposes the build-time version metadata for the
// glyph binary. The values are intended to be overridden at link
// time via -ldflags:
//
//	go build -ldflags "-X github.com/abdul-hamid-achik/glyphrun/internal/version.Version=v1.2.3 \
//	                   -X github.com/abdul-hamid-achik/glyphrun/internal/version.Commit=abc1234 \
//	                   -X github.com/abdul-hamid-achik/glyphrun/internal/version.BuildDate=2026-06-13" \
//	         -o ./bin/glyph ./cmd/glyph
//
// When the linker doesn't override the variables (e.g. `go run` or
// `go install` without flags), Version falls back to "dev" so the
// binary still prints something useful.
package version

// These vars are populated at link time. Treat them as read-only
// after process start.
var (
	// Version is the semantic version (e.g. "v0.1.0"). Default
	// "dev" is what `go install` from a working copy prints.
	Version = "dev"
	// Commit is the short git SHA the binary was built from.
	// Default "unknown" so the output stays well-formed.
	Commit = "unknown"
	// BuildDate is the RFC3339 timestamp of the build.
	BuildDate = "unknown"
)

// Full returns the canonical "version (commit buildDate)" string
// the binary prints. Keeping it in one place lets tests assert on
// the shape without depending on the linker.
func Full() string {
	return Version + " (" + Commit + " " + BuildDate + ")"
}
