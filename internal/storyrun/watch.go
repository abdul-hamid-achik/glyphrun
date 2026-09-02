package storyrun

import (
	"context"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/log"
	"github.com/abdul-hamid-achik/glyphrun/internal/watchfs"
)

// Watch runs the plan once, then re-discovers and re-runs whenever a watched
// path changes: the manifests, their `harness.watch` directories, and the
// directories of spec-file stories. Output roots (artifacts, goldens, the
// index) are excluded from the fingerprint and it is recomputed after every
// run, so a run's own writes never re-trigger it. onReport receives every
// iteration's report (and any run error) so the caller can print or
// broadcast it. Discovery is repeated per iteration so edits to the manifest
// itself (new stories, changed ready text) take effect without a restart.
func Watch(ctx context.Context, opts Options, extraRoots []string, onReport func(Report, error)) error {
	plan, err := Discover(opts)
	if err != nil {
		return err
	}
	roots := watchfs.Roots(append(append([]string(nil), plan.WatchRoots...), extraRoots...)...)
	excluded := plan.OutputRoots()
	log.Info("stories: watching", "paths", len(roots), "stories", len(plan.Jobs))

	report, runErr := Run(ctx, opts, plan)
	onReport(report, runErr)
	last := watchfs.FingerprintExcluding(roots, excluded)

	ticker := time.NewTicker(watchfs.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			fp := watchfs.FingerprintExcluding(roots, excluded)
			if fp == last {
				continue
			}
			log.Info("stories: change detected, re-running")
			plan, err := Discover(opts)
			if err != nil {
				onReport(Report{SchemaVersion: 1, ExitCode: 2}, err)
			} else {
				roots = watchfs.Roots(append(append([]string(nil), plan.WatchRoots...), extraRoots...)...)
				excluded = plan.OutputRoots()
				report, runErr := Run(ctx, opts, plan)
				onReport(report, runErr)
			}
			// Recompute after the run so build output under a watched
			// directory (the harness binary) does not immediately re-trigger.
			last = watchfs.FingerprintExcluding(roots, excluded)
		}
	}
}
