package docs

import "sort"

var byTopic = map[string]string{
	"overview": `# Glyphrun Overview

Glyphrun runs YAML or JSON terminal behavior specs against a target command in a real PTY. Assertions read a virtual terminal screen, not raw ANSI bytes.

Start with ` + "`glyph agent --format md`" + ` for the agent workflow, or ` + "`glyph docs topics --format md`" + ` to list focused docs.
`,
	"quickstart": `# Quickstart

1. Run ` + "`glyph doctor --format md`" + `.
2. Initialize a project with ` + "`glyph init --cmd ./bin/app --ready ready --format md`" + `, print a spec with ` + "`glyph spec scaffold > specs/smoke.yml`" + `, or bootstrap one from a real session with ` + "`glyph record --scaffold specs/smoke.yml -- ./bin/app`" + `.
3. Validate it with ` + "`glyph spec verify specs/glyphrun/smoke.yml --format json`" + `.
4. Run it with ` + "`glyph run specs/glyphrun/smoke.yml --format md --progress auto`" + `.
5. Inspect failures with ` + "`glyph context latest --format md`" + `.
`,
	"authoring": `# Authoring

Separate behavior contracts from repairable steps. Keep user intent in ` + "`intent`" + `, stable expectations in ` + "`outcomes`" + `, and navigation/input hints in ` + "`steps`" + `.

Run ` + "`glyph spec verify <spec> --format json`" + ` before running a spec. Use ` + "`glyph spec verify <spec> --stamp`" + ` only when the expected behavior intentionally changed.

Good specs assert user-visible behavior. Avoid coupling outcomes to implementation details, timing artifacts, or raw ANSI bytes.
`,
	"snippets": `# Reusable Actions

Create reusable terminal step snippets with ` + "`glyph spec scaffold --kind action`" + `. Import them from specs with ` + "`imports`" + ` and call them with ` + "`use`" + `.

Use ` + "`when`" + ` on a step to run it only when a verifier is currently true. This is useful for optional TUI prompts, warnings, login walls, and other state that may or may not appear. ` + "`when`" + ` accepts a full verifier object or a shorthand string such as ` + "`when: 'screen.contains:\"Login\"'`" + `. Optional ` + "`id:`" + ` labels make failures and ` + "`StepResult`" + ` readable. Set ` + "`mode: debug`" + ` on a spec to force verbose capture (frames, raw log, snapshots, agent context).

Use trusted ` + "`command`" + ` verifiers for Bash checks such as ` + "`test -x ./bin/app`" + `.
`,
	"steps": `# Steps

Supported v1 steps: ` + "`press`" + `, ` + "`type`" + `, ` + "`paste`" + `, ` + "`send`" + `, ` + "`mouse`" + `, ` + "`wait`" + `, ` + "`resize`" + `, ` + "`snapshot`" + `, imported ` + "`use`" + ` actions, ` + "`when`" + ` guards, the artifact-pipeline steps ` + "`download`" + `, ` + "`transform`" + `, ` + "`batch`" + ` (see ` + "`artifacts-pipeline`" + `), and the process-telemetry ` + "`monitor`" + ` step (see ` + "`process-telemetry`" + `).

` + "`mouse: { x, y, button?, action? }`" + ` sends a mouse event at the 0-based cell (button: left/middle/right/wheelUp/wheelDown; action: click/press/release/move). The runner encodes it as SGR (1006) or legacy X10 depending on the mode the target enabled.

Every step can include a ` + "`when`" + ` guard (full verifier or shorthand string). Prefer ` + "`wait`" + ` steps that synchronize on visible screen or process state. Use ` + "`snapshot`" + ` to capture named terminal states in the artifact pack.

` + "`paste`" + ` sends bracketed paste delimiters only after the target enables terminal mode ` + "`?2004`" + `; otherwise it writes literal text.
`,
	"verifiers": `# Verifiers

Supported v1 verifiers: ` + "`screen`" + `, ` + "`region`" + `, ` + "`cell`" + `, ` + "`cursor`" + `, ` + "`process`" + `, ` + "`snapshot`" + `, ` + "`file`" + `, ` + "`script`" + `, ` + "`count`" + `, ` + "`link`" + `, trusted ` + "`command`" + `, and the process-telemetry ` + "`metrics`" + ` verifier (see ` + "`process-telemetry`" + `). Screen/region matchers: ` + "`equals`" + `, ` + "`contains`" + `, ` + "`notContains`" + `, ` + "`matches`" + ` (preferred), or legacy ` + "`regex`" + `.

Screen verifiers support ` + "`contains`" + `, ` + "`notContains`" + `, and ` + "`regex`" + `. Cell verifiers can check characters and style attributes (fg, bg, bold, dim, italic, underline, reverse). Process verifiers can check exit state and exit code.

Colors use a canonical form: the 16 base colors are named (` + "`red`" + `, ` + "`brightblue`" + `, …), 256-palette indices 16-255 are their decimal string (` + "`\"201\"`" + `), and truecolor is lowercase hex (` + "`\"#ff8800\"`" + `). The same values color the rendered ` + "`screens/final.svg`" + `.

` + "`file`" + ` polls the filesystem for a file matching a glob (filename wildcards supported, directory portion is literal). The verifier passes when a match exists, and optionally requires the matched file's body to contain a needle. Default timeout is 5s; override with ` + "`timeoutMs`" + `.

` + "`script`" + ` runs an external Node module (or shell script) that returns ` + "`{ ok, evidence }`" + ` as JSON on stdout. Use the ` + "`run`" + ` form for inline bodies and the ` + "`file`" + ` form for external scripts. Fixtures resolve to ` + "`ctx.fixtures`" + ` in the script; large evidence payloads are written to ` + "`outcomes/<id>.raw.json`" + `.

Outcomes can set ` + "`timeoutMs`" + ` and ` + "`normalize`" + ` when a single assertion needs longer polling or custom volatile-text cleanup.

` + "`count`" + ` asserts how many cells on the screen, or within a region, match a character or pattern (see ` + "`count-verifier`" + `).

` + "`link`" + ` asserts that an OSC 8 hyperlink is present: ` + "`link: { url, text }`" + ` matches a substring of the link URI and (optionally) the linked text. Useful for TUIs that render clickable links.

See also: ` + "`file-script-verifiers`" + ` for end-to-end examples of ` + "`file`" + ` and ` + "`script`" + `.
`,
	"artifacts": `# Artifacts

Each run writes ` + "`run.json`" + `, ` + "`run.yaml`" + `, ` + "`run.md`" + `, ` + "`agent_context.md`" + `, ` + "`events.ndjson`" + `, ` + "`spec.resolved.yml`" + `, final screens, frames, raw logs, snapshots, outcomes, and diagnostics.

Start with ` + "`run.md`" + ` for a human summary, ` + "`run.json`" + ` for automation, ` + "`agent_context.md`" + ` for agent debugging, ` + "`diagnostics/environment.md`" + ` for runtime context, and ` + "`screens/final.txt`" + ` for the normalized terminal state.

The final screen is also rendered to a deterministic ` + "`screens/final.svg`" + ` (a pure function of the captured cell grid). Render any captured screen on demand with ` + "`glyph render <run|latest> [--screen <name>] [--out path|-]`" + `; the SVG is reproducible and safe to regenerate in CI or drop into a PR comment.

Scrub the recorded frames interactively with ` + "`glyph replay <run> --tui`" + `: step (←/→), jump (home/end), and play back ` + "`frames/frames.ndjson`" + ` to see exactly when the screen changed.
`,
	"agents": `# Agents

Call ` + "`glyph agent --format md`" + ` or ` + "`glyph explain --format json`" + ` before editing specs.

Recommended loop:

1. ` + "`glyph spec verify <spec> --format json`" + `
2. ` + "`glyph run <spec> --format json`" + `
3. ` + "`glyph context latest --format md`" + ` after a failure
4. inspect ` + "`diagnostics/failure.md`" + `, ` + "`screens/final.txt`" + `, and ` + "`frames/frames.ndjson`" + `

Do not edit ` + "`intent`" + ` or ` + "`outcomes`" + ` without surfacing the contract change. Repair ` + "`steps`" + ` when the route through the terminal UI changed.
`,
	"mcp": `# MCP

Run ` + "`glyph mcp`" + ` to start the stdio MCP server. The current server exposes tools for explain, docs, doctor (full check matrix), list, spec verification, spec scaffolding, runs, snapshot updates, diffs, context lookup, screen rendering (` + "`glyph_render`" + `), step repair with optional ` + "`verify`" + ` (` + "`glyph_repair`" + `), affected-spec selection (` + "`glyph_affected_specs`" + `), and artifact pruning (` + "`glyph_clean`" + `).
`,
	"configuration": `# Configuration

Glyphrun reads ` + "`glyphrun.config.yml`" + ` by walking up from the spec path. Defaults include ` + "`.glyphrun/runs`" + ` artifacts and an xterm-256color terminal profile.

Use config for shared terminal defaults, artifact behavior, variables, and redaction rules. Use ` + "`glyph doctor --format md`" + ` to confirm the active config and artifact root.

` + "`target.timeoutMs`" + ` wraps the whole target session after the PTY starts and exits with code ` + "`3`" + ` when it expires.

Use ` + "`glyph init [dir] --cmd <target> --ready <text>`" + ` to create ` + "`glyphrun.config.yml`" + `, ` + "`specs/glyphrun/smoke.yml`" + `, and ` + "`.gitignore`" + ` artifact entries.

Use ` + "`terminal.alternateScreen: require`" + ` when a full-screen TUI must enter alternate screen mode, or ` + "`forbid`" + ` when a command must stay on the main terminal screen. The default is ` + "`auto`" + `.

## Secrets (tvault env-group integration)

Declare a tvault env-group (or direct project) in the environment block and glyphrun resolves the secrets at run time, injecting them into the process environment. The config file carries only group/env/project names — never secret values.

` + "```" + `yaml
environments:
  local:
    secrets:
      group: liftclub        # tvault environment group
      env: preview            # environment within the group
      only:                   # optional: inject only these keys (least privilege)
        - DATABASE_URL
        - STRIPE_SECRET_KEY
    env:
      TVAULT_DIR: .glyphrun/tmp/vault
      TVAULT_PASSPHRASE: glyphpass
` + "```" + `

Or use a direct project (no env group):

` + "```" + `yaml
environments:
  ci:
    secrets:
      project: liftclub-preview
` + "```" + `

At run time glyphrun calls ` + "`tvault env --group <g> --env <e> --format json`" + ` (or ` + "`-p <project>`" + `), parses the JSON output, and merges the key/value pairs into the run environment. All resolved values are added to the per-run redactor so they are scrubbed from every artifact.

` + "`TVAULT_DIR`" + ` and ` + "`TVAULT_PASSPHRASE`" + ` (or ` + "`TVAULT_IDENTITY_KEY`" + `) must be in the environment — set them in the config ` + "`env`" + ` block or export them before running glyph.

The ` + "`only`" + ` allowlist and ` + "`prefix`" + ` filter are applied client-side after resolution. A key is kept if it matches either selector (union semantics, matching ` + "`tvault run --only/--prefix`" + `).

When ` + "`secrets`" + ` is absent, behavior is identical to today — the block is purely additive.
`,
	"install": `# Install

Build and install glyph globally:

` + "```" + `
$ task install
# → builds with -ldflags stamping version / commit / buildDate
# → copies the binary to /opt/homebrew/bin/glyph
# → prints ` + "`glyph --version`" + ` so you can confirm the install
` + "```" + `

Or build by hand:

` + "```" + `
$ go build \
    -ldflags "-X github.com/abdul-hamid-achik/glyphrun/internal/version.Version=$(git describe --tags --always) \
               -X github.com/abdul-hamid-achik/glyphrun/internal/version.Commit=$(git rev-parse --short HEAD) \
               -X github.com/abdul-hamid-achik/glyphrun/internal/version.BuildDate=$(date -u +%Y-%m-%d)" \
    -o /opt/homebrew/bin/glyph ./cmd/glyph
$ glyph --version
glyph version <version> (<sha> <date>)
` + "```" + `

When the linker doesn't override the version vars (a bare ` + "`go install`" + ` or ` + "`go run`" + `), glyph prints ` + "`dev (unknown unknown)`" + ` — useful for testing without a release build.

Confirm the install path is on ` + "`$PATH`" + `:

` + "```" + `
$ which glyph
/opt/homebrew/bin/glyph
$ glyph doctor --format md
` + "```" + `

For CI distribution, prefer the build command above wrapped in ` + "`goreleaser`" + ` so the binary ships as a tarball per platform/arch. A ` + "`brew tap`" + ` is a future option once the first tag is cut.
`,
	"troubleshooting": `# Troubleshooting

Use ` + "`glyph context latest --format md`" + ` after a failure. Inspect ` + "`screens/final.txt`" + `, ` + "`raw/pty.raw.log`" + `, ` + "`frames/frames.ndjson`" + `, and ` + "`diagnostics/failure.md`" + `.

Errored and runner-level-failed runs carry ` + "`errorKind`" + ` + ` + "`diagnostic`" + ` in the JSON envelope (` + "`glyph run <spec> --format json`" + `). Check ` + "`errorKind`" + ` first — it maps to an actionable next step (` + "`contract_hash_mismatch`" + ` → re-stamp, ` + "`timeout`" + ` → raise ` + "`timeoutMs`" + `, ` + "`target_exited`" + ` → fix the target and inspect ` + "`raw/pty.raw.log`" + ` (not ` + "`timeoutMs`" + `), ` + "`target_start`" + ` → fix ` + "`cmd`" + `, ` + "`unsupported_terminal`" + ` → switch profile). The same envelope carries a ` + "`nextActions`" + ` array with the concrete command and reason — read it before re-deriving a fix.

Use ` + "`glyph run <spec> --format md --progress always`" + ` for live step/outcome progress during long TUI runs. Progress is written to stderr.
`,
	"file-script-verifiers": `# File and Script Verifiers

The ` + "`file`" + ` verifier polls the filesystem until a glob match appears, optionally requiring the matched file to contain a needle:

` + "```yaml" + `
outcomes:
  - id: report_dropped
    description: the daemon wrote a report under the runs dir
    verify:
      file:
        glob: /var/run/myapp/report-*.json
        contains: '"status":"ok"'
        timeoutMs: 5000
` + "```" + `

The ` + "`script`" + ` verifier runs an external Node module (or shell script) that returns ` + "`{ ok, evidence }`" + ` JSON on stdout:

` + "```yaml" + `
outcomes:
  - id: rows_match_seed
    description: the rendered table has exactly the rows from the seed
    verify:
      script:
        runtime: node
        file: ./verifiers/check-rows.ts
        fixtures:
          expectedRows: "3"
          seedPath: ${artifacts.seed.path}
        timeoutMs: 10000
` + "```" + `

The Node script receives the resolved context as the second argv argument (a JSON file with ` + "`{ input, output, fixtures, runDir, specDir }`" + `). Shell scripts receive the same context via env vars: ` + "`$GLYPHRUN_INPUT`" + `, ` + "`$GLYPHRUN_OUTPUT`" + `, ` + "`$GLYPHRUN_FIXTURES_JSON`" + `, ` + "`$GLYPHRUN_RUN_DIR`" + `.

Any ` + "`evidence`" + ` returned by the script that doesn't fit in the outcome's markdown budget is written to ` + "`outcomes/<id>.raw.json`" + ` so long payloads (DB rows, large JSON, etc.) survive the trim.
`,
	"metadata-list": `# Metadata and \` + "`" + `glyph list` + "`" + `

Add a ` + "`metadata`" + ` block to any spec to classify it for filtering and reporting:

` + "```yaml" + `
version: 1
name: login_wall
metadata:
  feature: auth
  owner: security
  priority: high
  tags:
    - smoke
    - OPG-1234
intent: ...
` + "```" + `

` + "`glyph list`" + ` walks one or more spec paths (files or directories) and prints a compact table with every spec's name, metadata, contract hash, and step/outcome counts:

` + "```" + `
$ glyph list examples/specs
| name | feature | owner | priority | tags | steps | outcomes | contract | path |
| --- | --- | --- | --- | --- | ---: | ---: | --- | --- |
| \` + "`" + `junit_xml_demo` + "`" + ` | ci-integration | release | normal | junit, example | 4 | 2 | \` + "`" + `sha256:584af…` + "`" + ` | examples/specs/junit_xml_demo.yml |
` + "```" + `

Filter with ` + "`--feature`" + `, ` + "`--tag`" + `, or ` + "`--owner`" + `. Use ` + "`--format json`" + ` to feed the result into a CI dashboard. Specs that fail to parse are still listed (with a ` + "`parseError`" + ` field) so the table reflects the full surface of the input.
`,
	"import-export": `# Import / Export

` + "`glyph import bats <file.bats>`" + ` converts a BATS test file into a glyphrun spec. Each ` + "`@test`" + ` block becomes an outcome; the test body is replayed through a per-spec runner script. The importer writes both the spec (` + "`.yml`" + `) and the runner (` + "`.runner.sh`" + `) next to the source file. The spec tags itself with ` + "`feature: imported`" + ` and ` + "`tags: [bats-import]`" + ` so a single ` + "`glyph list --tag bats-import`" + ` finds every imported spec.

` + "```" + `
$ glyph import bats tests/login.bats --out specs/login.yml
$ glyph run specs/login.yml --format md
` + "```" + `

The reverse direction is ` + "`glyph export bats <spec.yml>`" + `, which emits a ` + "`.bats`" + ` file. The export is best-effort: outcomes whose verifier is a ` + "`command:`" + ` map cleanly; outcomes that depend on terminal-specific verifiers (screen, region, cell) are emitted with ` + "`# TODO`" + ` comments so a human reviewer can adapt them. Use this to ship a shell-runnable test artifact for environments where installing glyphrun is impractical.
`,
	"artifacts-pipeline": `# Artifact Pipeline

Glyphrun can capture files a TUI target wrote (` + "`download`" + `), run external scripts that produce new artifacts (` + "`transform`" + `), and queue multiple keystrokes as a single PTY write (` + "`batch`" + `).

` + "```yaml" + `
steps:
  - download:
      path: /var/run/myapp/report.txt
      saveAs: report.txt
      assign: report
      waitFor: true
      timeoutMs: 5000
  - transform:
      runtime: shell
      file: ./transforms/uppercase.sh
      input: ${artifacts.report.path}
      saveAs: upper.txt
      assign: reportUpper
      timeoutMs: 10000
  - batch:
      - press: "/"
      - type: "search query"
      - press: "enter"
      - wait:
          screen: { contains: "results" }
outcomes:
  - id: upper_matches
    verify:
      file:
        glob: /var/run/myapp/report.txt
        contains: "status=ok"
` + "```" + `

` + "`download`" + ` and ` + "`transform`" + ` register their outputs as **named artifacts** addressable by later steps via ` + "`${artifacts.<name>.path}`" + ` (absolute) or ` + "`${artifacts.<name>.relativePath}`" + ` (run-relative). The placeholders are resolved at *runtime*, just before each step runs, so a step can reference artifacts produced by earlier steps in the same spec.

` + "`batch`" + ` concatenates every ` + "`press`" + ` / ` + "`type`" + ` / ` + "`paste`" + ` / ` + "`send`" + ` sub-step into one ` + "`pty.write()`" + ` syscall, preserving transient TUI state (a command palette, a focused menu, a hover popover) that would be lost between separate top-level steps. An optional trailing ` + "`wait`" + ` is the only synchronization point.

The runner injects ` + "`$GLYPHRUN_RUN_DIR`" + ` into both the target process env and the ` + "`command:`" + ` verifier env, so shell commands can reference the run's path without re-deriving it.
`,
	"redaction-block": `# Per-Spec Redaction

The ` + "`redaction:`" + ` block on a spec adds per-spec redaction values on top of the project config. Use it for one-off secrets a single spec touches — a test user account, a fixture API key — without polluting the global redactor that ships in ` + "`glyphrun.config.yml`" + `.

` + "```yaml" + `
version: 1
name: dashboard_smoke
redaction:
  values:
    - "[email protected]"
    - "fixture-api-key-abc123"
    - "tenant=acme"
intent: ...
` + "```" + `

Rules:

- Values shorter than 4 characters are dropped. The bar matches cairn's behavior and prevents obvious false positives from corrupting artifacts.
- Values are sorted longest-first before substitution so ` + "\"abc123\"" + ` is not shadowed by ` + "\"abc\"" + `.
- Per-spec values are layered on top of the config redactor, never replace it. The config's ` + "`headers`" + ` and ` + "`patterns`" + ` still apply.
- The redactor only runs against text artifacts (` + "`run.md`" + `, ` + "`screens/*`" + `, ` + "`raw/pty.raw.log`" + `). The raw PTY log is also truncated at ` + "`artifacts.maxRawLogBytes`" + ` from the config; the truncation marker itself contains the byte cap so the loss is visible.

Per-spec redaction is a contract — the spec's ` + "`contractHash`" + ` covers the ` + "`redaction:`" + ` block (and ` + "`coversSymbol`" + ` when set). Changing it invalidates the hash on the next run.
`,
	"contract-hash": `# Contract Hash Enforcement

Specs carry a ` + "`contractHash`" + ` stamped over ` + "`intent`" + `, ` + "`outcomes`" + `, ` + "`redaction:`" + `, and ` + "`coversSymbol`" + ` (when set). Glyphrun refuses to run a spec whose on-disk content does not match the hash. The point is to detect silent contract drift: a contributor edits an outcome to make a flaky test pass, the hash stops matching, the run aborts with exit code ` + "`6`" + `, and the change shows up in code review.

Workflow:

1. ` + "`glyph spec verify <spec> --stamp`" + ` regenerates the hash after an intentional contract change.
2. ` + "`glyph run <spec>`" + ` compares the stamp against the on-disk content. On mismatch, no PTY is started, no artifacts are written; the CLI prints the expected and actual hashes plus a hint pointing at the ` + "`intent`" + ` / ` + "`outcomes`" + ` / ` + "`redaction`" + ` blocks.
3. CI gates: a ` + "`glyph spec verify <dir> --format json`" + ` step in the pipeline catches drift before merge.

The hash is sha256 over the canonical JSON of the contract fields. Map keys are sorted by Go's ` + "`encoding/json`" + `, so the hash is stable across editors that reorder YAML keys.

Exit codes:

- ` + "`0`" + ` — pass
- ` + "`1`" + ` — outcome failure
- ` + "`2`" + ` — runtime error
- ` + "`3`" + ` — target timeout
- ` + "`4`" + ` — spec parse / schema error
- ` + "`6`" + ` — contract-hash mismatch
- ` + "`7`" + ` — unsupported terminal (alternate-screen required, not entered)

Error classification — every errored run (and failed runs with a runner-level cause) carries ` + "`errorKind`" + ` + ` + "`diagnostic`" + ` in the JSON envelope:

- ` + "`target_start`" + ` — target command could not start; fix ` + "`cmd`" + `/` + "`cwd`" + `
- ` + "`timeout`" + ` — target or step exceeded timeout; raise ` + "`timeoutMs`" + `
- ` + "`target_exited`" + ` — target exited while a screen wait was still unsatisfied; inspect diagnostic/` + "`raw/pty.raw.log`" + ` and fix the target (not ` + "`timeoutMs`" + `). Structured details live on ` + "`targetExit`" + ` (` + "`exitCode`" + `, ` + "`lastPtyLine`" + `)
- ` + "`contract_hash_mismatch`" + ` — stamped hash drift; re-stamp with ` + "`glyph spec verify --stamp`" + `
- ` + "`unsupported_terminal`" + ` — switch ` + "`terminal.profile`" + `
- ` + "`step_failure`" + ` — a step errored; inspect ` + "`diagnostics/failure.md`" + `
- ` + "`precondition`" + ` — precondition command or secret resolution failed
- ` + "`spec_parse`" + ` — spec failed schema validation or parsing

On exit 4/6 the structured JSON envelope is printed to stdout (not only stderr) so consumers decoding stdout never see an empty payload. For ` + "`contract_hash_mismatch`" + ` the envelope also includes ` + "`contractHash`" + ` (computed) and ` + "`expectedHash`" + ` (stamped). Every errored run also carries an additive ` + "`nextActions`" + ` array — ordered actionable commands per ` + "`errorKind`" + ` (path-aware when possible; ` + "`safeToAutoRun`" + ` always false). Contract-hash mismatches suggest ` + "`glyph spec verify --stamp`" + `, not snapshot updates. Non-errored runs omit it. Run results embed ` + "`$schema: urn:glyphrun.dev:run:v1`" + `.
`,
	"retention": `# Retention and ` + "`glyph clean`" + `

Run directories accumulate fast: a 200-spec suite that runs on every PR fills ` + "`.glyphrun/runs/`" + ` with tens of thousands of files. The runner keeps the most recent N run directories per artifact root and prunes the rest after every successful run.

The default is **3**. A config file that omits ` + "`retention.keepRuns`" + ` keeps the default; an explicit ` + "`retention.keepRuns: 0`" + ` disables auto-prune ("keep everything"):

` + "```yaml" + `
retention:
  keepRuns: 20   # default is 3; 0 disables auto-prune
` + "```" + `

After each run, the runner prunes the oldest run directories, keeping the N most recent. The prune is best-effort — failures are logged as ` + "`retention.pruned`" + ` events in ` + "`events.ndjson`" + ` and never block the run result. The current run is always kept; the cap applies to historical runs only.

## Archiving pruned runs (fcheap / file.cheap)

Instead of deleting pruned runs, you can route them to an external storage tool (e.g. ` + "`fcheap`" + ` / ` + "`file.cheap`" + `). The runner invokes your command with the run directory appended as the final positional argument:

` + "```yaml" + `
retention:
  keepRuns: 3
  archive:
    enabled: true
    command: fcheap          # your storage binary
    args: ["store"]          # invoked as: fcheap store <runDir>
    timeout: 5m              # duration string; default 5m
` + "```" + `

On archive success (exit 0) the local run directory is deleted — move semantics. On a non-zero exit, a timeout, or a missing binary, the local directory is **preserved** and the failure is surfaced as a ` + "`retention.archive.error`" + ` event (and a warning on stderr). Archival never fails the run. Skip archival for a single ` + "`glyph clean`" + ` with ` + "`--no-archive`" + `.

For an explicit sweep, use ` + "`glyph clean`" + `:

` + "```" + `
# keep the 10 most recent runs
$ glyph clean --keep 10

# wipe everything under the artifact root
$ glyph clean --all

# delete pruned runs locally without archiving
$ glyph clean --no-archive

# wipe a custom root
$ glyph clean --all --artifact-root /tmp/glyph
` + "```" + `

` + "`glyph clean`" + ` always prints what it pruned (and archived) so a CI log captures the operation. Combine with ` + "`--format json`" + ` to feed the count into a release-notes generator.
`,
	"rerun-failed": `# Rerunning Failed Specs

` + "`glyph run`" + ` writes a path-aware failure index to ` + "`.glyphrun/runs/.last-failed.json`" + ` (plus a legacy name list in ` + "`.last-failed.txt`" + `) at the artifact root. Each entry records ` + "`name`" + ` and the filesystem ` + "`path`" + ` of the failing spec. Passing runs drop their entry automatically.

Re-execute only the failures:

` + "```" + `
$ glyph run unused.yml --rerun-failed --format json
# re-runs every path recorded in .last-failed.json
` + "```" + `

When the index only has names (legacy text file, no paths), Glyphrun lists them and exits 0 so you can re-run those specs once with real paths and rebuild a path-aware index.

Add ` + "`.glyphrun/`" + ` to ` + "`.gitignore`" + ` so the index never leaves the machine.
`,
	"capture-policy": `# Capture Policy

The runner writes several artifact channels per run: ` + "`screens/`" + `, ` + "`frames/`" + `, ` + "`raw/`" + `, ` + "`agent_context.md`" + `, and so on. Each can be tuned to one of three modes:

- ` + "`always`" + ` — write regardless of the run outcome
- ` + "`on-failure`" + ` — write only when the run failed or errored
- ` + "`never`" + ` — never write

The project config sets the defaults for every spec:

` + "```yaml" + `
artifacts:
  snapshots: true
  frames: true
  rawLog: false        # expensive on a fast TUI
  finalScreen: true
  agentContext: true
  retention:
    keepRuns: 20
` + "```" + `

A spec can override individual channels with an ` + "`artifacts:`" + ` block:

` + "```yaml" + `
version: 1
name: heavy_tui_smoke
artifacts:
  frames: never
  rawLog: always
  finalScreen: always
` + "```" + `

This is useful for two kinds of spec:

- **Expensive specs** that emit thousands of frames per second — turn frames off in the spec, keep them on for everything else.
- **Critical specs** that you want to debug no matter what — force ` + "`agentContext: always`" + ` and ` + "`rawLog: always`" + ` so the failure surface is always there, even on a passing run.

The ` + "`artifacts:`" + ` block is part of the contract hash. Adding a capture override invalidates the stamp; re-stamp with ` + "`glyph spec verify <spec> --stamp`" + `.
`,
	"count-verifier": `# Count Verifier

The ` + "`count:`" + ` verifier asserts the count of cells in a region. It is the terminal-shaped sibling of cairn's ` + "`count: { role: ... }`" + ` — cairn counts DOM nodes by role, glyphrun counts cells by rune.

` + "```yaml" + `
outcomes:
  - id: exactly_three_errors
    description: the error pane shows three error rows
    verify:
      count:
        region: { x: 0, y: 0, width: 80, height: 24 }
        matches: "x"            # optional: count cells equal to this rune
        equals: 3               # exactly one of equals / atLeast / atMost / between
` + "```" + `

Matcher (` + "`matches`" + `):

- omitted or ` + "\"nonEmpty\"" + ` — count non-blank cells
- a single rune — count cells equal to that rune
- multi-character strings are rejected (cells are single runes; a substring would be ambiguous)

Comparator (exactly one):

- ` + "`equals: N`" + ` — matched count must equal N
- ` + "`atLeast: N`" + ` — matched count must be >= N
- ` + "`atMost: N`" + ` — matched count must be <= N
- ` + "`between: [min, max]`" + ` — matched count must be in [min, max]

Region (optional):

- omitted — the full screen
- ` + "`region: { x, y, width, height }`" + ` — restrict to a sub-region

Evidence: the verifier returns the matched count as ` + "`{ matched, comparator, expected }`" + ` in ` + "`outcomes/<id>.raw.json`" + ` so a passing run can be inspected without re-running.
`,

	"process-telemetry": `# Process Telemetry (monitor integration)

Glyphrun can capture process-level telemetry of the target it spawns via the ` + "`monitor`" + ` CLI (~/projects/monitor): CPU, RSS, thread count, the process tree, and pprof/` + "`sample`" + ` profiles. Three opt-in surfaces share one foundation.

## Run-level sampling: ` + "`glyph run --monitor <path>`" + `

` + "`glyph run specs/foo.yml --monitor ./bin/monitor`" + ` samples the spawned target's CPU/RSS on a tick (default 250ms; ` + "`--monitor-interval`" + `) and writes ` + "`diagnostics/process.md`" + ` + ` + "`diagnostics/process.json`" + ` (peak/mean CPU+RSS, the sample timeline) into the run dir. Add ` + "`--monitor-profile heap|cpu|goroutine|sample`" + ` to capture an end-of-run profile. Zero-cost when the flag is absent. On Windows ConPTY the target PID is unavailable, so sampling is skipped with a note.

When glyphrun is launched by ` + "`monitor run <spec>`" + ` it detects ` + "`MONITOR=1`" + ` and writes the target PID (` + "`target.pid`" + ` in the run dir, and ` + "`glyphrun-target.pid`" + ` in ` + "`$MONITOR_RUN_DIR`" + ` when set) so the parent monitor can observe the exact process without ` + "`--monitor`" + `.

## Step-level capture: the ` + "`monitor:`" + ` step

A ` + "`monitor:`" + ` step takes a one-shot reading of the live target at a point in the flow and stores it as a named artifact (evidence to keep or assert on later). A snapshot is always captured; ` + "`tree`" + ` and ` + "`profile`" + ` add the process subtree and/or a profile.

` + "```yaml" + `
steps:
  - wait: { screen: { contains: "dashboard" } }
  - monitor:
      saveAs: dashboard_load
      tree: true
      profile: heap
` + "```" + `

The artifact lands at ` + "`monitors/<saveAs>.md`" + ` (+ ` + "`.json`" + `), addressable as ` + "`${artifacts.dashboard_load.path}`" + ` by later steps. Requires the target PID (Windows ConPTY: no) and ` + "`monitor`" + ` on $PATH (or the run's ` + "`--monitor`" + ` binary).

## Perf budgets: the ` + "`metrics:`" + ` verifier

An outcome can assert process-telemetry budgets against the run's sampled summary. Each set field is an upper bound (<=). Requires ` + "`--monitor`" + ` (or a ` + "`monitor:`" + ` step) — without samples the outcome fails with a clear message instead of silently passing.

` + "```yaml" + `
outcomes:
  - id: rss_budget
    description: peak RSS stays under 512 MiB
    verify:
      metrics:
        peakRss: 536870912
        peakCpuPercent: 90
` + "```" + `
`,
	"github": `# GitHub Integration

Run specs in CI and surface the results on the pull request:

1. ` + "`glyph run <specs> --junit glyphrun-junit.xml --format json`" + ` runs the suite and writes a JUnit report.
2. ` + "`glyph comment --last 50 --out glyphrun-comment.md`" + ` renders a PR-comment-ready Markdown summary (status table, failure focus, final screen, and pointers to the deterministic ` + "`screens/final.svg`" + ` screenshots).
3. Upload ` + "`.glyphrun/runs`" + ` as a build artifact so reviewers can open the SVG screenshots and ` + "`agent_context.md`" + `.
4. Post ` + "`glyphrun-comment.md`" + ` as a sticky PR comment.

A reusable composite action lives at ` + "`.github/actions/glyphrun`" + ` and an example workflow at ` + "`examples/github/glyphrun-pr.yml`" + `. ` + "`glyph comment`" + ` writes to stdout by default, so it also pipes straight into ` + "`gh pr comment -F -`" + `.
`,

	"distribution": `# Distribution & Releasing

Glyphrun ships cross-platform binaries via GoReleaser.

Install a published release with Homebrew (` + "`brew install abdul-hamid-achik/tap/glyph`" + `), by downloading an archive from the GitHub Releases page, or from source with ` + "`go install github.com/abdul-hamid-achik/glyphrun/cmd/glyph@latest`" + `.

Cut a release by pushing a ` + "`v*`" + ` tag: ` + "`.github/workflows/release.yml`" + ` runs GoReleaser to build the darwin/linux/windows × amd64/arm64 matrix, publish a GitHub Release with checksums, and update the Homebrew cask. Validate first with ` + "`goreleaser check`" + ` and ` + "`goreleaser build --snapshot --clean`" + `.
`,

	"topics": `# Docs Topics

- overview
- quickstart
- authoring
- snippets
- steps
- verifiers
- artifacts
- agents
- mcp
- configuration
- troubleshooting
- artifacts-pipeline
- file-script-verifiers
- metadata-list
- import-export
- redaction-block
- contract-hash
- retention
- rerun-failed
- capture-policy
- count-verifier
- process-telemetry
- github
- distribution
- install
- topics
`,
}

func Content(topic string) (string, bool) {
	content, ok := byTopic[topic]
	return content, ok
}

func Topics() []string {
	topics := make([]string, 0, len(byTopic))
	for topic := range byTopic {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	return topics
}
