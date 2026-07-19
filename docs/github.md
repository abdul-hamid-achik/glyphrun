# GitHub Integration

Run Glyphrun specs in CI and surface the results on the pull request.

The pieces:

- `glyph run <specs> --junit <path> --format json` runs the suite and writes a JUnit XML report consumable by the GitHub test UI.
- `glyph comment [run|latest ...]` renders GitHub-flavored Markdown summarizing the runs: a status table, failure focus, the final screen folded into a `<details>` block, and pointers to the deterministic `screens/final.svg` screenshots. It writes to stdout by default (pipe to `gh pr comment -F -`) or to a file with `--out`. Use `--last N` to summarize the N most recent runs.
- The run artifact packs under `.glyphrun/runs` carry the SVG screenshots and `agent_context.md`; upload them so reviewers can open them.

Reusable composite actions:

- [`.github/actions/glyph-run`](../.github/actions/glyph-run/action.yml) — thin `glyph run` wrapper (specs, parallel, junit, artifact-root, extra args). Expects `glyph` on `PATH` (or pass `glyph-bin`).
- [`.github/actions/glyphrun`](../.github/actions/glyphrun/action.yml) — fuller flow that builds `glyph`, runs specs, uploads artifact packs, and prepares PR-comment Markdown (when present in the tree).

An example workflow that wires Glyphrun to a sticky PR comment is at [`examples/github/glyphrun-pr.yml`](../examples/github/glyphrun-pr.yml).

Minimal manual usage without a composite action:

```bash
glyph run specs/glyphrun/*.yml --junit glyphrun-junit.xml --format json
glyph run unused.yml --rerun-failed --format json   # re-run only the previous failures
glyph comment --last 50 | gh pr comment "$PR" -F -
```

```yaml
# Using the thin glyph-run action after installing glyph
- uses: ./.github/actions/glyph-run
  with:
    specs: specs/glyphrun/*.yml
    parallel: "2"
    junit: glyphrun-junit.xml
    format: md
```
