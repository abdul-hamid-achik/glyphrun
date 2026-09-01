---
description: glyph mcp starts a stdio MCP server that mirrors the CLI, so coding agents can run terminal behavior specs, verify contracts, and read failure context from any MCP client.
---

# MCP

Run `glyph mcp` to start the stdio MCP server.

The server is dual-era. Legacy MCP clients handshake with `initialize` (`2024-11-05` and `2025-11-25`). Modern clients (`2026-07-28`) can call `server/discover` to learn supported versions and capabilities (`tools.filtering`). `tools/list` accepts an optional `query` filter. `glyph_search_tools` is the catalog search tool so a host can defer loading every schema and still discover `glyph_run`, repair, inventory, and the rest on demand.

The server exposes `glyph_search_tools`, `glyph_explain`, `glyph_docs`, `glyph_doctor`, `glyph_list`, `glyph_stories`, `glyph_stories_run`, `glyph_spec_verify`, `glyph_spec_scaffold`, `glyph_run`, `glyph_snapshot_update`, `glyph_snapshot_inventory`, `glyph_diff`, `glyph_context`, `glyph_render`, `glyph_repair`, `glyph_affected_specs`, and `glyph_clean`. Tools call the same internal paths as the CLI so agents get the same validation, artifact packs, and exit behavior.

`glyph_doctor` runs the full prerequisite matrix (platform/PTY/config/artifacts/emulator), not a config smoke test.

`glyph_list` returns specs under given paths (name, path, coversSymbol).

`glyph_spec_scaffold` accepts `kind: "spec"`, `kind: "action"`, or `kind: "story"` so agents can create reusable action snippets or a story spec without guessing the YAML shape.

`glyph_stories` returns the stories catalog (`stories.yml` manifests, expanded times their variants, plus specs tagged `story`, joined to their newest result and golden status). `glyph_stories_run` builds each manifest's harness once and runs every story (or the `only` selection), capturing missing goldens (`strict` fails instead) and refreshing the stories index; it accepts `paths`, `only`, `update`, `strict`, and `parallel` and returns the run report. HTML inspect, the terminal catalog, and `stories serve` are CLI-only (`glyph stories --html`, `glyph stories --tui`, `glyph stories serve`).

`glyph_render` returns a deterministic SVG of a run's final screen (or a named snapshot). `glyph_repair` analyzes a spec's failed run and proposes step fixes; with `write: true` it applies them, only ever editing `steps` so the contract hash stays valid. Set `verify: true` for a transactional cold-start verification (SPEC §7.2) before applying.

`glyph_affected_specs` selects the specs a git change can hit: it shells out to `codemap review --json` and intersects each spec's `coversSymbol` against the changed symbols plus their blast radius, returning the minimal spec set to run via `glyph_run`. One of `since`/`staged` selects the diff scope; passing neither means the working tree.

`glyph_clean` prunes old run directories from the artifact root per `retention.keepRuns` (default 3), archiving pruned dirs to the configured external command before deleting them when `retention.archive` is set. `all: true` wipes every run dir; `noArchive: true` deletes locally without archiving.

## Transport framing

The server speaks newline-delimited JSON-RPC over stdio (each message is a single JSON object followed by `\n`), which is what the MCP spec defines for stdio transport and what Claude Code, Codex, OpenCode, and Claude Desktop expect. The input reader also accepts `Content-Length`-framed requests for backwards compatibility with LSP-style clients, but responses are always line-delimited. Do not add `Content-Length` framing to the output — it breaks the Claude Code health check.
