# Per-Spec Redaction

The `redaction:` block on a spec adds per-spec redaction values on top of the project config. Use it for one-off secrets a single spec touches — a test user account, a fixture API key — without polluting the global redactor that ships in `glyphrun.config.yml`.

```yaml
version: 1
name: dashboard_smoke
redaction:
  values:
    - "[email protected]"
    - "fixture-api-key-abc123"
    - "tenant=acme"
intent: |
  a user can open the dashboard and see their tenant.
```

## Rules

- Values shorter than 4 characters are dropped. This matches cairn's behavior and prevents obvious false positives from corrupting artifacts.
- Values are sorted longest-first before substitution, so `"abc123"` is not shadowed by `"abc"`.
- Per-spec values are layered on top of the config redactor, never replace it. The config's `headers` and `patterns` still apply.
- The redactor only runs against text artifacts (`run.md`, `screens/*`, `raw/pty.raw.log`). The raw PTY log is also truncated at `artifacts.maxRawLogBytes` from the config; the truncation marker itself contains the byte cap so the loss is visible.

## Contract

Per-spec redaction is a contract — the spec's `contractHash` covers the `redaction:` block (and `coversSymbol` when set). Changing it invalidates the hash on the next run. Re-stamp with `glyph spec verify --stamp` after an intentional change. See [Contract Hash](/contract-hash).

For project-wide redaction patterns and the tvault secrets integration, see [Configuration](/configuration#secrets-tvault-env-group-integration).