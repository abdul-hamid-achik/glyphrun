# AGENTS.md - Glyphrun

Glyphrun is a local-first terminal behavior spec runner. Specs declare `intent + outcomes` as the behavior contract and `steps` as repairable hints.

## Required Agent Behavior

- Run `glyph explain --format json` before assuming the current CLI/spec surface.
- Use `glyph docs <topic> --format json` for focused authoring guidance.
- Use `glyph spec verify <spec> --format json` before running a spec.
- Use `glyph run <spec> --format json` for acceptance checks.
- Use `glyph context latest --format md` after a failure.
- Do not edit `intent` or `outcomes` of an existing spec without surfacing the diff.
- Do not change `contractHash` manually. Use `glyph spec verify --stamp`.
- Do not add per-agent code paths.
- Do not write secrets to artifacts.
- Keep CLI JSON/YAML paths non-interactive.
- Keep parser, runner, PTY backend, emulator, verifiers, and artifacts separate.

