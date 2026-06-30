# Overview

Glyphrun is a local-first CLI for terminal and TUI behavior specs. It runs a target command in a PTY, drives it with declarative YAML or JSON steps, evaluates outcomes against a virtual terminal screen, and writes artifact packs for people and coding agents.

Specs can import reusable action snippets with `imports` and `use`, guard optional TUI steps with `when`, and use trusted Bash checks through the `command` verifier. The same spec can run from the CLI, through the MCP server, or — when the contract hashes agree — be replayed to the byte against a recorded session.

The "intent + outcomes" pair is the behavior contract. The `steps` are repairable hints, not the contract. When the contract changes, the agent re-runs the spec, finds the steps that no longer match, and proposes rewrites — without ever editing `intent` or `outcomes` silently.

## Why a behavior runner for terminals

TUIs lack the affordances of GUI test frameworks. There is no DOM, no accessibility tree, no screenshot-to-element diff, no locator service. Glyphrun fills that gap with a virtual terminal emulator that knows about cells, regions, SGR styles, OSC 8 hyperlinks, mouse input, and the `gote`-backed screen model. The same spec reproduces byte-for-byte across runs because the emulator is deterministic — independent of the user's terminal, theme, or scrollback.

This determinism is what makes specs sharable. A spec an agent writes on one machine plays back the same way on another, in CI, or in your agent's session.

## Where to go next

- [Quickstart](/quickstart) — install `glyph` and run your first spec.
- The full authoring guide, step vocabulary, and verifier reference are under way and will land in subsequent releases.
