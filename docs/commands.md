# CLI Reference

Every command accepts `--format json|yaml|md` and writes its primary output to stdout (machine-readable in `json`/`yaml`, human-readable in `md`). Progress and diagnostics go to stderr. JSON/YAML paths never prompt or read stdin — they are safe for agents and CI.

## Global flags

These persistent flags apply to every command and must come before the subcommand's own flags.

| Flag | Default | Description |
| --- | --- | --- |
| `--config <path>` | discovered | Path to a `glyphrun.config.yml`. When omitted, Glyphrun walks up from the spec/cwd to find one. |
| `--artifact-root <path>` | from config | Override the run artifact root (default `.glyphrun/runs`). |
| `--format <fmt>` | `md` | Output format: `json`, `yaml`, or `md`. |
| `--env <name>` | none | Select a named config environment (e.g. `ci`, `local`). |
| `--quiet` | off | Suppress non-structured diagnostics. |
| `--verbose` | off | Enable verbose diagnostics. |
| `--no-color` | off | Disable ANSI color in Markdown output. |

## Project & spec setup

### `glyph init [dir]`

Initialize Glyphrun files in a project.

```bash
glyph init --cmd ./bin/app --ready "ready" --format md
```

Scaffolds a starter spec and config so a fresh checkout can `glyph run` immediately.

### `glyph spec verify <spec>`

Validate a spec and its contract hash. Run this before `glyph run` and before assuming a spec is current.

```bash
glyph spec verify specs/smoke.yml --format json
```

- Exits `6` on a contract-hash mismatch — the behavior contract changed without re-stamping.
- `--stamp`: write the computed `contractHash` back into the spec. Use **only** when `intent`/`outcomes` intentionally changed. Never edit the hash by hand.

### `glyph spec scaffold`

Print a starter spec (or reusable action) to stdout. Redirect to a file to seed a new spec.

```bash
glyph spec scaffold > specs/smoke.yml
glyph spec scaffold --kind action > examples/actions/wait_for_ready_and_quit.yml
glyph spec scaffold --coversSymbol MyApp.Quit > specs/quit.yml
```

| Flag | Applies to | Description |
| --- | --- | --- |
| `--kind <spec\|action>` | both | `spec` (default) prints a full spec; `action` prints a reusable step library with no contract. |
| `--coversSymbol <sym>` | `spec` only | Bind the starter spec to the code symbol it exercises, so `glyph affected-specs` can select it when that symbol's blast radius changes. Actions have no contract, so this is rejected for `--kind action`. |

### `glyph list [path...]`

Walk spec paths (files or directories; defaults to `.`) and print a compact table of every parseable spec — name, metadata, contract hash, and last run status. Specs that fail to parse are still listed (with a `parseError`) so the table always reflects the full input surface.

Discovery matches the runner's: directories named `actions/`, files starting with `_`, and files ending in `.draft.yml` are skipped.

```bash
glyph list --format md
glyph list specs/ --feature onboarding --tag smoke --owner payments --format json
```

| Flag | Description |
| --- | --- |
| `--feature <name>` | Filter to specs whose `metadata.feature` matches. |
| `--tag <name>` | Filter to specs whose `metadata.tags` includes the value. |
| `--owner <name>` | Filter to specs whose `metadata.owner` matches. |

## Running specs

### `glyph run <spec...>`

Run one or more terminal behavior specs. Each run writes a self-contained artifact pack under the artifact root (see [Artifacts](/artifacts)).

```bash
glyph run specs/smoke.yml --format md
glyph run specs/glyphrun/*.yml --junit glyphrun-junit.xml --format json
glyph run $(glyph affected-specs --since HEAD^) --format md
```

| Flag | Default | Description |
| --- | --- | --- |
| `--parallel <N>` | `1` | Number of specs to run concurrently. |
| `--progress <auto\|always\|never>` | `auto` | Live progress to stderr. The final report stays on stdout. |
| `--junit <path>` | off | Write a JUnit XML report (`.xml`) consumable by the GitHub test UI. |
| `--repeat <N>` | `1` | Run each spec N times and report flakiness/stability instead of a single result. Exits non-zero if any iteration of any spec fails. |
| `--rerun-failed` | off | Re-run only the specs that failed in the previous invocation (read from `<artifactRoot>/.last-failed.txt`). |
| `--update-snapshots` | off | Update committed terminal snapshots instead of comparing against them. |
| `--watch` | off | Re-run on spec/source changes. Interactive; requires `--format md`. Polls the filesystem (no file-notify dependency). |
| `--watch-path <path>` | none | Additional file or directory to watch (repeatable). Implies `--watch`. |
| `--monitor <path>` | off | Sample the spawned target's CPU/RSS via the `monitor` binary at this path; writes `diagnostics/process.{md,json}`. |
| `--monitor-interval <dur>` | `250ms` | Process-telemetry sample interval (use with `--monitor`). |
| `--monitor-profile <kind>` | off | Capture an end-of-run process profile: `heap\|cpu\|goroutine\|sample` (use with `--monitor`). |

