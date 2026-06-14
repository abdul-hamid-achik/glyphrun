# Cairntrace × Glyphrun — Comparison & Roadmap

Date: 2026-06-13
Scope: side-by-side feature comparison of `cairntrace` (browser specs, v1.8.0) and
`glyphrun` (terminal specs, current `main`), plus prioritized improvement ideas
for glyphrun derived from cairn's design choices and the real-world flows under
`~/projects/automations/graphite/flows/`.

This is a **design document**, not a PR. The recommendations are sequenced by
effort/value and written so any of them can be lifted into a single PR without
touching the rest.

---

## 1. TL;DR

Both tools are cut from the same cloth:

- **Contract-first, repairable hints.** `intent + outcomes` is the behavior
  contract; `steps` are the path to reach it. Both stamp a `contractHash` over
  `intent + outcomes` and refuse to change it without an explicit `--stamp`.
- **Local-first, no cloud.** No telemetry, no remote services; everything runs
  in a real process the runner owns (real PTY for glyphrun, real Chrome session
  for cairn).
- **Self-contained artifact packs.** Each run writes a directory humans and
  agents can both consume: `run.{json,yaml,md}`, `agent_context.md`,
  `events.ndjson`, per-outcome evidence, snapshots, diagnostics.
- **Agent-first CLI.** Every command supports `--format json|yaml|md`. Markdown
  is the human report; JSON is the contract. Both ship a stdio MCP server that
  mirrors the CLI surface.
- **No per-agent code paths.** Both reject agent-specific behavior in core;
  everything goes through the CLI / MCP / artifact surface.

The split is in the **domain**: cairn drives a browser, glyphrun drives a PTY.
The shape of every piece is a 1:1 mirror, which is the whole reason a
comparison is useful — anything cairn has that glyphrun doesn't is by design
either irrelevant to terminals, or a real gap to close.

The big takeaways:

1. **Glyphrun is structurally a year or two behind cairntrace** in *feature
   surface*, but the *architecture* is the same. Almost every improvement here
   is "port this cairn idea into the PTY model."
2. **Three things are pure wins**: typed verifiers with matchers, a real
   `transform`/`download`/named-artifact pipeline, and a CI report format
   (JUnit). These are the highest-leverage additions.
3. **Two things glyphrun has that cairn doesn't** (and probably shouldn't grow
   on cairn): a deterministic virtual terminal emulator in-process, and a
   `record`/`replay` PTY harness. Those are terminal-specific gifts and should
   be highlighted in glyphrun's own docs.
4. **Cairn's heal flow is the killer feature** for browser drift. Glyphrun's
   equivalent — `snapshot update` — is the same idea in a different costume
   (screen snapshot vs. selector). Worth making that surface more discoverable
   in the agent guide.

---

## 2. Side-by-side feature matrix

Legend: ✅ present · ⚠️ partial / not first-class · ❌ missing · — N/A

### 2.1 Spec model

| Capability | Cairntrace | Glyphrun |
|---|---|---|
| `version: 1` discriminated schema | ✅ Zod, `strict()` | ✅ JSON Schema + Go struct |
| `intent` + `outcomes` as contract | ✅ | ✅ |
| `contractHash` over `intent + outcomes` | ✅ SHA-256, canonical-JSON | ✅ SHA-256, Go `encoding/json` (sorted keys) |
| `contractHash` enforced on `run` | ✅ exit 6 on mismatch | ⚠️ parsed but not enforced as exit code — see §4.1 |
| Reusable actions (`imports:` / `use:`) | ✅ | ✅ |
| `preconditions.commands` shell setup | ✅ | ✅ |
| `preconditions.env` literal env (no leakage) | ✅ | ❌ — only `target.env` overrides and global process env |
| Per-environment `vars` from config | ✅ `config.environments.<env>.vars` | ✅ `Runtime.Vars` from `environments.<env>.vars` |
| Spec-local `vars` (merged on top) | ✅ | ❌ — only env-vars and `--var` |
| `session.resume` (browser-state checkpoint) | ✅ `cairn login` + checkpoint store | — N/A (PTY is process-local) |
| `viewport` per-spec override | ✅ | — N/A (cols/rows per-spec instead) |
| `metadata` (feature/owner/priority/tags) | ✅ first-class | ❌ — no metadata block |
| `mode: normal | debug` | ✅ | ❌ |
| `redaction` block (per-spec) | ✅ `headers/queryParams/storageKeys/values` | ⚠️ config-only redaction, no per-spec block |
| `artifacts.capture.{screenshots,snapshots,console,network,trace,storage,agentContext}` policy | ✅ `always\|on-failure\|never` per channel | ⚠️ config-only on/off booleans (no per-channel policy) |
| `when:` conditional steps (predicate DSL) | ✅ `urlContains:/path`, `notAuthenticated`, … | ⚠️ only on a full `Verify` struct, no mini-DSL |

### 2.2 Step vocabulary

Cairn's step set is browser-shaped; glyphrun's is terminal-shaped. The table
shows equivalents where they exist, gaps otherwise.

