package storyrun

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/stories"
)

// fixture writes a manifest whose harness is a shell one-liner (no PTY app
// needed) plus a config that keeps every root inside the temp dir.
func fixture(t *testing.T) (dir, cfg string) {
	t.Helper()
	dir = t.TempDir()
	manifest := `version: 1
kind: stories
harness:
  cmd: ["/bin/sh", "-c", "printf 'Inbox\\n\\n(empty) $0\\n' \"$STORY\"; exit 0"]
  build: "printf built > .built"
  watch: ["src"]
defaults:
  terminal: { cols: 40, rows: 6, profile: xterm-256color }
  quit: ""
stories:
  - id: list/empty
    ready: { contains: "(empty)" }
    variants:
      - name: narrow
        terminal: { cols: 30, rows: 6 }
  - id: list/rows
    env: { STORY: rows }
    ready: { contains: "(empty)" }
`
	if err := os.WriteFile(filepath.Join(dir, "stories.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg = filepath.Join(dir, "glyphrun.config.yml")
	body := "version: 1\nartifactRoot: " + filepath.Join(dir, "runs") + "\nsnapshotRoot: " + filepath.Join(dir, "goldens") + "\nstoriesRoot: " + filepath.Join(dir, "index") + "\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, cfg
}

func TestDiscoverExpandsAndFilters(t *testing.T) {
	dir, cfg := fixture(t)
	plan, err := Discover(Options{Paths: []string{dir}, ConfigPath: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Jobs) != 3 || len(plan.Manifests) != 1 {
		t.Fatalf("jobs = %d manifests = %d", len(plan.Jobs), len(plan.Manifests))
	}
	keys := make([]string, 0, len(plan.Jobs))
	for _, j := range plan.Jobs {
		keys = append(keys, j.Key)
	}
	if strings.Join(keys, " ") != "list/empty list/empty@narrow list/rows" {
		t.Fatalf("keys = %v", keys)
	}
	if plan.Jobs[0].GoldenName != "empty" || plan.Jobs[0].Parsed == nil || plan.Jobs[0].Source != stories.SourceManifest {
		t.Fatalf("job = %+v", plan.Jobs[0])
	}
	if plan.StoriesRoot != filepath.Join(dir, "index") || plan.SnapshotRoot != filepath.Join(dir, "goldens") {
		t.Fatalf("roots = %s %s", plan.StoriesRoot, plan.SnapshotRoot)
	}
	wantWatch := filepath.Join(dir, "src")
	found := false
	for _, w := range plan.WatchRoots {
		if w == wantWatch {
			found = true
		}
	}
	if !found {
		t.Fatalf("harness.watch missing from roots: %v", plan.WatchRoots)
	}

	plan.Filter([]string{"list/empty@narrow"}, false)
	if len(plan.Jobs) != 1 || plan.Jobs[0].Variant != "narrow" {
		t.Fatalf("filter by variant key = %+v", plan.Jobs)
	}
	plan, _ = Discover(Options{Paths: []string{dir}, ConfigPath: cfg, Only: []string{"list/empty"}})
	if len(plan.Jobs) != 2 {
		t.Fatalf("filter by id should keep base + variants, got %d", len(plan.Jobs))
	}
	plan, _ = Discover(Options{Paths: []string{dir}, ConfigPath: cfg, Only: []string{"list"}})
	if len(plan.Jobs) != 3 {
		t.Fatalf("filter by feature should keep all, got %d", len(plan.Jobs))
	}
	if _, err := Discover(Options{Paths: []string{dir}, ConfigPath: cfg, Only: []string{"nope"}}); err != ErrNoStories {
		t.Fatalf("expected ErrNoStories, got %v", err)
	}
}

func TestRunBuildsOnceCapturesGoldensAndIndexes(t *testing.T) {
	dir, cfg := fixture(t)
	opts := Options{Paths: []string{dir}, ConfigPath: cfg, Parallel: 2}
	plan, err := Discover(opts)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), opts, plan)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed != 3 || report.GoldensNew != 3 || report.ExitCode != 0 || len(report.Built) != 1 {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(filepath.Join(dir, ".built")); err != nil {
		t.Fatalf("harness.build did not run: %v", err)
	}
	for _, name := range []string{"story_list_empty", "story_list_empty__narrow", "story_list_rows"} {
		if _, err := os.Stat(filepath.Join(dir, "goldens", name, "empty.txt")); name != "story_list_rows" && err != nil {
			t.Fatalf("golden for %s missing: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "index", name, stories.IndexFile)); err != nil {
			t.Fatalf("index for %s missing: %v", name, err)
		}
	}
	idx := stories.ReadIndex(filepath.Join(dir, "index"))
	if e := idx["story_list_rows"]; e.Key != "list/rows" || e.GoldenName != "rows" || e.Status != "passed" || len(e.Screens) != 2 {
		t.Fatalf("index entry = %+v", e)
	}
	md := RenderReportMarkdown(report)
	if !strings.Contains(md, "goldens created: 3") || !strings.Contains(md, "`list/empty@narrow`") {
		t.Fatalf("markdown = %s", md)
	}

	// Second run: goldens match, nothing created. Strict mode with a
	// missing golden fails instead of creating it.
	report, err = Run(context.Background(), opts, plan)
	if err != nil || report.GoldensNew != 0 || report.Passed != 3 {
		t.Fatalf("second run = %+v err=%v", report, err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "goldens", "story_list_rows")); err != nil {
		t.Fatal(err)
	}
	strict := opts
	strict.Strict = true
	strict.Only = []string{"list/rows"}
	plan, err = Discover(strict)
	if err != nil {
		t.Fatal(err)
	}
	report, err = Run(context.Background(), strict, plan)
	if err != nil {
		t.Fatal(err)
	}
	if report.Failed != 1 || report.ExitCode != 1 || report.Results[0].Run.Status != artifacts.StatusFailed {
		t.Fatalf("strict report = %+v", report)
	}
	// --update recreates it; a golden that did not exist counts as created.
	update := strict
	update.Strict = false
	update.Update = true
	report, err = Run(context.Background(), update, plan)
	if err != nil || report.Passed != 1 || report.GoldensNew != 1 || report.GoldensUpdate != 0 {
		t.Fatalf("update report = %+v err=%v", report, err)
	}
}

func TestBuildFailureSurfacesOutput(t *testing.T) {
	dir, cfg := fixture(t)
	data, _ := os.ReadFile(filepath.Join(dir, "stories.yml"))
	broken := strings.Replace(string(data), `build: "printf built > .built"`, `build: "echo boom >&2; exit 3"`, 1)
	if err := os.WriteFile(filepath.Join(dir, "stories.yml"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{Paths: []string{dir}, ConfigPath: cfg}
	plan, err := Discover(opts)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), opts, plan)
	if err == nil || !strings.Contains(err.Error(), "boom") || report.ExitCode != 2 {
		t.Fatalf("expected build failure with output, got err=%v report=%+v", err, report)
	}
}

func TestServeCatalogRunAndEvents(t *testing.T) {
	dir, cfg := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ServeOptions{
			Options: Options{Paths: []string{dir}, ConfigPath: cfg, Parallel: 2},
			Addr:    "127.0.0.1:0",
			Ready:   func(url string) { ready <- url },
		})
	}()
	var url string
	select {
	case url = <-ready:
	case err := <-done:
		t.Fatalf("serve exited early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not become ready")
	}

	page, err := http.Get(url + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if !strings.Contains(string(body), `"live":true`) || !strings.Contains(string(body), "list/rows") {
		t.Fatalf("page missing live payload: %.200s", body)
	}

	// Trigger one story and wait for the catalog to report it as passed.
	resp, err := http.Post(url+"/run", "application/json", strings.NewReader(`{"key":"list/rows"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("run status = %d", resp.StatusCode)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		res, err := http.Get(url + "/catalog.json")
		if err != nil {
			t.Fatal(err)
		}
		var payload stories.PagePayload
		_ = json.NewDecoder(res.Body).Decode(&payload)
		res.Body.Close()
		var rows *stories.StoryPayload
		for i := range payload.Stories {
			if payload.Stories[i].Key == "list/rows" {
				rows = &payload.Stories[i]
			}
		}
		if rows != nil && rows.Status == "passed" && rows.Golden == stories.GoldenMatch {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("list/rows never reached passed/match: %+v", rows)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// The event stream opens with a catalog frame.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/events", nil)
	es, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _ := es.Body.Read(buf)
	es.Body.Close()
	if !strings.HasPrefix(string(buf[:n]), "event: catalog") {
		t.Fatalf("events stream = %q", buf[:n])
	}

	bad, _ := http.Post(url+"/update", "application/json", strings.NewReader(`{}`))
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("update without key = %d", bad.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestServeGuardsHostAndOrigin(t *testing.T) {
	dir, cfg := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	go func() {
		_ = Serve(ctx, ServeOptions{Options: Options{Paths: []string{dir}, ConfigPath: cfg}, Addr: "127.0.0.1:0", Ready: func(u string) { ready <- u }})
	}()
	var url string
	select {
	case url = <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("server did not become ready")
	}
	// DNS rebinding: a foreign Host header is refused even on loopback.
	req, _ := http.NewRequest(http.MethodGet, url+"/catalog.json", nil)
	req.Host = "evil.example:80"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign host = %d, want 403", res.StatusCode)
	}
	// CSRF: a simple form POST (text/plain) is refused; a cross-origin JSON
	// POST is refused; a same-origin JSON POST is accepted.
	form, _ := http.Post(url+"/run", "text/plain", strings.NewReader(`{"key":""}`))
	form.Body.Close()
	if form.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain POST = %d, want 415", form.StatusCode)
	}
	cross, _ := http.NewRequest(http.MethodPost, url+"/run", strings.NewReader(`{"key":""}`))
	cross.Header.Set("Content-Type", "application/json")
	cross.Header.Set("Origin", "http://evil.example")
	res, _ = http.DefaultClient.Do(cross)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin POST = %d, want 403", res.StatusCode)
	}
	same, _ := http.NewRequest(http.MethodPost, url+"/run", strings.NewReader(`{"key":""}`))
	same.Header.Set("Content-Type", "application/json")
	same.Header.Set("Origin", url)
	res, _ = http.DefaultClient.Do(same)
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("same-origin POST = %d, want 202", res.StatusCode)
	}
}

func TestUpdateRewritesExistingGolden(t *testing.T) {
	dir, cfg := fixture(t)
	opts := Options{Paths: []string{dir}, ConfigPath: cfg, Only: []string{"list/rows"}}
	plan, err := Discover(opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), opts, plan); err != nil {
		t.Fatal(err)
	}
	opts.Update = true
	report, err := Run(context.Background(), opts, plan)
	if err != nil || report.GoldensUpdate != 1 || report.GoldensNew != 0 || report.Passed != 1 {
		t.Fatalf("update report = %+v err=%v", report, err)
	}
}

func TestFilterFeaturePrefixAndExact(t *testing.T) {
	dir, cfg := fixture(t)
	data, _ := os.ReadFile(filepath.Join(dir, "stories.yml"))
	// Give list/rows an overridden feature so `shared/` must match by feature.
	withFeature := strings.Replace(string(data), "  - id: list/rows\n", "  - id: list/rows\n    feature: shared\n", 1)
	if err := os.WriteFile(filepath.Join(dir, "stories.yml"), []byte(withFeature), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := Discover(Options{Paths: []string{dir}, ConfigPath: cfg, Only: []string{"shared/"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Jobs) != 1 || plan.Jobs[0].Key != "list/rows" {
		t.Fatalf("feature prefix selection = %+v", plan.Jobs)
	}
	plan, err = Discover(Options{Paths: []string{dir}, ConfigPath: cfg, Only: []string{"list/empty"}, Exact: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Jobs) != 1 || plan.Jobs[0].Variant != "" {
		t.Fatalf("exact selection must not fan out to variants: %+v", plan.Jobs)
	}
}

func TestGoldenWrittenByFailedRunIsReported(t *testing.T) {
	dir, cfg := fixture(t)
	data, _ := os.ReadFile(filepath.Join(dir, "stories.yml"))
	// list/rows gets an extra outcome that always fails; the golden is still
	// captured at the snapshot step and must be reported as created.
	failing := strings.Replace(string(data), "    ready: { contains: \"(empty)\" }\n", "    ready: { contains: \"(empty)\" }\n    outcomes:\n      - id: never\n        description: always fails\n        verify:\n          screen: { contains: \"THIS TEXT IS NOT ON SCREEN\" }\n", 2)
	if err := os.WriteFile(filepath.Join(dir, "stories.yml"), []byte(failing), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{Paths: []string{dir}, ConfigPath: cfg, Only: []string{"list/rows"}}
	plan, err := Discover(opts)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Run(context.Background(), opts, plan)
	if err != nil {
		t.Fatal(err)
	}
	if report.Failed != 1 || report.GoldensNew != 1 || !report.Results[0].GoldenCreated {
		t.Fatalf("a failed run that wrote the golden must report it: %+v", report)
	}
	if !strings.Contains(RenderReportMarkdown(report), "| failed") || !strings.Contains(RenderReportMarkdown(report), "created") {
		t.Fatalf("markdown = %s", RenderReportMarkdown(report))
	}
}
