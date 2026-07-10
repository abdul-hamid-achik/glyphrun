# Artifacts

Every run writes a self-contained directory under `.glyphrun/runs` with run summaries, context, events, final screen state, raw PTY logs, frames, snapshots, outcomes, and diagnostics.

High-signal files:

- `run.md`: human run summary with status, target, outcome counts, artifacts, and next commands
- `run.json` and `run.yaml`: structured run result for scripts and agents
- `agent_context.md`: compact failure/run context with the final screen
- `diagnostics/failure.md`: failed outcomes and the final screen
- `diagnostics/environment.md`: project root, active config, target command, terminal profile, and key artifact paths
- `screens/final.txt`: normalized terminal screen text
- `screens/final.svg`: deterministic SVG render of the final screen
- `frames/frames.ndjson`: terminal frame timeline
- `raw/pty.raw.log`: redacted raw PTY byte stream
- `outcomes/results.md`: outcome-only summary
- `replay.json`: exact-replay manifest — the normalized target argv, terminal profile/viewport, resolved capture policy, redacted env KEY NAMES (never values), glyph version, and one exact `glyph run <spec>` command to reproduce the run

`agent_context.md` includes the target command, terminal profile, exit code, failed outcomes, recent events, final screen, and suggested inspection commands. Agents should read it first after a failed run.

Use `glyph diff <runA> <runB>` to compare two artifact packs by run status, outcome results, and final screen text.

Use `glyph record -- <command...>` to capture a one-off PTY session and `glyph replay <run>` to print its raw PTY log. `glyph replay <run> --tui` opens an interactive scrubber over `frames/frames.ndjson` — step (←/→), jump (home/end), and play back the captured frames to see exactly when the screen changed.

Use `glyph render <run|latest>` to render a run's final screen (or `--screen <name>` for a captured snapshot) to a deterministic SVG. The render is a pure function of the captured cell grid, so it is reproducible and safe to regenerate in CI; `--out -` streams the raw SVG to stdout.