| Concept | Cairn step | Glyphrun step | Notes |
|---|---|---|---|
| Open / navigate | `open: <path>` / `open: { path, waitUntil, timeoutMs }` | `target.cmd` (process-level) | glyphrun's "open" is starting the PTY; there's no navigation within it |
| Click | `click: { by, role, name }` | — | no pointer |
| Hover | `hover: { by, role, name }` | — | no pointer |
| Fill | `fill: { by, role, name, value }` | `type: "..."` / `paste: "..."` | glyphrun types into whatever has focus |
| Upload file | `upload: { by, path }` | `send: { bytes }` (raw) | glyphrun can do this via preconditions + pipes, but no first-class step |
| Download artifact | `download: { by, saveAs, assign }` | ❌ | **Major gap** — see §3.1 |
| Transform (Node script → artifact) | `transform: { runtime, file, input, saveAs, assign }` | ⚠️ via `command` verifier (one-shot, no artifact assignment) | **Major gap** — see §3.1 |
| Authenticated HTTP request | `request: { method, url, headers, body, expectStatus, assign }` | ❌ | no in-PTY HTTP. Could be useful for "test daemon started" checks. |
| Wait for text/load | `wait: { text \| notText \| load, timeoutMs }` | `wait: { screen: { contains \| notContains \| regex } \| process \| idle, timeoutMs }` | glyphrun's is richer (idle + process), but no `load: networkidle` equivalent — though that's a browser concept |
| Press key | `press: Enter` | `press: "ctrl+c"` | both key-name based, glyphrun is slightly richer (`input/keymap.go`) |
| Scroll | `scroll: { direction, px }` / `scroll: { to: locator }` | ❌ | TUI scrolling is VT100-based and can be asserted via `screen` |
| Snapshot | `snapshot: { interactive, label }` | `snapshot: <name>` (cell + text + JSON) | glyphrun captures emulator state; cairn captures accessibility tree |
| Use imported action | `use: <name>` | `use: <name>` | same |
| Composite (single invocation) | `batch: [sub, sub, …]` (selector-only) | ❌ | **Worth porting** — see §3.2 — a PTY batch of `press` + `wait` + `press` survives transient state |
| Reusable step IDs | `id:` per step | ❌ (only `step.<n>`) | helps failure messages + heal origins |
| `when:` on any step | ✅ mini-DSL string | ✅ full Verify struct | glyphrun's is more expressive; cairn's is more readable. Could adopt a simple `screen.contains:"X"` shorthand. |

### 2.3 Verifier / outcome vocabulary

This is where cairn has clearly run further. Each cairn verifier is a typed
shape with a small DSL of matchers; glyphrun verifiers are 1:1 with one
assertion (often requiring a `command:` for anything outside the screen).

| Outcome | Cairn | Glyphrun | Notes |
|---|---|---|---|
| Text on page / screen | `text: { equals \| contains \| matches }` | `screen: { contains \| notContains \| regex }` | glyphrun is regex-or-bust for "matches"; cairn exposes the full matcher fan-out |
| Text absent | `notText: { … }` | `screen: { notContains: … }` | ✅ both |
| URL post-condition | `url: { equals \| startsWith \| endsWith \| matches }` | — N/A | — |
| Network call happened | `network: { method, urlContains, status: { in: […] } }` | ❌ | TUI processes don't have a network log; this is browser-shaped |
| No failed requests | `noFailedRequests: { urlContains, method }` | ❌ | — |
| Console errors bounded | `console: { errorsMax }` | ❌ | could map to "PTY log contains no error pattern" but no first-class |
| Element count | `count: { role\|selector\|text, equals\|atLeast\|atMost\|between }` | ❌ | **Worth porting** as a `region.count` cell-count verifier |
| Workbook content (xlsx) | `xlsx: { path, sheets, validations }` | — N/A | TUI apps can `transform` an xlsx download via preconditions, so this is a stretch |
| File on disk | `file: { glob, contains, timeoutMs }` | ❌ (use `command:` + `test -f`) | **Worth porting** — see §3.3 |
| HTTP+JSON path | `httpJson: { url, jsonPath, equals\|contains\|matches\|atLeast\|atMost\|exists }` | ❌ | — N/A in pure TUI; useful for daemon integration |
| Cell at (x,y) | — | ✅ `cell: { x, y, char?, style }` | glyphrun's exclusive win |
| Cursor position | — | ✅ `cursor: { x, y, visible }` | glyphrun's exclusive win |
| Region (sub-rect) | — | ✅ `region: { x, y, width, height, contains\|notContains\|regex }` | glyphrun's exclusive win |
| Cell style | — | ✅ `style: { fg, bg, bold, dim, italic, underline, reverse }` | glyphrun's exclusive win |
| Process state | — | ✅ `process: { exitCode, exited }` | glyphrun's exclusive win |
| Snapshot match | — | ✅ `snapshot: { name, mode }` with text/cell/json modes | glyphrun's exclusive win |
| Shell command | ✅ `command: { run, cwd, timeoutMs }` (in preconditions) | ✅ `command:` verifier (in outcomes) | glyphrun can do it in *outcomes*; cairn only in preconditions |
| Script escape hatch | `script: { runtime: browser\|node, run\|file, fixtures }` | ⚠️ via `command:` + bash + `fixtures:` (env vars) | **Worth porting** — see §3.3 |