The `--monitor` family, the `monitor:` step, and the `metrics:` verifier share one foundation — see [Process Telemetry](/process-telemetry).

### `glyph snapshot update <spec...>`

Run specs and rewrite their committed terminal snapshots. Use when a snapshot intentionally changed (a deliberate UI update, not a regression).

```bash
glyph snapshot update specs/smoke.yml --format md
```

This is the dedicated path for snapshot refresh; `glyph run --update-snapshots` does the same during a normal run.

## Iteration & diagnostics

### `glyph context <run|latest>`

Print the agent context artifact — the compact failure/run context an agent should read first after a failed run. `latest` resolves to the most recent run.

```bash
glyph context latest --format md
```

### `glyph diff <runA> <runB>`

Compare two artifact packs by run status, outcome results, and final screen text. Use it to pin down what changed between two runs.

```bash
glyph diff 20260629-001 20260629-002 --format md
```

### `glyph repair <spec> [run|latest]`

Analyze a failed run and propose fixes to a spec's `steps` — for example a `wait` that timed out because the on-screen text changed. Only `steps` are touched; `intent` and `outcomes` are the contract and are left alone, so applying a repair keeps the contract hash valid.

```bash
glyph repair specs/smoke.yml latest --format md      # print proposals
glyph repair specs/smoke.yml --write --format md    # apply them
glyph repair specs/smoke.yml --verify --format json  # verify: rerun, apply only if it passes
```

| Flag | Description |
| --- | --- |
| `--write` | Apply the proposed step rewrites to the spec file. Without it, proposals are printed only. |
| `--verify` | Apply to a temp copy, cold-start rerun, and write to the spec only if the rerun passes (SPEC §7.2 transactional repair). Returns `verified`, `confidence`, `beforeRun`/`afterRun` run IDs, retained `evidence` (the after run dir), and the exact `replay` command. On failure the original spec is untouched (rollback). |

### `glyph record -- <command...>`

Record a one-off terminal command into a Glyphrun artifact pack. Optionally infer a draft spec from the recorded session.

```bash
glyph record -- ./bin/app
glyph record --scaffold specs/smoke.yml -- ./bin/app
glyph record --scaffold specs/quit.yml --coversSymbol MyApp.Quit -- ./bin/app
```

| Flag | Description |
| --- | --- |
| `--timeout-ms <n>` | Stop recording after this timeout. |
| `--cwd <path>` | Working directory for the recorded command (default `.`). |
| `--scaffold <path>` | Write a draft spec inferred from the recorded session to this path. The draft carries a stamped contract hash, ready to edit. |
| `--coversSymbol <sym>` | Bind a scaffolded spec to the code symbol it exercises (use with `--scaffold`). |

### `glyph replay <run>`

Print a run's raw PTY log, or scrub its frames interactively.

```bash
glyph replay 20260629-001              # print raw PTY log
glyph replay 20260629-001 --tui        # interactive frame scrubber
```

`--tui` opens an interactive scrubber over `frames/frames.ndjson` — step with `←`/`→`, jump with `home`/`end`, and play back the captured frames to see exactly when the screen changed.

### `glyph render <run|latest>`

Render a run's final screen (or a named snapshot) to a deterministic SVG. The render is a pure function of the captured cell grid, so it is reproducible and safe to regenerate in CI.

```bash
glyph render latest --format md
glyph render 20260629-001 --screen ready --out -
```

| Flag | Description |
| --- | --- |
| `--screen <name>` | Render a captured snapshot by name instead of the final screen. |
| `--out <path>` | Output path. Use `-` to stream the raw SVG to stdout. |

### `glyph clean`

Prune old run directories from the artifact root. By default keeps the newest N runs per the project config (`retention.keepRuns`, default **3**) and prunes older ones. When `retention.archive` is configured, pruned runs are sent to the external archive command (e.g. `fcheap`) before being deleted locally (move semantics); on archive failure the local dir is preserved. Safe to run while a parallel `glyph run` is executing.

```bash
glyph clean --format md
glyph clean --keep 10 --format json
glyph clean --all --format md
glyph clean --no-archive          # delete locally without archiving first
```

| Flag | Description |
| --- | --- |
| `--keep <N>` | Keep the N newest runs (overrides `retention.keepRuns` for this invocation). `--keep 0` disables pruning — use `--all` to wipe everything. |
| `--all` | Remove every run directory under the artifact root. |
| `--no-archive` | Delete pruned run dirs locally without first archiving them (skips `retention.archive` for this invocation). |

## CI & integration

### `glyph comment [run|latest ...]`

