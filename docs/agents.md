# Agents

Agents should use stable Glyphrun surfaces only: `glyph explain`, `glyph docs`, `glyph spec verify`, `glyph run`, `glyph context`, or the equivalent MCP tools from `glyph mcp`.

Treat `intent` and `outcomes` as the behavior contract. Treat `steps` as repairable hints. When a contract hash needs to change, use `glyph spec verify --stamp` instead of editing the hash manually.
