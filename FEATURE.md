# FEATURE: codemap + glyphrun Integration

## What

glyphrun is codemap's E2E testing framework. codemap ships glyphrun specs in
`specs/*.yml` that declare intent + outcomes (the contract) and steps (repairable
hints). This feature file documents the 4 new specs added for the performance,
UX, and cache work.

## New glyphrun specs

### specs/timing.yml

Verifies `codemap index` prints phase-level wall-clock timing (extract, precise,
embed) in the summary. Checks for the "time:" line and "extract" phase label.

### specs/progress_eta.yml

Verifies the live progress bar runs cleanly with the new ETA feature. The ETA
text itself is time-dependent, so the spec checks for the summary (which appears
after the bar clears) rather than the ETA string.

### specs/index_watch.yml

Verifies `codemap index --watch` runs the initial index, then starts the daemon.
Uses `timeout` to kill the daemon (it runs forever). Checks for "Indexed" and
"daemon" in the output.

### specs/cache_cli.yml

Verifies the `codemap cache` subcommand group exists and `cache list` runs
without crashing (even when fcheap is not on PATH — degrades gracefully to an
empty list).

## How codemap uses glyphrun

- `task flows` runs all `specs/*.yml` (E2E; local only — not in CI)
- `glyph spec verify specs/x.yml --stamp` stamps the `contractHash` after an
  intentional intent/outcome change
- Specs declare **intent + outcomes** (the contract) and **steps** (repairable)
- The contract is the source of truth; steps are hints that can be auto-repaired

## What glyphrun needs from codemap

- A built binary at `bin/codemap` (specs reference `$ROOT/bin/codemap`)
- Isolated state via `CODEMAP_DATA` env var (specs use temp dirs)
- Clean stdout/stderr output (specs grep for specific strings)
- Exit codes (specs check `exitCode: 0`)

## Why this matters

glyphrun specs are the E2E contract — they verify user-visible behavior end to
end, not just unit-level correctness. The 4 new specs cover the timing output,
progress ETA, --watch flag, and cache CLI — all the user-facing changes from this
work.