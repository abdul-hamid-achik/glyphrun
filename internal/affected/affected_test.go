package affected

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func fakeCodemap(t *testing.T, output string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake codemap shell script is Unix-only")
	}
	path := filepath.Join(t.TempDir(), "codemap")
	script := "#!/bin/sh\ncat <<'EOF'\n" + output + "\nEOF\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunReview_ProducerGoldenParsesAndSelects(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", "codemap.review.v1.json"))
	if err != nil {
		t.Fatal(err)
	}

	review, err := RunReview(fakeCodemap(t, string(golden)), "working", "", 3)
	if err != nil {
		t.Fatalf("RunReview: %v", err)
	}
	if review.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", review.SchemaVersion)
	}
	if len(review.ChangedSymbols) != 1 || review.ChangedSymbols[0].Symbol != "Run" || review.ChangedSymbols[0].FQN != "app.Run" {
		t.Fatalf("ChangedSymbols = %+v, want Run/app.Run", review.ChangedSymbols)
	}
	if len(review.BlastRadius) != 2 || review.BlastRadius[0].Symbol != "TestRun" || review.BlastRadius[1].FQN != "app.Other" {
		t.Fatalf("BlastRadius = %+v, want TestRun and app.Other", review.BlastRadius)
	}

	report := Select([]Spec{
		{Name: "changed", Path: "changed.yml", CoversSymbol: "app.Run"},
		{Name: "test", Path: "test.yml", CoversSymbol: "TestRun"},
		{Name: "caller", Path: "caller.yml", CoversSymbol: "app.Other"},
		{Name: "miss", Path: "miss.yml", CoversSymbol: "Missing"},
	}, review)
	if report.Matched != 3 || report.Unmatched != 1 {
		t.Fatalf("matched/unmatched = %d/%d, want 3/1: %+v", report.Matched, report.Unmatched, report.Specs)
	}
	for _, entry := range report.Specs {
		want := "blast"
		if entry.Path == "changed.yml" {
			want = "changed"
		}
		if entry.MatchedBy != want {
			t.Errorf("%s matchedBy = %q, want %q", entry.Path, entry.MatchedBy, want)
		}
	}
}

func TestRunReview_AcceptsUnversionedLegacySchema(t *testing.T) {
	review, err := RunReview(fakeCodemap(t, `{"changed_symbols":[{"symbol":"Legacy"}],"blast_radius":[]}`), "working", "", 3)
	if err != nil {
		t.Fatalf("RunReview: %v", err)
	}
	if review.SchemaVersion != 0 || len(review.ChangedSymbols) != 1 || review.ChangedSymbols[0].Symbol != "Legacy" {
		t.Fatalf("review = %+v, want unversioned legacy schema with Legacy symbol", review)
	}
}

func TestRunReview_RejectsUnsupportedSchemaVersion(t *testing.T) {
	review, err := RunReview(fakeCodemap(t, `{"schema_version":2,"changed_symbols":[],"blast_radius":[]}`), "working", "", 3)
	if err == nil {
		t.Fatal("RunReview returned nil error for schema_version 2")
	}
	if !strings.Contains(err.Error(), "unsupported codemap review schema_version 2") {
		t.Fatalf("error = %q, want explicit unsupported schema_version 2 error", err)
	}
	if len(review.ChangedSymbols) != 0 || len(review.BlastRadius) != 0 {
		t.Fatalf("review = %+v, want zero value on unsupported schema", review)
	}
}

func TestRunReview_RejectsExplicitLegacyAndMalformedV1(t *testing.T) {
	fixtures := []string{
		`{"schema_version":0,"changed_symbols":[],"blast_radius":[]}`,
		`{"schema_version":null,"changed_symbols":[],"blast_radius":[]}`,
		`{"schema_version":1,"indexed":true}`,
		`{"schema_version":1,"project":"x","mode":"working","depth":3,"is_repo":true,"indexed":true,"changed_files":[],"changed_symbols":[{"name":"Run"}],"blast_radius":[],"covering_tests":[],"untested_symbols":[],"stale":false}`,
	}
	for _, fixture := range fixtures {
		review, err := RunReview(fakeCodemap(t, fixture), "working", "", 3)
		if err == nil {
			t.Errorf("RunReview accepted incompatible v1 payload: %s", fixture)
		}
		if len(review.ChangedSymbols) != 0 || len(review.BlastRadius) != 0 {
			t.Errorf("malformed v1 returned selection data: %+v", review)
		}
	}
}

