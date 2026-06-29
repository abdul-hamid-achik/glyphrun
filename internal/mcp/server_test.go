package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