### 2.4 Backend / process model

| Concept | Cairntrace | Glyphrun |
|---|---|---|
| Pluggable backends | ✅ `BrowserBackend` interface; agent-browser (default), Playwright, Mock | ❌ (only one: `ptyrunner` w/ creack/pty) |
| Default backend | `agent-browser` | `ptyrunner` |
| Alternate backends | `--backend playwright` (native traces), `--backend mock` (smoke) | none |
| Per-step timeout + grace | ✅ 60s default, step-level `timeoutMs` + 5s | ✅ step-level `timeoutMs`, default 5s |
| Wedged-process killer | ✅ SIGKILL via execa timeout | ✅ cleanup window (`CleanupTimeout` = 2s) |
| SIGTERM/teardown of sibling sessions | ✅ daemon-aware (PID-file SIGTERM → SIGKILL) | N/A (one process per run) |
| Cold-start gate | ✅ `--cold-start` / `CI=true` clears browser state before run | N/A (each run is a fresh PTY) |
| Auto-retry on transient backend errors | ✅ 2x backoff on `os error 35` | N/A |
| Concurrency | ✅ `--parallel N` (each spec in its own browser session) | ❌ |
| Process pool | ✅ `src/core/runner/pool.ts` | ❌ |
| Pre-step viewport | ✅ spec viewport, set before first step | ✅ terminal cols/rows, set at PTY start |
| Page/network/console recording | ✅ `getNetworkRequests()`, `getConsole()`, `clearNetworkLog()` between runs | ✅ raw PTY log + frames, no clear-between-runs concept (one process per run) |

### 2.5 Artifacts and observability

| Concept | Cairntrace | Glyphrun |
|---|---|---|
| Self-contained run dir | ✅ | ✅ |
| `run.{json,yaml,md}` | ✅ | ✅ |
| `agent_context.md` | ✅ (compact, hand-curated) | ✅ (similar shape) |
| `events.ndjson` | ✅ typed | ✅ typed |
| `spec.resolved.yml` | ✅ after env/vars expansion | ✅ |
| Screenshots | ✅ `screenshots/<NN>_<stepId>.png` | — N/A |
| Screen text dumps | ✅ `snapshots/<NN>_<stepId>.txt` | ✅ `snapshots/<name>.txt` and `.json` |
| Final screen | implicit | ✅ `screens/final.{txt,json}` |
| Frames (timeline) | ❌ | ✅ `frames/frames.ndjson` (per `screen.Feed`) |
| Raw PTY log | ❌ | ✅ `raw/pty.raw.log` + `raw/input.raw.log` |
| Network log | ✅ `network/requests.ndjson` + `network/failed_requests.ndjson` | — N/A |
| Console log | ✅ `console/console.ndjson` + `console/errors.ndjson` | implicit in raw log |
| Per-outcome evidence file | ✅ `outcomes/<id>.md` (with budgeted summary) | ✅ `outcomes/<id>.md` |
| Per-outcome raw JSON sidecar | ✅ `outcomes/<id>.raw.json` (for script verifiers) | ❌ |
| Per-step diagnostics on failure | ✅ `diagnostics/<NN>_<stepId>.json` (visible controls, table headers, body excerpts) | ⚠️ only `diagnostics/failure.md` + `environment.md`; no per-step structured diag |
| Failure diagnostic | ✅ rendered in `run.md` + per-outcome | ✅ `diagnostics/failure.md` |
| Environment diagnostic | ✅ | ✅ `diagnostics/environment.md` |
| Trace recording | ✅ `traces/<backend>-trace.zip` (Playwright native, agent-browser best-effort) | ❌ |
| Trace capture policy `on-failure` deletes zip on pass | ✅ | N/A |
| Artifact redaction (write-side) | ✅ `createArtifactRedactor` — literal secrets + header / cookie / token regexes | ✅ `artifacts.Redactor` — compiled regex patterns |
| Redaction auto-derives from env | ✅ `SENSITIVE_KEY_RE` matches `*token*`, `*secret*`, `*password*`, `*api_key*`, etc. in env | ❌ — only the literal pattern list in config |
| Per-spec redaction override | ✅ `redaction: { values: [...] }` | ❌ |
| Auto-prune old runs | ✅ `retention.keepRuns` (per-config) | ❌ — no retention at all |
| Disk full (`ENOSPC`) hint | ✅ `addEnospcHint` on errors | ❌ |

### 2.6 CLI / agent surface

