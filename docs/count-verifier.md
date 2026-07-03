# Count Verifier

The `count:` verifier asserts the count of cells in a region. It is the terminal-shaped sibling of cairn's `count: { role: ... }` — cairn counts DOM nodes by role, glyphrun counts cells by rune.

```yaml
outcomes:
  - id: exactly_three_errors
    description: the error pane shows three error rows
    verify:
      count:
        region:
          x: 0
          y: 0
          width: 80
          height: 24
        matches: "x"          # optional: count cells equal to this rune
        equals: 3             # exactly one of equals / atLeast / atMost / between
```

## Matcher (`matches`)

- omitted or `"nonEmpty"` — count non-blank cells
- a single rune — count cells equal to that rune
- multi-character strings are rejected (cells are single runes; a substring would be ambiguous)

## Comparator (exactly one)

| Field | Meaning |
| --- | --- |
| `equals: N` | matched count must equal N |
| `atLeast: N` | matched count must be `>=` N |
| `atMost: N` | matched count must be `<=` N |
| `between: [min, max]` | matched count must be in `[min, max]` |

## Region (optional)

- omitted — the full screen
- `region: { x, y, width, height }` — restrict to a sub-region

## Evidence

The verifier returns the matched count as `{ matched, comparator, expected }` in `outcomes/<id>.raw.json` so a passing run can be inspected without re-running.

See [Verifiers](/verifiers) for the full vocabulary.