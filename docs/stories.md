---
description: Catalog isolated TUI states as Glyphrun stories — a Go harness, HTML inspect overlays, and glyph stories --tui.
---

# Stories

Glyphrun stories are regular specs, usually tagged `story`, whose `target.cmd` mounts an isolated TUI state (a story harness binary). Glyphrun stays black-box: it does not import Bubble Tea into the runner.

`glyph stories init --lang go` writes a Bubble Tea v2 harness under `stories/` and a stamped spec under `specs/stories/`. `glyph spec scaffold --kind story` prints only the YAML.

`glyph stories` lists those specs joined to the newest run.

- `glyph stories --html` is the inspect surface: a self-contained page (Alpine.js + Tailwind-shaped utility CSS, no CDN, no CSS toolchain) with terminal chrome, grid/rulers/spaces overlays, and a cell hover inspector (x, y, char, style).
- `glyph stories --tui` is the feel surface: a two-pane catalog in the host terminal (feature-grouped sidebar + preview) that fills the window. `j`/`k` stories, `[`/`]` snapshots, `s` spaces. The HTML `grid` overlay is SVG-only.

`glyph render --grid --rulers --spaces` inspects one snapshot as SVG. Default `screens/final.svg` from `glyph run` does not include overlays, so CI screenshots stay unchanged.

The repo ships a Bubble Tea harness under [`examples/stories/`](../examples/stories/) built from shared components. Feature `list` has empty / rows / error; feature `agent` is a chat session (`empty`, `messages`, `streaming`, `tool`, `error`). Specs live in `examples/specs/story_*.yml`.

`glyph stories` joins each spec to its newest run. Raise `retention.keepRuns` so a batch of stories is not pruned down to the last few (this repo uses 32).

```bash
task example:stories
glyph stories examples/specs --html --out /tmp/glyph-stories.html
glyph stories examples/specs --tui
```

