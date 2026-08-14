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

## Known issue: TUI target shuts down early on Linux PTY (under investigation)

Observed 2026-08-04 while driving Teak (a Bubbletea TUI editor) from glyphrun specs:
three `tui_agent_*` specs pass on macOS but fail consistently on Linux. The target
completes its startup work (the ACP agent connects in ~0.5s and stays alive), reaches
the expected UI state for one frame, then the *target itself* shuts down before the
spec's first `wait` step can observe it. No crash and no `target_exited` from the
child process — the final frame shows Teak's own shutdown screen.

This is not yet fully root-caused on every Bubble Tea app, but Glyphrun now puts the Unix PTY master in raw mode and re-applies `winsize` shortly after start so Linux CI (non-TTY parent stdin) is less likely to look like EOF to the target. Re-test Teak/`tui_agent_*` on `ubuntu-latest` before treating Linux as a known failure.

Where to look if it still happens: the PTY open/write/close ordering in `internal/ptyrunner/backend_unix.go`.

```bash
docker run --rm -v $PWD:/glyphrun -v /path/to/teak:/src -w /src golang:1.26-bookworm \
  bash -c 'cd /glyphrun && go build -o /tmp/glyph ./cmd/glyph && cd /src && \
  go build -o bin/teak ./cmd/teak && /tmp/glyph run specs/tui_agent_prompt.yml --parallel 1'
```

Until this is fixed, Linux runs of specs against long-lived TUI targets may fail on
the first `wait` step even though the target itself starts correctly.
