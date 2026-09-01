# Changelog

All notable changes to Glyphrun are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
follows Semantic Versioning for its pre-1.0 release line.

## [Unreleased]

### Added

- `stories.yml` manifest (`kind: stories`): one harness declaration expands into many isolated-state specs, so authoring a story no longer requires a hand-written spec file. Schema at `schemas/glyphrun.stories.v1.schema.json`.
- `glyph stories run [--watch] [--update] [--strict] [--only <key>] [--parallel N]`: builds each manifest's harness once, runs every story in parallel, captures missing goldens (or fails with `--strict`), and refreshes the stories index.
- `glyph stories serve [--watch] [--addr] [--no-run]`: a loopback HTTP server hosting the live inspect page, with server-sent-events catalog updates, rerun (one or all), and accept-golden from the browser.
- Story variants (`stories[].variants[].{name,terminal,env,args}`): re-run a story with overrides layered on top, each with its own spec, golden, and `<id>@<variant>` catalog key.
- A stories index under `storiesRoot` (new config key, default `.glyphrun/stories`): the newest result plus a copy of its captured screens per story, so the catalog survives `retention.keepRuns` pruning. `.glyphrun/stories/` is gitignored.
- The HTML inspect page now renders screens client-side as SVG from the captured cell grid instead of server-rendered SVG blobs, with plain/grid/spaces/diff modes (keys `1`-`4`), a `g` golden-toggle in diff mode, a `/` filter shortcut, and in live mode rerun/rerun-all/accept-golden controls (`r`/`a`).
- `glyph stories --tui` diff highlighting (`d`, on by default), golden-vs-current toggle (`o`), and rulers (`r`); the sidebar marks each story `✓`/`±`/`?` for golden match/changed/missing.
- `internal/terminal.DiffSnapshots` (cell-level screen diff) and `render.Options.Changed` (SVG diff overlay).
- `glyph stories init --lang sh`: a POSIX shell story harness that needs no toolchain, alongside the existing `--lang go` Bubble Tea v2 harness.
- MCP tool `glyph_stories_run` (`paths`, `only`, `update`, `strict`, `parallel`).
- `internal/storyrun` (schedules stories over the runner: build-once, parallel run, index, watch, serve) and `internal/watchfs` (shared polling change detector, also usable by `run --watch`).
- `examples/stories.yml` (8 stories, one `wide` variant on `list/rows`) and `examples/stories-sh/` (POSIX shell harness); Taskfile targets `example:stories`, `example:stories:sh`, `example:stories:serve`.

### Changed

- `glyph_stories` (MCP and CLI) now returns manifest-derived stories (joined to their newest result and golden status) alongside specs tagged `story`, keyed `<id>` or `<id>@<variant>`.
- `glyph spec scaffold --kind story` now prints a starter `stories.yml` manifest instead of a bare spec.

### Removed

- `examples/specs/story_*.yml` — replaced by `examples/stories.yml`, which expands to the same stories via the manifest.

## [0.18.0] - 2026-08-22

### Added

- `glyph stories` catalogs specs tagged `story` joined to their newest run.
- `glyph stories --html` writes a self-contained inspect page (grid / rulers / spaces overlays and a cell hover inspector).
- `glyph stories --tui` browses the same snapshots in a two-pane terminal catalog.
- `glyph stories init --lang go` and `glyph spec scaffold --kind story` scaffold a Bubble Tea harness and starter spec.
- `glyph render --grid --rulers --spaces` inspects one snapshot as SVG. Default `screens/final.svg` is unchanged.
- Example list and agent-chat stories under `examples/stories/` (`task example:stories`).
- MCP tool `glyph_stories`.

### Changed

- Go toolchain pin is 1.26.6 (`.tool-versions` and CI Verify).

## [0.17.0] - 2026-08-14

### Added

- Dual-era MCP: `server/discover`, protocol versions `2024-11-05` / `2025-11-25` / `2026-07-28`, `tools/list` query filtering, and `glyph_search_tools` for deferred/infinite tool catalogs.
- `glyph snapshot inventory` and `glyph_snapshot_inventory` for authoring hints from a recorded screen.
- `glyph replay --html` and `screens/trace.html` in run packs.
- `glyph record` captures keystrokes (raw TTY) into `raw/input.raw.log` and scaffolds `press`/`type` steps.

### Changed

- Unix PTY re-applies `winsize` shortly after start (Linux TUI SIGWINCH).

## [0.16.1] - 2026-07-24

### Security

- Replaced the remaining workplace-specific ticket example with a neutral
  reserved identifier.
- Centralized the repository privacy gate so local verification, CI, and
  release workflows reject prohibited workplace product and ticket markers in
  both tracked paths and tracked content.

## [0.16.0] - 2026-07-24

### Added

- Added `retention.archive.mode: fcheap-publish`, which packages a completed
  run as a deterministic, bounded evidence pack and publishes it through
  file.cheap with an expiring retention policy.
- Added strict validation of the credential-free `filecheap-publish/1` receipt
  before a local run may be removed.

### Changed

- TinyVault secret resolution now requires an explicit `only` allowlist or
  `prefix` selector and sends that selector to TinyVault before values are
  returned to Glyphrun.
- Configuration schema validation now rejects empty selectors and ambiguous
  direct-project versus environment-group secret sources.

### Security

- Targets, command and script verifiers, and transform subprocesses no longer
  inherit TinyVault control variables or the file.cheap publisher credential.
- External command diagnostics are redacted before reaching artifacts or
  machine-readable CLI results.
- Evidence packs reject symlinks and special files, use private temporary
  permissions, enforce file and entry limits, and remain local after any
  packaging, publication, timeout, or receipt-validation failure.
- Updated `golang.org/x/text` to the non-vulnerable v0.39.0 line.
- CI and release workflows reject private employer references in tracked
  release content.

[Unreleased]: https://github.com/abdul-hamid-achik/glyphrun/compare/v0.18.0...HEAD
[0.18.0]: https://github.com/abdul-hamid-achik/glyphrun/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/abdul-hamid-achik/glyphrun/compare/v0.16.1...v0.17.0
[0.16.1]: https://github.com/abdul-hamid-achik/glyphrun/compare/v0.16.0...v0.16.1
[0.16.0]: https://github.com/abdul-hamid-achik/glyphrun/compare/v0.15.0...v0.16.0