| Command / capability | Cairntrace | Glyphrun |
|---|---|---|
| Self-bootstrap for agents | `cairn explain --json` | `glyph explain --json` (lighter) + `glyph agent --format md` |
| Focused docs | `cairn docs <topic> --format json` (9 topics) | `glyph docs <topic>` (10 topics; no `--format` switch) |
| Run specs | `cairn run <spec...>` | `glyph run <spec...>` |
| Cold-start / clean state | `--cold-start` (default true in CI) | N/A |
| Parallel | `--parallel N` | ❌ |
| Backend choice | `--backend agent-browser\|playwright\|mock` | ❌ |
| Format | `--format json\|yaml\|md` | `--format json\|yaml\|md` ✅ |
| Live progress (TTY) | ✅ `src/cli/progress.ts` | ✅ `src/cli/progress.go` |
| JUnit / CI report | ✅ `--junit <file>` | ❌ — see §3.5 |
| Spec verify + stamp | `cairn spec verify --stamp` | `glyph spec verify --stamp` ✅ |
| Spec scaffold | `cairn spec scaffold` + `--kind action` | `glyph spec scaffold --kind spec\|action` |
| Snapshot update | `cairn spec heal --apply` + `--format json` (also drift auto-fix) | `glyph snapshot update <spec>` |
| Selector-drift heal (auto) | ✅ name-drift + wait-insertion; tracks step origin across imports | ❌ |
| Init (scaffold config + smoke) | ❌ (manual `cairntrace.config.yml`) | ✅ `glyph init --cmd … --ready …` |
| Diff runs | `cairn diff <A> <B>` (steps/outcomes/console/network) | `glyph diff <runA> <runB>` |
| Latest run context | `cairn context latest` | `glyph context latest` |
| Clean old runs | `cairn clean [--keep N\|--all]` | ❌ — see retention gap |
| Checkpoint capture (login once) | `cairn login <name>` (opens headed browser) | N/A |
| Checkpoint mgmt | `cairn checkpoint …` | N/A |
| Locator inventory (snapshot for spec authoring) | `cairn snapshot <url> --roles --testids` | ❌ — see §3.6 (PTY equivalent would be `glyph snapshot` of a running TUI) |
| Import from another framework | `cairn import playwright` | ❌ — see §3.4 |
| Export to another framework | `cairn export playwright` | ❌ — see §3.4 |
| Doctor | `cairn doctor` | `glyph doctor` |
| MCP server | `cairn mcp` | `glyph mcp` |
| MCP tool count | 11 | 9 (`explain`, `docs`, `doctor`, `spec_verify`, `run`, `context`, `snapshot_update`, `diff`, `spec_scaffold`) |
| JSON output schema for `run` | `urn:cairntrace.dev:run:v1` | inline only |
| JSON output schema for `explain` | `urn:cairntrace.dev:explain:v1` | inline only |

### 2.7 Distribution

| Concept | Cairntrace | Glyphrun |
|---|---|---|
| Language | TypeScript (Bun) | Go 1.26 |
| Package | git-clone + `bun install` + `./bin/cairn` symlink | `go install` / `go build` |
| Install in 1 cmd | ✅ (git clone + bun install) | ✅ (`go install ./cmd/glyph`) |
| Single static binary | ❌ (Bun runtime needed) | ✅ (single binary, no runtime) |
| Toolchain pin | `bun.lock` (transitive) | `.tool-versions` |
| CI | GitHub Actions | GitHub Actions |

---

## 3. Prioritized improvements for glyphrun

Ordered by **value per unit of work**. Each item is a standalone, shippable
PR-sized change. Where a cairntrace example or file is the reference, the path
is included so you can study it before designing.

### 3.1 Named artifacts: `download` + `transform` steps — **P0**

**Why.** This is the single biggest authoring gap. Today, when a TUI app
prints a path to a file, spawns a child that writes a file, or downloads
something, there's no way for a glyphrun spec to (a) capture that file path
into a named artifact and (b) reference it from a later step or outcome.

The pattern shows up *everywhere* in the cairn graphite flows — `template`
artifact is downloaded, `invalidTemplate` is built via a Node `transform`,
and they're referenced by `${artifacts.template.path}` from a Node verifier
in `outcomes`.

**What to add.**

1. New step kind `capture: { kind: file, saveAs, assign }` — does not
   navigate or interact; simply runs a configured `command` (or reads stdin)
   and saves the output to the run dir under `artifacts/<assign>/<saveAs>`.
   `assign` makes the path available as `${artifacts.<assign>.path}` and
   `.relativePath` everywhere a string can appear.
2. New step kind `transform: { runtime: node|shell, file, input, saveAs, assign }`
   — runs an external script (the same Node-shaped contract cairn uses:
   default-export async fn receiving `{ input, output, fixtures, runDir }`),
   passes `input` resolved via `${artifacts.<X>.path}`, and registers the
   output as a new named artifact.
3. Extend `glyph run` to materialize `${artifacts.<X>.path}` placeholders
   in step `command:` strings, `wait` needles (if you want to compare a
   generated checksum to a screen line), and outcome `command.run`.
4. Add `artifacts` block to `RunResult.Artifacts` (currently
   `map[string]string`) for download / transform entries.
5. Schema additions go in `internal/spec/model.go` +
   `schemas/glyphrun.spec.v1.schema.json` together, per AGENTS.md.

**Reference.** `cairntrace/src/core/runner/Runner.ts:347-396` for the
download/transform capture pattern, `cairntrace/src/core/runner/nodeScripts.ts`
for the Node default-export contract, `cairntrace/src/core/parser/parseSpec.ts`
for the `resolveArtifactPlaceholders` helper.

**Effort.** M (3–5 days). Mostly new code; the plumbing is already in
`artifacts.Writer`. Add an example spec `examples/specs/transform_artifact.yml`
that downloads a build artifact, mutates it, and re-verifies it.

### 3.2 `batch` step — **P0**

