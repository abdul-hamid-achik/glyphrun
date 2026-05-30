# Glyphrun Overview

Glyphrun is a local-first CLI for terminal and TUI behavior specs. It runs a target command in a PTY, drives it with declarative YAML or JSON steps, evaluates outcomes against a virtual terminal screen, and writes artifact packs for people and coding agents.

Specs can import reusable action snippets with `imports` and `use`, guard optional TUI steps with `when`, and use trusted Bash checks through the `command` verifier.
