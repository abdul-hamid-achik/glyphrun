package artifacts

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	safe := safeName(name)
	if err := os.WriteFile(w.Resolve("snapshots/"+safe+".txt"), []byte(w.redactor.Text(snapshot.Text)+"\n"), 0o644); err != nil {
		return err
	}
	return writeJSON(w.Resolve("snapshots/"+safe+".json"), snapshot, w.redactor)
}

func (w *Writer) WriteOutcome(result OutcomeResult, raw any) error {
	safe := safeName(result.ID)
	md := "# Outcome: " + result.ID + "\n\n" +
		"- status: " + string(result.Status) + "\n" +
		"- message: " + result.Message + "\n"
	if err := os.WriteFile(w.Resolve("outcomes/"+safe+".md"), []byte(w.redactor.Text(md)), 0o644); err != nil {
		return err
	}
	if raw != nil {
		return writeJSON(w.Resolve("outcomes/"+safe+".raw.json"), raw, w.redactor)
	}
	return nil
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
	for _, outcome := range result.Outcomes {
		b.WriteString("- ")
		b.WriteString(string(outcome.Status))
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

func (w *Writer) WriteAgentContext(s spec.Spec, result RunResult, finalScreen string) error {
	content := RenderAgentContext(s, result, finalScreen)
	return os.WriteFile(w.Resolve("agent_context.md"), []byte(w.redactor.Text(content)), 0o644)
}

func (w *Writer) WriteDiagnostic(name string, content string) error {
	return os.WriteFile(w.Resolve("diagnostics/"+safeName(name)+".md"), []byte(w.redactor.Text(content)), 0o644)
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

func safeName(name string) string {
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
