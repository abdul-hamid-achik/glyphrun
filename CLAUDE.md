# CLAUDE.md

This file guides Claude Code (and other coding agents) working in this repository.

**The canonical contributor guide is [AGENTS.md](./AGENTS.md).** Read it first — it
covers the architecture, package boundaries, code conventions, and the required
agent workflow. This file only restates the essentials and the few things worth
repeating up front. When the two disagree, AGENTS.md wins.

## The gate

`task verify` (fmt + vet + test + build + doctor) is the gate. CI runs the same
checks. Do not skip `gofmt` or `go vet`. Just the tests: `go test ./...`.

## Architecture in one line

`cmd/glyph` → `internal/cli` (thin Cobra handlers) → `internal/runner` (the
orchestrator) over `internal/ptyrunner` (process) + `internal/terminal` (virtual
emulator), evaluating `internal/spec` outcomes and writing `internal/artifacts`.
`internal/mcp` is a thin pass-through to the CLI surface. Package boundaries are
part of the contract — keep PTY, emulator, runner, spec, verifiers, and artifacts
separate, and keep `internal/cli` free of business logic.

## Non-negotiables (see AGENTS.md for the full list)

- No per-agent code paths. Anything an agent touches goes through the regular
  CLI / MCP / artifact surface.
- Do not edit a spec's `intent` or `outcomes` without surfacing the diff, and
  never hand-edit `contractHash` — use `glyph spec verify --stamp`.
- Keep CLI JSON/YAML output non-interactive (never prompt or read stdin).
- Do not write secrets to artifacts.
- Exit codes 1–7 are reserved with distinct meanings (see the table in
  [README.md](./README.md#exit-codes)). Reuse the right code; don't invent new ones.
- Don't push directly to `main` — open a PR even for small fixes.

## Built-in docs

The `glyph docs` command serves the embedded docs in `internal/docs/docs.go`.
The standalone `docs/*.md` files mirror that content for readers browsing the
repo. When you change the step or verifier vocabulary, update **both** plus the
JSON schema under `schemas/` and an example under `examples/specs/`.
