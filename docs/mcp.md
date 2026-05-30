# MCP

Run `glyph mcp` to start the stdio MCP server.

The server exposes `glyph_explain`, `glyph_docs`, `glyph_doctor`, `glyph_spec_verify`, `glyph_spec_scaffold`, `glyph_run`, `glyph_snapshot_update`, `glyph_diff`, and `glyph_context`. Tools call the same internal paths as the CLI so agents get the same validation, artifact packs, and exit behavior.

`glyph_spec_scaffold` accepts `kind: "spec"` or `kind: "action"` so agents can create reusable action snippets without guessing the YAML shape.
