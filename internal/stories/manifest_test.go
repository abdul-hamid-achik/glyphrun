package stories

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

const sampleManifest = `version: 1
kind: stories
name: demo
harness:
  cmd: ["./bin/stories"]
  build: "go build -o ./bin/stories ./stories"
  watch: ["stories"]
defaults:
  terminal: { cols: 80, rows: 24, alternateScreen: require }
  tags: [ui]
stories:
  - id: list/rows
    ready: { contains: "hello" }
    tags: [rows]
    outcomes:
      - id: selected
        description: first row selected
        verify:
          region: { x: 0, y: 2, width: 20, height: 1, contains: "> hello" }
    variants:
      - name: wide
        terminal: { cols: 120, rows: 40 }
        env: { THEME: light }
  - id: agent/error
    intent: the agent story shows a PTY failure
    golden: false
    quit: ""
`

func TestParseManifestRejectsUnknownFieldsAndDuplicates(t *testing.T) {
	if _, err := ParseManifest([]byte(sampleManifest+"  - id: list/rows\n"), "stories.yml", spec.ParseOptions{}); err == nil || !strings.Contains(err.Error(), "duplicate story id") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
	bad := strings.Replace(sampleManifest, "harness:", "harnesss:", 1)
	if _, err := ParseManifest([]byte(bad), "stories.yml", spec.ParseOptions{}); err == nil {
		t.Fatal("expected schema error for unknown top-level field")
	}
	if _, err := ParseManifest([]byte("version: 1\nkind: spec\nharness: {cmd: [x]}\nstories: [{id: a}]\n"), "s.yml", spec.ParseOptions{}); err == nil {
		t.Fatal("expected kind error")
	}
}