Render GitHub-flavored Markdown summarizing one or more runs: a status table, failure focus, the final screen folded into a `<details>` block, and pointers to the deterministic `screens/final.svg` screenshots. Writes to stdout by default — pipe to `gh pr comment -F -`. See [GitHub Integration](/github).

```bash
glyph comment --last 50 | gh pr comment "$PR" -F -
```

| Flag | Description |
| --- | --- |
| `--last <N>` | Summarize the N most recent runs. |
| `--out <path>` | Write to a file instead of stdout. |

### `glyph affected-specs [path...]`

Select specs whose `coversSymbol` a git change can hit. Walks the given spec paths (defaults to `.`), parses every spec, and intersects each spec's `coversSymbol` against the changed symbols + blast radius reported by `codemap review`. This closes the structure→behavior loop: CI runs only the specs a change can reach.

```bash
glyph run $(glyph affected-specs --since HEAD^) --format json
glyph affected-specs --staged --format md
```

| Flag | Default | Description |
| --- | --- | --- |
| `--since <ref>` | none | Review changes since this git ref (committed + uncommitted). |
| `--staged` | off | Review only staged changes (the git index). |
| `--codemap <path>` | `codemap` | Path to the `codemap` binary (default: `$PATH`). |
| `--depth <N>` | `3` | Max blast-radius hops passed to `codemap review`. |

The diff→symbol work is delegated to `codemap`; Glyphrun owns only the spec↔symbol link (the `coversSymbol` field) and the intersection.

Glyphrun accepts legacy unversioned codemap review output and the canonical
`schema_version: 1` contract. The consumer fixture is an exact copy of codemap's producer golden.
An unknown future schema version is an error, not an empty blast radius, so contract drift cannot
silently skip all behavioral specs.

### `glyph import <format> <file>`

Convert a foreign test format to a glyphrun spec. The dispatcher is flat — `glyph import <format> <file>` — so new importers slot in without a tree of nested subcommands.

```bash
glyph import bats tests/smoke.bats --out specs/smoke_from_bats.yml
```

Supported formats:

- `bats` — a `.bats` file. One `@test` block per outcome; bodies are replayed through a per-spec runner script with the same line-level failure semantics as bats.

| Flag | Description |
| --- | --- |
| `--out <path>` | Output path (default: `<source>.yml` next to the source). |
| `--name <name>` | Override the spec name (default: derived from the source basename). |

### `glyph export <format> <spec>`

Convert a glyphrun spec to a foreign test format. The export is best-effort: steps/outcomes without a clean foreign mapping are wrapped in `TODO` comments for a human reviewer.

```bash
glyph export bats specs/smoke.yml --out tests/smoke.bats
```

Supported formats:

- `bats` — emits one `@test` per outcome, with the target command as the test body and the outcome's `command` verifier as the assertion. Screen-only outcomes are flagged with a `TODO: glyphrun screen verifier` comment.

| Flag | Description |
| --- | --- |
| `--out <path>` | Output path (default: `<source>.bats` next to the source). |

## Info & introspection

### `glyph docs [topic]`

Show focused, built-in documentation. The same topics back this site.

```bash
glyph docs topics --format md          # list every available topic
glyph docs steps --format md
glyph docs snippets --format md
```

See [Docs Topics](/topics) for the full list.

### `glyph agent`

Print the shortest agent-facing Glyphrun workflow guide. Run this when entering a Glyphrun-enabled repository for the first time.

```bash
glyph agent --format md
```

### `glyph explain`

Describe the current CLI/spec/artifact vocabulary — the authoritative list of commands, steps, verifiers, formats, progress modes, and artifacts the running binary supports. Run this before assuming the current surface.

```bash
glyph explain --format json
```

### `glyph doctor`

Check local Glyphrun prerequisites: active config, artifact root, schema source, and PTY availability. Run after install or when a run behaves unexpectedly.

```bash
glyph doctor --format md
```

### `glyph version`

Print the binary's version, commit, and build date. Mirrors `glyph --version` but is useful in non-interactive contexts where flag parsing may be limited.

```bash
glyph version --format json
```

### `glyph mcp`

Start the stdio MCP server, exposing the same internal paths as the CLI to agents. See [MCP](/mcp).

```bash
glyph mcp
```

## Spec-level fields that drive the CLI

These top-level spec fields are referenced by commands above and are worth knowing together.

### `metadata`

Org-facing classification, all optional. Lets `glyph list` and CI dashboards group and filter specs.

```yaml
metadata:
  feature: onboarding
  owner: payments
  priority: high
  tags: [smoke, critical]
```

### `coversSymbol`

Binds a spec to the code symbol it exercises. `glyph affected-specs` selects a spec when a change reaches that symbol's blast radius, so CI runs only the specs a diff can hit.

```yaml
coversSymbol: MyApp.Quit
```

Stamp a starter spec with this binding in one call:

```bash
glyph spec scaffold --coversSymbol MyApp.Quit > specs/quit.yml
```