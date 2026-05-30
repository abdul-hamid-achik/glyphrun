# Configuration

Glyphrun discovers `glyphrun.config.yml` by walking up from the spec path. The default artifact root is `.glyphrun/runs`.

Example:

```yaml
version: 1
artifactRoot: .glyphrun/runs
snapshotRoot: .glyphrun/snapshots

terminal:
  cols: 120
  rows: 36
  profile: xterm-256color
  normalize:
    trimRight: true
    normalizeLineEndings: true

artifacts:
  rawLog: true
  frames: true
  finalScreen: true
  snapshots: true
  agentContext: true
```

Specs can override target, terminal, artifact, and environment settings locally. Keep secrets out of config and specs; pass them through the environment or setup commands instead.

Use `glyph doctor --format md` to confirm the active config, artifact root, schema source, and PTY availability.
