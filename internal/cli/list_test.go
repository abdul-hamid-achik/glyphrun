package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestListSpecs_ParsesAndFilters is the canonical smoke test for
// `glyph list`. It writes three specs into a temp dir with different
// metadata, walks the dir, and asserts the list returns them sorted
// (priority desc, then name asc) and that the filters narrow the
// result correctly.
func TestListSpecs_ParsesAndFilters(t *testing.T) {
	dir := t.TempDir()
	specs := map[string]string{
		"alpha.yml": `version: 1
name: alpha
metadata:
  feature: auth
  priority: high
  tags: [smoke, login]
intent: alpha
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: ok
    description: smoke check
    verify:
      command:
        run: "true"
`,
		"beta.yml": `version: 1
name: beta
metadata:
  feature: reporting
  priority: low
  tags: [smoke]
intent: beta
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: ok
    description: smoke check
    verify:
      command:
        run: "true"
`,
		"gamma.yml": `version: 1
name: gamma
metadata:
  feature: auth
  priority: normal
  tags: [login]
intent: gamma
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: ok
    description: smoke check
    verify:
      command:
        run: "true"
`,
	}
	for name, body := range specs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("no filter", func(t *testing.T) {
		rows, err := listSpecs([]string{dir}, &globalOptions{}, listFilters{})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 {
			t.Fatalf("expected 3 rows, got %d", len(rows))
		}
		// listSpecs doesn't sort (the cmd handler does). Apply the
		// same priority-then-name ordering so the test reflects what
		// the CLI surfaces.
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Priority != rows[j].Priority {
				return priorityRank(rows[i].Priority) > priorityRank(rows[j].Priority)
			}
			return rows[i].Name < rows[j].Name
		})
		// Priority desc: alpha (high) > gamma (normal) > beta (low).
		want := []string{"alpha", "gamma", "beta"}
		for i, row := range rows {
			if row.Name != want[i] {
				t.Errorf("row %d: got %q, want %q", i, row.Name, want[i])
			}
		}
	})

	t.Run("feature filter", func(t *testing.T) {
		rows, err := listSpecs([]string{dir}, &globalOptions{}, listFilters{Feature: "auth"})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 auth rows, got %d", len(rows))
		}
		for _, row := range rows {
			if row.Feature != "auth" {
				t.Errorf("row %q has feature %q, want auth", row.Name, row.Feature)
			}
		}
	})

	t.Run("tag filter", func(t *testing.T) {
		rows, err := listSpecs([]string{dir}, &globalOptions{}, listFilters{Tag: "login"})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 login rows, got %d", len(rows))
		}
	})

	t.Run("owner filter with no match", func(t *testing.T) {
		rows, err := listSpecs([]string{dir}, &globalOptions{}, listFilters{Owner: "nobody"})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0 rows for unmatched owner, got %d", len(rows))
		}
	})
}

// TestListSpecs_HandlesParseError writes one good spec and one broken
// spec into the same dir. The broken one must still surface in the
// table (with a parseError field) so the contributor sees the full
// surface, not a partial listing.
func TestListSpecs_HandlesParseError(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yml")
	if err := os.WriteFile(good, []byte(`version: 1
name: good
intent: a working spec
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: ok
    description: smoke check
    verify:
      command:
        run: "true"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.yml")
	if err := os.WriteFile(bad, []byte("not: [valid: yaml: :::\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := listSpecs([]string{dir}, &globalOptions{}, listFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (1 good + 1 with parseError), got %d", len(rows))
	}
	// Sort by priority: good is priority 0 (none set) > bad is 0 too.
	// We don't depend on order, just on the parseError flag.
	var goodRow, badRow *listRow
	for i := range rows {
		switch filepath.Base(rows[i].Path) {
		case "good.yml":
			goodRow = &rows[i]
		case "bad.yml":
			badRow = &rows[i]
		}
	}
	if goodRow == nil {
		t.Fatal("good spec missing from list")
	}
	if badRow == nil {
		t.Fatalf("bad spec missing from list; got %d rows", len(rows))
	}
	if badRow.ParseError == "" {
		t.Errorf("expected parseError on bad spec, got %+v", badRow)
	}
	if goodRow.ParseError != "" {
		t.Errorf("did not expect parseError on good spec, got %q", goodRow.ParseError)
	}
}

// TestListSpecs_JSONRoundTrip ensures the json output is well-formed
// and includes the metadata fields. CI dashboards consume the json
// form for filtering; this guards the schema.
func TestListSpecs_JSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.yml"), []byte(`version: 1
name: x
metadata:
  feature: smoke
  owner: release
  priority: high
  tags: [a, b]
intent: a spec
target:
  cmd: ["/bin/echo"]
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps: []
outcomes:
  - id: ok
    description: smoke check
    verify:
      command:
        run: "true"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := listSpecs([]string{dir}, &globalOptions{}, listFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	data, err := json.Marshal(rows[0])
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{`"feature":"smoke"`, `"owner":"release"`, `"priority":"high"`, `"a"`, `"b"`, `"contractHash"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json output missing %q: %s", want, out)
		}
	}
}

// TestPriorityRank_Stable locks the priority -> rank mapping so the
// list sort order doesn't drift between releases.
func TestPriorityRank_Stable(t *testing.T) {
	cases := map[string]int{
		"":         0,
		"low":      1,
		"normal":   2,
		"high":     3,
		"critical": 4,
		"CRITICAL": 4,
		"  high  ": 3,
	}
	for in, want := range cases {
		if got := priorityRank(in); got != want {
			t.Errorf("priorityRank(%q) = %d, want %d", in, got, want)
		}
	}
}
