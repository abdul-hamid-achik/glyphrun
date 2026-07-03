// Package log is glyphrun's thin wrapper around github.com/charmbracelet/log.
//
// It exists to give the CLI a single, configurable diagnostic sink: leveled
// (debug/info/warn/error), machine-readable in JSON mode, and quiet by
// default. The package owns no runner, artifact, or config state — it is
// shared infrastructure configured once in cli.Execute and then used via the
// package-level Debug/Info/Warn/Error helpers (which delegate to the
// charmbracelet/log default logger).
//
// Output contract: glyphrun writes the command result to stdout (machine
// readable for --format json/yaml) and all diagnostics to stderr. This
// package's logger is pinned to stderr so it never contaminates the result
// stream.
package log

import (
	"io"
	"os"
	"strings"

	clog "github.com/charmbracelet/log"
)

// Options configures the package logger. Configure maps the CLI flags
// (quiet/verbose/no-color) and the active output format onto a single
// charmbracelet/log logger. Env overrides GLYPHRUN_LOG_LEVEL and
// GLYPHRUN_LOG_FORMAT win over the flag-derived defaults so CI can
// force a level without changing invocations.
type Options struct {
	// Writer is the diagnostic sink. Defaults to os.Stderr.
	Writer io.Writer
	// Quiet suppresses info/debug (level Warn).
	Quiet bool
	// Verbose raises the level to Debug.
	Verbose bool
	// NoColor forces a plain (non-styled) formatter.
	NoColor bool
	// JSON switches the formatter to JSON lines. This is set when the
	// CLI output format is json or yaml, so stderr diagnostics stay
	// machine-parseable alongside the stdout result.
	JSON bool
}

// Configure installs the package logger from Options. It is safe to call
// once at process start (in cli.Execute). The returned restore func is a
// no-op placeholder kept for future symmetry; callers may ignore it.
func Configure(opts Options) {
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}
	logger := clog.New(w)
	level := resolveLevel(opts)
	logger.SetLevel(level)
	logger.SetOutput(w)
	logger.SetFormatter(resolveFormatter(opts))
	logger.SetReportTimestamp(false)
	clog.SetDefault(logger)
}

// Debug, Info, Warn, Error delegate to the charmbracelet/log default
// logger configured by Configure. They accept a message plus alternating
// key/value pairs, matching charmbracelet/log's API. Use these for all CLI
// diagnostics that are not the run-progress UI.

func Debug(msg string, keyvals ...any) { clog.Debug(msg, keyvals...) }
func Info(msg string, keyvals ...any)  { clog.Info(msg, keyvals...) }
func Warn(msg string, keyvals ...any)  { clog.Warn(msg, keyvals...) }
func Error(msg string, keyvals ...any) { clog.Error(msg, keyvals...) }

// With returns a logger pre-populated with key/value context. It mirrors
// charmbracelet/log.With so callers can attach stable fields (e.g. spec
// name, run id) without threading them through every call.
func With(keyvals ...any) *clog.Logger { return clog.With(keyvals...) }

// resolveLevel picks the effective level from env (GLYPHRUN_LOG_LEVEL)
// first, then the quiet/verbose flags, then the Info default.
func resolveLevel(opts Options) clog.Level {
	if env := strings.TrimSpace(os.Getenv("GLYPHRUN_LOG_LEVEL")); env != "" {
		if lvl, err := clog.ParseLevel(strings.ToLower(env)); err == nil {
			return lvl
		}
	}
	switch {
	case opts.Verbose:
		return clog.DebugLevel
	case opts.Quiet:
		return clog.WarnLevel
	default:
		return clog.InfoLevel
	}
}

// resolveFormatter picks the formatter from env (GLYPHRUN_LOG_FORMAT) first,
// then JSON mode, then the NoColor/plain vs styled-text choice.
func resolveFormatter(opts Options) clog.Formatter {
	if env := strings.TrimSpace(strings.ToLower(os.Getenv("GLYPHRUN_LOG_FORMAT"))); env != "" {
		switch env {
		case "json":
			return clog.JSONFormatter
		case "logfmt":
			return clog.LogfmtFormatter
		case "text":
			return clog.TextFormatter
		}
	}
	switch {
	case opts.JSON:
		return clog.JSONFormatter
	case opts.NoColor:
		return clog.LogfmtFormatter
	default:
		return clog.TextFormatter
	}
}
