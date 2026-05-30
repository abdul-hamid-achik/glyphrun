# Verifiers

The v1 verifier vocabulary is `screen`, `region`, `cell`, `cursor`, `process`, `snapshot`, and trusted `command`.

Examples:

```yaml
outcomes:
  - id: welcome_visible
    verify:
      screen:
        contains: "Welcome"
  - id: clean_exit
    verify:
      process:
        exitCode: 0
  - id: cursor_hidden
    verify:
      cursor:
        visible: false
  - id: title_is_bold
    verify:
      cell:
        x: 2
        y: 1
        char: "W"
        style:
          bold: true
  - id: binary_exists
    verify:
      command:
        run: "test -x ./bin/app"
```

Screen verifiers support `contains`, `notContains`, and `regex`. Cell verifiers can check characters and style attributes such as foreground color, background color, bold, dim, italic, underline, and reverse.

Outcomes can set their own `timeoutMs` and `normalize` block when a single assertion needs different polling or text cleanup than the rest of the spec:

```yaml
outcomes:
  - id: stable_build_id
    description: volatile build ids are normalized for this assertion
    timeoutMs: 10000
    normalize:
      replace:
        - regex: "build-[0-9]+"
          with: "build-<id>"
    verify:
      screen:
        contains: "build-<id>"
```

Use trusted `command` verifiers only for local checks you would be comfortable running from a shell script.
