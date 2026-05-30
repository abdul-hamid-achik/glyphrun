# Steps

The v1 step vocabulary is `press`, `type`, `paste`, `send`, `wait`, `resize`, `snapshot`, and `use`.

Common patterns:

```yaml
steps:
  - wait:
      screen:
        contains: "Welcome"
      timeoutMs: 5000
  - type: "hello"
  - press: "enter"
  - resize:
      cols: 120
      rows: 36
  - snapshot: after_submit
  - wait:
      process:
        exitCode: 0
```

Use `wait` to synchronize on screen text, process state, snapshots, or trusted commands. Prefer visible screen conditions over sleeps.

Use `snapshot` to capture named terminal states inside the artifact pack. Use `glyph snapshot update <spec>` when a committed snapshot intentionally changes.
