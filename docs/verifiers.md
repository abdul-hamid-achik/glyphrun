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

Use trusted `command` verifiers only for local checks you would be comfortable running from a shell script.
