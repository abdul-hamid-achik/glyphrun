# Example stories harness

A Bubble Tea v2 app built from shared components (header, transcript,
composer, frame). Glyphrun drives each story as a black-box `target.cmd`;
it does not import this package.

```
examples/stories/                 harness source (components + story states)
examples/apps/stories/stories     built binary (gitignored)
examples/specs/story_*.yml        specs tagged `story`
```

| story id          | spec                          | screen                                      |
|-------------------|-------------------------------|---------------------------------------------|
| `list/empty`      | `story_list_empty.yml`        | Inbox, no rows                              |
| `list/rows`       | `story_list_rows.yml`         | hello / drafts / sent                       |
| `list/error`      | `story_list_error.yml`        | failed to load                              |
| `agent/empty`     | `story_agent_empty.yml`       | empty session, focused composer             |
| `agent/messages`  | `story_agent_messages.yml`    | you / glyph measuring a cell                |
| `agent/streaming` | `story_agent_streaming.yml`   | partial reply, status streaming             |
| `agent/tool`      | `story_agent_tool.yml`        | glyph run in flight                         |
| `agent/error`     | `story_agent_error.yml`       | PTY closed before ready                     |

From the repo root:

```bash
task example:stories

./bin/glyph stories examples/specs --html --out /tmp/glyph-stories.html
./bin/glyph stories examples/specs --tui
```

`task example:stories` builds `./bin/glyph`, runs every story spec (each spec
rebuilds the harness as a precondition), and prints the catalog. Open the HTML
file to inspect cells, regions, and spacing; `--tui` paints the same snapshots
in the terminal (`j`/`k` stories, `[`/`]` snapshots, `s` spaces).
