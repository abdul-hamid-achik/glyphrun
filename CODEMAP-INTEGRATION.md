# glyphrun ⇄ codemap integration

> **Status:** design / proposed (2026-06-24). Authored from a codemap-side ecosystem survey.
> **One line:** behavior outcomes become graph annotations; blast radius drives which specs to run.

## Boundary
glyphrun is the runtime/behavior oracle — does the binary work end-to-end, is it flaky, did the rendered
output drift. codemap is the structure index. **Neither links a spec to the symbols it exercises today** —
that join is the whole opportunity. codemap already *consumes* glyphrun (its `specs/*.yml` + `task flows`);
this makes the relationship bidirectional.

## Integrations

### A — pin run outcomes to the entrypoint symbol  ·  S→M · **high**  ·  (adapter; glyphrun unchanged)
After `glyph run --format json`, map `target.cmd[0..N]` → the handler symbol via `codemap_find`, then
`codemap_annotate{source:'glyphrun', note:'<specName> <status>', data:{runId, status, exitCode, durationMs,
outcomes, feature, owner, tags}}`. `codemap_impact` advertises test coverage but only sees *unit* tests via
the call graph — this adds **end-to-end behavioral coverage** as a first-class durable fact, keyed by
`contractHash` so a re-stamp invalidates stale green badges. *(codemap EI.7.)*

### B — blast-radius-driven spec selection  ·  M · **high**
`codemap affected-specs --since <ref>`: git diff → `codemap_impact` per changed symbol → transitive blast
radius → intersect against the spec↔symbol links from A → emit the minimal spec-path set; `glyph run <those>`.
Run the 3 specs a change can hit, not the whole suite. The structure→behavior half that closes the loop with
A. *(codemap EI.5.)*

### C — flakiness / duration as a hotspot signal  ·  M · medium
`glyph run --repeat N` flaky-result + p50 `durationMs` pinned (`source:'glyphrun-flaky'`); studio
Metrics/Impact overlays a "behaviorally flaky" / "slow" badge on the relevant hubs.

### D — semantic spec discovery + scaffolding  ·  L · **high**
Index spec intent + outcome text into codemap's veclite (`kind:spec`, payload `{specName, feature, owner,
contractHash}`). `codemap_semantic('jwt login flow')` then returns matching **specs alongside code symbols** —
one query spanning code AND behavior contracts. An uncovered symbol (`codemap_orphans`) emits a
`glyph spec scaffold` stub seeded from the symbol's signature/docstring with a `coversSymbol:` binding (which
feeds A + B precisely). *(codemap EI.17.)*

### E — repair proposals annotated onto the drifted call path  ·  M · medium
Resolve a changed UI string → symbol via `codemap_find`/`codemap_semantic`; `codemap_annotate{kind:path,
source:'glyphrun-repair', data:{proposals}}` so a snapshot drift is anchored to the code path that caused it.

### F — registry handshake  ·  S · medium  ·  (codemap implements; glyphrun unchanged)
codemap's registry entry gains `{glyphrunRoot, specDir}`; `codemap_status` reads the newest
`.glyphrun/runs/*/run.json` to report `{specs:int, lastSuite:{status, ranAt}}`. *(codemap EI.3.)*

## Build order
A (behavioral badges — establishes the spec↔symbol link everything else needs) → B (spec selection) →
F (registry) → C (flaky signal) → E (repair) → D (semantic spec index — highest effort, do last).