func TestExpandBuildsSpecsAndVariants(t *testing.T) {
	m, err := ParseManifest([]byte(sampleManifest), "/repo/stories.yml", spec.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	def := spec.Terminal{Cols: 100, Rows: 30, Profile: "xterm-256color"}
	ex, err := Expand(m, "/repo/stories.yml", def)
	if err != nil {
		t.Fatal(err)
	}
	if len(ex) != 3 {
		t.Fatalf("expanded %d stories, want 3 (base + variant + second)", len(ex))
	}
	rows := ex[0]
	if rows.Key() != "list/rows" || rows.Spec.Name != "story_list_rows" || rows.SnapshotName != "rows" || rows.Feature != "list" {
		t.Fatalf("base story identity = %+v", rows)
	}
	if got := rows.Spec.Target.Cmd; len(got) != 2 || got[0] != "./bin/stories" || got[1] != "list/rows" {
		t.Fatalf("cmd = %v", got)
	}
	if rows.Spec.Terminal.Cols != 80 || rows.Spec.Terminal.Rows != 24 || rows.Spec.Terminal.Profile != "xterm-256color" || rows.Spec.Terminal.AlternateScreen != "require" {
		t.Fatalf("terminal = %+v", rows.Spec.Terminal)
	}
	if !hasTag(rows.Spec.Metadata.Tags, "story") || !hasTag(rows.Spec.Metadata.Tags, "ui") || !hasTag(rows.Spec.Metadata.Tags, "rows") {
		t.Fatalf("tags = %v", rows.Spec.Metadata.Tags)
	}
	kinds := make([]string, 0, len(rows.Spec.Steps))
	for _, st := range rows.Spec.Steps {
		switch {
		case st.Wait != nil && st.Wait.Screen != nil:
			kinds = append(kinds, "wait-screen")
		case st.Wait != nil && st.Wait.Process != nil:
			kinds = append(kinds, "wait-exit")
		case st.Snapshot != "":
			kinds = append(kinds, "snapshot:"+st.Snapshot)
		case st.Press != "":
			kinds = append(kinds, "press:"+st.Press)
		}
	}
	if strings.Join(kinds, " ") != "wait-screen snapshot:rows press:q wait-exit" {
		t.Fatalf("steps = %v", kinds)
	}
	ids := make([]string, 0, len(rows.Spec.Outcomes))
	for _, o := range rows.Spec.Outcomes {
		ids = append(ids, o.ID)
	}
	if strings.Join(ids, " ") != "golden ready selected" {
		t.Fatalf("outcomes = %v", ids)
	}
	if rows.Spec.ContractHash == "" || !rows.Parsed.ContractHashValid || rows.Parsed.Path != "/repo/stories.yml" {
		t.Fatalf("parsed = %+v", rows.Parsed)
	}

	wide := ex[1]
	if wide.Key() != "list/rows@wide" || wide.Spec.Name != "story_list_rows__wide" || wide.Variant != "wide" {
		t.Fatalf("variant identity = %+v", wide)
	}
	if wide.Spec.Terminal.Cols != 120 || wide.Spec.Terminal.Rows != 40 || wide.Spec.Target.Env["THEME"] != "light" {
		t.Fatalf("variant overrides = %+v %v", wide.Spec.Terminal, wide.Spec.Target.Env)
	}
	if !hasTag(wide.Spec.Metadata.Tags, "variant:wide") {
		t.Fatalf("variant tag missing: %v", wide.Spec.Metadata.Tags)
	}

	agent := ex[2]
	if agent.Golden || agent.Spec.Name != "story_agent_error" {
		t.Fatalf("agent = %+v", agent)
	}
	for _, st := range agent.Spec.Steps {
		if st.Press != "" {
			t.Fatalf("quit \"\" must not press a key: %+v", st)
		}
		if st.Wait != nil && st.Wait.Idle == nil && st.Wait.Screen == nil && st.Wait.Process != nil {
			t.Fatalf("quit \"\" must not wait for exit: %+v", st)
		}
	}
	if len(agent.Spec.Outcomes) != 1 || agent.Spec.Outcomes[0].ID != "mounted" {
		t.Fatalf("golden=false without outcomes should synthesize a mounted outcome: %+v", agent.Spec.Outcomes)
	}
	if !strings.Contains(agent.Spec.Intent, "PTY failure") {
		t.Fatalf("intent = %q", agent.Spec.Intent)
	}
}

func TestFindManifestsByNameAndKind(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSpec(t, filepath.Join(dir, "ui"), "stories.yml", sampleManifest)
	writeSpec(t, dir, "list.stories.yaml", sampleManifest)
	writeSpec(t, dir, "custom.yml", sampleManifest)
	writeSpec(t, dir, "plain.yml", sampleSpec("plain", "x", "[story]"))
	found, err := FindManifests([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("dir walk should find the two conventionally named manifests, got %v", found)
	}
	explicit, err := FindManifests([]string{filepath.Join(dir, "custom.yml")})
	if err != nil {
		t.Fatal(err)
	}
	if len(explicit) != 1 {
		t.Fatalf("explicit file with kind: stories should be a manifest, got %v", explicit)
	}
	none, err := FindManifests([]string{filepath.Join(dir, "plain.yml")})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("plain spec is not a manifest: %v", none)
	}
}

func TestSpecNameForStory(t *testing.T) {
	cases := map[[2]string]string{
		{"list/rows", ""}:        "story_list_rows",
		{"list/rows", "wide"}:    "story_list_rows__wide",
		{"Agent Chat/Empty", ""}: "story_agent-chat_empty",
	}
	for in, want := range cases {
		if got := SpecNameForStory(in[0], in[1]); got != want {
			t.Errorf("SpecNameForStory(%q,%q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestExpandRejectsSpecNameCollisions(t *testing.T) {
	m, err := ParseManifest([]byte("version: 1\nkind: stories\nharness:\n  cmd: [x]\nstories:\n  - id: list/rows\n  - id: list_rows\n"), "stories.yml", spec.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Expand(m, "stories.yml", spec.Terminal{Cols: 80, Rows: 24}); err == nil || !strings.Contains(err.Error(), "story_list_rows") {
		t.Fatalf("expected spec name collision error, got %v", err)
	}
}

func TestGoldenModePlumbsIntoTheSnapshotVerifier(t *testing.T) {
	m, err := ParseManifest([]byte("version: 1\nkind: stories\nharness:\n  cmd: [x]\ndefaults:\n  goldenMode: cell\nstories:\n  - id: a/b\n  - id: a/c\n    goldenMode: json\n"), "stories.yml", spec.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ex, err := Expand(m, "stories.yml", spec.Terminal{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if got := ex[0].Spec.Outcomes[0].Verify.Snapshot.Mode; got != "cell" {
		t.Fatalf("defaults.goldenMode not applied: %q", got)
	}
	if got := ex[1].Spec.Outcomes[0].Verify.Snapshot.Mode; got != "json" {
		t.Fatalf("story goldenMode not applied: %q", got)
	}
	if _, _, mode := GoldenOutcomeMode(ex[1].Spec); mode != "json" {
		t.Fatalf("GoldenOutcomeMode = %q", mode)
	}
	if _, err := ParseManifest([]byte("version: 1\nkind: stories\nharness:\n  cmd: [x]\ndefaults:\n  goldenMode: pixels\nstories:\n  - id: a\n"), "stories.yml", spec.ParseOptions{}); err == nil {
		t.Fatal("schema should reject an unknown goldenMode")
	}
}
