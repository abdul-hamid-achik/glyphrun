// Package procmon is glyphrun's adapter to the `monitor` CLI
// (~/projects/monitor) for process-level observability of the target a spec
// spawns. It is the shared foundation for three opt-in surfaces:
//
//   - `glyph run --monitor <path>` samples the target's CPU/RSS during the run
//     and writes a `diagnostics/process.md` + `process.json` artifact.
//   - a `monitor:` step captures a one-shot process snapshot / profile / tree
//     at a point in the flow and stores it as a named artifact.
//   - a `metrics:` outcome verifier asserts on the sampled summary (perf
//     budgets: peak RSS, peak CPU).
//
// The package owns only the monitor CLI wrapping, the sample→summary
// reduction, and the markdown rendering — pure, testable, no runner state.
// The runner owns the sampling goroutine lifecycle (see internal/runner); this
// package provides the per-tick `Sample` and the pure `Summarize`.
package procmon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ProcessInfo is the shape of `monitor process <pid> --json` and one node of
// `monitor tree <pid> --json`. `monitor` is the source of truth for the field
// names; this struct mirrors them so unmarshal is direct.
type ProcessInfo struct {
	PID           int           `json:"pid"`
	Name          string        `json:"name"`
	CPUPercent    float64       `json:"cpu_percent"`
	Memory        int64         `json:"memory"` // RSS, bytes
	MemoryPercent float64       `json:"memory_percent"`
	Threads       int           `json:"threads"`
	User          string        `json:"user"`
	Parent        int           `json:"parent"`
	IsSystem      bool          `json:"is_system"`
	IsProtected   bool          `json:"is_protected"`
	Children      []ProcessInfo `json:"children,omitempty"`
}

// Profile is the shape of `monitor profile <pid> --type <t> --json`. `sample`
// (macOS) is symbolicated; `heap` is symbolicated; `cpu`/`goroutine` come from
// the target's pprof server and `Text` may carry raw/base64 content. Glyphrun
// treats the profile as opaque evidence — it stores `Text` verbatim.
type Profile struct {
	PID     int          `json:"pid"`
	Type    string       `json:"type"`
	Taken   string       `json:"taken"`
	Text    string       `json:"text"`
	Symbols []ProfileSym `json:"symbols,omitempty"`
}

type ProfileSym struct {
	Func string `json:"func"`
	File string `json:"file"`
	Line int    `json:"line"`
}

// Client wraps the `monitor` binary. Bin is the path to the binary (resolved
// from $PATH when "monitor"); an empty Bin means "monitor" on $PATH.
type Client struct {
	Bin string
}

func (c *Client) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return "monitor"
}

// Process runs `monitor process <pid> --json`. Returns the parsed info, or an
// error if monitor is missing / the PID is gone (caller degrades gracefully).
func (c *Client) Process(pid int) (ProcessInfo, error) {
	out, err := exec.Command(c.bin(), "process", strconv.Itoa(pid), "--json").Output()
	if err != nil {
		return ProcessInfo{}, wrap(err, "monitor process")
	}
	var info ProcessInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return ProcessInfo{}, fmt.Errorf("parse monitor process output: %w", err)
	}
	return info, nil
}

// Tree runs `monitor tree <pid> --json`. Returns the root + its descendants.
func (c *Client) Tree(pid int) ([]ProcessInfo, error) {
	out, err := exec.Command(c.bin(), "tree", strconv.Itoa(pid), "--json").Output()
	if err != nil {
		return nil, wrap(err, "monitor tree")
	}
	var nodes []ProcessInfo
	if err := json.Unmarshal(out, &nodes); err != nil {
		return nil, fmt.Errorf("parse monitor tree output: %w", err)
	}
	return nodes, nil
}

// TreeText runs `monitor tree <pid>` (human output, no --json) and returns
// the formatted process tree verbatim — it is embedded in process.md as-is.
func (c *Client) TreeText(pid int) (string, error) {
	out, err := exec.Command(c.bin(), "tree", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", wrap(err, "monitor tree")
	}
	return string(out), nil
}

// Profile runs `monitor profile <pid> --type <ptype> --json`. ptype is one of
// heap|cpu|goroutine|sample. The capture blocks for ~1s (sample) or a pprof
// scrape; pass a context-bound caller if you need cancellation.
func (c *Client) Profile(pid int, ptype string) (Profile, error) {
	if ptype == "" {
		ptype = "heap"
	}
	out, err := exec.Command(c.bin(), "profile", strconv.Itoa(pid), "--type", ptype, "--json").Output()
	if err != nil {
		return Profile{}, wrap(err, "monitor profile")
	}
	var p Profile
	if err := json.Unmarshal(out, &p); err != nil {
		return Profile{}, fmt.Errorf("parse monitor profile output: %w", err)
	}
	return p, nil
}

