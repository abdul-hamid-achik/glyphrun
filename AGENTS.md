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

## Docs site (Vercel)

The public VitePress site uses repo-root `vercel.json`. Git auto-builds **`main`
only**. Feature branches do not create Preview deployments. `ignoreCommand`
skips non-docs commits. Do not `vercel promote`; `main` is the site release.
CLI binaries ship from tags / GoReleaser, not from a docs deploy.

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
| `internal/artifacts` | Writer, markdown, redaction, diffs, retention prune, external archival. No runner state. |
| `internal/render` | Deterministic SVG rendering of a screen snapshot. Pure. |
| `internal/repair` | Failed-run analysis → step-repair proposals. No cobra. |
| `internal/affected` | `codemap review --json` subprocess contract + changed-symbol/blast-radius intersection for `affected-specs`. Accept legacy unversioned/v1 output; reject unknown schema versions rather than selecting nothing. |
| `internal/flaky` | Stability/divergence summary for repeated runs. Pure. |
| `internal/scaffold` | Draft spec inference from a recorded session. |
| `internal/ghreport` | GitHub PR-comment Markdown rendering. |
| `internal/tui` | Interactive frame scrubber (`replay --tui`). The only Bubble Tea dependency; keep it isolated here. |
| `internal/mcp` | Stdio MCP server. Thin pass-through to CLI commands. |
| `internal/config` | Config loading, defaults, schema validation. |
| `internal/input` | Key name → escape sequence mapping. Pure function. |
| `internal/stories` | Stories manifest (`stories.yml`) model + expansion into specs, the retention-proof stories index, and the HTML inspect page. No PTY. |
| `internal/storyrun` | Schedules stories over the runner: discovers manifests/tagged specs, builds each harness once, runs in parallel, writes the index, and owns the `--watch` loop and `stories serve` HTTP server. The runner never imports it. |
| `internal/watchfs` | Shared polling filesystem change detector (fingerprint + poll interval) used by `run --watch` and `stories run/serve --watch`. |
| `internal/docs` | Built-in documentation text. |
| `internal/log` | Thin wrapper over charmbracelet/log. Configured once in `cli.Execute`. Shared diagnostic sink (stderr). No runner/artifact/config state. |

The "no per-agent code paths" rule applies: any surface that touches a coding agent must go through the regular CLI / MCP / artifact surface, not a sidecar.

### Code Conventions

- Go 1.26 toolchain (see `.tool-versions`).
- `gofmt` clean, `go vet` clean, no third-party deps beyond what is in `go.mod`.
- Table-driven tests for parsers, verifiers, and key mappings.
- Prefer value receivers; use pointer receivers when the type owns mutable state (e.g. `*runState`).
- Comment exported types and non-obvious unexported functions. Comments are part of the docs.
- Keep `internal/cli` thin. New commands wire flags into `globalOptions` and call into runner/artifacts; do not put logic in handlers.
- Diagnostics (non-result output) go through `internal/log` (`log.Info`/`Warn`/`Error`/`Debug`), never raw `fmt.Fprintf` to stderr. The run-progress UI in `internal/cli/progress.go` is the exception — it is the human status table, not log lines. `--format json|yaml` switches the logger to JSON lines so stderr stays machine-parseable; the command result always goes to stdout.
- Schema changes go in `schemas/*.json` and the corresponding model in `internal/spec/model.go` together.

### Common Tasks

- **Add a new step kind**: extend `spec.Step` in `internal/spec/model.go`, add a case in `validateStep` (`internal/spec/verify.go`), add a case in `executeStep` (`internal/runner/runner.go`), add a `stepSummary` branch in `internal/cli/progress.go`. Add a JSON-schema `oneOf` branch in `schemas/glyphrun.spec.v1.schema.json`. Add an example spec under `examples/specs/`. Update the docs vocabulary in **both** `internal/docs/docs.go` (served by `glyph docs`) and the mirror in `docs/steps.md`, and refresh the `glyph explain` lists in `internal/cli/explain.go`.
- **Add a new verifier**: same shape as a new step kind, but on the outcome side. The dispatch lives in `checkVerify` in `internal/runner/runner.go`. Update the verifier vocabulary in `internal/docs/docs.go`, `docs/verifiers.md`, and `internal/cli/explain.go`.
- **Add a new CLI command**: create `internal/cli/<name>.go`, register in `newRootCommand` in `internal/cli/root.go`. Always accept `--format` and route through `resolveFormat` + `emitForCLI`. JSON/YAML output must never prompt or read stdin. Add the command to the `commands` list in `internal/cli/explain.go`.
- **Add a new artifact field**: extend `artifacts.RunResult` (`internal/artifacts/types.go`), populate in `runner.finish`, and surface in `RenderRunMarkdown` and `RenderAgentContext`. Update `schemas/glyphrun.run.v1.schema.json`.
- **Add a new redaction pattern**: append to `Defaults().Redaction.Patterns` in `internal/config/config.go`. The redactor compiles them on construction.
- **Add a secrets provider**: the `secrets` config block (`internal/config/config.go`) currently supports only `tvault`. To add a provider, extend `Secrets.Provider`, add resolution logic in `internal/runner/secrets.go`, and update the schema `anyOf` for provider-specific validation. Resolved values must be added to the per-run redactor via `buildRedactor`.
- **Add a manifest field**: extend `stories.Manifest`/`Entry` in `internal/stories/manifest.go`, the schema `schemas/glyphrun.stories.v1.schema.json`, `expandOne`, and the docs in `docs/stories.md` + `internal/docs/docs.go`.

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

The vocabulary changes between releases, so read it from the binary instead of
memory: `glyph explain --format json` lists the current commands, steps, and
verifiers, and `glyph docs <topic>` (start with `authoring`, then `snippets`
for reusable actions) explains them. `glyph spec verify` before `glyph run`
turns a stale contract hash or a typo into a fast, structured error instead of
a failed run; after a failure, `glyph context latest --format md` is the
summary written for you.

A spec's `intent` and `outcomes` are the behavior contract that reviewers
approved, and `contractHash` is how CI proves nobody changed it quietly. Edit
them only when the expected behavior really changed, say so in the PR, and
re-stamp with `glyph spec verify --stamp` rather than hand-editing the hash.
Artifacts are shared and archived, so nothing secret goes into them; the
redaction layer is best-effort, not a guarantee. Any agent-facing behavior
goes through the regular CLI, MCP, and artifact surfaces, because a sidecar
path is one nobody tests. JSON and YAML output must stay non-interactive
(agents run those modes without a TTY), and the parser, runner, PTY backend,
emulator, verifiers, and artifacts stay separate packages.

## Useful Human Commands

- `glyph doctor --format md`
- `glyph docs topics --format md`
- `glyph run <spec> --format md`
- `glyph context latest --format md`

Markdown output may use ANSI color in a real terminal. Use `--no-color` for plain output.
