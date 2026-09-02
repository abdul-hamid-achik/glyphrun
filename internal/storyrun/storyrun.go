// Package storyrun schedules stories: it discovers manifests and spec files
// tagged `story`, builds each harness once, runs every story through the
// regular runner, and records the newest result in the stories index so the
// catalog survives run-directory pruning. It also owns the watch loop and the
// live server behind `glyph stories run --watch` and `glyph stories serve`.
//
// The runner stays ignorant of stories: this package hands it a parsed spec
// (runner.Options.Parsed) and reads back an ordinary artifacts.RunResult.
package storyrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/affected"
	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/log"
	"github.com/abdul-hamid-achik/glyphrun/internal/runner"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/stories"
)

// Options select and configure a stories run.
type Options struct {
	Paths []string
	// Only narrows the run to matching stories: a key (`list/rows`,
	// `list/rows@wide`), a spec name, a feature, or `feature/` prefix. A bare
	// id also selects its variants unless Exact is set.
	Only []string
	// Exact makes Only match keys and spec names literally (no variant or
	// feature fan-out). Accepting a golden from the live page uses it so one
	// row never rewrites another row's golden.
	Exact        bool
	ConfigPath   string
	Environment  string
	ArtifactRoot string
	Parallel     int
	// Update rewrites every golden with the captured screen.
	Update bool
	// Strict fails a story whose golden is missing instead of creating it.
	Strict   bool
	Listener runner.ProgressListener
}

// Job is one runnable story.
type Job struct {
	Key        string `json:"key"`
	ID         string `json:"id,omitempty"`
	Variant    string `json:"variant,omitempty"`
	Feature    string `json:"feature,omitempty"`
	Source     string `json:"source"`
	SourcePath string `json:"sourcePath"`
	SpecName   string `json:"specName"`
	// GoldenName is the snapshot compared against a committed golden ("" =
	// none); GoldenOutcomeID is the outcome that performs the comparison.
	GoldenName      string            `json:"goldenName,omitempty"`
	GoldenOutcomeID string            `json:"goldenOutcomeId,omitempty"`
	Parsed          *spec.ParseResult `json:"-"`
	SpecPath        string            `json:"-"`
}

// ManifestPlan is one manifest with its expanded jobs.
type ManifestPlan struct {
	Path     string
	Manifest stories.Manifest
	Jobs     []Job
}

// Plan is everything Run needs, resolved once.
type Plan struct {
	Runtime      config.Runtime
	Roots        stories.Roots
	Manifests    []ManifestPlan
	Jobs         []Job
	WatchRoots   []string
	ArtifactRoot string
	SnapshotRoot string
	StoriesRoot  string
}

// OutputRoots are the directories a run writes; watchers exclude them so a
// run never looks like a source change.
func (p *Plan) OutputRoots() []string {
	return []string{p.ArtifactRoot, p.SnapshotRoot, p.StoriesRoot}
}

// Result is one story's outcome inside a Report.
type Result struct {
	Job           Job                 `json:"job"`
	Run           artifacts.RunResult `json:"run"`
	Error         string              `json:"error,omitempty"`
	GoldenCreated bool                `json:"goldenCreated"`
	GoldenUpdated bool                `json:"goldenUpdated"`
}

// Report is the machine-readable result of `glyph stories run`.
type Report struct {
	SchemaVersion int      `json:"schemaVersion"`
	StoriesRoot   string   `json:"storiesRoot"`
	Built         []string `json:"built,omitempty"`
	Results       []Result `json:"results"`
	Passed        int      `json:"passed"`
	Failed        int      `json:"failed"`
	Errored       int      `json:"errored"`
	GoldensNew    int      `json:"goldensCreated"`
	GoldensUpdate int      `json:"goldensUpdated"`
	DurationMS    int64    `json:"durationMs"`
	ExitCode      int      `json:"exitCode"`
}

// ErrNoStories is returned when the paths hold nothing runnable.
var ErrNoStories = errors.New("no stories found: add a stories.yml manifest or tag a spec with `story`")

