package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
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
