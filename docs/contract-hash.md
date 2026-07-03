# Contract Hash

Specs carry a `contractHash` stamped over `intent`, `outcomes`, `redaction:`, and `coversSymbol` (when set). Glyphrun refuses to run a spec whose on-disk content does not match the hash. The point is to detect silent contract drift: a contributor edits an outcome to make a flaky test pass, the hash stops matching, the run aborts, and the change shows up in code review.

## What the hash covers

The hash is sha256 over the canonical JSON of the contract fields — `intent`, `outcomes`, `redaction:`, and `coversSymbol`. Map keys are sorted by Go's `encoding/json`, so the hash is stable across editors that reorder YAML keys. `steps` are deliberately **not** covered: they are repairable hints, not the contract.

## Workflow

1. **Re-stamp after an intentional change.** `glyph spec verify <spec> --stamp` regenerates the hash after `intent`/`outcomes`/`redaction`/`coversSymbol` deliberately changed.
2. **Run compares the stamp.** `glyph run <spec>` compares the stamped hash against the on-disk content. On mismatch, no PTY is started and no artifacts are written; the CLI prints the expected and actual hashes plus a hint pointing at the `intent` / `outcomes` / `redaction` blocks.
3. **CI gate.** A `glyph spec verify <dir> --format json` step in the pipeline catches drift before merge.

```bash
glyph spec verify specs/smoke.yml --format json     # check
glyph spec verify specs/smoke.yml --stamp           # re-stamp after an intentional change
```

Never edit `contractHash` by hand — always use `--stamp`. See [CLI Reference](/commands#glyph-spec-verify-spec).

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | pass |
| `1` | outcome failure |
| `2` | runtime error |
| `3` | target timeout |
| `5` | unsupported terminal (alternate-screen required, not entered) |
| `6` | contract-hash mismatch |

The contract/repair split is core to how Glyphrun thinks about specs: `intent` + `outcomes` are the durable behavior contract; `steps` are repairable hints. See [Authoring](/authoring) and [`glyph repair`](/commands#glyph-repair-spec-run-latest).