// Discover resolves the runtime, finds manifests and story specs under the
// paths, expands manifests, and applies Only.
func Discover(opts Options) (*Plan, error) {
	paths := opts.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}
	rt, err := config.LoadRuntime(paths[0], opts.ConfigPath, opts.Environment)
	if err != nil {
		return nil, err
	}
	roots := stories.ResolveRoots(rt, opts.ArtifactRoot)
	plan := &Plan{Runtime: rt, Roots: roots, ArtifactRoot: roots.ArtifactRoot, SnapshotRoot: roots.SnapshotRoot, StoriesRoot: roots.StoriesRoot}

	manifests, err := stories.FindManifests(paths)
	if err != nil {
		return nil, err
	}
	parseOpts := rt.SpecParseOptions()
	parseOpts.AllowHashMismatch = true
	watch := map[string]bool{}
	identities := map[string]string{}
	for _, mp := range manifests {
		m, err := stories.LoadManifest(mp, parseOpts)
		if err != nil {
			return nil, err
		}
		expanded, err := stories.Expand(m, mp, parseOpts.DefaultTerminal)
		if err != nil {
			return nil, err
		}
		item := ManifestPlan{Path: mp, Manifest: m}
		for _, ex := range expanded {
			ex := ex
			if err := stories.RegisterStorySpecName(identities, ex.Spec.Name, fmt.Sprintf("%q in %s", ex.Key(), mp)); err != nil {
				return nil, err
			}
			job := Job{
				Key:        ex.Key(),
				ID:         ex.ID,
				Variant:    ex.Variant,
				Feature:    ex.Feature,
				Source:     stories.SourceManifest,
				SourcePath: mp,
				SpecName:   ex.Spec.Name,
				Parsed:     &ex.Parsed,
			}
			job.GoldenName, job.GoldenOutcomeID = stories.GoldenOutcome(ex.Spec)
			item.Jobs = append(item.Jobs, job)
		}
		plan.Manifests = append(plan.Manifests, item)
		plan.Jobs = append(plan.Jobs, item.Jobs...)
		watch[mp] = true
		// harness.watch entries are project-root relative, like harness.cwd
		// and a spec's target.cwd.
		for _, w := range m.Harness.Watch {
			if filepath.IsAbs(w) {
				watch[w] = true
			} else {
				watch[filepath.Join(rt.ProjectRoot, w)] = true
			}
		}
	}

	files, err := affected.CollectSpecFiles(paths)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if stories.IsManifestPath(f) || strings.Contains(filepath.ToSlash(f), "/.glyphrun/") ||
			filepath.Base(f) == "spec.resolved.yml" || stories.UnderRoots(f, plan.ArtifactRoot, plan.SnapshotRoot, plan.StoriesRoot) {
			continue
		}
		parsed, err := spec.ParseFile(f, parseOpts)
		if err != nil {
			continue
		}
		if parsed.Spec.Metadata == nil || !hasTag(parsed.Spec.Metadata.Tags, "story") {
			continue
		}
		job := Job{
			Key:        parsed.Spec.Name,
			Feature:    parsed.Spec.Metadata.Feature,
			Source:     stories.SourceSpec,
			SourcePath: f,
			SpecName:   parsed.Spec.Name,
			SpecPath:   f,
		}
		if err := stories.RegisterStorySpecName(identities, job.SpecName, fmt.Sprintf("%q in %s", job.Key, f)); err != nil {
			return nil, err
		}
		job.GoldenName, job.GoldenOutcomeID = stories.GoldenOutcome(parsed.Resolved)
		plan.Jobs = append(plan.Jobs, job)
		watch[filepath.Dir(f)] = true
	}
	for w := range watch {
		plan.WatchRoots = append(plan.WatchRoots, w)
	}
	sort.Strings(plan.WatchRoots)
	if len(opts.Only) > 0 {
		plan.Filter(opts.Only, opts.Exact)
	}
	if len(plan.Jobs) == 0 {
		return nil, ErrNoStories
	}
	return plan, nil
}

// Filter keeps the jobs matching one of the selectors. A selector matches a
// job's key, spec name, or feature exactly; a bare id also matches its
// variants and a trailing `/` matches a feature or id prefix. With exact set,
// only key and spec name matches count.
func (p *Plan) Filter(only []string, exact bool) {
	keep := func(j Job) bool {
		for _, sel := range only {
			sel = strings.TrimSpace(sel)
			if sel == "" {
				continue
			}
			if j.Key == sel || j.SpecName == sel {
				return true
			}
			if exact {
				continue
			}
			if j.ID == sel || j.Feature == sel {
				return true
			}
			if strings.HasSuffix(sel, "/") {
				prefix := strings.TrimSuffix(sel, "/")
				if j.Feature == prefix || strings.HasPrefix(j.ID+"/", sel) {
					return true
				}
			}
			if strings.HasPrefix(j.Key, sel+"@") {
				return true
			}
		}
		return false
	}
	var jobs []Job
	for _, j := range p.Jobs {
		if keep(j) {
			jobs = append(jobs, j)
		}
	}
	p.Jobs = jobs
	for i := range p.Manifests {
		var mj []Job
		for _, j := range p.Manifests[i].Jobs {
			if keep(j) {
				mj = append(mj, j)
			}
		}
		p.Manifests[i].Jobs = mj
	}
}

