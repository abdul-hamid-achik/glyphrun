# Verifiers

The v1 verifier vocabulary is `screen`, `region`, `cell`, `cursor`, `process`, `snapshot`, trusted `command`, `file`, `script`, and `count`.

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

The `file`, `script`, and `count` verifiers extend assertions beyond the screen:

- `file` polls the filesystem for a glob match (filename wildcards supported, directory portion literal) and optionally requires the matched file's body to contain a needle.
- `script` runs an external `node` or `shell` script that returns a JSON `{ ok, evidence }` body; the evidence is written to `outcomes/<id>.raw.json` (on both pass and failure).
- `count` asserts how many cells on the screen, or within a region, match a character or pattern.

See `glyph docs file-script-verifiers --format md` and `glyph docs count-verifier --format md` for end-to-end examples.
