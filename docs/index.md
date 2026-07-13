---
layout: home

title: Glyphrun — Terminal & TUI Testing Framework
titleTemplate: false
description: Black-box behavior tests for terminal apps. Glyphrun drives CLIs and TUIs in a real PTY, asserts against a deterministic terminal emulator, and writes artifact packs built for humans and coding agents.

head:
  - - script
    - type: application/ld+json
    - '{"@context":"https://schema.org","@type":"FAQPage","mainEntity":[{"@type":"Question","name":"Is Glyphrun like Playwright, but for terminal apps?","acceptedAnswer":{"@type":"Answer","text":"Conceptually yes — Glyphrun drives a real process in a PTY the way Playwright drives a real browser, and asserts against a deterministic virtual terminal the way Playwright asserts against the DOM."}},{"@type":"Question","name":"Does Glyphrun work with any language?","acceptedAnswer":{"@type":"Answer","text":"Yes. Glyphrun is black-box: if your app runs in a PTY, Glyphrun can drive and assert against it, regardless of implementation language."}},{"@type":"Question","name":"Does Glyphrun support Windows?","acceptedAnswer":{"@type":"Answer","text":"Yes, via ConPTY on Windows 10 1809+, behind the same platform-neutral backend used for macOS and Linux PTYs."}},{"@type":"Question","name":"How is Glyphrun different from expect or tmux scripts?","acceptedAnswer":{"@type":"Answer","text":"Specs are declarative YAML/JSON with a stamped contract hash, not imperative scripts — outcomes are separated from repairable interaction steps, and every run produces a structured artifact pack instead of raw terminal output."}}]}'

hero:
  name: glyphrun
  text: Stop eyeballing your terminal app.
  tagline: Glyphrun runs your CLI or TUI in a real PTY and checks the rendered screen against a deterministic terminal emulator. Any language, no framework bindings — if it runs in a terminal, you can test it.
  image:
    src: /hero-terminal.svg
    alt: glyph run output showing 2 of 2 outcomes passed
  actions:
    - theme: brand
      text: Get started in 5 minutes
      link: /quickstart
    - theme: alt
      text: View on GitHub
      link: https://github.com/abdul-hamid-achik/glyphrun

features:
  - title: Test any terminal app
    details: Glyphrun is black-box — if it runs in a PTY, you can spec it. Go, Rust, Python, Bash, anything. Specs describe user-visible behavior in YAML, not your framework's internals.
  - title: Same screen, every run
    details: Input goes through a real pseudo-terminal; assertions run against a built-in deterministic terminal emulator. A spec that passes on your laptop renders byte-for-byte the same in CI or an agent's session.
  - title: Specs that survive UI drift
    details: intent and outcomes are stamped with a contract hash — silent edits abort the run. When a banner is renamed or a prompt moves, glyph repair fixes the navigation steps and never touches what "pass" means.
  - title: Failures your agent can read
    details: Every run writes a self-contained artifact pack — JSON/YAML/Markdown reports, agent_context.md with suggested next commands, a frame-by-frame timeline, and a deterministic SVG of the final screen. glyph mcp exposes it all to any MCP client.
  - title: Start from a recording
    details: Skip blank-page authoring. glyph record --scaffold watches a real session and drafts a runnable spec — target command, terminal size, ready-string, and a clean-exit outcome, hash already stamped.
  - title: Green means green in CI
    details: --repeat N probes for flakiness before you trust a suite, --junit feeds any CI dashboard, and glyph comment posts a PR summary with the final screen. A composite GitHub Action ships in the repo.
---

## A terminal test in ten lines {#example}

Testing terminal applications usually means expect scripts, framework-specific harnesses, or a human keyboard-mashing before every release. A Glyphrun spec replaces all three:

```yaml
name: hello_quits
intent: a user can open the app and quit with q.
target: { cmd: ["./bin/app"] }
steps:
  - wait: { screen: { contains: "hello" } }
  - press: "q"
outcomes:
  - id: clean_exit
    description: q exits the application cleanly
    verify: { process: { exitCode: 0 } }
```

```bash
glyph run specs/hello.yml --format md
```

One command launches the app in a real PTY, evaluates every outcome against the emulated screen, and writes a run directory containing the report in JSON, YAML, and Markdown, the final screen as text and SVG, per-outcome evidence, and `agent_context.md`. Exit 0 means every outcome passed — [exit codes 1–7](/commands) each mean one distinct kind of failure.

## How Glyphrun tests a TUI {#how-it-works}

1. **Declare the contract.** Write `intent` and `outcomes` — the durable definition of correct behavior. `glyph spec verify --stamp` seals them with a [contract hash](/contract-hash).
2. **Run it for real.** Glyphrun launches your app in a genuine pseudo-terminal, plays the steps, and evaluates each outcome against a deterministic virtual terminal — cells, regions, cursor, colors, even OSC 8 hyperlinks. See the full [step](/steps) and [verifier](/verifiers) vocabulary.
3. **Read the evidence.** Pass or fail, you get a self-contained [artifact pack](/artifacts). On failure, `glyph context latest` surfaces exactly what went wrong, and `glyph repair` proposes step fixes.

## Your coding agent's eyes in the terminal {#agents}

Agents can't see a TUI — Glyphrun can. It was designed so agents use the same surface humans do, with no per-agent code paths: `glyph mcp` starts a stdio [MCP server](/mcp) that mirrors the CLI — run specs, verify contracts, read failure context, diff runs. After a failure, `agent_context.md` hands the agent recent events and suggested inspection commands.

And because the contract hash refuses silent edits to `intent` or `outcomes`, an agent can repair drifted steps all day without ever redefining success behind your back:

```bash
glyph run specs/app.yml --format json   # fails: the banner text changed
glyph context latest --format md        # read what actually happened
glyph repair specs/app.yml --write      # fix the steps, never the contract
glyph run specs/app.yml --format json   # green — contract untouched
```

The full loop is documented in the [agent guide](/agents).

## Coming from expect scripts or BATS? {#migrate}

Most terminal testing either binds to your app's internals or lives in fragile expect scripts. Glyphrun keeps the app black-box and the assertion deterministic — and it meets you where you are: `glyph import bats` converts an existing BATS file into a spec, and `glyph export bats` goes the other way. Local-first by design: no cloud, no telemetry, one static Go binary on macOS, Linux, and Windows (ConPTY).

## FAQ {#faq}

**Is this like Playwright, but for terminal apps?**
Conceptually yes — Glyphrun drives a real process (PTY) the same way Playwright drives a real browser, and asserts against a deterministic virtual terminal the way Playwright asserts against the DOM.

**Does it work with any language?**
Yes. Glyphrun is black-box: if your app runs in a PTY, Glyphrun can drive and assert against it, regardless of implementation language.

**Does it support Windows?**
Yes, via ConPTY (Windows 10 1809+), behind the same platform-neutral backend used for macOS and Linux PTYs.

**How is this different from `expect` or `tmux` scripts?**
Specs are declarative YAML/JSON with a stamped contract hash, not imperative scripts — outcomes are separated from the repairable interaction steps, and every run produces a structured artifact pack instead of raw terminal output.

## Install {#install}

```bash
brew install abdul-hamid-achik/tap/glyph
# or
go install github.com/abdul-hamid-achik/glyphrun/cmd/glyph@latest
```

MIT licensed. Run `glyph init` in your project and you'll have a passing smoke spec in five minutes — the [Quickstart](/quickstart) walks you through it.
