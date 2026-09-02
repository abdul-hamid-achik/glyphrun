---
description: Author isolated TUI states as Glyphrun stories — the stories.yml manifest, generated specs, goldens and visual diff, variants, and the catalog/HTML/TUI/serve surfaces.
---

# Stories

Glyphrun stories are Storybook for the terminal: one harness binary, many isolated states, each mounted and captured black-box (no Bubble Tea, Ratatui, or other TUI kit import in Glyphrun itself). A `stories.yml` manifest declares the harness once and lists the states it can mount; `glyph stories run` expands every story into an ordinary spec in memory, so the runner, verifiers, goldens, and artifacts are identical to a hand-written spec — the manifest only removes the boilerplate.

Spec files tagged `story` are still supported directly (see [Spec-file stories](#spec-file-stories) below); a manifest is the recommended path for anything with more than a couple of states.

## Quickstart

```bash
glyph stories init --lang go        # or --lang sh for a toolchain-free harness
glyph stories run                   # build the harness once, run every story, capture missing goldens
glyph stories serve --watch         # live catalog at http://127.0.0.1:4649
```

`glyph stories init` writes a harness (a Bubble Tea v2 app for `--lang go`, a POSIX shell script for `--lang sh`) plus a `stories.yml` that lists a couple of starter states. Edit the harness to read a story id from `os.Args[1]` and render the matching state, then add entries to `stories.yml` for every state you want cataloged.

## The manifest

```yaml
version: 1
kind: stories
name: examples

harness:
  cmd: ["./examples/apps/stories/stories"]   # story id (or args) is appended to this
  cwd: "."
  build: "go build -o ./examples/apps/stories/stories ./examples/stories"
  buildTimeoutMs: 60000
  watch: ["examples/stories"]                # extra paths `--watch` polls besides the manifest

defaults:
  terminal: { cols: 80, rows: 24, profile: xterm-256color, alternateScreen: require }
  readyTimeoutMs: 5000
  quit: "q"
  golden: true

stories:
  - id: list/rows
    intent: the list story renders three inbox rows with the first item selected.
    tags: [rows]
    ready: { contains: "hello" }
    outcomes:
      - id: selected_marker
        description: the first row is marked selected
        verify:
          region: { x: 0, y: 2, width: 20, height: 1, contains: "> hello" }
    variants:
      - name: wide
        terminal: { cols: 120, rows: 40 }
```

### Field reference

| Field | Type | Description |
| --- | --- | --- |
| `version` | int | Must be `1`. |
| `kind` | string | Must be `stories`. |
| `name` | string | Manifest name; used as the fallback `feature` for a story whose `id` has no `/`. Defaults to the manifest's parent directory name. |
| `harness.cmd` | []string | Argv prefix. The story's `id` (or its `args`) is appended to build the full command. |
| `harness.cwd` | string | Working directory for both the build command and the harness process, relative to the project root (where `glyphrun.config.yml` lives), like a spec's `target.cwd`. |
| `harness.env` | map | Base environment, merged under a story's and variant's `env`. |
| `harness.build` | string | Shell command run once per `glyph stories run` invocation (not once per story), before any story executes. |
| `harness.buildTimeoutMs` | int | Build timeout, default 60000. |
| `harness.watch` | []string | Extra paths `--watch`/`serve --watch` polls (typically the harness source directory) in addition to the manifest and any story spec directories. Relative entries resolve against the project root, like `cwd`. The artifact, golden, and stories roots are never watched, so a run's own output cannot re-trigger it. |
| `defaults.terminal` | Terminal | `cols`/`rows`/`profile`/`color`/`alternateScreen` applied to every story unless overridden. |
| `defaults.readyTimeoutMs` | int | Default 5000. |
| `defaults.quit` | string | Key pressed to end the story before the exit-code wait. Default `"q"`; set to `""` to skip the quit/exit steps entirely. |
| `defaults.exitTimeoutMs` | int | Default 3000. |
| `defaults.golden` | bool | Default `true` — whether stories get a `golden` outcome. |
| `defaults.goldenMode` | `text` \| `cell` \| `json` | Snapshot verifier mode for the `golden` outcome. `text` (default) enforces characters only; `cell` also enforces styles, so a color regression fails `glyph stories run`; `json` adds the cursor. |
| `defaults.tags` / `defaults.owner` | []string / string | Applied to every story's generated spec metadata. |
| `stories[].id` | string | Required. Passed as the harness argument. `feature/name` shape: the segment before the first `/` becomes the feature (unless `feature` is set), the segment after the last `/` becomes the snapshot name. |
| `stories[].feature` | string | Overrides the id-derived feature. |
| `stories[].intent` | string | Human-readable intent for the generated spec. Defaults to a generic sentence naming the id. |
| `stories[].tags` | []string | Added to `defaults.tags` (deduped). |
| `stories[].args` | []string | Replaces the default single-argument `[id]` passed to the harness. |
| `stories[].env` | map | Merged over `harness.env`. |
| `stories[].terminal` | Terminal | Merged over `defaults.terminal` (only the fields set are overridden). |
| `stories[].ready` | screen condition | Marks the state as mounted (`equals`/`contains`/`notContains`/`matches`/`regex`). If omitted, Glyphrun waits for 300ms of terminal idle instead. |
| `stories[].readyTimeoutMs` | int | Overrides `defaults.readyTimeoutMs`. |
| `stories[].quit` | string | Overrides `defaults.quit`. |
| `stories[].golden` | bool | Overrides `defaults.golden`. |
| `stories[].goldenMode` | string | Overrides `defaults.goldenMode` for one story. |
| `stories[].steps` | []Step | Extra steps run after ready and before the snapshot (e.g. `press`, `wait`). |
| `stories[].outcomes` | []Outcome | Appended after the generated `golden`/`ready` outcomes. |
| `stories[].variants[].name` | string | Required. Re-runs the story with overrides layered on top — the terminal-story equivalent of Storybook args. |
| `stories[].variants[].terminal` / `.env` / `.args` | | Layered over the story's own values for this variant only. |

Validate a manifest against the schema directly with the JSON Schema at `schemas/glyphrun.stories.v1.schema.json`, or just run `glyph stories run` — a bad manifest fails fast with a schema error.

Generated spec names must be unique across every discovered manifest and spec-file story because run ids, goldens, and the retention-proof index use that name as their storage identity. Glyphrun rejects cross-file collisions before running or rendering the catalog.

## What gets generated per story

Each story (times its variants) expands to one spec:

- **Spec name**: `story_<id with / replaced by _>`, plus `__<variant>` for a variant — `list/rows` → `story_list_rows`, its `wide` variant → `story_list_rows__wide`.
- **Snapshot name**: the last `/`-segment of the id — `list/rows` → `rows`.
- **Steps**: wait for `ready` (or 300ms of idle if `ready` is unset) → the story's own `steps` → `snapshot` named after the snapshot name → (if `quit` is non-empty) press `quit`, then wait for process exit code `0`.
- **Outcomes**: a `golden` snapshot-match outcome when `golden` is true → a `ready` screen-match outcome when `ready` is set → the story's own `outcomes` → if none of the above produced any outcome, a fallback `mounted` process outcome (asserts the process exited only if `quit` is set).
- **Tags**: always include `story`, plus `defaults.tags`, the story's own `tags`, and `variant:<name>` for a variant.

## Goldens and visual diff

A story with `golden: true` (the default) gets a `golden` outcome comparing its snapshot against a committed golden JSON/text pair under `snapshotRoot` (default `.glyphrun/snapshots`, same layout as `glyph snapshot update`).

- **First run**: a missing golden is captured automatically (`glyph stories run` reports it as `created`), not treated as a failure.
- **`--strict`**: fails a story instead of capturing a missing golden — use this in CI so a forgotten golden shows up as a real failure.
- **`--update`**: rewrites every golden with the current capture, regardless of whether it matched.
- **From the browser**: `glyph stories serve` renders an "accept golden" action per story that does the equivalent of `--update` for exactly that row (a base story's variants keep their own goldens until you accept them too), then reruns and refreshes the live catalog.
- **Reporting is file-based**: `created` / `updated` in the run report mean the golden file changed on disk, even when the run then failed on another outcome, so an unreviewed golden never lands silently.
- **Text vs cell contracts**: the default `text` mode makes goldens portable across terminals (a machine that renders fewer colors still passes), but it means a color-only regression does not fail the run. The catalog and the HTML page still diff every cell: a style-only difference shows as `~N` (amber, informational) next to a `text`-mode golden and as `±N` (a real change) under `goldenMode: cell`. Pick `cell` when the harness controls its own colors (a fixed `TERM`/`COLORTERM` in CI) and you want strict runs to catch them.

`.glyphrun/snapshots` (the committed goldens) should be committed to version control. `.glyphrun/stories` (the derived index — the newest result and a copy of its screens, keyed by spec name, used so the catalog survives `retention.keepRuns` pruning) is derived output and is gitignored.

## Variants

A `variants` entry re-runs the same story id with a different terminal size, env, or args layered on top of the story's own values. Each variant gets its own spec (`story_<id>__<variant>`), its own snapshot/golden pair, and a `variant:<name>` tag, and shows up as its own row in the catalog keyed `<id>@<variant>` (e.g. `list/rows@wide`).

## The three surfaces

- **Catalog (`glyph stories`)**: `md`/`json`/`yaml` output — a table (or structured payload) of every story joined to its newest result and golden status (`match`/`changed`/`missing`/`none`). Filter with `--feature`, `--tag`, `--owner`, or include everything (including specs not tagged `story`) with `--all`.
- **HTML inspect (`glyph stories --html`)**: a self-contained page that renders terminal screens as SVG client-side from the captured cell grid (no server-rendered SVG blobs). Modes: `plain`/`grid`/`spaces`/`diff` (keys `1`-`4`), `g` toggles golden vs. current while in diff mode, `/` focuses the story filter box. In live mode (served via `glyph stories serve`) it adds rerun-one, rerun-all, and accept-golden buttons with `r`/`a` shortcuts.
- **TUI (`glyph stories --tui`)**: a two-pane catalog in the host terminal. `j`/`k` move between stories, `[`/`]` between snapshots, `s` toggles space visualization, `d` toggles diff highlighting (on by default), `o` toggles golden vs. current, `r` toggles rulers, `q` quits. The sidebar marks each story `✓` (golden match), `±` (changed), or `?` (missing).

## Running, watching, and serving

`glyph stories run [path...]` discovers manifests (files named `stories.yml`, `stories.yaml`, `*.stories.yml`/`.yaml`, or any explicitly passed file with `kind: stories`) plus spec files tagged `story`, builds each manifest's harness exactly once (`harness.build`, not once per story), then runs every story in parallel (`--parallel`, default 4).

- `--only <selector>` (repeatable) narrows to a story key (`list/rows`, `list/rows@wide`), spec name, or feature — a trailing `/` selects everything under a feature prefix.
- `--update` rewrites every golden; `--strict` fails instead of capturing a missing one.
- `--watch` re-runs on changes to the manifests, `harness.watch` paths, and story spec directories (interactive; `--format md` only); `--watch-path` adds extra paths and implies `--watch`.

`glyph stories serve [path...] [--watch] [--addr 127.0.0.1:4649] [--no-run] [--parallel N]` starts a loopback-only HTTP server hosting the live inspect page: `GET /` the page, `GET /catalog.json` the current payload, `GET /events` a server-sent-events stream that pushes a fresh catalog after every run, `POST /run {"key": "..."}` reruns one story (or all, with an empty key), and `POST /update {"key": "..."}` accepts a golden. Stories run once on start unless `--no-run` is set; `--watch` re-runs on the same source changes as `stories run --watch`.

## Spec-file stories

A hand-written spec still counts as a story if its `metadata.tags` includes `story` — no manifest required. `glyph stories`, `glyph stories run`, and `glyph stories serve` all pick these up alongside manifest-derived stories. `glyph spec scaffold --kind story` prints a starter **manifest** (not a bare spec) to get you started with the manifest shape directly.

## The sh harness

`glyph stories init --lang sh` writes a POSIX shell harness (`story.sh`, driven entirely by ANSI escape codes) and its own `stories.yml` — no Go toolchain, no build step, useful when you want to validate the stories workflow itself or story a target that already speaks raw ANSI. See [`examples/stories-sh/`](https://github.com/abdul-hamid-achik/glyphrun/tree/main/examples/stories-sh).

## CI usage

`glyph stories run --strict --format json` in CI turns a changed or missing golden into a real, non-zero exit (see the exit codes table in [README.md](https://github.com/abdul-hamid-achik/glyphrun#exit-codes)) instead of silently recapturing it — pair it with committing `.glyphrun/snapshots` and reviewing golden diffs in the same PR review as the code change.

## Example

The repo ships a Bubble Tea harness under [`examples/stories/`](https://github.com/abdul-hamid-achik/glyphrun/tree/main/examples/stories) driven by [`examples/stories.yml`](https://github.com/abdul-hamid-achik/glyphrun/blob/main/examples/stories.yml) (8 stories: `list/empty`, `list/rows` with a `wide` variant, `list/error`, and five `agent/*` chat states) and a POSIX-shell harness under [`examples/stories-sh/`](https://github.com/abdul-hamid-achik/glyphrun/tree/main/examples/stories-sh). Raise `retention.keepRuns` (this repo uses 32) so a batch of stories is not pruned down to the last few before you can inspect them — the stories index survives pruning regardless, but individual run directories do not.

```bash
task example:stories
task example:stories:sh
./bin/glyph stories examples --html --out /tmp/glyph-stories.html
./bin/glyph stories examples --tui
task example:stories:serve
```

See also [CLI Reference](/commands) for full flag tables and [Configuration](/configuration) for `storiesRoot`/`snapshotRoot`.