// TestSelect is the pure-logic test for the intersection: direct-change match,
// blast-radius match, both, fqn match, no-match (unmatched), no-coversSymbol
// (noCover), and parse-error skip. It never invokes codemap.
func TestSelect(t *testing.T) {
	rows := []Spec{
		{Name: "run_spec", Path: "run.yml", CoversSymbol: "Run"},
		{Name: "other_spec", Path: "other.yml", CoversSymbol: "Other"},
		{Name: "both_spec", Path: "both.yml", CoversSymbol: "main.Handle"},
		{Name: "miss_spec", Path: "miss.yml", CoversSymbol: "Missing"},
		{Name: "nocover_spec", Path: "nocover.yml"},
		{Name: "broken_spec", Path: "broken.yml", ParseError: "bad yaml"},
	}
	review := Review{
		ChangedSymbols: []ReviewSymbol{
			{Symbol: "Run", FQN: "app.Run"},
			{Symbol: "Handle", FQN: "main.Handle"},
		},
		BlastRadius: []ReviewSymbol{
			{Symbol: "Other", FQN: "app.Other"},
			{Symbol: "Handle", FQN: "main.Handle"},
		},
	}
	report := Select(rows, review)

	if report.Total != 5 {
		t.Errorf("Total = %d, want 5 (broken_spec skipped, not counted)", report.Total)
	}
	if report.Matched != 3 {
		t.Fatalf("Matched = %d, want 3: %+v", report.Matched, report.Specs)
	}
	if report.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1 (miss_spec)", report.Unmatched)
	}
	if report.NoCover != 1 {
		t.Errorf("NoCover = %d, want 1 (nocover_spec)", report.NoCover)
	}
	byPath := map[string]string{}
	for _, s := range report.Specs {
		byPath[s.Path] = s.MatchedBy
	}
	if byPath["run.yml"] != "changed" {
		t.Errorf("run.yml matchedBy = %q, want changed", byPath["run.yml"])
	}
	if byPath["other.yml"] != "blast" {
		t.Errorf("other.yml matchedBy = %q, want blast", byPath["other.yml"])
	}
	if byPath["both.yml"] != "both" {
		t.Errorf("both.yml matchedBy = %q, want both (main.Handle in changed + blast)", byPath["both.yml"])
	}
	gotOrder := make([]string, 0, len(report.Specs))
	for _, s := range report.Specs {
		gotOrder = append(gotOrder, s.Path)
	}
	if strings.Join(gotOrder, ",") != "both.yml,other.yml,run.yml" {
		t.Errorf("order = %v, want both.yml,other.yml,run.yml", gotOrder)
	}
}

func TestSelect_EmptyReview(t *testing.T) {
	rows := []Spec{
		{Name: "a", Path: "a.yml", CoversSymbol: "A"},
		{Name: "b", Path: "b.yml"},
	}
	report := Select(rows, Review{})
	if report.Matched != 0 || report.Unmatched != 1 || report.NoCover != 1 || report.Total != 2 {
		t.Fatalf("got matched=%d unmatched=%d noCover=%d total=%d, want 0/1/1/2",
			report.Matched, report.Unmatched, report.NoCover, report.Total)
	}
	if len(report.Specs) != 0 {
		t.Errorf("specs = %+v, want empty", report.Specs)
	}
}

func TestIsSpecFile_AcceptsYAMLAndJSON(t *testing.T) {
	cases := map[string]bool{
		"a.yml":  true,
		"a.yaml": true,
		"a.json": true,
		"a.txt":  false,
		"a.md":   false,
		"":       false,
	}
	for in, want := range cases {
		if got := IsSpecFile(in); got != want {
			t.Errorf("IsSpecFile(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCollectSpecFiles_SkipsActionsAndDrafts(t *testing.T) {
	dir := t.TempDir()
	mkFile := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mkFile("specs/good.yml", "version: 1\nname: g\n")
	mkFile("actions/snippet.yml", "version: 1\nname: a\n")
	mkFile("specs/_draft.yml", "version: 1\nname: d\n")
	mkFile("specs/wip.draft.yml", "version: 1\nname: w\n")
	mkFile("specs/normal.json", `{"version":1,"name":"j"}`)

	files, err := CollectSpecFiles([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	basenames := make([]string, len(files))
	for i, f := range files {
		basenames[i] = filepath.Base(f)
	}
	want := []string{"good.yml", "normal.json"}
	if len(basenames) != len(want) {
		t.Fatalf("got %v, want %v", basenames, want)
	}
	for i := range want {
		if basenames[i] != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, basenames[i], want[i])
		}
	}
}
