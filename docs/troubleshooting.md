# Troubleshooting

After a failure, run `glyph context latest --format md` and inspect `diagnostics/failure.md`, `screens/final.txt`, `frames/frames.ndjson`, and `raw/pty.raw.log`.

Errored and runner-level-failed runs carry `errorKind` + `diagnostic` in the JSON envelope (`glyph run <spec> --format json`). Check `errorKind` first — it maps to an actionable next step (`contract_hash_mismatch` → re-stamp, `timeout` → raise `timeoutMs`, `target_exited` → fix the target and inspect `raw/pty.raw.log` (not `timeoutMs`), `target_start` → fix `cmd`, `unsupported_terminal` → switch profile). The same envelope carries a `nextActions` array with the concrete command and reason — read it before re-deriving a fix.

Useful sequence:

```bash
glyph spec verify <spec> --format json
glyph run <spec> --format md
glyph context latest --format md
glyph diff <previous-run> <latest-run> --format md
```

For long or interactive specs, enable progress:

```bash
glyph run <spec> --format md --progress always
```

Progress is written to stderr; the final Markdown, JSON, or YAML report remains on stdout.

If the terminal screen looks wrong, compare `screens/final.txt` with `raw/pty.raw.log`. The screen file is normalized and assertion-friendly; the raw log is useful for escape sequence or terminal-emulation issues.

If a spec stopped reaching the expected state, adjust `steps`. If the expected behavior changed, update `intent` or `outcomes` deliberately and run `glyph spec verify --stamp`.
