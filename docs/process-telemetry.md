# Process Telemetry

Glyphrun can capture process-level telemetry of the target it spawns via the `monitor` CLI: CPU, RSS, thread count, the process tree, and `pprof`/`sample` profiles. Three opt-in surfaces share one foundation, and all are zero-cost when the flag/step is absent.

## Run-level sampling: `glyph run --monitor`

```bash
glyph run specs/foo.yml --monitor ./bin/monitor
glyph run specs/foo.yml --monitor ./bin/monitor --monitor-interval 500ms --monitor-profile heap
```

`--monitor <path>` samples the spawned target's CPU/RSS on a tick (default 250ms; `--monitor-interval`) and writes `diagnostics/process.md` + `diagnostics/process.json` (peak/mean CPU+RSS and the sample timeline) into the run dir. Add `--monitor-profile heap|cpu|goroutine|sample` to capture an end-of-run profile.

On Windows ConPTY the target PID is unavailable, so sampling is skipped with a note rather than failing the run.

When Glyphrun is itself launched by `monitor run <spec>`, it detects `MONITOR=1` and writes the target PID (`target.pid` in the run dir, and `glyphrun-target.pid` in `$MONITOR_RUN_DIR` when set) so the parent monitor can observe the exact process without `--monitor`.

## Step-level capture: the `monitor:` step

A `monitor:` step takes a one-shot reading of the live target at a point in the flow and stores it as a named artifact — evidence to keep or assert on later. A snapshot is always captured; `tree` and `profile` add the process subtree and/or a profile.

```yaml
steps:
  - wait:
      screen:
        contains: "dashboard"
  - monitor:
      saveAs: dashboard_load
      tree: true
      profile: heap
```

| Field | Description |
| --- | --- |
| `saveAs` | Named artifact name (default `monitor`). |
| `tree` | Capture the process subtree. |
| `profile` | `heap`, `cpu`, `goroutine`, or `sample`. |
| `timeoutMs` | Per-step timeout. |

The artifact lands at `monitors/<saveAs>.md` (+ `.json`), addressable as `${artifacts.dashboard_load.path}` by later steps. Requires the target PID (Windows ConPTY: no) and `monitor` on `$PATH` (or the run's `--monitor` binary). A missing `monitor` or an unavailable PID fails the step with a clear message.

## Perf budgets: the `metrics:` verifier

An outcome can assert process-telemetry budgets against the run's sampled summary. Each set field is an upper bound (`<=`): the run passes only if the observed peak/mean stays at or below the budget.

```yaml
outcomes:
  - id: rss_budget
    description: peak RSS stays under 512 MiB
    verify:
      metrics:
        peakRss: 536870912        # bytes
        peakCpuPercent: 90
        meanCpuPercent: 45
        meanRss: 268435456        # bytes
```

Requires process telemetry — run with `--monitor` (or add a `monitor:` step). Without samples the outcome fails with a clear message instead of silently passing.

| Field | Unit |
| --- | --- |
| `peakCpuPercent` | percent (upper bound) |
| `peakRss` | bytes |
| `meanCpuPercent` | percent |
| `meanRss` | bytes |

## When telemetry is unavailable

Both the `monitor:` step and the `metrics:` verifier require the target PID. On Windows ConPTY the PID is not exposed, so sampling is skipped and a `metrics:` outcome fails with a message rather than passing vacuously. This keeps a budget assertion honest: it never silently passes on a platform that can't measure it.