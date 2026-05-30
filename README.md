# Glyphrun

[![CI](https://github.com/abdul-hamid-achik/glyphrun/actions/workflows/ci.yml/badge.svg)](https://github.com/abdul-hamid-achik/glyphrun/actions/workflows/ci.yml)

Glyphrun is a local-first behavior runner for terminal applications and interactive CLI workflows. It launches a target command inside a real pseudo-terminal, drives it from YAML or JSON steps, evaluates outcomes against a deterministic virtual terminal screen, and writes a self-contained artifact pack for humans and coding agents.

The command-line binary is `glyph`.

## Why Glyphrun Exists

Most terminal tests either bind directly to application internals or depend on a human terminal app. Glyphrun keeps the target application black-box: if it can run in a PTY, Glyphrun can drive it. Specs describe the user-visible behavior, not the implementation framework.

Glyphrun is designed around four stable surfaces:

- A CLI for local use and automation.
- YAML/JSON behavior specs that can target any language or terminal app.
- Artifact packs with run evidence, terminal frames, snapshots, and diagnostics.
- An optional MCP server so coding agents can inspect docs, run specs, and read failure context without special per-agent integrations.

## Status

This repository contains a working MVP. It supports PTY execution, spec parsing and validation, contract hashes, snapshots, structured output, artifact packs, basic recording/replay, run diffs, and an MCP stdio server.

Some terminal features are intentionally still future work: full xterm parity, mouse protocols, Sixel/images, terminal hyperlinks, and Windows ConPTY support.

## Requirements

- Go 1.26.x
- macOS or another Unix-like environment with PTY support
- Optional: `asdf` and `task`

The pinned toolchain is in `.tool-versions`.

## Quick Start

```bash
asdf install
task verify
task example
```

Without `task`, use the underlying Go commands:

```bash
go mod tidy
go test ./...
go build -o ./bin/glyph ./cmd/glyph
./bin/glyph doctor --format json
./bin/glyph run examples/specs/hello.yml --format md
```

To install the CLI globally from this checkout:

```bash
go build -o ~/.local/bin/glyph ./cmd/glyph
glyph doctor --format md
```

After a run, inspect the newest agent-readable context:

```bash
./bin/glyph context latest --format md
```

For a guided command map:

```bash
glyph agent --format md
glyph docs topics --format md
glyph explain --format json
```

## Example Spec

Specs separate the behavior contract from the repairable path used to reach it. `intent` and `outcomes` define the contract. `steps` are the interaction path and can be repaired without changing the contract hash.

```yaml
version: 1
name: hello_quits

intent: |
  a user can open the hello demo app and quit with q.

target:
  cmd: ["${vars.helloBin}"]
  cwd: "."

terminal:
  cols: 80
  rows: 24
  profile: xterm-256color

steps:
  - wait:
      screen:
        contains: "hello from glyphrun"
  - snapshot: home
  - press: "q"
  - wait:
      process:
        exitCode: 0

outcomes:
  - id: greeting_visible
    description: the greeting is visible on the rendered terminal screen
    verify:
      screen:
        contains: "hello from glyphrun"
  - id: clean_exit
    description: q exits the application cleanly
    verify:
      process:
        exitCode: 0
```

Run and verify:

```bash
./bin/glyph spec verify examples/specs/hello.yml --format json
./bin/glyph run examples/specs/hello.yml --format json
./bin/glyph snapshot update examples/specs/hello.yml --format md
```

## CLI Commands

```text
glyph run <spec...>                 Run one or more behavior specs
glyph spec verify <spec> [--stamp]  Validate a spec and optionally stamp its contract hash
glyph spec scaffold                 Print a starter spec
glyph snapshot update <spec...>     Refresh committed terminal snapshots
glyph diff <runA> <runB>            Compare two run artifact directories
glyph record -- <command...>        Capture a PTY session as an artifact pack
glyph replay <run>                  Replay or print a recorded PTY log
glyph context <run|latest>          Print agent-focused failure/run context
glyph docs [topic]                  Print built-in docs
glyph agent                         Print the recommended agent workflow
glyph explain                       Explain project concepts and command flow
glyph doctor                        Check local setup
glyph mcp                           Start the MCP stdio server
```

Agent-callable commands support `--format json|yaml|md`. JSON and YAML modes do not prompt interactively. Markdown is the default human report format.

## Human And Agent DX

For humans, `--format md` prints a compact report with run status, target command, terminal size, outcome counts, failure focus, key artifact paths, and suggested next commands. Markdown output is colorized on real terminals. Set `GLYPHRUN_COLOR=always` to force color, `GLYPHRUN_COLOR=never` or `--no-color` to disable it, and `NO_COLOR=1` to follow the common no-color convention.

For agents, start with:

```bash
glyph agent --format md
glyph explain --format json
glyph docs agents --format md
glyph spec verify <spec> --format json
glyph run <spec> --format json
glyph context latest --format md
```

The agent contract is simple: treat `intent` and `outcomes` as the behavior contract, treat `steps` as repairable navigation hints, and use `glyph context latest` after failures before editing code.

## MCP

Run `glyph mcp` to start the stdio MCP server. The MCP tools mirror the CLI surface for docs, doctor checks, spec verification, spec scaffolding, runs, snapshot updates, diffs, and agent context lookup.

## Artifact Packs

Every `glyph run` writes a run directory under `.glyphrun/runs/` by default. Depending on config and spec settings, a pack can include:

- `run.json`, `run.yaml`, and `run.md`
- `agent_context.md`
- `events.ndjson`
- `spec.resolved.yml`
- `screens/final.txt` and `screens/final.json`
- `frames/frames.ndjson`
- `raw/pty.raw.log`
- `snapshots/*.txt` and `snapshots/*.json`
- `outcomes/results.*`
- `diagnostics/*.md`

Run artifacts are ignored by Git. Committed snapshots can live under `.glyphrun/snapshots/` when you choose to update them.

The most useful files during debugging are `run.md`, `agent_context.md`, `diagnostics/failure.md`, `screens/final.txt`, and `frames/frames.ndjson`.

## Configuration

Glyphrun reads `glyphrun.config.yml` from the working tree. Config can define shared variables, default terminal size/profile, artifact behavior, redaction rules, and text normalization.

Specs can override relevant settings locally. Secrets should be passed through environment variables or external setup, not hard-coded in specs.

## Project Layout

```text
cmd/glyph/              CLI entrypoint
internal/cli/           Cobra command handlers
internal/spec/          Spec model, parsing, validation, stamping
internal/config/        Config loading and schema validation
internal/ptyrunner/     PTY process backend
internal/terminal/      Virtual terminal emulator
internal/runner/        Step execution and outcome evaluation
internal/artifacts/     Artifact writer, markdown, redaction, diffs
internal/mcp/           MCP stdio server
schemas/                JSON schemas for specs, config, and run output
docs/                   Built-in documentation topics
examples/               Small runnable terminal app and spec
```

## Development

```bash
task verify
task example
task context
```

`task verify` runs formatting, vetting, tests, build, and `glyph doctor`. The same checks can be run manually with:

```bash
gofmt -w ./cmd ./internal
go vet ./...
go test ./...
go build -o ./bin/glyph ./cmd/glyph
./bin/glyph doctor --format md
```

## Security Model

Glyphrun specs are trusted local automation, similar to shell scripts or Playwright tests. A spec can launch commands, pass environment variables, and write artifacts. Do not run untrusted specs.

Artifacts are redacted by default using configured patterns, but raw PTY logs can still contain sensitive output if the target app prints it. Review artifact packs before sharing them.

## License

MIT
