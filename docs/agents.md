# Agents

Agents should use stable Glyphrun surfaces only: `glyph agent`, `glyph explain`, `glyph docs`, `glyph spec verify`, `glyph run`, `glyph context`, `glyph diff`, or the equivalent MCP tools from `glyph mcp`.

## Recommended Loop

```bash
glyph agent --format md
glyph explain --format json
glyph docs agents --format md
glyph docs snippets --format md
glyph spec verify <spec> --format json
glyph run <spec> --format json
glyph context latest --format md
```

Treat `intent` and `outcomes` as the behavior contract. Treat `steps` as repairable hints. When a contract hash needs to change, use `glyph spec verify --stamp` instead of editing the hash manually.

After a failed run, inspect `agent_context.md` first. If more detail is needed, inspect `diagnostics/failure.md`, `screens/final.txt`, `frames/frames.ndjson`, and `raw/pty.raw.log`.

Use `--format json` or `--format yaml` for machine parsing. Use `--format md` for human-readable terminal reports and PR comments.
