# Configuration

Glyphrun discovers `glyphrun.config.yml` by walking up from the spec path. The default artifact root is `.glyphrun/runs`.

Example:

```yaml
version: 1
artifactRoot: .glyphrun/runs
snapshotRoot: .glyphrun/snapshots

terminal:
  cols: 120
  rows: 36
  profile: xterm-256color
  alternateScreen: auto
  normalize:
    trimRight: true
    normalizeLineEndings: true

artifacts:
  rawLog: true
  frames: true
  finalScreen: true
  snapshots: true
  agentContext: true
```

Specs can override target, terminal, artifact, and environment settings locally. Keep secrets out of config and specs; pass them through the environment or setup commands instead.

## Secrets (tvault env-group integration)

Declare a tvault env-group (or direct project) in the environment block and glyphrun resolves the secrets at run time, injecting them into the process environment. The config file carries only source identifiers and key selectors — never secret values.

```yaml
environments:
  local:
    secrets:
      group: liftclub        # tvault environment group
      env: preview            # environment within the group
      only:                   # required unless prefix is set; inject only these keys
        - DATABASE_URL
        - STRIPE_SECRET_KEY
```

Or use a direct project (no env group):

```yaml
environments:
  ci:
    secrets:
      project: liftclub-preview
```

At run time glyphrun calls `tvault env --group <g> --env <e> --format json --only <key>` (or `-p <project>`), parses only the selected JSON output, and merges those values into the run environment. All resolved values are added to the per-run redactor before target or verifier subprocesses start.

Set TinyVault's own unlock material in the environment of the `glyph` process. It is passed only to TinyVault itself and is never inherited by targets or verifiers. Do not put passphrases or identity keys in `glyphrun.config.yml`.

An `only` allowlist or `prefix` selector is required and is passed directly to TinyVault. Glyphrun does not fetch all secrets and filter them locally.
This contract requires TinyVault v0.19.0 or newer; earlier releases do not
provide selectors on `tvault env`.

When `secrets` is absent, behavior is identical to today — the block is purely additive.

`target.timeoutMs` wraps the whole target session after the PTY starts. If it expires, Glyphrun exits with code `3` and writes diagnostics before cleaning up the process.

Use `terminal.alternateScreen: require` when a full-screen TUI must enter alternate screen mode, or `forbid` when a command must stay on the main terminal screen. The default `auto` records terminal behavior without enforcing it.

## Retention

Run directories accumulate fast. Set the top-level `retention.keepRuns` to auto-prune everything but the N most recent run directories after each successful run:

```yaml
retention:
  keepRuns: 20   # default is 3; 0 disables auto-prune
```

The default is **3**. A config that omits `retention.keepRuns` keeps the default; an explicit `0` disables auto-prune ("keep everything"). The current run is always kept; the cap applies to historical runs only. For an explicit sweep, use [`glyph clean --keep N`](/commands#glyph-clean) (or `--all` to wipe the artifact root).

### Publishing pruned evidence packs (fcheap / file.cheap)

Use `fcheap-publish` to package the complete run directory as a deterministic tar.gz in a private temporary directory. Glyphrun rejects symlinks and special files, bounds the compressed package to 2 MiB, and invokes `fcheap publish` with fixed metadata and an explicit remote lifetime. It removes the local run directory only after a strict credential-free `filecheap-publish/1` receipt reports `server-sha256` verification and exactly matches the uploaded package's digest and size. A timeout, oversized or incomplete package, publish failure, or malformed receipt preserves the local directory and records `retention.archive.error`. The publisher receives only the scoped file.cheap ingest environment; targets and verifiers receive neither it nor TinyVault material.
The strict publisher contract requires `fcheap` v0.31.0 or newer.

```yaml
retention:
  keepRuns: 3
  archive:
    enabled: true
    mode: fcheap-publish
    command: fcheap
    timeout: 5m              # duration string; default 5m
    retentionDays: 7         # remote lifetime; 1–31 days
```

Skip archival for a single `glyph clean` with `--no-archive`.

## Capture policy

The `artifacts:` block above sets project-wide capture defaults as booleans (`rawLog`, `frames`, `finalScreen`, `snapshots`, `agentContext`). A spec can override individual channels with an `artifacts:` block using one of three modes — `always`, `on-failure`, or `never`:

```yaml
artifacts:
  frames: never
  rawLog: always
  finalScreen: always
```

Use this to turn off an expensive channel (e.g. `frames: never`) for a single heavy spec, or to force a channel on for a critical spec you want to debug even on a pass. The per-spec `artifacts:` block is part of the contract hash — re-stamp with `glyph spec verify --stamp` after changing it (see [Contract Hash](/contract-hash)). For per-spec secret scrubbing, see [Per-Spec Redaction](/redaction-block).

`artifacts.maxRawLogBytes` (default 10 MiB) truncates the raw PTY log; the truncation marker carries the byte cap so the loss is visible.

Use `glyph doctor --format md` to confirm the active config, artifact root, schema source, and PTY availability.
