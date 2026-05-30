# Reusable Actions

Reusable actions are YAML snippets for repeated terminal flows. They contain `steps` but no `intent` or `outcomes`, so they are not behavior contracts by themselves.

Create one with:

```bash
glyph spec scaffold --kind action > examples/actions/wait_for_ready_and_quit.yml
```

Use it from a spec:

```yaml
imports:
  - ../actions/wait_for_ready_and_quit.yml

steps:
  - use: wait_for_ready_and_quit
```

Action files are resolved relative to the importing spec. Placeholders such as `${vars.name}`, `${env.NAME}`, and `${projectRoot}` work inside actions.

Use actions for stable repeated mechanics such as opening a TUI, waiting for the ready screen, dismissing an optional prompt, or quitting cleanly. Keep assertions in the spec's `outcomes`.

## Conditional Steps

Use `when` on any step to run it only when a verifier is currently true:

```yaml
steps:
  - when:
      screen:
        contains: "optional prompt"
    press: "enter"
```

This is useful for optional onboarding prompts, login walls, model warnings, and other TUI state that may or may not appear.

## Bash Checks

Use `command` verifiers for local shell checks:

```yaml
outcomes:
  - id: binary_was_built
    description: the precondition built the binary
    verify:
      command:
        run: "test -x ./bin/app"
```

Treat command verifiers as trusted local automation. They are intentionally equivalent to shell scripts.
