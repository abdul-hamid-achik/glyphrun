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

Use `glyph spec verify <spec> --format json` before running a spec. Use `glyph spec verify <spec> --stamp` only when the behavior contract intentionally changed.

Good specs assert user-visible behavior. Avoid coupling outcomes to implementation details, timing artifacts, or raw ANSI bytes.
