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

func responsePayload(t *testing.T, framed string) []byte {
	t.Helper()
	parts := strings.SplitN(framed, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("response was not framed: %q", framed)
	}
	return []byte(parts[1])
}
