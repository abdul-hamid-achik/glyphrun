# GitHub Integration

Run Glyphrun specs in CI and surface the results on the pull request.

The pieces:

- `glyph run <specs> --junit <path> --format json` runs the suite and writes a JUnit XML report consumable by the GitHub test UI.
- `glyph comment [run|latest ...]` renders GitHub-flavored Markdown summarizing the runs: a status table, failure focus, the final screen folded into a `<details>` block, and pointers to the deterministic `screens/final.svg` screenshots. It writes to stdout by default (pipe to `gh pr comment -F -`) or to a file with `--out`. Use `--last N` to summarize the N most recent runs.
- The run artifact packs under `.glyphrun/runs` carry the SVG screenshots and `agent_context.md`; upload them so reviewers can open them.

A reusable composite action lives at [`.github/actions/glyphrun`](../.github/actions/glyphrun/action.yml): it builds `glyph`, runs the specs, uploads the artifact packs, and writes the comment Markdown. An example workflow that wires it to a sticky PR comment is at [`examples/github/glyphrun-pr.yml`](../examples/github/glyphrun-pr.yml).

Minimal manual usage without the composite action:

```bash
glyph run specs/glyphrun/*.yml --junit glyphrun-junit.xml --format json
glyph comment --last 50 | gh pr comment "$PR" -F -
```