**Why.** Glyphrun's biggest authoring gotcha is transient PTY state. Examples
that motivated this idea in cairn:

- "Type a slash, wait for a command palette to appear, then press Enter" —
  the palette closes between `type` and the next `press` if you split them.
- "Press F1, then quickly press Down twice, then Enter" — a help menu
  renderer that draws once and then redraws, lost between steps.
- "Send `^U` (clear line), then `yes`" — key buffer may not flush between
  the two writes.

Cairn solves this with a single `batch` step that runs sub-steps in one
backend invocation. For PTYs the equivalent is **one PTY write batch** — the
runner queues multiple input operations and flushes them in a single
`pty.write()` syscall so the TUI sees them as a single redraw cycle.

**What to add.**

1. New step kind `batch:` with sub-steps `{ press }`, `{ type }`, `{ paste }`,
   `{ send }`, `{ wait: { screen, idle } }` (no `use` or `snapshot` inside —
   they require separate sync points).
2. Runner: collect sub-step bytes, write them all at once, then run the
   trailing `wait` if present. Sub-step `wait` is the only one allowed to
   span the batch.
3. Same `when:` and `id:` semantics as top-level steps.

**Reference.** `cairntrace/src/core/schema/spec.v1.ts:331-399` for the
batch shape, `cairntrace/src/adapters/agent-browser/AgentBrowserAdapter.ts`
for the `--bail` semantics, and `cairntrace/examples/flows/11-batch-hover-click.yml`
for the canonical example.

**Effort.** S (1–2 days). Pure new code in `runner.go` + schema additions.

### 3.3 `file:` and `script:` verifiers — **P0**

**Why.** Today, asserting "the daemon wrote a file matching this glob" or
"the row count from the spec's view matches what the database says" requires
hand-rolled `command:` verifiers. cairn's typed verifiers are easier to
author, easier to read, and easier to render in `agent_context.md`.

**What to add.**

1. New outcome verifier `file: { glob, contains, timeoutMs }` — polls the
   filesystem for a file matching `glob` (resolved against the spec dir),
   optionally requires its text to contain `contains`, times out per
   `timeoutMs`. Maps cleanly to `cairntrace/src/core/runner/verifiers/file.ts`.
2. New outcome verifier `script: { runtime: node, file|run, fixtures }` —
   the typed `script` shape from cairn, minus `runtime: browser` (PTY has
   no DOM). Default-export `async function verify(ctx) { return { ok, evidence } }`.
   Pass `fixtures:` as the `ctx.fixtures` bag, with `${artifacts.<X>.path}`
   resolution so files referenced from Node verifiers just work.
3. Add a `raw.json` sidecar per outcome (`outcomes/<id>.raw.json`) when the
   verifier returns an evidence object too large for the markdown budget —
   same shape as cairn §13b.

**Reference.** `cairntrace/src/core/runner/verifiers/{file,script}.ts`,
`cairntrace/src/core/runner/OutcomeEvaluator.ts:53-78` (the `ctx.failedStep`
short-circuit that turns "missing artifact" into `skipped` instead of
`failed`).

**Effort.** M (2–3 days). The `script` verifier needs the Node runtime
subprocess plumbing — cairn has it via Bun's native TS loader; for glyphrun
either shell out to `node --experimental-strip-types <file>` (works in
Node 22+) or ship a tiny embedded V8 interpreter. Node subprocess is
simpler and matches the cairn "import project deps" use case.

### 3.4 Spec metadata + Playwright-style import/export — **P1**

**Why.** Two real cairn features that earn their place:

- **`metadata:` block.** Today the graphite flows encode
  `feature / priority / tags` only in a YAML comment, which means you can't
  filter, group, or report on specs by feature. Adding a typed `metadata:`
  block lets `glyph run --feature table-import` or `glyph list --tag OPG-14010`
  work. The cairn spec schema has this at lines 499-507.
- **`glyph import <tool>` / `glyph export <tool>`.** Cairn ships
  Playwright import (`cairntrace/src/core/importers/playwrightImporter.ts`)
  and export (`cairntrace/src/core/exporters/playwrightExporter.ts`).
  For glyphrun, the natural targets are:
  - **Import:** `bats`, `shell spec`, `scripttest` — all are shell-script
    TUI tests today. Importing them preserves the value of existing test
    suites while shifting the contract model.
  - **Export:** `bats` — round-trip a glyphrun spec into a Bash script that
    drives the same PTY via `expect(1)` for CI environments where
    installing glyphrun is hard.

**What to add.**

1. `metadata: { feature, owner, priority, tags: [] }` on `spec.Spec` (model
   + schema). Add a `glyph list` command that prints
   `name | intent | feature | priority | tags | lastStatus`.
2. `glyph import <file.bats>` and `glyph export <file.bats>`. The import
   is line-by-line mapping (BATS `@test "..." → spec name`, `run … → command
   verifier`, `[[ "$output" =~ "x" ]] → screen.regex`). The export
   reverses the mapping.

**Reference.** `cairntrace/src/core/importers/playwrightImporter.ts` and
`cairntrace/src/core/exporters/playwrightExporter.ts` for the structural
pattern. The bash ↔ glyphrun mapping is the novel part.

