package artifacts

import (
	"regexp"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
)

type Redactor struct {
	enabled  bool
	patterns []compiledPattern
}

type compiledPattern struct {
	re      *regexp.Regexp
	replace string
}

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

func (r Redactor) Text(input string) string {
	if !r.enabled {
		return input
	}
	out := input
	for _, pattern := range r.patterns {
		out = pattern.re.ReplaceAllString(out, pattern.replace)
	}
	return out
}

func (r Redactor) Bytes(input []byte) []byte {
	return []byte(r.Text(string(input)))
}
