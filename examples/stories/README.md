# Example stories harness

A Bubble Tea v2 app built from shared components (header, transcript,
composer, frame). Glyphrun drives each story as a black-box `target.cmd`;
it does not import this package.

```
examples/stories/                 harness source (components + story states)
examples/apps/stories/stories     built binary (gitignored)
examples/stories.yml              manifest: harness + 8 stories (kind: stories)
```

The stories are entries in [`examples/stories.yml`](../stories.yml), not
individual spec files — `glyph stories run` expands each one into a spec in
memory.

| story id          | feature | screen                                      |
|-------------------|---------|----------------------------------------------|
| `list/empty`      | list    | Inbox, no rows                              |
| `list/rows`       | list    | hello / drafts / sent (variant: `wide`)     |
| `list/error`      | list    | failed to load                              |
| `agent/empty`     | agent   | empty session, focused composer             |
| `agent/messages`  | agent   | you / glyph measuring a cell                |
| `agent/streaming` | agent   | partial reply, status streaming             |
| `agent/tool`      | agent   | glyph run in flight                         |
| `agent/error`     | agent   | PTY closed before ready                     |

From the repo root:

```bash
task example:stories

./bin/glyph stories run examples
./bin/glyph stories examples --html --out /tmp/glyph-stories.html
./bin/glyph stories examples --tui
./bin/glyph stories serve examples --watch
```

`task example:stories` builds `./bin/glyph`, runs `glyph stories run
examples/stories.yml` (which builds the harness once as a precondition and
runs every story), and prints the catalog. Open the HTML file to inspect
cells, regions, and spacing; `--tui` paints the same snapshots in the
terminal (`j`/`k` stories, `[`/`]` snapshots, `s` spaces, `d` diff, `o`
golden, `r` rulers); `stories serve --watch` serves the same catalog live and
rebuilds/reruns on source changes.

See also [`examples/stories-sh/`](../stories-sh/) for a POSIX-shell harness
that needs no Go toolchain (`task example:stories:sh`).