**Effort.** M (3–4 days) for the metadata block + `list`. M (1 week) for
the bats importer/exporter; this is the single most leverage-creating
feature for adoption.

### 3.5 JUnit XML output for CI — **P1**

**Why.** Most CI dashboards (GitHub Actions, GitLab, Jenkins, Buildkite) only
know how to surface test results via JUnit XML. cairn has
`--junit reports/cairn.xml`; glyphrun has nothing equivalent. Today you'd
have to write a script that parses `events.ndjson` and reshapes it.

**What to add.**

1. New flag on `glyph run --junit <file>` that writes a JUnit XML file
   alongside the run dir. One `<testcase>` per outcome (or per spec when
   running multiple), with `<failure>` blocks for failed outcomes and
   `<skipped>` for the new "outcome blocked by failed step" case.
2. `glyph run <dir>` should expand directories recursively (skipping
   `actions/`, `_*.yml` drafts) the same way cairn does — a natural
   complement to `--junit`.

**Reference.** `cairntrace/src/cli/commands/run.ts:9` (the `--junit` flag
binding) and the JUnit writer (look for the `junit` module import in
`run.ts`).

**Effort.** S (0.5–1 day). One new file, one new flag, one test.

### 3.6 Locator inventory / `glyph snapshot inventory` — **P1**

**Why.** Cairn's `cairn snapshot <url> --roles --testids` is the
agents' authoring accelerator: it returns a compact list of available
locators on the current page so an agent can write the next step without
guessing at selectors. Glyphrun's equivalent would dump the current
emulator screen as a structured list of (row, col, char, style) "tui
elements" or a region map, and call it `glyph snapshot inventory
<spec-or-name>`.

**What to add.**

1. `glyph snapshot inventory` — runs a spec up to the current point (or
   just opens the target binary and waits for a ready text) and prints a
   compact table of: row ranges, text present, hotkey-ish patterns
   (`^X`, `<F1>`), detected prompt markers (`>`, `$`, `:`, `?`). Each
   row gets a `name` so an agent can write `wait: { screen: { region: … } }`
   or `press: "f1"` directly.
2. Wire it into the agent workflow: `glyph agent --format md` should call
   out the inventory command alongside the existing command list.

**Reference.** `cairntrace/src/core/snapshot/locatorInventory.ts` and
`cairntrace/src/cli/commands/snapshot.ts`.

**Effort.** S–M (1–2 days). The structure exists in the terminal emulator
(`internal/terminal/emulator.go:Screen()`) — the work is picking which
"elements" to surface.

### 3.7 Auto-prune + retention — **P2**

**Why.** Glyphrun artifact dirs accumulate forever. A long-lived project
that runs specs multiple times a day can easily hit "where did my disk go"
in a few weeks. Cairn has both a per-config `retention.keepRuns` and a
`cairn clean [--keep N | --all]` CLI. Glyphrun has neither.

**What to add.**

1. `Config.Artifacts.KeepRuns int` — auto-prune after each successful run,
   keeping the N newest. Best-effort (per cairn's "a prune failure must
   never fail the run" comment).
2. `glyph clean [--keep N | --all]` CLI.
3. Honor `maxRawLogBytes` in the prune — already set in defaults, but no
   periodic enforcement.

**Reference.** `cairntrace/src/core/artifacts/retention.ts` and
`cairntrace/src/cli/commands/clean.ts`.

**Effort.** XS (0.5 day). Straight port.

### 3.8 Per-spec redaction block — **P2**

**Why.** Cairn lets a spec declare `redaction: { values: ["${secrets.X}"] }`
so that a leaked value in PTY output gets scrubbed before it lands in
artifacts. Glyphrun's redaction is config-only, and the graphite-style
flows are exactly the use case that motivated cairn's per-spec override
(see the `redaction: { values: ["${secrets.GRAPHITE_E2E_EMAIL}"] }` block in
`automations/graphite/flows/table_template_download_and_upload_validation.yml:22-25`).

**What to add.**

1. `Redaction` block on `spec.Spec` — `{ values: [] }` only for now.
   `Redactor` already accepts patterns; just extend it to fold per-spec
   literal values into the secret list before writing.
2. Also extend the redactor to **auto-derive secrets from env** the way
   cairn does — every env var whose name matches `*token*|*secret*|*password*|…`
   should be redacted. This is one new helper in `artifacts/redaction.go`.

**Reference.** `cairntrace/src/core/artifacts/redaction.ts:55-67`
(`collectLiteralSecrets`).

**Effort.** S (0.5 day).

### 3.9 Contract-hash enforcement on `run` — **P2**

**Why.** Glyphrun currently *parses* the contract hash but does not *enforce*
it. A spec that has drifted from its stamped contract should fail loudly,
not silently. Cairn exits 6 on mismatch and the `contractHash` field is
stamped at scaffold time (see `cairn AGENTS.md:98-99`).

**What to add.**

1. If the spec has a `contractHash:` and the recomputed hash over the
   current `intent + outcomes` differs, exit code `6` (matches cairn).
   Use the same exit-code namespace pattern already established in
   `runner.go:34-40`.
2. Also expose this in `spec verify` — already there as a warning, but
   make it the same exit code.

**Reference.** `cairntrace/src/core/contractHash.ts` and the runner
integration in `cairntrace/src/core/runner/Runner.ts`.

**Effort.** XS (0.5 day). One new exit code constant, one comparison in
`runner.go`, one test.

### 3.10 `parallel` runs — **P2**

**Why.** Two `glyph run` invocations on independent specs can safely run
in parallel because each is its own PTY process. The `automations/graphite/`
flows run one at a time today; cutting that wall-time by running
independent specs concurrently is a real CI win.

**What to add.**

1. `glyph run <spec...> --parallel N` — spawn N runners in goroutines,
   each with its own artifact dir under a per-runner UUID.
2. Aggregate results into a single JUnit (or new `glyph run` summary
   output) at the end. Per-runner failures stay isolated.
3. Cap the parallel count from `runtime.NumCPU()` by default.

**Reference.** `cairntrace/src/core/runner/pool.ts` and
`cairntrace/src/cli/commands/run.ts:9` (the `--parallel` flag).

**Effort.** M (1 week). The trickiest part is progress reporting — each
runner already emits via `ProgressListener`; the orchestrator needs to
serialize that to stderr without interleaving.

### 3.11 `cairn import/export` ↔ `glyph` parity (cross-tool interop) — **P3**

**Why.** Not in scope for "improve glyphrun" per se, but worth noting: if
glyphrun and cairntrace ever want a shared `intentspec` (intent + outcomes
contract), the verifier vocabularies are now 80% aligned. A future v2 could
define a top-level `IntentSpec.v2` schema that both runners consume, with
backend-specific `steps` extensions (PTY: `press/type/wait`; browser:
`click/fill/navigate`).

This is a long-arc idea — don't do it now, but consider preserving the
*shape* of the cross-tool parts (intent, outcomes, contract hash,
imports, preconditions) so it's not painful later.

**Effort.** Investigation only at this stage.

### 3.12 Heuristic `glyph spec heal` for terminal drift — **P3**

**Why.** Glyphrun's `snapshot update` is the snapshot-drift equivalent of
cairn's `cairn spec heal`. But cairn also handles **behavior drift** at the
step level (wait insertion, name swap) and writes the patch in place.
Glyphrun currently leaves that to the human.

A TUI version is more constrained (you can't auto-heal a renamed menu
item), but you *can*:
- Detect a `wait` timeout and propose inserting a longer timeout
- Detect a screen-regex outcome that no longer matches and propose
  widening the regex (with a `--dry-run` flag)
- Detect a `preconditions.commands` failure and propose a retry policy

This is mostly a "would be nice" — it has to be careful not to silently
weaken tests. Ship behind a `--suggest` flag first, never `--apply`.

**Reference.** `cairntrace/src/core/healer/Healer.ts` and
`cairntrace/src/core/healer/snapshotParser.ts`.

**Effort.** L (1–2 weeks). Do this last, only if the snapshot-update
ergonomics need it.

---

## 4. Specific cross-tool borrow notes

These are smaller, surgical ideas. Each is a half-day of work, mostly
typing.

### 4.1 Typed matcher shape for screen conditions

Cairn's `TextMatcher` shape:
```yaml
text: { equals | contains | matches }   # exactly one
```
Glyphrun's `ScreenCondition`:
```yaml
screen: { contains | notContains | regex }
```

Cairn's is more expressive (the same `TextMatcher` shape appears on
`notText`, on `text`, and is the same shape you can imagine for
`region.text`, etc.). If glyphrun adopted the discriminated matcher
shape, you'd get:

```yaml
verify:
  screen:
    equals: "ready"
  # or
  screen:
    matches: "^Ready \\d+ items$"
