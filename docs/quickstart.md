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

Use live progress while iterating locally:

```bash
glyph run specs/smoke.yml --format md --progress auto
```

Progress goes to stderr. The final report stays on stdout.

Try the reusable action and conditional examples:

```bash
glyph run examples/specs/reusable_action.yml --format md
glyph run examples/specs/conditional_step.yml --format md
```
