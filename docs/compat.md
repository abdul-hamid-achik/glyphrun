---
description: Compatibility matrix for TUI kits Glyphrun is intended to drive.
---

# Compatibility

Glyphrun is black-box: if the target runs in a PTY, a spec can drive it. The table is the current support bar, not a guarantee for every app.

| Kit / surface | Status | Notes |
|---|---|---|
| Charm Bubble Tea | supported | Unix PTY is put in raw mode and resized after start to avoid Linux stdin-EOF shutdowns |
| Charm Lip Gloss / Huh | supported | Screen text + SGR assertions |
| Ratatui / crossterm | supported | Mouse steps need the target to enable 1000/1006 |
| Textual / Rich | supported | Prefer `wait.idle` around animations |
| Windows ConPTY | supported | Windows 10 1809+ |
| OSC 8 hyperlinks | supported | `link` verifier |
| Sixel / Kitty graphics | consumed | Sequences are eaten, not rendered |

Use `glyph doctor --format json` on the machine that will run CI. Use `glyph run --repeat 20` before trusting a new kit.