// wrap keeps the "is monitor installed?" signal in the error chain so callers
// can produce a clear message on the first failed sample.
func wrap(err error, cmd string) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return fmt.Errorf("%s failed: %s: %s", cmd, err, strings.TrimSpace(string(ee.Stderr)))
	}
	return fmt.Errorf("%s: %w (is monitor installed and on $PATH?)", cmd, err)
}

// Sample is one telemetry reading of the target at a point in time.
type Sample struct {
	At      time.Time `json:"at"`
	CPU     float64   `json:"cpuPercent"`
	RSS     int64     `json:"rss"`
	Threads int       `json:"threads"`
}

// SampleOnce takes a single reading of pid via client and returns it. A
// missing/dead process yields a zero sample + error; the runner treats one
// failure as non-fatal and keeps prior samples.
func SampleOnce(client *Client, pid int, now func() time.Time) (Sample, error) {
	info, err := client.Process(pid)
	if err != nil {
		return Sample{}, err
	}
	if now == nil {
		now = time.Now
	}
	return Sample{At: now().UTC(), CPU: info.CPUPercent, RSS: info.Memory, Threads: info.Threads}, nil
}

// Summary is the reduced telemetry over a run: peak/mean CPU + RSS, the sample
// timeline, and the target identity. It is what `glyph run --monitor` writes
// to `diagnostics/process.json` and what a `metrics:` verifier asserts on.
type Summary struct {
	PID         int       `json:"pid" yaml:"pid"`
	Name        string    `json:"name,omitempty" yaml:"name,omitempty"`
	SampleCount int       `json:"sampleCount" yaml:"sampleCount"`
	StartedAt   time.Time `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
	EndedAt     time.Time `json:"endedAt,omitempty" yaml:"endedAt,omitempty"`
	DurationMS  int64     `json:"durationMs,omitempty" yaml:"durationMs,omitempty"`
	PeakCPU     float64   `json:"peakCpuPercent" yaml:"peakCpuPercent"`
	PeakRSS     int64     `json:"peakRss" yaml:"peakRss"`
	MeanCPU     float64   `json:"meanCpuPercent" yaml:"meanCpuPercent"`
	MeanRSS     int64     `json:"meanRss" yaml:"meanRss"`
	PeakThreads int       `json:"peakThreads,omitempty" yaml:"peakThreads,omitempty"`
	Samples     []Sample  `json:"samples,omitempty" yaml:"samples,omitempty"`
	Note        string    `json:"note,omitempty" yaml:"note,omitempty"`
}

// Summarize reduces a sample slice into a Summary. It is pure (no I/O) so the
// reduction is unit-tested without monitor. pid/name/started are stamped from
// the caller (the runner knows the target identity); an empty slice yields a
// zero Summary with SampleCount 0.
func Summarize(pid int, name string, started time.Time, samples []Sample) Summary {
	s := Summary{PID: pid, Name: name, StartedAt: started, Samples: samples, SampleCount: len(samples)}
	if len(samples) == 0 {
		return s
	}
	s.EndedAt = samples[len(samples)-1].At
	if !started.IsZero() {
		s.DurationMS = s.EndedAt.Sub(started).Milliseconds()
	}
	var sumCPU, sumRSS float64
	for _, x := range samples {
		if x.CPU > s.PeakCPU {
			s.PeakCPU = x.CPU
		}
		if x.RSS > s.PeakRSS {
			s.PeakRSS = x.RSS
		}
		if x.Threads > s.PeakThreads {
			s.PeakThreads = x.Threads
		}
		sumCPU += x.CPU
		sumRSS += float64(x.RSS)
	}
	s.MeanCPU = sumCPU / float64(len(samples))
	s.MeanRSS = int64(sumRSS / float64(len(samples)))
	return s
}

// RenderProcessMarkdown turns a Summary + optional process-tree text into the
// `diagnostics/process.md` artifact. The tree is renderered verbatim (monitor
// already formats it); when empty, only the summary table is emitted.
func RenderProcessMarkdown(s Summary, tree string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Glyphrun Process Telemetry\n\n")
	if s.Name != "" {
		fmt.Fprintf(&b, "- target: `%s` (pid %d)\n", s.Name, s.PID)
	} else {
		fmt.Fprintf(&b, "- pid: %d\n", s.PID)
	}
	fmt.Fprintf(&b, "- samples: %d\n", s.SampleCount)
	if s.DurationMS > 0 {
		fmt.Fprintf(&b, "- observed: %dms\n", s.DurationMS)
	}
	if s.SampleCount == 0 {
		b.WriteString("\nNo samples captured (monitor unavailable, PID not exposed, or target exited before the first tick).\n")
		return b.String()
	}
	fmt.Fprintf(&b, "- peak CPU: %.1f%%\n", s.PeakCPU)
	fmt.Fprintf(&b, "- mean CPU: %.1f%%\n", s.MeanCPU)
	fmt.Fprintf(&b, "- peak RSS: %s\n", FormatBytes(s.PeakRSS))
	fmt.Fprintf(&b, "- mean RSS: %s\n", FormatBytes(s.MeanRSS))
	if s.PeakThreads > 0 {
		fmt.Fprintf(&b, "- peak threads: %d\n", s.PeakThreads)
	}
	if s.Note != "" {
		fmt.Fprintf(&b, "- note: %s\n", s.Note)
	}
	if tree = strings.TrimSpace(tree); tree != "" {
		b.WriteString("\n## Process Tree\n\n```\n")
		b.WriteString(tree)
		b.WriteString("\n```\n")
	}
	return b.String()
}

