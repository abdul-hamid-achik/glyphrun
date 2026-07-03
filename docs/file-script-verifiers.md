# File & Script Verifiers

The `file` and `script` verifiers extend assertions beyond the virtual screen. They are part of the [verifier vocabulary](/verifiers); this page covers them end to end.

## `file` — poll the filesystem

The `file` verifier polls the filesystem until a glob match appears, optionally requiring the matched file to contain a needle. The glob is resolved relative to the spec file's directory; wildcards (`*`, `?`) are supported in the filename portion. The literal `*` is treated as a wildcard, so a path with a literal `*` in a directory component is not supported.

```yaml
outcomes:
  - id: report_dropped
    description: the daemon wrote a report under the runs dir
    verify:
      file:
        glob: /var/run/myapp/report-*.json
        contains: '"status":"ok"'
        timeoutMs: 5000
```

| Field | Description |
| --- | --- |
| `glob` | Filesystem glob to match. |
| `contains` | Optional needle the matched file body must contain. |
| `timeoutMs` | Poll timeout (default 5s). |

The verifier passes when a match exists (and the body contains the needle, if set).

## `script` — run an external checker that returns `{ ok, evidence }`

The `script` verifier runs an external Node module (or shell script) that returns `{ ok, evidence }` as JSON on stdout. Use the `run` form for inline bodies and the `file` form for external scripts. Fixtures resolve to `ctx.fixtures` in the script.

```yaml
outcomes:
  - id: rows_match_seed
    description: the rendered table has exactly the rows from the seed
    verify:
      script:
        runtime: node
        file: ./verifiers/check-rows.ts
        fixtures:
          expectedRows: "3"
          seedPath: ${artifacts.seed.path}
        timeoutMs: 10000
```

| Field | Description |
| --- | --- |
| `runtime` | `node` (default) or `shell`. |
| `run` | Inline script body. |
| `file` | External script path. |
| `fixtures` | Map of fixture values passed to the script. |
| `timeoutMs` | Per-outcome timeout. |

### Context injection

- **Node** receives the resolved context as the second argv argument — a JSON file with `{ input, output, fixtures, runDir, specDir }`.
- **Shell** receives the same context via env vars: `$GLYPHRUN_INPUT`, `$GLYPHRUN_OUTPUT`, `$GLYPHRUN_FIXTURES_JSON`, `$GLYPHRUN_RUN_DIR`.

### Evidence

Any `evidence` returned by the script that doesn't fit in the outcome's markdown budget is written to `outcomes/<id>.raw.json` so long payloads (DB rows, large JSON, etc.) survive the trim.

See [Verifiers](/verifiers) for the full vocabulary and the [Artifact Pipeline](/artifacts-pipeline) for producing named artifacts that `script` fixtures can reference.