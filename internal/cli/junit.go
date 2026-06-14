package cli

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

// junitTestSuites is the standard JUnit XML format. The runner emits one
// <testsuites> root with one <testsuite> per spec; each spec carries one
// <testcase> per outcome. Outcomes that the spec declared pass through as
// passed; failed outcomes become <failure> elements with the outcome
// message; outcome evaluation errors become <error> elements.
type junitTestSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Name     string       `xml:"name,attr"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Errors   int          `xml:"errors,attr"`
	Time     string       `xml:"time,attr"`
	Suites   []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Errors   int         `xml:"errors,attr"`
	Time     string      `xml:"time,attr"`
	Cases    []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Error     *junitFailure `xml:"error,omitempty"`
	SystemOut string        `xml:"system-out,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

// WriteJUnitReport renders the JUnit XML for the given run results and
// writes it to `path`. The file's parent directory is created if it does
// not exist. Returns an error if the file cannot be written.
func WriteJUnitReport(path string, results []artifacts.RunResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	report := buildJUnitReport(results)
	data, err := xml.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	// xml.MarshalIndent does not emit the XML declaration; CI tools that
	// consume JUnit expect the declaration on the first line.
	header := []byte(xml.Header)
	body := append(header, data...)
	return os.WriteFile(path, body, 0o644)
}

func buildJUnitReport(results []artifacts.RunResult) junitTestSuites {
	report := junitTestSuites{
		Name:  "glyphrun",
		Tests: 0,
	}
	var totalTime time.Duration
	for _, result := range results {
		suite := junitSuite{
			Name: result.SpecName,
			Time: formatJunitDuration(time.Duration(result.DurationMS) * time.Millisecond),
		}
		totalTime += time.Duration(result.DurationMS) * time.Millisecond
		for _, outcome := range result.Outcomes {
			caseName := outcome.ID
			tc := junitCase{
				Name:      caseName,
				Classname: result.SpecName,
				Time:      suite.Time,
			}
			switch outcome.Status {
			case artifacts.OutcomePassed:
				// pass
			default:
				// Failed outcome — surface as a JUnit <failure> element so
				// CI dashboards mark the run red and link to the message.
				tc.Failure = &junitFailure{
					Message: outcome.Message,
					Type:    "OutcomeFailure",
					Body:    fmt.Sprintf("outcome %s in spec %s failed: %s", outcome.ID, result.SpecName, outcome.Message),
				}
				suite.Failures++
				report.Failures++
			}
			suite.Cases = append(suite.Cases, tc)
			suite.Tests++
			report.Tests++
		}
		// A run that errored (PTY crash, parse failure, etc.) has no
		// outcomes. Emit a synthetic testcase so the dashboard shows it.
		if len(suite.Cases) == 0 {
			tc := junitCase{
				Name:      "run",
				Classname: result.SpecName,
				Time:      suite.Time,
			}
			if result.Status == artifacts.StatusErrored {
				tc.Error = &junitFailure{
					Message: "run errored",
					Type:    "RunError",
					Body:    "the run errored before any outcome could be evaluated — see " + result.RunDir,
				}
				suite.Errors++
				report.Errors++
			} else if result.Status == artifacts.StatusFailed {
				tc.Failure = &junitFailure{
					Message: "run failed",
					Type:    "RunFailure",
					Body:    "the run failed (step error) before any outcome could be evaluated — see " + result.RunDir,
				}
				suite.Failures++
				report.Failures++
			}
			suite.Cases = append(suite.Cases, tc)
			suite.Tests++
			report.Tests++
		}
		report.Suites = append(report.Suites, suite)
	}
	report.Time = formatJunitDuration(totalTime)
	return report
}

// formatJunitDuration renders a duration in JUnit's "1.234s" form.
func formatJunitDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	// JUnit accepts decimal seconds; millisecond precision is plenty.
	return fmt.Sprintf("%.3fs", d.Seconds())
}

// junitMessageSanitize strips characters that would break the XML
// attribute encoding (defensive: xml.Marshal handles these, but keeping
// the helper here documents the contract for callers that build messages
// by hand).
func junitMessageSanitize(s string) string {
	if !strings.ContainsAny(s, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0c\x0e\x0f") {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' && r != '\r' {
			return -1
		}
		return r
	}, s)
}