```

Plus a `notScreen` for symmetry. The schema addition is in
`schemas/glyphrun.spec.v1.schema.json` and the Go struct is in
`internal/spec/model.go` (and `verify.go` for the runner dispatch).

**Effort.** XS (0.5 day).

### 4.2 `when:` mini-DSL string shorthand

Cairn's `when: "urlContains:/dashboard"` is more readable than glyphrun's
full-Verify `when: { screen: { contains: "X" } }`. Add a string shorthand:

```yaml
- when: 'screen.contains:"optional prompt"'
  press: enter
```

The parser would compile the string into the existing `Verify` struct
behind the scenes. New `when` parser lives in `internal/spec/parser.go`.

**Reference.** `cairntrace/src/core/runner/conditions.ts`.

**Effort.** S (0.5 day).

### 4.3 Per-step `id:` for better failure messages

Cairn's `id:` per step makes failure messages vastly more useful
(`step "open_table" failed: locator "Open dashboard" not found` vs
`step 2 failed`). Add `id: z.string().min(1).optional()` to glyphrun's
`Step` struct in `internal/spec/model.go`, then thread it through
`runner.go` so the failure event includes it.

**Effort.** XS (0.5 day). Pure additive.

### 4.4 Capture policy in spec instead of config

Cairn's `artifacts.capture.{trace,console,screenshots,…}` per spec is
much nicer than glyphrun's config-level on/off booleans. Adopt the same
shape. Default to `on-failure` for `trace` (matches cairn) so green runs
stay small.

**Reference.** `cairntrace/src/core/schema/spec.v1.ts:471-487`.

**Effort.** S (1 day). Wire it into the `runner.finish()` conditional
file writers.

### 4.5 `mode: normal | debug`

A spec-level flag that, when `debug`, increases the verbosity of the
artifact pack (full frames, all terminal screen snapshots, all regex
match attempts) and emits more progress to stderr. Cheap to add and
useful for the "this one spec is flaky and I need to see everything"
loop.

**Reference.** `cairntrace/src/core/schema/spec.v1.ts:525` (the enum
definition) — cairn doesn't actually use it yet but reserves it.

**Effort.** S (1 day).

### 4.6 Output schema URIs

Cairn ships `$schema: urn:cairntrace.dev:run:v1` in `run.json` and
`urn:cairntrace.dev:explain:v1` in `explain.json`. The Glyphrun run
schema is `glyphrun.run.v1.schema.json` on disk but the *output* doesn't
reference it. Add a `"$schema": "urn:glyphrun.dev:run:v1"` field to
`run.json` and `run.yaml` outputs so downstream tools can negotiate the
shape. Also add it to `agent_context.md` (front matter) for the same
reason.

**Effort.** XS (0.5 day). One field on `artifacts.RunResult`.

### 4.7 Better MUX/CI ergonomics: `--seed`, `--rerun-failed`, `--list`

Three small flags that fall out of `metadata:` (3.4) and a small index
in the artifact root:

- `glyph run --seed N` — deterministic random seed for any randomized
  step (useful for property-style tests).
- `glyph run --rerun-failed` — read `runs/.last-failed.txt` (new file,
  written at the end of each run) and re-run only those.
- `glyph list [--feature X] [--tag Y]` — print the metadata table.

**Effort.** S–M (1–2 days combined).

---

## 5. What glyphrun has that cairn doesn't (highlight, don't lose)

These are terminal-specific gifts. They should be in the README's "Why
Glyphrun" section so the boundary between the two tools is clear to
someone considering both.

1. **Deterministic virtual emulator.** Glyphrun's `gote`-backed
   `terminal.Emulator` lets you write a spec, run it, and assert
   *cell-level* outcomes (`cell: { x, y, char: "X" }`,
   `cursor: { x, y, visible }`) without depending on what the TUI
   actually did to the terminal. Cairn's analogous level is
   accessibility-tree text only.

2. **Committed `snapshot` testing.** Glyphrun can commit a snapshot of
   a specific screen state and assert on it across runs, with
   `ignoreRegions` for "this part of the screen is volatile." The
   closest cairn equivalent is the "expect no console errors" outcome,
   which is much coarser.

3. **`record` + `replay`.** You can capture a PTY session
   (`glyph record -- cmd`) and replay it later as a determinism test.
   Cairn has no equivalent — Playwright traces are the closest, and
   they're heavyweight (zips) and not first-class in the spec.

4. **Single static binary.** `go build` → no-runtime binary. Cairn
   needs Bun installed.

5. **In-process emulator snapshots + `snapshot:` step.** Glyphrun's
   `snapshot: <name>` step captures a full screen at that point and
   is queryable by name from any later outcome. Cairn's snapshots are
   per-step diagnostics, not first-class outcome sources.

6. **`alternateScreen: auto | require | forbid`** outcome — a domain
   check that doesn't exist in browser land at all.

---

## 6. Sequencing recommendation

If you want a roadmap quarter:

- **Sprint 1 (week 1–2):** 3.1 (`download`/`transform` steps) +
  3.2 (`batch` step) + 3.5 (JUnit). The three together turn glyphrun
  from "PTY smoke tester" into "PTY CI test suite."
- **Sprint 2 (week 3–4):** 3.3 (`file:` / `script:` verifiers) +
  3.4 (metadata + bats import/export). Metadata unlocks the next ten
  improvements; verifiers close the test-expressiveness gap.
- **Sprint 3 (week 5–6):** 3.6 (locator inventory) + 3.7 (retention) +
  3.8 (per-spec redaction) + 3.9 (contract hash enforcement). Polish.
- **Backlog (sprint 4+):** 3.10 (parallel) + 3.12 (heal). Both are
  high-leverage but high-effort.

The two documents you should also update when any of this lands:

- `AGENTS.md` — extend the "Required Agent Behavior" list with
  `metadata:`, `${artifacts.<X>.path}`, and the new commands.
- `README.md` — add a "Why Glyphrun" subsection (§5 above) so the
  browser-vs-terminal positioning is explicit, and update the CLI
  commands table.

---

## 7. Open questions for you

1. **Do you want glyphrun and cairntrace to converge on a shared spec
   model eventually?** If yes, I'd avoid schema changes that preclude
   it (e.g. don't change the `outcomes.verify` shape to be PTY-only).
2. **What TUI app is the day-1 target?** Knowing whether it's a TUI
   that prints paths (download), spawns children (transform input),
   or uses the mouse (none of this helps) shapes which of 3.1/3.2/3.3
   is most urgent.
3. **Are the `automations/graphite/flows/` specs the gold standard for
   the kind of things you want glyphrun to express?** If yes, the
   `download`/`transform` artifact pipeline is the first thing to
   build — every graphite flow uses it.
