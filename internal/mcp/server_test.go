package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/affected"
)

func TestServeToolsListLineJSON(t *testing.T) {
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")
	var output bytes.Buffer
	if err := Serve(context.Background(), input, &output, ServerOptions{}); err != nil {
		t.Fatal(err)
	}
	payload := responsePayload(t, output.String())
	var resp response
	if err := json.Unmarshal(payload, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}
	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("glyph_run")) {
		t.Fatalf("tools/list missing glyph_run: %s", string(data))
	}
}

func TestServeDocsWorksWithoutDocsDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"glyph_docs","arguments":{"topic":"agents"}}}` + "\n")
	var output bytes.Buffer
	if err := Serve(context.Background(), input, &output, ServerOptions{}); err != nil {
		t.Fatal(err)
	}
	payload := responsePayload(t, output.String())
	var resp response
	if err := json.Unmarshal(payload, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}
	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("glyph context latest --format md")) {
		t.Fatalf("docs response missing agent workflow: %s", string(data))
	}
}

// TestServeExplainSurfacesMonitorVocabulary guards that the MCP explain tool
// advertises the monitor step, metrics verifier, and the new commands so an
// agent harness learns the current surface from one call.
func TestServeExplainSurfacesMonitorVocabulary(t *testing.T) {
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"glyph_explain","arguments":{}}}` + "\n")
	var output bytes.Buffer
	if err := Serve(context.Background(), input, &output, ServerOptions{}); err != nil {
		t.Fatal(err)
	}
	data := responsePayload(t, output.String())
	for _, want := range []string{"monitor", "metrics", "affected-specs", "coversSymbol"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("glyph_explain missing %q in vocabulary", want)
		}
	}
}

// TestServeScaffoldCoversSymbol guards the scaffold tool injects the binding.
func TestServeScaffoldCoversSymbol(t *testing.T) {
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"glyph_spec_scaffold","arguments":{"coversSymbol":"Handler.ServeHTTP"}}}` + "\n")
	var output bytes.Buffer
	if err := Serve(context.Background(), input, &output, ServerOptions{}); err != nil {
		t.Fatal(err)
	}
	data := responsePayload(t, output.String())
	if !bytes.Contains(data, []byte("coversSymbol: Handler.ServeHTTP")) {
		t.Fatalf("scaffold missing coversSymbol binding: %s", string(data))
	}
}

// TestServeAffectedSpecs drives the glyph_affected_specs MCP tool with a
// fake codemap binary: it loads specs from a temp dir, intersects their
// coversSymbol against the canned review, and returns the matched report.
func TestServeAffectedSpecs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake codemap shell script is Unix-only")
	}
	dir := t.TempDir()
	mk := func(name, covers string) {
		body := "version: 1\nname: " + name + "\n"
		if covers != "" {
			body += "coversSymbol: " + covers + "\n"
		}
		body += "intent: " + name + "\ntarget:\n  cmd: [\"/bin/echo\"]\nterminal:\n  cols: 80\n  rows: 24\n  profile: xterm-256color\nsteps: []\noutcomes:\n  - id: ok\n    description: smoke\n    verify:\n      command:\n        run: \"true\"\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("run.yml", "Run")
	mk("miss.yml", "Missing")
	fake := filepath.Join(t.TempDir(), "fake-codemap")
	script := "#!/bin/sh\ncat <<'EOF'\n{\"changed_symbols\":[{\"symbol\":\"Run\",\"fqn\":\"app.Run\"}],\"blast_radius\":[]}\nEOF\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"glyph_affected_specs","arguments":{"paths":["` + dir + `"],"since":"HEAD^","codemap":"` + fake + `"}}}`
	input := strings.NewReader(req + "\n")
	var output bytes.Buffer
	if err := Serve(context.Background(), input, &output, ServerOptions{}); err != nil {
		t.Fatal(err)
	}
	payload := responsePayload(t, output.String())
	var resp response
	if err := json.Unmarshal(payload, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}
	// The tool returns toolText(report) — a content[0].text holding the
	// JSON-encoded affected.Report. Parse it rather than substring-matching
	// the escaped inner JSON.
	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not an object: %#v", resp.Result)
	}
	content, _ := resultMap["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("result has no content: %#v", resp.Result)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	var report affected.Report
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatalf("unmarshal report: %v\n%s", err, text)
	}
	if report.Matched != 1 || report.Unmatched != 1 || report.Total != 2 {
		t.Fatalf("matched=%d unmatched=%d total=%d, want 1/1/2", report.Matched, report.Unmatched, report.Total)
	}
	if len(report.Specs) != 1 || report.Specs[0].CoversSymbol != "Run" || report.Specs[0].MatchedBy != "changed" {
		t.Fatalf("selected spec = %+v, want Run matchedBy changed", report.Specs)
	}
}

