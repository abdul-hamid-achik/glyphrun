package artifacts

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
	"gopkg.in/yaml.v3"
)

type Writer struct {
	RunDir   string
	redactor Redactor
}

func NewWriter(runDir string, redactor Redactor) *Writer {
	return &Writer{RunDir: runDir, redactor: redactor}
}

func (w *Writer) EnsureDirs() error {
	for _, dir := range []string{
		w.RunDir,
		filepath.Join(w.RunDir, "screens"),
		filepath.Join(w.RunDir, "raw"),
		filepath.Join(w.RunDir, "frames"),
		filepath.Join(w.RunDir, "snapshots"),
		filepath.Join(w.RunDir, "outcomes"),
		filepath.Join(w.RunDir, "diagnostics"),
		filepath.Join(w.RunDir, "artifacts"),
		filepath.Join(w.RunDir, "transforms"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) Resolve(rel string) string {
	return filepath.Join(w.RunDir, rel)
}

func (w *Writer) WriteRun(result RunResult) error {
	if err := writeJSON(w.Resolve("run.json"), result, w.redactor); err != nil {
		return err
	}
	if err := writeYAML(w.Resolve("run.yaml"), result, w.redactor); err != nil {
		return err
	}
	return os.WriteFile(w.Resolve("run.md"), []byte(w.redactor.Text(RenderRunMarkdown(result))), 0o644)
}

func (w *Writer) WriteResolvedSpec(s spec.Spec) error {
	return writeYAML(w.Resolve("spec.resolved.yml"), s, w.redactor)
}

func (w *Writer) AppendEvent(event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.OpenFile(w.Resolve("events.ndjson"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(w.redactor.Bytes(data))
	return err
}

func (w *Writer) WriteFinalScreen(snapshot terminal.ScreenSnapshot) error {
	if err := os.WriteFile(w.Resolve("screens/final.txt"), []byte(w.redactor.Text(snapshot.Text)+"\n"), 0o644); err != nil {
		return err
	}
	return writeJSON(w.Resolve("screens/final.json"), snapshot, w.redactor)
}

func (w *Writer) WriteRawPTY(raw []byte) error {
	return os.WriteFile(w.Resolve("raw/pty.raw.log"), w.redactor.Bytes(raw), 0o644)
}

func (w *Writer) WriteInputLog(raw []byte) error {
	return os.WriteFile(w.Resolve("raw/input.raw.log"), w.redactor.Bytes(raw), 0o644)
}

func (w *Writer) WriteFrames(frames []terminal.Frame) error {
	f, err := os.OpenFile(w.Resolve("frames/frames.ndjson"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, frame := range frames {
		data, err := json.Marshal(frame)
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if _, err := f.Write(w.redactor.Bytes(data)); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) WriteSnapshot(name string, snapshot terminal.ScreenSnapshot) error {
	safe := SafeName(name)
	if err := os.WriteFile(w.Resolve("snapshots/"+safe+".txt"), []byte(w.redactor.Text(snapshot.Text)+"\n"), 0o644); err != nil {
		return err
	}
	return writeJSON(w.Resolve("snapshots/"+safe+".json"), snapshot, w.redactor)
}

func (w *Writer) WriteOutcome(result OutcomeResult, raw any) error {
	safe := SafeName(result.ID)
	md := "# Outcome: " + result.ID + "\n\n" +
		"- status: " + string(result.Status) + "\n" +
		"- message: " + result.Message + "\n"
	if result.Evidence != "" {
		md += "- evidence: " + result.Evidence + "\n"
	}
	if err := os.WriteFile(w.Resolve("outcomes/"+safe+".md"), []byte(w.redactor.Text(md)), 0o644); err != nil {
		return err
	}
	if raw != nil {
		return writeJSON(w.Resolve("outcomes/"+safe+".raw.json"), raw, w.redactor)
	}
	return nil
}

// WriteOutcomeRaw writes the raw evidence sidecar for an outcome. It is
// called separately from WriteOutcome so the caller can stream a verifier's
// `evidence` payload to disk in its own goroutine or after the outcome's
// markdown has been written. The redaction layer is applied so secrets
// emitted by `script:` verifiers don't leak into artifacts.
func (w *Writer) WriteOutcomeRaw(outcomeID string, raw any) error {
	if raw == nil {
		return nil
	}
	safe := SafeName(outcomeID)
	return writeJSON(w.Resolve("outcomes/"+safe+".raw.json"), raw, w.redactor)
}

func (w *Writer) WriteOutcomesIndex(result RunResult) error {
	summary := map[string]any{
		"runId":    result.RunID,
		"status":   result.Status,
		"outcomes": result.Outcomes,
	}
	if err := writeJSON(w.Resolve("outcomes/results.json"), summary, w.redactor); err != nil {
		return err
	}
	if err := writeYAML(w.Resolve("outcomes/results.yaml"), summary, w.redactor); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Outcomes\n\n")
	passed, failed := outcomeCounts(result.Outcomes)
	b.WriteString("## Summary\n\n")
	b.WriteString("- passed: ")
	b.WriteString(strconv.Itoa(passed))
	b.WriteByte('\n')
	b.WriteString("- failed: ")
	b.WriteString(strconv.Itoa(failed))
	b.WriteByte('\n')
	b.WriteString("- total: ")
	b.WriteString(strconv.Itoa(len(result.Outcomes)))
	b.WriteString("\n\n## Results\n\n")
	for _, outcome := range result.Outcomes {
		b.WriteString("- ")
		if outcome.Status == OutcomePassed {
			b.WriteString("PASS")
		} else {
			b.WriteString("FAIL")
		}
		b.WriteByte(' ')
		b.WriteString(outcome.ID)
		if outcome.Message != "" {
			b.WriteString(": ")
			b.WriteString(outcome.Message)
		}
		b.WriteByte('\n')
	}
	return os.WriteFile(w.Resolve("outcomes/results.md"), []byte(w.redactor.Text(b.String())), 0o644)
}

func (w *Writer) RecentEvents(limit int) []Event {
	if limit <= 0 {
		return nil
	}
	f, err := os.Open(w.Resolve("events.ndjson"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var events []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		events = append(events, event)
		if len(events) > limit {
			copy(events, events[1:])
			events = events[:limit]
		}
	}
	return events
}

func (w *Writer) WriteAgentContext(s spec.Spec, result RunResult, finalScreen string, recentEvents []Event) error {
	content := RenderAgentContext(s, result, finalScreen, recentEvents)
	return os.WriteFile(w.Resolve("agent_context.md"), []byte(w.redactor.Text(content)), 0o644)
}

func (w *Writer) WriteDiagnostic(name string, content string) error {
	return os.WriteFile(w.Resolve("diagnostics/"+SafeName(name)+".md"), []byte(w.redactor.Text(content)), 0o644)
}

// WriteArtifactBytes writes a named artifact (download or transform output)
// to the run dir under `relPath`, redacting through the configured patterns.
// The caller resolves the relative path so the runner controls the on-disk
// layout (artifacts/<assign>/<saveAs> vs transforms/<assign>/<saveAs>).
func (w *Writer) WriteArtifactBytes(relPath string, data []byte) error {
	abs := w.Resolve(relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, w.redactor.Bytes(data), 0o644)
}

// LastFailedFile is the conventional filename the runner writes
// per-run, listing the names of specs that did NOT pass. The
// `glyph run --rerun-failed` flag reads this file to scope the
// next invocation to the failures. The file lives at the artifact
// root (one level above the run dir).
const LastFailedFile = ".last-failed.txt"

// WriteLastFailed records a list of spec names to the artifact
// root's .last-failed.txt. The previous file is replaced wholesale
// (not appended) so a re-run of a previously-passing spec clears
// the list. Names are written one per line, sorted, to make diffs
// readable.
func WriteLastFailed(artifactRoot string, names []string) error {
	if artifactRoot == "" {
		return nil
	}
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		return err
	}
	// Sort + dedup for stable diffs.
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(names))
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		cleaned = append(cleaned, n)
	}
	sort.Strings(cleaned)
	contents := strings.Join(cleaned, "\n")
	if contents != "" {
		contents += "\n"
	}
	return os.WriteFile(filepath.Join(artifactRoot, LastFailedFile), []byte(contents), 0o644)
}

// ReadLastFailed returns the list of spec names in the artifact
// root's .last-failed.txt, or an empty slice if the file doesn't
// exist. The runner uses this to scope `--rerun-failed`.
func ReadLastFailed(artifactRoot string) ([]string, error) {
	if artifactRoot == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(artifactRoot, LastFailedFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var out []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}

func writeJSON(path string, value any, redactor Redactor) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, redactor.Bytes(data), 0o644)
}

func writeYAML(path string, value any, redactor Redactor) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, redactor.Bytes(data), 0o644)
}

func SafeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "unnamed"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
