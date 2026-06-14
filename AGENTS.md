# AGENTS.md - Glyphrun

Glyphrun is a local-first terminal behavior spec runner. Specs declare `intent + outcomes` as the behavior contract and `steps` as repairable hints.

## Working On This Project

This section is for agents (or humans) making code changes to Glyphrun itself. If you are using Glyphrun to test a terminal app you maintain, skip to the next section.

### Build, Test, Lint

```bash
task verify          # fmt + vet + test + build + doctor
task example         # build the example app and run the hello spec
task context         # print agent_context.md for the most recent run
go test ./...        # just tests
go test -run <name>   # one test
```

`task verify` is the gate. CI runs the same checks. Do not skip `go vet` or `gofmt`.

### Architecture

Package boundaries are part of the contract — do not blur them.

| Package | Owns |
|---|---|
| `cmd/glyph` | Entrypoint only. Defers to `internal/cli.Execute()`. |
| `internal/cli` | Cobra command handlers. No business logic. |
| `internal/spec` | Spec model, parsing, validation, contract hash, stamping. |
| `internal/ptyrunner` | Process backend behind a platform `backend` interface: Unix PTY (`creack/pty`) and Windows ConPTY. No terminal semantics. |
| `internal/terminal` | Virtual emulator + adapters (`gote`, `fake`). No PTY or spec knowledge. |
| `internal/runner` | Step execution and outcome evaluation. The orchestrator. |
| `internal/artifacts` | Writer, markdown, redaction, diffs. No runner state. |
| `internal/render` | Deterministic SVG rendering of a screen snapshot. Pure. |
| `internal/repair` | Failed-run analysis → step-repair proposals. No cobra. |
| `internal/flaky` | Stability/divergence summary for repeated runs. Pure. |
| `internal/scaffold` | Draft spec inference from a recorded session. |
| `internal/ghreport` | GitHub PR-comment Markdown rendering. |
| `internal/tui` | Interactive frame scrubber (`replay --tui`). The only Bubble Tea dependency; keep it isolated here. |
| `internal/mcp` | Stdio MCP server. Thin pass-through to CLI commands. |
| `internal/config` | Config loading, defaults, schema validation. |
| `internal/input` | Key name → escape sequence mapping. Pure function. |
| `internal/docs` | Built-in documentation text. |

The "no per-agent code paths" rule applies: any surface that touches a coding agent must go through the regular CLI / MCP / artifact surface, not a sidecar.

### Code Conventions

- Go 1.26 toolchain (see `.tool-versions`).
- `gofmt` clean, `go vet` clean, no third-party deps beyond what is in `go.mod`.
- Table-driven tests for parsers, verifiers, and key mappings.
- Prefer value receivers; use pointer receivers when the type owns mutable state (e.g. `*runState`).
- Comment exported types and non-obvious unexported functions. Comments are part of the docs.
- Keep `internal/cli` thin. New commands wire flags into `globalOptions` and call into runner/artifacts; do not put logic in handlers.
- Schema changes go in `schemas/*.json` and the corresponding model in `internal/spec/model.go` together.
- Avoid premature interface extraction. The `terminal.Emulator` interface exists because there are two adapters; do not add interfaces for single implementations.

### Common Tasks

- **Add a new step kind**: extend `spec.Step` in `internal/spec/model.go`, add a case in `validateStep` (`internal/spec/verify.go`), add a case in `executeStep` (`internal/runner/runner.go`), add a `stepSummary` branch in `internal/cli/progress.go`. Add a JSON-schema `oneOf` branch in `schemas/glyphrun.spec.v1.schema.json`. Add an example spec under `examples/specs/`. Update the docs vocabulary in **both** `internal/docs/docs.go` (served by `glyph docs`) and the mirror in `docs/steps.md`, and refresh the `glyph explain` lists in `internal/cli/explain.go`.
- **Add a new verifier**: same shape as a new step kind, but on the outcome side. The dispatch lives in `checkVerify` in `internal/runner/runner.go`. Update the verifier vocabulary in `internal/docs/docs.go`, `docs/verifiers.md`, and `internal/cli/explain.go`.
- **Add a new CLI command**: create `internal/cli/<name>.go`, register in `newRootCommand` in `internal/cli/root.go`. Always accept `--format` and route through `resolveFormat` + `emitForCLI`. JSON/YAML output must never prompt or read stdin. Add the command to the `commands` list in `internal/cli/explain.go`.
- **Add a new artifact field**: extend `artifacts.RunResult` (`internal/artifacts/types.go`), populate in `runner.finish`, and surface in `RenderRunMarkdown` and `RenderAgentContext`. Update `schemas/glyphrun.run.v1.schema.json`.
- **Add a new redaction pattern**: append to `Defaults().Redaction.Patterns` in `internal/config/config.go`. The redactor compiles them on construction.

### Things To Avoid

- Editing `intent` or `outcomes` of an existing example spec without updating its `contractHash` (use `glyph spec verify --stamp`).
- Changing `contract_hash.go` ordering or serialization — Go's `encoding/json` sorts map keys, so the hash is stable today; a struct refactor would break it silently.
- Writing secrets to artifacts. The redaction layer is best-effort, especially for raw PTY logs.
- Interactive prompts in JSON/YAML code paths. Agents use those modes; TTY-only behavior must be guarded by `isTerminalWriter`.
- Wholesale struct replacement in config merge (`base.X = overlay.X`) — it loses defaults that the user did not explicitly set. Use per-field checks or a defaults-aware merge.
- Ad-hoc goroutines without a clear lifecycle. The `runState` mutex is the only synchronization primitive in the runner; if you need a new channel or goroutine, document why.

### Commit Conventions

- One logical change per commit. Multi-purpose commits make bisect and revert painful.
- Subject line ≤ 72 chars, imperative mood ("Fix ...", "Add ...", not "Fixed ...").
- Body explains *why*, not *what* (the diff shows the what).
- Reference any issue or spec section that motivated the change.
- Run `task verify` before pushing. The CI matrix is the same; if it fails locally it will fail there.
- Do not push directly to `main` — open a PR even for small fixes so the diff is reviewable.

## Required Agent Behavior

- Run `glyph agent --format md` when entering a Glyphrun-enabled repository for the first time.
- Run `glyph explain --format json` before assuming the current CLI/spec surface.
- Use `glyph docs <topic> --format json` for focused authoring guidance.
- Use `glyph docs snippets --format md` before creating reusable action files or conditional steps.
- Use `glyph spec verify <spec> --format json` before running a spec.
- Use `glyph run <spec> --format json` for acceptance checks.
- Use `glyph context latest --format md` after a failure.
- Do not edit `intent` or `outcomes` of an existing spec without surfacing the diff.
- Do not change `contractHash` manually. Use `glyph spec verify --stamp`.
- Do not add per-agent code paths.
- Do not write secrets to artifacts.
- Keep CLI JSON/YAML paths non-interactive.
- Keep parser, runner, PTY backend, emulator, verifiers, and artifacts separate.

## Useful Human Commands

- `glyph doctor --format md`
- `glyph docs topics --format md`
- `glyph run <spec> --format md`
- `glyph context latest --format md`

Markdown output may use ANSI color in a real terminal. Use `--no-color` for plain output.
