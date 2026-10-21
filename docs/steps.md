---
description: The Glyphrun step vocabulary — press, type, paste, send, mouse, wait, resize, snapshot, use, when, download, transform, batch, and monitor — for driving terminal apps in a PTY.
---

# Steps

The v1 step vocabulary is `press`, `type`, `paste`, `send`, `mouse`, `wait`, `resize`, `snapshot`, `use`, `when`, the artifact-pipeline steps `download`, `transform`, and `batch`, and the process-telemetry `monitor` step (see [Process Telemetry](/process-telemetry) and `glyph docs process-telemetry --format md`).

`mouse: { x, y, button?, action? }` sends a mouse event at the 0-based cell `(x, y)`. `button` is one of `left` (default), `middle`, `right`, `wheelUp`, `wheelDown`; `action` is `click` (default), `press`, `release`, or `move`. The runner encodes the event as SGR (1006) when the target enabled that mode, otherwise as the legacy X10 encoding.

Every step can include a `when` guard. The guard uses the same verifier shape as an outcome (or a shorthand string) and skips the step when false.

Optional `id:` labels make failure messages and `StepResult` entries readable (`open_menu` instead of `step 3`).

Screen/region matchers accept `equals`, `contains`, `notContains`, `matches` (preferred), or legacy `regex`.

A `wait` takes exactly one condition: `screen`, `scrollback`, `process`, or `idle`, plus an optional `timeoutMs` beside it.

- `wait: { screen: { … } }` matches the visible screen. Add `includeScrollback: true` to match the scrollback followed by the visible screen instead.
- `wait: { scrollback: { contains | notContains | matches | lines } }` matches the lines that have scrolled off the top of the main screen. Use it to wait until an inline-mode TUI has committed a line (see [Inline-mode TUIs](/authoring#inline-mode-tuis)). `lines` compares how many rows the scrollback holds with exactly one of `equals`, `atLeast`, `atMost`, or `between`.
- A scrollback wait, like a screen wait, ends early with a target-exit error if the process exits before the condition holds.

```yaml
steps:
  - id: wait_ready
    wait:
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
      scrollback:
        contains: "build finished"
  - wait:
      screen:
        includeScrollback: true
        matches: "(?s)build finished.*\\$$"
  - when: 'screen.contains:"optional prompt"'
    press: "enter"
  - when:
      screen:
        matches: "^Ready \\d+"
    press: "enter"
  - use: quit_cleanly
  - wait:
      process:
        exitCode: 0
```

Set `mode: debug` at the top of a spec to force frames, raw logs, snapshots, and agent context on for flaky diagnosis.

Use `wait` to synchronize on screen text, process state, snapshots, or trusted commands. Prefer visible screen conditions over sleeps.

`press` accepts printable single characters plus common terminal keys: `enter`, `tab`, `shift+tab`, `esc`, `backspace`, `delete`, `space`, arrow keys, `pgup`, `pgdown`, `home`, `end`, and `ctrl+<letter>`/`c-<letter>` aliases such as `ctrl+c`, `ctrl+u`, and `c-m`.

Use `snapshot` to capture named terminal states inside the artifact pack. Use `glyph snapshot update <spec>` when a committed snapshot intentionally changes. When the scrollback is not empty, a snapshot also writes `snapshots/<name>.scrollback.txt` beside the screen; committed goldens stay screen-only.

Use `paste` for multi-character clipboard-style input. Glyphrun sends bracketed paste delimiters only after the target enables terminal mode `?2004`; otherwise it writes the literal text.

Use `use` with `imports` to reuse action files. Actions are best for repeated mechanics; keep behavior assertions in `outcomes`.

Use the artifact-pipeline steps to work with files a target produces: `download` captures a file the target wrote, `transform` runs an external script that produces a new artifact, and `batch` concatenates several `press`/`type`/`paste`/`send` sub-steps into a single PTY write (preserving transient TUI state, with an optional trailing `wait` as the only sync point). `download` and `transform` register named artifacts addressable by later steps via `${artifacts.<name>.path}`. See the [Artifact Pipeline](/artifacts-pipeline) page for end-to-end examples.
