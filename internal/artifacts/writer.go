package artifacts

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

type Writer struct {
	RunDir  string
	manager *ArtifactManager
	initErr error
}

func NewWriter(runDir string, redactor Redactor) *Writer {
	manager, err := NewArtifactManager(runDir, redactor)
	if err != nil {
		return &Writer{RunDir: runDir, initErr: err}
	}
	return &Writer{RunDir: manager.Root(), manager: manager}
}

func (w *Writer) EnsureDirs() error {
	if w.initErr != nil {
		return w.initErr
	}
	if err := w.manager.EnsureRoot(); err != nil {
		return err
	}
	for _, dir := range []string{
		"screens",
		"raw",
		"frames",
		"snapshots",
		"outcomes",
		"diagnostics",
		"artifacts",
		"transforms",
	} {
		if err := w.manager.EnsureDir(dir); err != nil {
			return err
		}
	}
	return nil
}

// Resolve preserves the existing convenience API for trusted constant paths.
// New variable-path call sites must use ResolvePath so confinement errors are
// surfaced instead of discarded.
func (w *Writer) Resolve(rel string) string {
	path, _ := w.ResolvePath(rel)
	return path
}

func (w *Writer) ResolvePath(rel string) (string, error) {
	if w.initErr != nil {
		return "", w.initErr
	}
	return w.manager.Resolve(rel)
}

func (w *Writer) WriteRun(result RunResult) error {
	if err := w.manager.WriteJSON("run.json", result); err != nil {
		return err
	}
	if err := w.manager.WriteYAML("run.yaml", result); err != nil {
		return err
	}
	return w.manager.WriteText("run.md", RenderRunMarkdown(result))
}

// WriteReplay writes the exact-replay manifest (SPEC §7.3) as replay.json,
// redacted like every other artifact so no env value leaks (only key names
// are ever present in the manifest, but the redactor is still applied).
func (w *Writer) WriteReplay(m ReplayManifest) error {
	return w.manager.WriteJSON("replay.json", m)
}

func (w *Writer) WriteResolvedSpec(s spec.Spec) error {
	return w.manager.WriteYAML("spec.resolved.yml", s)
}

func (w *Writer) AppendEvent(event Event) error {
	return w.manager.AppendJSONLine("events.ndjson", event)
}

func (w *Writer) WriteFinalScreen(snapshot terminal.ScreenSnapshot) error {
	if err := w.manager.WriteText("screens/final.txt", snapshot.Text+"\n"); err != nil {
		return err
	}
	return w.manager.WriteJSON("screens/final.json", snapshot)
}

// WriteScreenSVG writes a rendered SVG screenshot to relPath under the run
// dir. The SVG text is redacted like every other artifact so configured
// secret values don't leak into a picture of the screen.
func (w *Writer) WriteScreenSVG(relPath string, svg string) error {
	return w.manager.WriteText(relPath, svg)
}

func (w *Writer) WriteRawPTY(raw []byte) error {
	return w.manager.WriteRedactedBytes("raw/pty.raw.log", "text", raw)
}

func (w *Writer) WriteInputLog(raw []byte) error {
	return w.manager.WriteRedactedBytes("raw/input.raw.log", "text", raw)
}

func (w *Writer) WriteFrames(frames []terminal.Frame) error {
	return w.manager.WriteRedactedStream("frames/frames.ndjson", "ndjson", func(emit func([]byte) error) error {
		for _, frame := range frames {
			data, err := json.Marshal(frame)
			if err != nil {
				return err
			}
			data = append(data, '\n')
			if err := emit(data); err != nil {
				return err
			}
		}
		return nil
	})
}

func (w *Writer) WriteSnapshot(name string, snapshot terminal.ScreenSnapshot) error {
	safe := SafeName(name)
	if err := w.manager.WriteText("snapshots/"+safe+".txt", snapshot.Text+"\n"); err != nil {
		return err
	}
	return w.manager.WriteJSON("snapshots/"+safe+".json", snapshot)
}

func (w *Writer) WriteOutcome(result OutcomeResult, raw any) error {
	safe := SafeName(result.ID)
	md := "# Outcome: " + result.ID + "\n\n" +
		"- status: " + string(result.Status) + "\n" +
		"- message: " + result.Message + "\n"
	if result.Evidence != "" {
		md += "- evidence: " + result.Evidence + "\n"
	}
	if err := w.manager.WriteText("outcomes/"+safe+".md", md); err != nil {
		return err
	}
	if raw != nil {
		return w.manager.WriteJSON("outcomes/"+safe+".raw.json", raw)
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
	return w.manager.WriteJSON("outcomes/"+safe+".raw.json", raw)
}

func (w *Writer) WriteOutcomesIndex(result RunResult) error {
	summary := map[string]any{
		"runId":    result.RunID,
		"status":   result.Status,
		"outcomes": result.Outcomes,
	}
	if err := w.manager.WriteJSON("outcomes/results.json", summary); err != nil {
		return err
	}
	if err := w.manager.WriteYAML("outcomes/results.yaml", summary); err != nil {
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
	return w.manager.WriteText("outcomes/results.md", b.String())
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
	return w.manager.WriteText("agent_context.md", content)
}

func (w *Writer) WriteDiagnostic(name string, content string) error {
	return w.manager.WriteText("diagnostics/"+SafeName(name)+".md", content)
}

// WriteArtifactBytes writes a bounded in-memory named artifact while
// preserving the legacy redaction contract.
func (w *Writer) WriteArtifactBytes(relPath string, data []byte) error {
	return w.manager.WriteRedactedBytes(relPath, "binary", data)
}

// CopyArtifact streams a named artifact through bounded redaction I/O.
func (w *Writer) CopyArtifact(relPath string, src io.Reader) error {
	return w.manager.CopyRedacted(relPath, "binary", src)
}

// RegisterArtifact adds an externally produced transform output to the
// deterministic manifest.
func (w *Writer) RegisterArtifact(relPath string) error {
	return w.manager.RegisterFile(relPath, "binary")
}

// WriteText writes a confined, redacted run-relative text artifact.
func (w *Writer) WriteText(relPath string, content string) error {
	return w.manager.WriteText(relPath, content)
}

// FinalizeManifest snapshots and writes manifest.json, and exposes the same
// entries additively on the run result.
func (w *Writer) FinalizeManifest(result *RunResult) error {
	entries := w.manager.Manifest()
	result.Manifest = entries
	if result.Artifacts == nil {
		result.Artifacts = make(map[string]string)
	}
	result.Artifacts["manifest"] = "manifest.json"
	return w.manager.WriteManifest("manifest.json", entries)
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