// responsePayload extracts the JSON-RPC payload from a server response. The
// server writes newline-delimited JSON-RPC (the stdio MCP convention used by
// Claude Code, Codex, OpenCode, and Claude Desktop). For robustness against
// future framing changes, this helper also accepts Content-Length-framed
// output by splitting on the blank-line separator.
func responsePayload(t *testing.T, framed string) []byte {
	t.Helper()
	trimmed := strings.TrimRight(framed, "\n")
	if strings.HasPrefix(strings.ToLower(trimmed), "content-length:") {
		parts := strings.SplitN(trimmed, "\r\n\r\n", 2)
		if len(parts) != 2 {
			t.Fatalf("response was content-length framed but missing body: %q", framed)
		}
		return []byte(parts[1])
	}
	return []byte(trimmed)
}

// fakeFcheapArchive writes a shell script that records every invocation
// (the full argv, `store <runDir>`) to logPath, one per line, and exits 0.
// Used to drive glyph_clean's archival path end-to-end without a real
// fcheap/file.cheap install. Skipped on non-Unix where /bin/sh is absent.
func fakeFcheapArchive(t *testing.T, logPath string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake fcheap shell script is Unix-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-fcheap")
	script := "#!/bin/sh\necho \"$@\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeCleanConfig writes a glyphrun.config.yml that points at the given
// absolute artifact root, keeps keepRuns newest runs, and routes pruned
// dirs through the given archive command (`args: [store]`).
func writeCleanConfig(t *testing.T, cfgPath, artifactRoot, archiveCmd string, keepRuns int) {
	t.Helper()
	yml := fmt.Sprintf("version: 1\nartifactRoot: %s\nretention:\n  keepRuns: %d\n  archive:\n    enabled: true\n    command: %s\n    args:\n      - store\n",
		artifactRoot, keepRuns, archiveCmd)
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
}

// stageRuns creates n run directories under root with deterministic, strictly
// increasing mtimes so PruneRuns' newest-first sort is stable. Names follow
// the runner's `YYYY-MM-DDTHH-MM-SSZ-...` convention so CleanAll also picks
// them up. Returns the directory paths in creation (oldest→newest) order.
func stageRuns(t *testing.T, root string, n int) []string {
	t.Helper()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dirs := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("2026-01-01T00-00-%02dZ-r%d", i, i)
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		mt := base.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(dir, mt, mt); err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, dir)
	}
	return dirs
}

// cleanReportJSON mirrors artifacts.CleanReport as seen in the tool's
// content[0].text payload.
type cleanReportJSON struct {
	Pruned        int      `json:"pruned"`
	Kept          int      `json:"kept"`
	Paths         []string `json:"paths,omitempty"`
	Archived      int      `json:"archived,omitempty"`
	ArchiveErrors []string `json:"archiveErrors,omitempty"`
}

