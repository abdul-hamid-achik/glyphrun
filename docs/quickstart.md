# Quickstart

Build and check the CLI:

```bash
go build -o ./bin/glyph ./cmd/glyph
./bin/glyph doctor --format md
```

Install it globally from this checkout:

```bash
go build -o ~/.local/bin/glyph ./cmd/glyph
glyph doctor --format md
```

Create and run a starter spec:

```bash
glyph spec scaffold > specs/smoke.yml
glyph spec verify specs/smoke.yml --format json
glyph run specs/smoke.yml --format md
glyph context latest --format md
```

Use `--format json` or `--format yaml` for automation. Use `--format md` for readable terminal output and artifact summaries.
