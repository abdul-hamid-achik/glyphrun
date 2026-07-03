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

Glyphrun is feature-complete for the v0.1 surface: PTY execution, spec parsing and validation, contract hashes, snapshots, structured output, artifact packs, recording and replay, run diffs, an MCP stdio server, file and script verifiers, an artifact pipeline (`download` / `transform` / `batch`), per-spec redaction, retention and `glyph clean`, `count:` verifier, per-spec capture policy, BATS import and export, JUnit output, and a `glyph list` catalog. Exit codes 1–7 are reserved with distinct meanings (see [Exit Codes](#exit-codes)).

The virtual terminal handles the common xterm control set: cursor movement and
absolute positioning, line/screen erase, SGR attributes and the full 16/256/
truecolor palette, alternate screen and bracketed paste, deferred autowrap,
scroll regions (DECSTBM) with region-aware line feed and reverse index, insert/
delete line (IL/DL), insert/delete character (ICH/DCH), scroll up/down (SU/SD),
save/restore cursor (DECSC/DECRC), and origin mode (DECOM).

It runs on Unix PTYs (macOS, Linux) and on Windows via ConPTY (Windows 10 1809+),
behind a platform-neutral backend interface. SGR colors (16/256/truecolor),
OSC 8 hyperlinks (`link` verifier), and mouse input (`mouse` step) are supported;
graphics string sequences (Sixel, Kitty) are consumed safely rather than rendered.

Some terminal features are intentionally still future work: remaining xterm edge
cases and rendering inline images.

## Requirements

- Go 1.26.x
- macOS or another Unix-like environment with PTY support, or Windows 10 1809+ (ConPTY)
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
./bin/glyph --version
./bin/glyph doctor --format json
./bin/glyph run examples/specs/hello.yml --format md
```

To install a published release (no checkout needed):

```bash
brew install abdul-hamid-achik/tap/glyph          # macOS / Linux
go install github.com/abdul-hamid-achik/glyphrun/cmd/glyph@latest
# or download a prebuilt archive from the Releases page
```

See [Distribution & Releasing](docs/distribution.md) for the release process and
Homebrew tap setup.

To install the CLI globally from this checkout:

```bash
task install
# → builds with version metadata, copies to /opt/homebrew/bin/glyph
# → prints `glyph --version` as confirmation
```

Without `task`:

```bash
go build \
  -ldflags "-X github.com/abdul-hamid-achik/glyphrun/internal/version.Version=v0.1.0 \
             -X github.com/abdul-hamid-achik/glyphrun/internal/version.Commit=$(git rev-parse --short HEAD) \
             -X github.com/abdul-hamid-achik/glyphrun/internal/version.BuildDate=$(date -u +%Y-%m-%d)" \
  -o /opt/homebrew/bin/glyph ./cmd/glyph
glyph --version
```

To add Glyphrun to another terminal project:

```bash
glyph init --cmd ./bin/app --ready "ready" --format md
glyph spec verify specs/glyphrun/smoke.yml --format json
glyph run specs/glyphrun/smoke.yml --format md --progress auto
```

After a run, inspect the newest agent-readable context:

```bash
glyph context latest --format md
```

For a guided command map:

```bash
glyph agent --format md
glyph docs topics --format md
glyph docs snippets --format md
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
glyph spec verify examples/specs/hello.yml --format json
glyph run examples/specs/hello.yml --format json
glyph snapshot update examples/specs/hello.yml --format md
```

## Scaffolding A Spec From A Recording

Authoring a spec from scratch is the main cost of adopting Glyphrun, so you can
bootstrap one from a real session:

```bash
glyph record --scaffold specs/glyphrun/app_smoke.yml -- ./bin/app
```

`record` drives the command in a PTY as usual and, with `--scaffold`, writes a
draft spec inferred from what it observed: the target command, the terminal
size, a representative "ready" string taken from the final screen, and a
`clean_exit` outcome when the process exited on its own. The draft's contract
hash is stamped so it runs immediately. Because `record` does not capture your
keystrokes, the *interaction* steps are left for you to fill in — `intent` and
`outcomes` are the contract, `steps` are repairable hints. After editing the
contract, re-stamp with `glyph spec verify --stamp`.

## CLI Commands

```text
glyph init [dir]                       Create config, .gitignore entries, and a starter smoke spec
glyph run <spec...>                    Run one or more behavior specs; --progress for live status
                                       --rerun-failed re-plays the most recent failures
                                       --junit <path> writes a JUnit XML report
                                       --repeat N reports flakiness/stability over N runs
                                       --watch re-runs on spec/source changes (--watch-path adds roots)
glyph spec verify <spec> [--stamp]     Validate a spec and optionally stamp its contract hash
glyph spec scaffold [--kind spec|action] Print a starter spec or reusable action
glyph snapshot update <spec...>        Refresh committed terminal snapshots
glyph diff <runA> <runB>               Compare two run artifact directories
glyph record -- <command...>           Capture a PTY session as an artifact pack
                                       --scaffold <path> also writes a draft spec from the session
glyph replay <run>                     Replay or print a recorded PTY log; --tui scrubs frames interactively
glyph render <run|latest>              Render a screen to a deterministic SVG (--screen <name>, --out path|-)
glyph context <run|latest>             Print agent-focused failure/run context
glyph repair <spec> [run|latest]       Propose step fixes for a failed run; --write applies them
glyph comment [run|latest ...]         Render a PR-comment Markdown summary (--last N, --out path)
glyph list <dir-or-file>...             Catalog specs with --feature/--tag/--owner filters
glyph clean --keep N | --all           Prune old run directories; --format json for CI
glyph import bats <file> [--out path]   Convert a BATS file into a glyphrun spec
glyph export bats <spec> [--out path]  Emit a BATS file from a glyphrun spec
glyph docs [topic]                     Print built-in docs (try `topics` for the index)
glyph version                          Print the binary's version, commit, and build date
glyph agent                            Print the recommended agent workflow
glyph explain                          Explain project concepts and command flow
glyph doctor                           Check local setup
glyph mcp                              Start the MCP stdio server
```

`glyph --version` and `glyph version --format json|yaml` are the same surface; the flag is the quick form, the subcommand is the programmatic one.

Agent-callable commands support `--format json|yaml|md`. JSON and YAML modes do not prompt interactively. Markdown is the default human report format.

## Verifiers

| Verifier | What it checks |
|---|---|
| `screen: { contains \| notContains \| regex }` | Substring or regex match against the normalized screen text |
| `region: { x, y, width, height, contains \| notContains \| regex }` | Same, restricted to a sub-region |
| `cell: { x, y, char, style }` | A specific cell's character and optional style attributes |
| `cursor: { x, y, visible }` | Cursor position and visibility |
| `process: { exitCode \| exited }` | Target process exit state |
| `snapshot: { name, mode? }` | A snapshot captured earlier in the spec |
| `file: { glob, contains?, timeoutMs? }` | A file matching the glob exists (optionally contains a needle) |
| `script: { runtime?, run \| file, fixtures?, timeoutMs? }` | An external Node module or shell script that returns `{ ok, evidence }` |
| `command: { run, cwd?, timeoutMs? }` | A trusted shell command (`test -x ./bin/app`) |
| `count: { region?, matches?, equals \| atLeast \| atMost \| between }` | Cell-level count over a region (Cairn's `count: { role }` analogue) |
| `link: { url?, text? }` | An OSC 8 hyperlink is present (URI substring and/or linked-text substring) |

`count.matches` is a single rune or the literal `"nonEmpty"`. The comparator is exactly one of `equals` / `atLeast` / `atMost` / `between`. The verifier returns `{ matched, comparator, expected }` as evidence, written to `outcomes/<id>.raw.json`.

## Steps

Supported v1 steps: `press`, `type`, `paste`, `send`, `mouse`, `wait`, `resize`, `snapshot`, `download`, `transform`, `batch`, and imported `use` actions. `mouse: { x, y, button?, action? }` sends a mouse event at a 0-based cell, encoded as SGR (1006) or legacy X10 depending on the mode the target enabled. Every step can carry a `when` guard that uses the same verifier shape as an outcome — useful for optional TUI prompts, login walls, or transient menus.

`download`, `transform`, and `batch` are the artifact pipeline:

- `download` copies a file the target wrote into the run dir, optionally waiting for it to stabilize. Set `assign: <name>` to make it addressable as `${artifacts.<name>.path}` in later steps.
- `transform` runs a Node module or shell script whose stdout becomes a new artifact. Pair it with `download` for "fetch + process" workflows.
- `batch` queues a list of `press` / `type` / `paste` / `send` sub-steps into a single PTY write so transient TUI state (a focused menu, a command palette, a hover popover) survives.

`paste` sends bracketed paste delimiters only after the target enables terminal mode `?2004`; otherwise it writes literal text.

## Reusable Actions And Conditional Steps

Glyphrun supports reusable action files for repeated TUI mechanics:

```yaml
imports:
  - ../actions/wait_for_hello_and_quit.yml

steps:
  - use: wait_for_hello_and_quit
```

Create an action starter with:

```bash
glyph spec scaffold --kind action
```

Steps can also be conditional:

```yaml
steps:
  - when:
      screen:
        contains: "optional prompt"
    press: "enter"
```

## Per-Spec Redaction

A spec can declare its own redaction values on top of the project config:

```yaml
redaction:
  values:
    - "[email protected]"
    - "fixture-api-key-abc123"
```

Values shorter than 4 characters are dropped, and the list is sorted longest-first so substrings are not shadowed. The `redaction:` block is part of the contract hash; changing it invalidates the stamp. See `glyph docs redaction-block` for the full shape.

## Contract Hash

Specs carry a `contractHash` stamped over `intent`, `outcomes`, `redaction:`, and `coversSymbol` (when set). `glyph run` refuses to start a spec whose on-disk content does not match the hash, exiting with code `6`. This catches silent contract drift: a contributor edits an outcome to make a flaky test pass, the hash stops matching, the run aborts, and the change shows up in review.

```bash
glyph spec verify <spec> --stamp    # regenerate the hash after an intentional contract change
glyph run <spec>                    # compares the stamp; mismatches are exit 6
```

## Run Hygiene

Retention, last-failed tracking, and explicit cleanup are first-class commands:

```yaml
# glyphrun.config.yml
retention:
  keepRuns: 20   # default is 3; 0 disables auto-prune
  # archive pruned runs to an external store (e.g. fcheap / file.cheap)
  # instead of deleting them. On exit 0 the local dir is removed (move);
  # on failure it is preserved. Archival never fails the run.
  archive:
    enabled: true
    command: fcheap
    args: ["store"]   # invoked as: fcheap store <runDir>
```

```bash
glyph clean --keep 10          # keep the 10 most recent runs, prune the rest
glyph clean --all               # wipe everything under the artifact root
glyph clean --no-archive        # delete locally without archiving first

glyph run <spec> --rerun-failed  # replay only the specs in .last-failed.txt
```

The default is **3**: a config that omits `retention.keepRuns` keeps the 3 newest run dirs; an explicit `0` disables auto-prune. Auto-prune runs after every successful run (best-effort; logged as `retention.pruned` / `retention.archived` in `events.ndjson`). The current run is always kept; the cap applies to historical runs only.

## Repairing Drifted Steps

When a `wait` times out because the UI changed (a renamed banner, a moved
prompt), the *navigation* is stale but the *contract* is fine. `glyph repair`
leans into that split:

```bash
glyph repair specs/glyphrun/smoke.yml            # propose step fixes from the last run
glyph repair specs/glyphrun/smoke.yml --write    # apply them in place
```

It reads the failing run, finds each timed-out `wait: screen: contains:` whose
text is no longer on screen, and proposes the closest on-screen line. `--write`
edits the spec surgically through the YAML node tree, touching only `steps` —
never `intent` or `outcomes` — so the stamped contract hash stays valid and no
re-stamp is needed. This is the agent self-heal loop the contract model implies,
exposed as a plain command (and the `glyph_repair` MCP tool) rather than a
per-agent code path.

## Flakiness Probe And Watch Mode

Two run modes help while iterating and before trusting a green suite:

```bash
glyph run <spec> --repeat 20 --format json   # run 20 times; report stability
glyph run <spec> --watch                     # re-run on spec/source changes
glyph run <spec> --watch --watch-path ./src  # also watch the app's source tree
```

`--repeat N` runs each spec `N` times and emits a flakiness report instead of a
single result: per spec it reports `passed`/`failed` counts, whether the run was
`stable` (identical outcomes *and* identical final screen every time), whether it
was `flaky` (some runs passed, some failed), and the first iteration that
diverged. It exits non-zero if any iteration failed, so CI catches both flaky and
consistently-failing specs. This is how the determinism Glyphrun promises is kept
honest.

`--watch` is an interactive, human-only loop (markdown output only): it polls the
spec directories — plus any `--watch-path` roots — and re-runs whenever a watched
file changes, skipping the artifact root and VCS metadata. Press Ctrl-C to stop.

## Per-Spec Capture Policy

Each artifact channel (snapshots, frames, raw log, final screen, agent context) can be set to `always`, `on-failure`, or `never`. The project config sets the defaults, and a spec can override individual channels:

```yaml
artifacts:
  frames: never
  rawLog: always
  finalScreen: always
```

Useful for two cases: expensive specs that emit thousands of frames per second (turn frames off in the spec, keep them on for the rest), and critical specs that you always want to debug (force `agentContext: always` and `rawLog: always` so the failure surface is there even on pass).

The `artifacts:` block is part of the contract hash.

## Human And Agent DX

For humans, `--format md` prints a compact report with run status, target command, terminal size, outcome counts, failure focus, key artifact paths, and suggested next commands. Markdown output is colorized on real terminals. Set `GLYPHRUN_COLOR=always` to force color, `GLYPHRUN_COLOR=never` or `--no-color` to disable it, and `NO_COLOR=1` to follow the common no-color convention.

`glyph run` also supports live progress for local terminal use:

```bash
glyph run examples/specs/hello.yml --format md --progress auto
glyph run examples/specs/hello.yml --format json --progress always
```

Progress is written to stderr so JSON/YAML stdout stays machine-readable. Use `--progress never` or `GLYPHRUN_PROGRESS=never` to disable it; use `GLYPHRUN_PROGRESS=always` to force it.

For agents, start with:

```bash
glyph agent --format md
glyph explain --format json
glyph docs agents --format md
glyph docs snippets --format md
glyph spec verify <spec> --format json
glyph run <spec> --format json
glyph context latest --format md
```

The agent contract is simple: treat `intent` and `outcomes` as the behavior contract, treat `steps` as repairable navigation hints, and use `glyph context latest` after failures before editing code.

## MCP

Run `glyph mcp` to start the stdio MCP server. The MCP tools mirror the CLI surface for docs, doctor checks, spec verification, spec scaffolding, runs, snapshot updates, diffs, agent context lookup, and the catalog (`glyph list`).

## GitHub Integration

Run specs in CI and surface the results on the pull request:

```bash
glyph run specs/glyphrun/*.yml --junit glyphrun-junit.xml --format json
glyph comment --last 50 | gh pr comment "$PR" -F -
```

`glyph comment` renders GitHub-flavored Markdown — a status table, failure
focus, the final screen in a `<details>` block, and pointers to the
deterministic `screens/final.svg` screenshots. It writes to stdout (so it pipes
into `gh pr comment -F -`) or to a file with `--out`. A reusable composite
action lives at [`.github/actions/glyphrun`](.github/actions/glyphrun/action.yml)
and an example workflow that posts a sticky PR comment is at
[`examples/github/glyphrun-pr.yml`](examples/github/glyphrun-pr.yml). See
`glyph docs github` for details.

## Artifact Packs

Every `glyph run` writes a run directory under `.glyphrun/runs/` by default. Depending on config and the spec's capture policy, a pack can include:

- `run.json`, `run.yaml`, and `run.md`
- `agent_context.md`
- `events.ndjson`
- `spec.resolved.yml`
- `screens/final.txt`, `screens/final.json`, and `screens/final.svg`
- `frames/frames.ndjson`
- `raw/pty.raw.log` and `raw/input.raw.log`
- `snapshots/*.txt` and `snapshots/*.json`
- `outcomes/results.*` and `outcomes/<id>.raw.json` (verifier evidence)
- `diagnostics/failure.md` and `diagnostics/environment.md`
- `.last-failed.txt` (consumed by `--rerun-failed`)

Run artifacts are ignored by Git. Committed snapshots can live under `.glyphrun/snapshots/` when you choose to update them.

The most useful files during debugging are `run.md`, `agent_context.md`, `diagnostics/failure.md`, `diagnostics/environment.md`, `screens/final.txt`, and `frames/frames.ndjson`. `agent_context.md` includes recent events and suggested inspection commands for coding agents.

## Configuration

Glyphrun reads `glyphrun.config.yml` from the working tree. Config can define shared variables, default terminal size and profile, artifact behavior, retention, capture defaults, and redaction rules.

```yaml
artifacts:
  snapshots: true
  frames: true
  rawLog: false
  finalScreen: true
  agentContext: true
retention:
  keepRuns: 20   # default is 3; 0 disables auto-prune
redaction:
  values: ["$HOME/.config/private-token"]
```

Specs override what they need: `target.timeoutMs` wraps the whole PTY session and maps to exit code `3` when it expires. `terminal.alternateScreen` supports `auto`, `require`, and `forbid`. Outcomes can set their own `timeoutMs` and `normalize` when a single assertion needs longer polling or custom volatile-text cleanup.

## Exit Codes

| Code | Meaning |
|---:|---|
| 0 | All outcomes passed |
| 1 | At least one outcome failed |
| 2 | Runtime error before outcomes could run |
| 3 | `target.timeoutMs` expired |
| 4 | Spec parse, validation, or config load error |
| 5 | Required alternate-screen mode was not entered |
| 6 | Contract hash mismatch (run refused before the PTY started) |
| 7 | Reserved |

## Project Layout

```text
cmd/glyph/              CLI entrypoint
internal/cli/           Cobra command handlers
internal/spec/          Spec model, parsing, validation, contract hash
internal/config/        Config loading and schema validation
internal/log/           Structured diagnostic logging (charmbracelet/log)
internal/ptyrunner/     PTY process backend
internal/terminal/      Virtual terminal emulator
internal/runner/        Step execution and outcome evaluation
internal/artifacts/     Artifact writer, markdown, redaction, retention, archival, last-failed
internal/repair/        Failed-run analysis and step-repair proposals
internal/flaky/         Stability/divergence summary for repeated runs
internal/scaffold/      Draft spec inference from a recorded session
internal/ghreport/      GitHub PR-comment Markdown rendering
internal/tui/           Interactive frame scrubber (Bubble Tea v2; replay --tui)
internal/version/       Build-time version metadata
internal/mcp/           MCP stdio server
schemas/                JSON schemas for specs, config, and run output
docs/                   Built-in documentation topics
examples/               Runnable terminal apps and specs
```

## Development

```bash
task verify
task example
task context
task install
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

Artifacts are redacted by default using configured patterns, and a spec can add per-spec redaction values on top of the project config. The redactor's minimum value length is 4 characters; longer values are matched first. Raw PTY logs can still contain sensitive output if the target app prints it, so review artifact packs before sharing them.

## License

MIT
