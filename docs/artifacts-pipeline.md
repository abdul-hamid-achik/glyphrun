# Artifact Pipeline

Glyphrun can capture files a TUI target wrote (`download`), run external scripts that produce new artifacts (`transform`), and queue multiple keystrokes as a single PTY write (`batch`). These are the artifact-pipeline steps — they turn files the target produces into named artifacts that later steps and verifiers can reference.

## `download` — capture a file the target wrote

```yaml
steps:
  - download:
      path: /var/run/myapp/report.txt
      saveAs: report.txt
      assign: report
      waitFor: true
      timeoutMs: 5000
```

`download` captures a file from a known filesystem path into the run artifact directory under `artifacts/<assign>/<saveAs>`. The `path` may use `${vars.*}` and `${env.*}` placeholders (resolved at parse time) and `${artifacts.<name>.path}` placeholders (resolved at run time, after earlier steps have populated their artifacts).

| Field | Description |
| --- | --- |
| `path` | Source path on disk. Placeholders supported. |
| `saveAs` | Filename to write under the artifact dir. |
| `assign` | Named-artifact key for later `${artifacts.<name>.path}` reference. |
| `waitFor` | Wait for the file to appear before capturing. |
| `timeoutMs` | How long to wait for the file. |

## `transform` — produce a new artifact with an external script

```yaml
steps:
  - transform:
      runtime: shell
      file: ./transforms/uppercase.sh
      input: ${artifacts.report.path}
      saveAs: upper.txt
      assign: reportUpper
      timeoutMs: 10000
```

`transform` runs an external script that produces a new named artifact. Supported runtimes: `node` (default: `shell`). The script receives a JSON context on its argv (Node) or via env vars (shell) and writes its output to the path advertised as `output.path`.

| Field | Description |
| --- | --- |
| `runtime` | `shell` (default) or `node`. |
| `file` | Script to run. |
| `input` | Artifact path or value to feed the script. |
| `saveAs` | Output filename. |
| `assign` | Named-artifact key. |
| `fixtures` | Map of fixture values passed to the script. |
| `timeoutMs` | Per-step timeout. |

## `batch` — one PTY write for transient TUI state

```yaml
steps:
  - batch:
      - press: "/"
      - type: "search query"
      - press: "enter"
      - wait:
          screen:
            contains: "results"
```

`batch` concatenates every `press` / `type` / `paste` / `send` sub-step into one `pty.write()` syscall. This preserves transient TUI state — a command palette, a focused menu, a hover popover — that would be lost between separate top-level steps, where the target could repaint between writes. An optional trailing `wait` is the only synchronization point.

## Named artifacts

`download` and `transform` register their outputs as **named artifacts** addressable by later steps via:

- `${artifacts.<name>.path}` — absolute path
- `${artifacts.<name>.relativePath}` — run-relative path

Placeholders are resolved at *runtime*, just before each step runs, so a step can reference artifacts produced by earlier steps in the same spec.

## Run-dir env

The runner injects `$GLYPHRUN_RUN_DIR` into both the target process env and the `command:` verifier env, so shell commands can reference the run's path without re-deriving it.

See [Steps](/steps) for the full step vocabulary and [Artifacts](/artifacts) for the run artifact pack layout.