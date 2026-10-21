---
description: How to write Glyphrun behavior specs — keep intent and outcomes stable as the contract, treat steps as repairable navigation hints, and assert user-visible terminal behavior.
---

# Authoring

Keep `intent` and `outcomes` stable. Treat `steps` as repairable hints that an agent can adjust when a UI flow changes without changing the behavior contract.

## Spec Shape

```yaml
version: 1
name: smoke_test
intent: |
  a user can open the app and see the ready state.
target:
  cmd: ["./bin/app"]
  cwd: "."
steps:
  - wait:
      screen:
        contains: "ready"
  - snapshot: ready
outcomes:
  - id: ready_visible
    description: the ready state is visible
    verify:
      screen:
        contains: "ready"
```

Use `glyph spec verify <spec> --format json` before running a spec. Use `glyph spec verify <spec> --stamp` only when the behavior contract intentionally changed — see [Contract Hash](/contract-hash).

Good specs assert user-visible behavior. Avoid coupling outcomes to implementation details, timing artifacts, or raw ANSI bytes.

Use reusable actions for repeated step sequences:

```yaml
imports:
  - ../actions/wait_for_ready_and_quit.yml
steps:
  - use: wait_for_ready_and_quit
```

Use `when` for optional TUI state:

```yaml
steps:
  - when:
      screen:
        contains: "Confirm"
    press: "enter"
```

## Placeholders

Specs and action files can refer to values from the config and the environment with `${...}` placeholders. They are replaced when the file is read:

| Placeholder | Value |
| --- | --- |
| `${projectRoot}` | the project root (the directory of the config, or of the spec) |
| `${vars.NAME}` | `environments.<env>.vars.NAME` from the config |
| `${env.NAME}` | `environments.<env>.env.NAME` from the config |
| `${config.NAME}` | a config value such as `${config.artifactRoot}` |
| `${artifacts.NAME.path}`, `${artifacts.NAME.relativePath}` | a named artifact; resolved just before the step that uses it runs |

Only text shaped like one of these keys is a placeholder. Everything else is left as written, so scripts in `command` verifiers, `transform` steps, and `when` guards can use shell parameter expansion and JavaScript template literals directly: `${HOME}`, `${var#*:}`, `${x:-y}`, `${#arr}`, `${arr[@]}`, and `${a + b}` all pass through to the script. A bare identifier is not a key (`projectRoot` is the only bare one), which is why `${HOME}` needs no escape.

A key-shaped placeholder that has no value is an error, so a typo such as `${vars.nmae}` fails with `unresolved placeholder(s)` instead of reaching the script as text.

To write a literal `${...}` that looks like a key, double the dollar: `$${...}` becomes `${...}`. Use it for a shell variable or template that happens to be shaped like a key, such as `$${projectRoot}` or a JavaScript `$${vars.x}`. The escape works for any `${...}`, so `$${HOME}` is `${HOME}` too, but `${HOME}` alone is enough. A `$$` that is not followed by `{` (the shell's process id) is untouched.

## Inline-mode TUIs

An inline-mode TUI never uses the alternate screen. It keeps a small live region (a prompt, a status line) at the bottom of the terminal and commits finished lines above it, so they stay in the shell's scrollback after the app exits. Chat and agent UIs work this way. Their evidence is mostly not on the visible screen: after a long reply, `screen` shows only the last few rows.

Glyphrun keeps the rows that scroll off the top of the main screen, so a spec can assert on them:

```yaml
terminal:
  cols: 80
  rows: 24
  alternateScreen: forbid     # an inline TUI must stay on the main screen
steps:
  - type: "write a haiku"
  - press: "enter"
  # Wait for the reply to be committed, not for it to be visible.
  - wait:
      scrollback:
        contains: "[done]"
      timeoutMs: 15000
outcomes:
  - id: reply_committed_whole
    description: the streamed reply reached the scrollback whole and in order
    verify:
      scrollback:
        matches: "(?s)assistant: .*first line.*second line.*third line"
  - id: live_region_ready
    description: the prompt is back in the live region
    verify:
      screen:
        contains: ">"
  - id: whole_session
    description: scrollback plus the live region reads start to finish
    verify:
      screen:
        includeScrollback: true
        matches: "(?s)write a haiku.*third line.*>$"
```

Tips:

- Use `scrollback` for what the app committed, `screen` for the live region, and `screen` with `includeScrollback: true` for the transcript as a user would scroll back through it.
- Wait on the scrollback (`wait: { scrollback: … }`) for committed output. A line is in the scrollback only once enough later output pushed it off the top, so the last lines of a short run may still be on the screen: assert them with `screen`, or with `includeScrollback`.
- Trailing spaces are trimmed from every row, so a prompt of `> ` reads as `>`: match `>$`, not `> $`.
- Size the terminal for the test. A taller terminal keeps more on the screen and scrolls less; a short one scrolls early. Use `lines` (`equals`, `atLeast`, `between`) when the number of committed rows is the contract, and `terminal.scrollbackLines` if a run commits more than 10000.
- A shell prompt, or whatever ran before the app, may already be in the scrollback. Match with `matches` or `contains` instead of `equals`, and anchor on lines your app prints.
- Scrollback is never fed by the alternate screen. A full-screen TUI that also commits a summary after leaving it will show the summary in the scrollback and nothing from the alternate screen.
- A `resize` step keeps what is on the screen, like a real terminal: use it to test how the live region redraws on `SIGWINCH`. Shrinking the rows with the cursor at the bottom pushes the top rows into the scrollback, in order; growing adds blank rows at the bottom and pulls nothing back.
- Every run with a non-empty scrollback writes `scrollback.txt`, and each `snapshot:` step writes `snapshots/<name>.scrollback.txt`. Read them first when a scrollback outcome fails; the failure message also says how many rows were kept and dropped.

`examples/specs/scrollback.yml` runs a small inline-mode program that commits 100 lines above a two-line live region and asserts what reached the scrollback.
`examples/specs/resize.yml` resizes a terminal around an inline-mode program that redraws its live region with relative cursor moves.
