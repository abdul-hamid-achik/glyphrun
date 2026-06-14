package artifacts

import (
	"regexp"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
)

type Redactor struct {
	enabled  bool
	patterns []compiledPattern
	// values is a set of literal strings (>=4 chars) to scrub. A
	// separate list (not a pattern) so the runner can accept them
	// from a per-spec block without forcing the contributor to write
	// regexes. Sorted longest-first so the longer secret wins over
	// its prefix.
	values []string
}

type compiledPattern struct {
	re      *regexp.Regexp
	replace string
}

// NewRedactor builds a redactor from the project-level config. To
// extend the redactor with per-spec values (or extra patterns),
// call WithValues / WithPatterns. Each call returns a new redactor;
// the original is left untouched so the caller's config-level
// instance stays reusable.
func NewRedactor(cfg config.Redaction) Redactor {
	r := Redactor{enabled: cfg.Enabled}
	if !r.enabled {
		return r
	}
	defaults := config.Defaults().Redaction.Patterns
	all := append(defaults, cfg.Patterns...)
	for _, pattern := range all {
		re, err := regexp.Compile(pattern.Regex)
		if err == nil {
			r.patterns = append(r.patterns, compiledPattern{re: re, replace: pattern.Replace})
		}
	}
	return r
}

// WithValues returns a copy of the redactor with the given literal
// values added. Values shorter than 4 chars are dropped (matching
// cairn's policy: too short a string redacts too aggressively and
// risks destroying unrelated artifact content).
func (r Redactor) WithValues(values []string) Redactor {
	if !r.enabled {
		return r
	}
	seen := map[string]bool{}
	for _, v := range r.values {
		seen[v] = true
	}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if len(v) < 4 {
			continue
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		r.values = append(r.values, v)
	}
	// Sort longest-first so substring matches don't shadow longer
	// secrets. e.g. redacting "abc-123" must win over redacting
	// "abc" when both are listed.
	sort.SliceStable(r.values, func(i, j int) bool {
		return len(r.values[i]) > len(r.values[j])
	})
	return r
}

// WithPatterns returns a copy of the redactor with the given extra
// regex patterns added. Invalid patterns are silently dropped (the
// caller can pass user-supplied regex without worrying about
// crashing the runner).
func (r Redactor) WithPatterns(patterns []config.RedactionPattern) Redactor {
	if !r.enabled {
		return r
	}
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern.Regex)
		if err == nil {
			r.patterns = append(r.patterns, compiledPattern{re: re, replace: pattern.Replace})
		}
	}
	return r
}

func (r Redactor) Text(input string) string {
	if !r.enabled {
		return input
	}
	out := input
	for _, pattern := range r.patterns {
		out = pattern.re.ReplaceAllString(out, pattern.replace)
	}
	for _, v := range r.values {
		if v == "" {
			continue
		}
		out = strings.ReplaceAll(out, v, "[redacted]")
	}
	return out
}

func (r Redactor) Bytes(input []byte) []byte {
	return []byte(r.Text(string(input)))
}

// RedactBytes is a clearer alias for Bytes, used when the caller is writing
// an arbitrary file (e.g. a download step that copies a binary artifact)
// and wants to make the redaction intent obvious.
func (r Redactor) RedactBytes(input []byte) []byte {
	return r.Bytes(input)
}
