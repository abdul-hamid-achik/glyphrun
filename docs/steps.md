# Steps

The v1 step vocabulary is `press`, `type`, `paste`, `send`, `wait`, `resize`, `snapshot`, `use`, `when`, and the artifact-pipeline steps `download`, `transform`, and `batch`.

Every step can include a `when` guard. The guard uses the same verifier shape as an outcome and skips the step when false.

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
  - when:
      screen:
        contains: "optional prompt"
    press: "enter"
  - use: quit_cleanly
  - wait:
      process:
        exitCode: 0
```

Use `wait` to synchronize on screen text, process state, snapshots, or trusted commands. Prefer visible screen conditions over sleeps.

`press` accepts printable single characters plus common terminal keys: `enter`, `tab`, `shift+tab`, `esc`, `backspace`, `delete`, `space`, arrow keys, `pgup`, `pgdown`, `home`, `end`, and `ctrl+<letter>`/`c-<letter>` aliases such as `ctrl+c`, `ctrl+u`, and `c-m`.

Use `snapshot` to capture named terminal states inside the artifact pack. Use `glyph snapshot update <spec>` when a committed snapshot intentionally changes.

Use `paste` for multi-character clipboard-style input. Glyphrun sends bracketed paste delimiters only after the target enables terminal mode `?2004`; otherwise it writes the literal text.

Use `use` with `imports` to reuse action files. Actions are best for repeated mechanics; keep behavior assertions in `outcomes`.

Use the artifact-pipeline steps to work with files a target produces: `download` captures a file the target wrote, `transform` runs an external script that produces a new artifact, and `batch` concatenates several `press`/`type`/`paste`/`send` sub-steps into a single PTY write (preserving transient TUI state, with an optional trailing `wait` as the only sync point). `download` and `transform` register named artifacts addressable by later steps via `${artifacts.<name>.path}`. See `glyph docs artifacts-pipeline --format md` for end-to-end examples.