// Build runs each manifest's harness.build once (only for manifests that
// still have jobs after filtering). Output is captured and surfaced on
// failure so a broken harness reads like a compile error, not a PTY timeout.
func Build(ctx context.Context, plan *Plan) ([]string, error) {
	var built []string
	for _, mp := range plan.Manifests {
		cmdline := strings.TrimSpace(mp.Manifest.Harness.Build)
		if cmdline == "" || len(mp.Jobs) == 0 {
			continue
		}
		timeout := 60 * time.Second
		if mp.Manifest.Harness.BuildTimeoutMS > 0 {
			timeout = time.Duration(mp.Manifest.Harness.BuildTimeoutMS) * time.Millisecond
		}
		buildCtx, cancel := context.WithTimeout(ctx, timeout)
		cmd := exec.CommandContext(buildCtx, "/bin/sh", "-lc", cmdline)
		cmd.Dir = plan.Runtime.ProjectRoot
		if cwd := strings.TrimSpace(mp.Manifest.Harness.Cwd); cwd != "" && cwd != "." {
			if filepath.IsAbs(cwd) {
				cmd.Dir = cwd
			} else {
				cmd.Dir = filepath.Join(plan.Runtime.ProjectRoot, cwd)
			}
		}
		cmd.Env = envSlice(plan.Runtime.Env)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		started := time.Now()
		log.Info("stories: building harness", "manifest", mp.Path, "cmd", cmdline)
		err := cmd.Run()
		cancel()
		if err != nil {
			return built, fmt.Errorf("harness build failed for %s: %v\n%s", mp.Path, err, strings.TrimSpace(out.String()))
		}
		log.Debug("stories: harness built", "manifest", mp.Path, "ms", time.Since(started).Milliseconds())
		built = append(built, mp.Path)
	}
	return built, nil
}

// Run builds harnesses, runs every job with a worker pool, writes the stories
// index, and aggregates the report. Missing goldens are created on the first
// run unless Strict is set; Update rewrites them all.
func Run(ctx context.Context, opts Options, plan *Plan) (Report, error) {
	started := time.Now()
	report := Report{SchemaVersion: 1, StoriesRoot: plan.StoriesRoot}
	built, err := Build(ctx, plan)
	report.Built = built
	if err != nil {
		report.ExitCode = 2
		report.DurationMS = time.Since(started).Milliseconds()
		return report, err
	}

	parallel := opts.Parallel
	if parallel < 1 {
		parallel = 1
	}
	if parallel > len(plan.Jobs) {
		parallel = len(plan.Jobs)
	}
	results := make([]Result, len(plan.Jobs))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < parallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				results[idx] = runOne(ctx, opts, plan, plan.Jobs[idx])
			}
		}()
	}
	for idx := range plan.Jobs {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()

	report.Results = results
	for _, r := range results {
		switch {
		case r.Error != "":
			report.Errored++
		case r.Run.Status == artifacts.StatusPassed:
			report.Passed++
		case r.Run.Status == artifacts.StatusErrored:
			report.Errored++
		default:
			report.Failed++
		}
		if r.GoldenCreated {
			report.GoldensNew++
		}
		if r.GoldenUpdated {
			report.GoldensUpdate++
		}
		if r.Run.ExitCode > report.ExitCode {
			report.ExitCode = r.Run.ExitCode
		}
		if r.Error != "" && report.ExitCode < 2 {
			report.ExitCode = 2
		}
	}
	report.DurationMS = time.Since(started).Milliseconds()
	return report, nil
}

// goldenStamp identifies the on-disk state of a golden so a run can report
// whether it created or rewrote it, whatever the run's outcome status.
type goldenStamp struct {
	exists  bool
	modTime time.Time
	size    int64
}

