package log

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// resetLogger swaps the package default logger to a discard sink. The
// log package mutates a global default (clog.SetDefault); without a
// reset, a Writer from a prior test could leak into the next one. Each
// test below calls this first, then Configure with its own buffer.
func resetLogger(t *testing.T) {
	t.Helper()
	Configure(Options{Writer: io.Discard})
}

// TestLogJSONMode verifies the JSON formatter writes structured output
// with the level and message as quoted JSON fields.
func TestLogJSONMode(t *testing.T) {
	resetLogger(t)
	var buf bytes.Buffer
	Configure(Options{Writer: &buf, JSON: true})
	Info("hi", "k", "v")
	out := buf.String()
	if !strings.Contains(out, `"level":"info"`) {
		t.Errorf("json output missing %q: %q", `"level":"info"`, out)
	}
	if !strings.Contains(out, `"msg":"hi"`) {
		t.Errorf("json output missing %q: %q", `"msg":"hi"`, out)
	}
}

// TestLogQuietSuppressesInfo confirms that Quiet raises the level to
// Warn: an Info call writes nothing, while a Warn call still emits.
func TestLogQuietSuppressesInfo(t *testing.T) {
	resetLogger(t)
	var buf bytes.Buffer
	Configure(Options{Writer: &buf, Quiet: true})
	Info("info-should-be-suppressed")
	Warn("warn-should-appear")
	out := buf.String()
	if strings.Contains(out, "info-should-be-suppressed") {
		t.Errorf("quiet mode should suppress info: %q", out)
	}
	if !strings.Contains(out, "warn-should-appear") {
		t.Errorf("quiet mode should emit warn: %q", out)
	}
}

// TestLogVerboseEmitsDebug confirms that Verbose lowers the level to
// Debug so a Debug call is emitted (the default Info level would drop
// it). JSON mode keeps the assertion deterministic.
func TestLogVerboseEmitsDebug(t *testing.T) {
	resetLogger(t)
	var buf bytes.Buffer
	Configure(Options{Writer: &buf, Verbose: true, JSON: true})
	Debug("debug-here")
	out := buf.String()
	if !strings.Contains(out, `"level":"debug"`) {
		t.Errorf("verbose+json should emit debug level: %q", out)
	}
	if !strings.Contains(out, "debug-here") {
		t.Errorf("debug message missing: %q", out)
	}
}

// TestLogNoColorLogfmt confirms that NoColor selects the logfmt
// formatter (key=value, no ANSI escape codes).
func TestLogNoColorLogfmt(t *testing.T) {
	resetLogger(t)
	var buf bytes.Buffer
	Configure(Options{Writer: &buf, NoColor: true})
	Info("hello")
	out := buf.String()
	if !strings.Contains(out, "level=info") {
		t.Errorf("logfmt output missing level=info: %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("logfmt output should have no ANSI codes: %q", out)
	}
}

// TestLogEnvLevelOverridesFlags confirms GLYPHRUN_LOG_LEVEL wins over
// the flag-derived level: with no Quiet flag, env=warn still suppresses
// Info and emits Warn.
func TestLogEnvLevelOverridesFlags(t *testing.T) {
	resetLogger(t)
	t.Setenv("GLYPHRUN_LOG_LEVEL", "warn")
	var buf bytes.Buffer
	Configure(Options{Writer: &buf})
	Info("info-suppressed-by-env")
	Warn("warn-from-env")
	out := buf.String()
	if strings.Contains(out, "info-suppressed-by-env") {
		t.Errorf("env level=warn should suppress info: %q", out)
	}
	if !strings.Contains(out, "warn-from-env") {
		t.Errorf("env level=warn should emit warn: %q", out)
	}
}

// TestLogEnvFormatOverridesFlags confirms GLYPHRUN_LOG_FORMAT wins
// over the flag-derived formatter: with no JSON flag, env=json still
// produces JSON output.
func TestLogEnvFormatOverridesFlags(t *testing.T) {
	resetLogger(t)
	t.Setenv("GLYPHRUN_LOG_FORMAT", "json")
	var buf bytes.Buffer
	Configure(Options{Writer: &buf})
	Info("hi")
	out := buf.String()
	if !strings.Contains(out, `"level":"info"`) {
		t.Errorf("env format=json should produce json output: %q", out)
	}
}

// TestLogEnvLevelCaseInsensitive confirms the env level parser is
// case-insensitive (e.g. "WARN" works the same as "warn").
func TestLogEnvLevelCaseInsensitive(t *testing.T) {
	resetLogger(t)
	t.Setenv("GLYPHRUN_LOG_LEVEL", "WARN")
	var buf bytes.Buffer
	Configure(Options{Writer: &buf})
	Info("info-suppressed")
	Warn("warn-emitted")
	out := buf.String()
	if strings.Contains(out, "info-suppressed") {
		t.Errorf("env level=WARN should suppress info: %q", out)
	}
	if !strings.Contains(out, "warn-emitted") {
		t.Errorf("env level=WARN should emit warn: %q", out)
	}
}