// FormatBytes renders a byte count in IEC-ish units for the markdown report.
// Kept here so procmon has no dependency on the rest of glyphrun.
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// RenderSnapshotMarkdown renders a one-shot process reading (from a `monitor:`
// step) as a compact evidence report: the reading, an optional process tree,
// and an optional profile summary. It is the step-level counterpart of
// RenderProcessMarkdown (which summarizes a sampled timeline).
func RenderSnapshotMarkdown(info ProcessInfo, tree string, prof *Profile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Monitor Snapshot\n\n")
	fmt.Fprintf(&b, "- pid: %d\n- name: %s\n", info.PID, info.Name)
	fmt.Fprintf(&b, "- cpu: %.1f%%\n", info.CPUPercent)
	fmt.Fprintf(&b, "- rss: %s\n", FormatBytes(info.Memory))
	if info.Threads > 0 {
		fmt.Fprintf(&b, "- threads: %d\n", info.Threads)
	}
	if info.Parent > 0 {
		fmt.Fprintf(&b, "- parent: %d\n", info.Parent)
	}
	if info.IsSystem || info.IsProtected {
		fmt.Fprintf(&b, "- flags: %s\n", flagsString(info))
	}
	if tree = strings.TrimSpace(tree); tree != "" {
		b.WriteString("\n## Process Tree\n\n```\n")
		b.WriteString(tree)
		b.WriteString("\n```\n")
	}
	if prof != nil {
		fmt.Fprintf(&b, "\n## Profile (%s)\n\n- taken: %s\n- symbols: %d\n", prof.Type, prof.Taken, len(prof.Symbols))
	}
	return b.String()
}

func flagsString(info ProcessInfo) string {
	var fs []string
	if info.IsSystem {
		fs = append(fs, "system")
	}
	if info.IsProtected {
		fs = append(fs, "protected")
	}
	return strings.Join(fs, ",")
}

// AssertMetrics checks process-telemetry perf budgets against a summary.
// Each non-nil field is an upper bound (<=): the assertion passes only if the
// observed peak/mean stays at or below the budget. A zero-sample summary
// fails with a clear "no telemetry" message. Pure — the runner's checkMetrics
// delegates here so the budget logic is unit-testable without a run.
func AssertMetrics(summary Summary, peakCpu, meanCpu *float64, peakRss, meanRss *int64) (bool, string) {
	if summary.SampleCount == 0 {
		return false, "no process telemetry captured (run with --monitor, or add a monitor: step) to assert metrics"
	}
	var problems []string
	if peakCpu != nil && summary.PeakCPU > *peakCpu {
		problems = append(problems, fmt.Sprintf("peak cpu %.1f%% > %.1f%%", summary.PeakCPU, *peakCpu))
	}
	if meanCpu != nil && summary.MeanCPU > *meanCpu {
		problems = append(problems, fmt.Sprintf("mean cpu %.1f%% > %.1f%%", summary.MeanCPU, *meanCpu))
	}
	if peakRss != nil && summary.PeakRSS > *peakRss {
		problems = append(problems, fmt.Sprintf("peak rss %d > %d", summary.PeakRSS, *peakRss))
	}
	if meanRss != nil && summary.MeanRSS > *meanRss {
		problems = append(problems, fmt.Sprintf("mean rss %d > %d", summary.MeanRSS, *meanRss))
	}
	if len(problems) > 0 {
		return false, "metrics budget exceeded: " + strings.Join(problems, "; ")
	}
	return true, fmt.Sprintf("metrics within budget (peak cpu %.1f%%, peak rss %d)", summary.PeakCPU, summary.PeakRSS)
}
