# Changelog

All notable changes to Glyphrun are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
follows Semantic Versioning for its pre-1.0 release line.

## [Unreleased]

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
