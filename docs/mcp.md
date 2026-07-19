---
description: glyph mcp starts a stdio MCP server that mirrors the CLI, so coding agents can run terminal behavior specs, verify contracts, and read failure context from any MCP client.
---

# MCP

Run `glyph mcp` to start the stdio MCP server.

The server exposes `glyph_explain`, `glyph_docs`, `glyph_doctor`, `glyph_list`, `glyph_spec_verify`, `glyph_spec_scaffold`, `glyph_run`, `glyph_snapshot_update`, `glyph_diff`, `glyph_context`, `glyph_render`, `glyph_repair`, `glyph_affected_specs`, and `glyph_clean`. Tools call the same internal paths as the CLI so agents get the same validation, artifact packs, and exit behavior.

`glyph_doctor` runs the full prerequisite matrix (platform/PTY/config/artifacts/emulator), not a config smoke test.

`glyph_list` returns specs under given paths (name, path, coversSymbol).

`glyph_spec_scaffold` accepts `kind: "spec"` or `kind: "action"` so agents can create reusable action snippets without guessing the YAML shape.

`glyph_render` returns a deterministic SVG of a run's final screen (or a named snapshot). `glyph_repair` analyzes a spec's failed run and proposes step fixes; with `write: true` it applies them, only ever editing `steps` so the contract hash stays valid. Set `verify: true` for a transactional cold-start verification (SPEC §7.2) before applying.

`glyph_affected_specs` selects the specs a git change can hit: it shells out to `codemap review --json` and intersects each spec's `coversSymbol` against the changed symbols plus their blast radius, returning the minimal spec set to run via `glyph_run`. One of `since`/`staged` selects the diff scope; passing neither means the working tree.

`glyph_clean` prunes old run directories from the artifact root per `retention.keepRuns` (default 3), archiving pruned dirs to the configured external command before deleting them when `retention.archive` is set. `all: true` wipes every run dir; `noArchive: true` deletes locally without archiving.

## Transport framing

The server speaks newline-delimited JSON-RPC over stdio (each message is a single JSON object followed by `\n`), which is what the MCP spec defines for stdio transport and what Claude Code, Codex, OpenCode, and Claude Desktop expect. The input reader also accepts `Content-Length`-framed requests for backwards compatibility with LSP-style clients, but responses are always line-delimited. Do not add `Content-Length` framing to the output — it breaks the Claude Code health check.
