package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestExplainMarkdownMatchesJSONVocabulary guards against the two renders of
// `glyph explain` drifting apart: an agent reading the markdown must see the
// same step and verifier names the JSON envelope advertises.
func TestExplainMarkdownMatchesJSONVocabulary(t *testing.T) {
	var jsonOut bytes.Buffer
	cmd := newRootCommand(&globalOptions{format: "json"})
	cmd.SetOut(&jsonOut)
	cmd.SetArgs([]string{"--format", "json", "explain"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Steps     []string `json:"steps"`
		Verifiers []string `json:"verifiers"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, jsonOut.String())
	}
	var mdOut bytes.Buffer
	cmd = newRootCommand(&globalOptions{format: "md", noColor: true})
	cmd.SetOut(&mdOut)
	cmd.SetArgs([]string{"--format", "md", "--no-color", "explain"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	md := mdOut.String()
	for _, name := range append(append([]string{}, payload.Steps...), payload.Verifiers...) {
		if !strings.Contains(md, name) {
			t.Errorf("markdown omits %q, which the JSON envelope advertises", name)
		}
	}
	for _, must := range []string{"monitor", "metrics"} {
		if !strings.Contains(md, must) {
			t.Errorf("markdown omits %q", must)
		}
	}
}