func statGolden(path string) goldenStamp {
	info, err := os.Stat(path)
	if err != nil {
		return goldenStamp{}
	}
	return goldenStamp{exists: true, modTime: info.ModTime(), size: info.Size()}
}

func runOne(ctx context.Context, opts Options, plan *Plan, job Job) Result {
	res := Result{Job: job}
	update := opts.Update
	var goldenPath string
	var before goldenStamp
	if job.GoldenName != "" {
		goldenPath = runner.CommittedSnapshotPath(plan.Runtime, job.SpecName, job.GoldenName)
		before = statGolden(goldenPath)
		if !before.exists && !update && !opts.Strict {
			update = true
			log.Info("stories: no golden yet, capturing", "story", job.Key, "snapshot", job.GoldenName)
		}
	}
	ropts := runner.Options{
		SpecPath:        job.SourcePath,
		ConfigPath:      opts.ConfigPath,
		Environment:     opts.Environment,
		ArtifactRoot:    opts.ArtifactRoot,
		UpdateSnapshots: update,
		Listener:        opts.Listener,
	}
	if job.Parsed != nil {
		ropts.Parsed = job.Parsed
	} else {
		ropts.SpecPath = job.SpecPath
	}
	result, err := runner.RunSpec(ctx, ropts)
	res.Run = result
	if err != nil {
		res.Error = err.Error()
	}
	// The runner writes the golden at the capture step, before any outcome
	// is verified, so the file — not the run status — says whether it was
	// created or rewritten. A failed run that wrote a golden is reported as
	// such so an unreviewed golden never lands silently.
	if goldenPath != "" && update {
		after := statGolden(goldenPath)
		switch {
		case !before.exists && after.exists:
			res.GoldenCreated = true
		case before.exists && after.exists && (after.modTime != before.modTime || after.size != before.size):
			res.GoldenUpdated = true
		}
	}
	if err != nil {
		return res
	}
	entry := stories.EntryFromRun(job.Key, job.ID, job.Variant, job.Feature, job.Source, job.SourcePath, job.GoldenName, result)
	if err := stories.WriteIndexEntry(plan.StoriesRoot, entry, result.RunDir); err != nil {
		log.Warn("stories: index write failed", "story", job.Key, "err", err)
	}
	return res
}

// RenderReportMarkdown is the human summary printed by `glyph stories run`.
func RenderReportMarkdown(r Report) string {
	var b strings.Builder
	b.WriteString("# Glyphrun Stories Run\n\n")
	fmt.Fprintf(&b, "- stories: %d (passed %d, failed %d, errored %d)\n", len(r.Results), r.Passed, r.Failed, r.Errored)
	if r.GoldensNew > 0 {
		fmt.Fprintf(&b, "- goldens created: %d\n", r.GoldensNew)
	}
	if r.GoldensUpdate > 0 {
		fmt.Fprintf(&b, "- goldens updated: %d\n", r.GoldensUpdate)
	}
	fmt.Fprintf(&b, "- index: `%s`\n", r.StoriesRoot)
	fmt.Fprintf(&b, "- duration: %dms\n\n", r.DurationMS)
	b.WriteString("| story | spec | status | golden | run |\n| --- | --- | --- | --- | --- |\n")
	for _, res := range r.Results {
		status := string(res.Run.Status)
		if res.Error != "" {
			status = "error: " + res.Error
		} else if res.Run.Status != artifacts.StatusPassed && res.Run.Diagnostic != "" {
			status += " — " + res.Run.Diagnostic
		}
		golden := goldenLabel(res)
		run := res.Run.RunID
		if run == "" {
			run = "—"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s | `%s` |\n", res.Job.Key, res.Job.SpecName, status, golden, run)
	}
	return b.String()
}

// goldenLabel describes what happened to a story's golden: created/updated
// (the file changed), match/changed (from the golden outcome by id), or none.
func goldenLabel(res Result) string {
	switch {
	case res.Job.GoldenName == "":
		return "none"
	case res.GoldenCreated:
		return "created"
	case res.GoldenUpdated:
		return "updated"
	}
	for _, o := range res.Run.Outcomes {
		if o.ID != res.Job.GoldenOutcomeID {
			continue
		}
		if o.Status == artifacts.OutcomePassed {
			return "match"
		}
		return "changed"
	}
	return "—"
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func envSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}