// callCleanTool invokes the glyph_clean MCP tool with the given arguments
// and returns the parsed report. Fails the test on any JSON-RPC error.
func callCleanTool(t *testing.T, args map[string]any, cfgPath string) cleanReportJSON {
	t.Helper()
	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"glyph_clean","arguments":` + string(argsJSON) + `}}`
	input := strings.NewReader(req + "\n")
	var output bytes.Buffer
	if err := Serve(context.Background(), input, &output, ServerOptions{ConfigPath: cfgPath}); err != nil {
		t.Fatal(err)
	}
	payload := responsePayload(t, output.String())
	var resp response
	if err := json.Unmarshal(payload, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}
	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not an object: %#v", resp.Result)
	}
	content, _ := resultMap["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("result has no content: %#v", resp.Result)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	var report cleanReportJSON
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatalf("unmarshal report: %v\n%s", err, text)
	}
	return report
}

// TestServeCleanPrunesAndArchives drives glyph_clean with a config that
// keeps 2 of 5 staged runs and routes pruned dirs through a fake fcheap.
// The 3 oldest are pruned, each archived first (fcheap records `store
// <runDir>`), and the 2 newest survive on disk.
func TestServeCleanPrunesAndArchives(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake fcheap shell script is Unix-only")
	}
	tmp := t.TempDir()
	runsRoot := filepath.Join(tmp, "runs")
	logPath := filepath.Join(tmp, "archive.log")
	fcheap := fakeFcheapArchive(t, logPath)
	cfgPath := filepath.Join(tmp, "glyphrun.config.yml")
	writeCleanConfig(t, cfgPath, runsRoot, fcheap, 2)
	staged := stageRuns(t, runsRoot, 5)

	report := callCleanTool(t, map[string]any{}, cfgPath)

	if report.Pruned != 3 {
		t.Errorf("Pruned = %d, want 3", report.Pruned)
	}
	if report.Kept != 2 {
		t.Errorf("Kept = %d, want 2", report.Kept)
	}
	if report.Archived != 3 {
		t.Errorf("Archived = %d, want 3", report.Archived)
	}
	if len(report.ArchiveErrors) != 0 {
		t.Errorf("ArchiveErrors = %v, want empty", report.ArchiveErrors)
	}
	// Oldest 3 pruned locally; newest 2 kept.
	for _, dir := range staged[:3] {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("pruned dir %s still exists", dir)
		}
	}
	for _, dir := range staged[3:] {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("kept dir %s removed: %v", dir, err)
		}
	}
	// fcheap invoked once per pruned dir, recording `store <runDir>`.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("fcheap log not written: %v", err)
	}
	for _, dir := range staged[:3] {
		want := "store " + dir
		if !strings.Contains(string(data), want) {
			t.Errorf("fcheap log missing %q\n%s", want, data)
		}
	}
}

// TestServeCleanAll drives glyph_clean with all=true so every run dir is
// removed via CleanAll (no archival). All 5 run-dir-named entries are
// removed and Pruned reflects the count.
func TestServeCleanAll(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake fcheap shell script is Unix-only")
	}
	tmp := t.TempDir()
	runsRoot := filepath.Join(tmp, "runs")
	// Archive command is configured but must NOT be invoked under --all.
	fcheap := fakeFcheapArchive(t, filepath.Join(tmp, "archive.log"))
	cfgPath := filepath.Join(tmp, "glyphrun.config.yml")
	writeCleanConfig(t, cfgPath, runsRoot, fcheap, 2)
	staged := stageRuns(t, runsRoot, 5)

	report := callCleanTool(t, map[string]any{"all": true}, cfgPath)

	if report.Pruned != 5 {
		t.Errorf("Pruned = %d, want 5", report.Pruned)
	}
	for _, dir := range staged {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("run dir %s still exists after --all", dir)
		}
	}
}

// TestServeCleanNoArchive drives glyph_clean with noArchive=true: pruned
// dirs are deleted locally, the configured fcheap is NOT invoked (its log
// stays absent/empty), and the report records zero archived.
func TestServeCleanNoArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake fcheap shell script is Unix-only")
	}
	tmp := t.TempDir()
	runsRoot := filepath.Join(tmp, "runs")
	logPath := filepath.Join(tmp, "archive.log")
	fcheap := fakeFcheapArchive(t, logPath)
	cfgPath := filepath.Join(tmp, "glyphrun.config.yml")
	writeCleanConfig(t, cfgPath, runsRoot, fcheap, 2)
	staged := stageRuns(t, runsRoot, 5)

	report := callCleanTool(t, map[string]any{"noArchive": true}, cfgPath)

	if report.Pruned != 3 {
		t.Errorf("Pruned = %d, want 3", report.Pruned)
	}
	if report.Archived != 0 {
		t.Errorf("Archived = %d, want 0", report.Archived)
	}
	// Oldest 3 pruned locally; newest 2 kept.
	for _, dir := range staged[:3] {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("pruned dir %s still exists", dir)
		}
	}
	for _, dir := range staged[3:] {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("kept dir %s removed: %v", dir, err)
		}
	}
	// fcheap must not have been invoked.
	if data, err := os.ReadFile(logPath); err == nil && len(data) > 0 {
		t.Errorf("fcheap invoked under noArchive; log:\n%s", data)
	}
}
