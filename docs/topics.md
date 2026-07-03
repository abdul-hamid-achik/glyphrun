# Docs Topics

Every built-in topic below is available in-binary with `glyph docs <topic> --format md` (or `--format json`). The site mirrors them as dedicated pages — use the links to read on the web, or run the command from a terminal for the version shipped with your binary.

```bash
glyph docs topics --format md          # list every available topic in your binary
glyph docs <topic> --format md
```

## Getting started

- [overview](/overview) · [quickstart](/quickstart) · [authoring](/authoring) · [snippets](/snippets) · install (see [Distribution](/distribution))

## Steps & verifiers

- [steps](/steps) · [verifiers](/verifiers) · [file-script-verifiers](/file-script-verifiers) · [count-verifier](/count-verifier) · [process-telemetry](/process-telemetry) · [artifacts-pipeline](/artifacts-pipeline)

## Specs & config

- [contract-hash](/contract-hash) · [configuration](/configuration) · [redaction-block](/redaction-block) · capture-policy (see [Configuration](/configuration#capture-policy)) · retention (see [Configuration](/configuration#retention)) · [artifacts](/artifacts)

## Commands & integration

- [commands](/commands) — metadata-list, import-export, and rerun-failed are covered here
- [github](/github) · [distribution](/distribution) · [mcp](/mcp) · [agents](/agents) · [troubleshooting](/troubleshooting)

Use `glyph agent --format md` for the shortest agent bootstrap guide. Use `glyph explain --format json` for the authoritative list of commands, steps, verifiers, formats, and artifacts the running binary supports.