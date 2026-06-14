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
glyph init --cmd ./bin/app --ready "ready" --format md
glyph spec verify specs/glyphrun/smoke.yml --format json
glyph run specs/glyphrun/smoke.yml --format md
glyph context latest --format md
```

If you only want the YAML printed to stdout, use `glyph spec scaffold > specs/smoke.yml`.

To bootstrap a spec from a real session instead of from scratch, run `glyph record --scaffold specs/smoke.yml -- ./bin/app`. It writes a draft spec (target, terminal size, an inferred "ready" string, and a `clean_exit` outcome) with its contract hash stamped, ready to edit.

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
