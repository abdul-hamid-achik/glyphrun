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

Declare a tvault env-group (or direct project) in the environment block and glyphrun resolves the secrets at run time, injecting them into the process environment. The config file carries only group/env/project names — never secret values.

```yaml
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
```

Or use a direct project (no env group):

```yaml
environments:
  ci:
    secrets:
      project: liftclub-preview
```

At run time glyphrun calls `tvault env --group <g> --env <e> --format json` (or `-p <project>`), parses the JSON output, and merges the key/value pairs into the run environment. All resolved values are added to the per-run redactor so they are scrubbed from every artifact.

`TVAULT_DIR` and `TVAULT_PASSPHRASE` (or `TVAULT_IDENTITY_KEY`) must be in the environment — set them in the config `env` block or export them before running glyph.

The `only` allowlist and `prefix` filter are applied client-side after resolution. A key is kept if it matches either selector (union semantics, matching `tvault run --only/--prefix`).

When `secrets` is absent, behavior is identical to today — the block is purely additive.

`target.timeoutMs` wraps the whole target session after the PTY starts. If it expires, Glyphrun exits with code `3` and writes diagnostics before cleaning up the process.

Use `terminal.alternateScreen: require` when a full-screen TUI must enter alternate screen mode, or `forbid` when a command must stay on the main terminal screen. The default `auto` records terminal behavior without enforcing it.

Use `glyph doctor --format md` to confirm the active config, artifact root, schema source, and PTY availability.
