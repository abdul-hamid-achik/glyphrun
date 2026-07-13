---
description: glyph mcp starts a stdio MCP server that mirrors the CLI, so coding agents can run terminal behavior specs, verify contracts, and read failure context from any MCP client.
---

# MCP

Run `glyph mcp` to start the stdio MCP server.

The server exposes `glyph_explain`, `glyph_docs`, `glyph_doctor`, `glyph_spec_verify`, `glyph_spec_scaffold`, `glyph_run`, `glyph_snapshot_update`, `glyph_diff`, `glyph_context`, `glyph_render`, and `glyph_repair`. Tools call the same internal paths as the CLI so agents get the same validation, artifact packs, and exit behavior.

`glyph_spec_scaffold` accepts `kind: "spec"` or `kind: "action"` so agents can create reusable action snippets without guessing the YAML shape.

`glyph_render` returns a deterministic SVG of a run's final screen (or a named snapshot). `glyph_repair` analyzes a spec's failed run and proposes step fixes; with `write: true` it applies them, only ever editing `steps` so the contract hash stays valid.

## Transport framing

The server speaks newline-delimited JSON-RPC over stdio (each message is a single JSON object followed by `\n`), which is what the MCP spec defines for stdio transport and what Claude Code, Codex, OpenCode, and Claude Desktop expect. The input reader also accepts `Content-Length`-framed requests for backwards compatibility with LSP-style clients, but responses are always line-delimited. Do not add `Content-Length` framing to the output — it breaks the Claude Code health check.
