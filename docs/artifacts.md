# Artifacts

Every run writes a self-contained directory under `.glyphrun/runs` with run summaries, context, events, final screen state, raw PTY logs, frames, snapshots, outcomes, and diagnostics.

High-signal files:

- `run.md`: human run summary with status, target, outcome counts, artifacts, and next commands
- `run.json` and `run.yaml`: structured run result for scripts and agents
- `agent_context.md`: compact failure/run context with the final screen
- `diagnostics/failure.md`: failed outcomes and the final screen
- `screens/final.txt`: normalized terminal screen text
- `frames/frames.ndjson`: terminal frame timeline
- `raw/pty.raw.log`: redacted raw PTY byte stream
- `outcomes/results.md`: outcome-only summary

Use `glyph diff <runA> <runB>` to compare two artifact packs by run status, outcome results, and final screen text.

Use `glyph record -- <command...>` to capture a one-off PTY session and `glyph replay <run>` to print its raw PTY log.
