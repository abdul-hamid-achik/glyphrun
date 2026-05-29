# Artifacts

Every run writes a self-contained directory under `.glyphrun/runs` with run summaries, context, events, final screen state, raw PTY logs, frames, snapshots, outcomes, and diagnostics.

Use `glyph diff <runA> <runB>` to compare two artifact packs by run status, outcome results, and final screen text.

Use `glyph record -- <command...>` to capture a one-off PTY session and `glyph replay <run>` to print its raw PTY log.
