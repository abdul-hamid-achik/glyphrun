---
description: The Glyphrun verifier vocabulary — screen, scrollback, region, cell, cursor, process, snapshot, command, file, script, count, link, and metrics — typed, schema-validated terminal assertions.
---

# Verifiers

The v1 verifier vocabulary is `screen`, `scrollback`, `region`, `cell`, `cursor`, `process`, `snapshot`, trusted `command`, `file`, `script`, `count`, `link`, and the process-telemetry `metrics` verifier (see [Process Telemetry](/process-telemetry) and `glyph docs process-telemetry --format md`).

The `snapshot` verifier compares a captured screen with its committed golden under `snapshotRoot`. `mode` selects the contract: `text` (default) compares the normalized characters, so a golden stays portable across terminals that color differently; `cell` also compares every cell's style (color, bold, dim, italic, underline, reverse) against the committed `.json` grid, so a color regression fails the run; `json` additionally requires the cursor to match. Stories pick the mode with `goldenMode` (see [Stories](/stories)).

Examples:

```yaml
outcomes:
  - id: welcome_visible
    verify:
      screen:
        contains: "Welcome"
  - id: ready_exact
    verify:
      screen:
        equals: "ready\n"
  - id: ready_pattern
    verify:
      screen:
        matches: "^Welcome \\d+"
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

## Scrollback

An inline-mode TUI never enters the alternate screen. It commits finished lines above a small live region, and those lines scroll off the top of the terminal into its scrollback. Glyphrun's emulator keeps them, so a spec can assert that a line was committed, and that a long reply was committed whole and in order.

```yaml
outcomes:
  - id: reply_committed
    description: the streamed reply reached the scrollback whole
    verify:
      scrollback:
        matches: "(?s)first line.*last line"
  - id: nothing_leaked
    description: the secret never scrolled by
    verify:
      scrollback:
        notContains: "api-key"
  - id: reply_length
    description: the reply took at least 40 lines
    verify:
      scrollback:
        lines:
          atLeast: 40
  - id: transcript
    description: scrollback plus the live region reads start to finish
    verify:
      screen:
        includeScrollback: true
        matches: "(?s)first line.*last line.*>$"
```

`scrollback` takes exactly one of `contains`, `notContains`, `matches`, or `lines`. The text it matches has one row per line, oldest first, with trailing spaces trimmed, and goes through the same normalization as screen text (`trimRight`, `replace` rules, per-outcome `normalize`); ignore regions are screen coordinates and do not apply. Blank rows that scrolled off are kept, so `lines` counts them. `lines` compares the number of rows with exactly one of `equals`, `atLeast`, `atMost`, or `between: [min, max]`.

`includeScrollback: true` on a `screen` verifier (or wait) matches against the scrollback followed by the visible screen, joined by a line break, instead of the visible screen alone.

What counts as scrollback follows xterm:

- A row enters it when it scrolls off the top while the scroll region starts at the top row: a line feed at the bottom margin, `IND`/`NEL`, autowrap at the bottom-right cell, and `CSI n S`. A region that starts below the first row (`CSI 2;24 r`) adds nothing, and neither do `DL`, `IL`, or reverse index.
- The alternate screen never adds to it, and entering or leaving the alternate screen leaves it intact.
- `CSI 3 J` (erase saved lines) and a full reset (`ESC c`) clear it. `CSI 2 J` does not.
- A resize keeps it; rows are not reflowed.
- It holds at most `terminal.scrollbackLines` rows (default 10000; see [Configuration](/configuration)). Older rows are dropped first. A failing scrollback check says how many were dropped, and a passing `notContains` on a run that dropped rows only vouches for the rows that were kept.

Screen and region verifiers accept exactly one of: `equals`, `contains`, `notContains`, `matches` (preferred regex form), or legacy `regex` (alias of `matches`). Cell verifiers can check characters and style attributes such as foreground color, background color, bold, dim, italic, underline, and reverse. Cell verifiers compare the raw cell style: a cell on the terminal's default background has an empty `bg` even when the app set a default background with OSC 11. That color is reported separately as `defaultBg` (and `defaultFg`) on the snapshot and is used only when rendering SVG. Cells the app erased while a background was set carry that background, as in xterm.

Cell coordinates are columns. A wide character (East Asian text, most emoji) covers two: its `char` is in the first cell, and the second is a continuation cell with an empty `char` and `width` 0 that region and screen text skip. Combining marks and emoji sequences are part of their base cell's `char`.

Color values use a canonical form so `style: { fg, bg }` assertions are stable across SGR encodings:

- the 16 base colors are named: `black`, `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`, and their `bright`-prefixed variants (e.g. `brightred`). SGR `31` and `38;5;1` both read as `red`.
- 256-palette colors 16–255 are their decimal index as a string (e.g. `"201"`).
- truecolor (`38;2;r;g;b`) is lowercase hex (e.g. `"#ff8800"`).

The same values drive the colors in the rendered `screens/final.svg` screenshot.

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
- `link` asserts that an OSC 8 hyperlink is present: `link: { url, text }` matches a substring of the link URI and, optionally, the linked text.

See [File & Script Verifiers](/file-script-verifiers) and [Count Verifier](/count-verifier) for end-to-end examples.
