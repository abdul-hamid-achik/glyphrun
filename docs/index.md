---
layout: home

title: Glyphrun - Terminal & TUI Testing Framework
titleTemplate: false
description: Black-box behavior tests for terminal apps. Glyphrun drives CLIs and TUIs in a real PTY, asserts against a deterministic terminal emulator, and writes artifact packs built for humans and coding agents.

head:
  - - script
    - type: application/ld+json
    - '{"@context":"https://schema.org","@type":"FAQPage","mainEntity":[{"@type":"Question","name":"Is Glyphrun like Playwright, but for terminal apps?","acceptedAnswer":{"@type":"Answer","text":"Conceptually yes. Glyphrun drives a real process in a PTY the way Playwright drives a real browser, and asserts against a deterministic virtual terminal the way Playwright asserts against the DOM."}},{"@type":"Question","name":"Does Glyphrun work with any language?","acceptedAnswer":{"@type":"Answer","text":"Yes. Glyphrun is black-box: if your app runs in a PTY, Glyphrun can drive and assert against it, regardless of implementation language."}},{"@type":"Question","name":"Does Glyphrun support Windows?","acceptedAnswer":{"@type":"Answer","text":"Yes, via ConPTY on Windows 10 1809+, behind the same platform-neutral backend used for macOS and Linux PTYs."}},{"@type":"Question","name":"How is Glyphrun different from expect or tmux scripts?","acceptedAnswer":{"@type":"Answer","text":"Specs are declarative YAML/JSON with a stamped contract hash, not imperative scripts. Outcomes are separated from repairable interaction steps, and every run produces a structured artifact pack instead of raw terminal output."}}]}'

hero:
  name: glyphrun
  text: Stop eyeballing your terminal app.
  tagline: Specs for CLIs and TUIs. Real PTY, deterministic screen, artifacts an agent can read.
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
---

## A spec is the contract {#example}

Testing a TUI usually means expect scripts, a framework harness, or a human at the keyboard. A Glyphrun spec replaces all three: `intent` and `outcomes` are the contract, `steps` are repairable hints.

<SpecFrame file="hello_quits.yml">

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

</SpecFrame>

```bash
glyph run specs/hello.yml --format md
```

One command launches the app in a real PTY, checks every outcome against the emulated screen, and writes a run directory: JSON/YAML/Markdown reports, the final screen as text and SVG, per-outcome evidence, and `agent_context.md`. Exit 0 means every outcome passed. [Exit codes 1-7](/commands) each mean one distinct kind of failure.

## Drive the PTY. Assert the screen. {#how-it-works}

<ol class="gr-beats">
<li>
<span class="n" aria-hidden="true">1</span>
<div>
<p><strong>Stamp the contract.</strong> Write <code>intent</code> and <code>outcomes</code>. <code>glyph spec verify --stamp</code> seals them with a <a href="/contract-hash">contract hash</a>. Silent edits abort the run.</p>
</div>
</li>
<li>
<span class="n" aria-hidden="true">2</span>
<div>
<p><strong>Run it in a real PTY.</strong> Glyphrun launches your app, plays the steps, and evaluates each outcome against a deterministic virtual terminal: cells, regions, cursor, colors, OSC 8 hyperlinks. See the <a href="/steps">step</a> and <a href="/verifiers">verifier</a> vocabulary.</p>
</div>
</li>
<li>
<span class="n" aria-hidden="true">3</span>
<div>
<p><strong>Read the evidence.</strong> Pass or fail, you get a self-contained <a href="/artifacts">artifact pack</a>. On failure, <code>glyph context latest</code> shows what happened, and <code>glyph repair</code> proposes step fixes without touching what pass means.</p>
</div>
</li>
</ol>

## Agents see what you see {#agents}

Agents cannot see a TUI. Glyphrun can, on the same CLI humans use. No per-agent code paths: `glyph mcp` starts a stdio [MCP server](/mcp) that mirrors the commands. After a failure, `agent_context.md` hands the agent recent events and inspection commands.

The contract hash refuses silent edits to `intent` or `outcomes`, so an agent can repair drifted steps without redefining success:

```bash
glyph run specs/app.yml --format json   # banner changed
glyph context latest --format md
glyph repair specs/app.yml --write      # steps only
glyph run specs/app.yml --format json   # green
```

The full loop is in the [agent guide](/agents).

## Stories are isolated TUI states {#stories}

`glyph stories` catalogs specs that mount one TUI state at a time. `--html` inspects cells (grid, rulers, hover). `--tui` is the feel catalog in the host terminal. Same black-box runner, no framework bindings.

The [Stories guide](/stories) covers `glyph stories init`, the inspect overlays, and the example harness.

## FAQ {#faq}

<dl class="gr-faq">
<dt>Is this like Playwright, but for terminal apps?</dt>
<dd>Conceptually yes. Glyphrun drives a real process in a PTY the way Playwright drives a real browser, and asserts against a deterministic virtual terminal the way Playwright asserts against the DOM.</dd>
<dt>Does it work with any language?</dt>
<dd>Yes. Glyphrun is black-box: if your app runs in a PTY, Glyphrun can drive and assert against it, regardless of implementation language.</dd>
<dt>Does it support Windows?</dt>
<dd>Yes, via ConPTY (Windows 10 1809+), behind the same platform-neutral backend used for macOS and Linux PTYs.</dd>
<dt>How is this different from expect or tmux scripts?</dt>
<dd>Specs are declarative YAML/JSON with a stamped contract hash, not imperative scripts. Outcomes are separated from repairable interaction steps, and every run produces a structured artifact pack instead of raw terminal output. <code>glyph import bats</code> converts an existing BATS file; <code>glyph export bats</code> goes the other way.</dd>
</dl>

## Install {#install}

```bash
brew install abdul-hamid-achik/tap/glyph
# or
go install github.com/abdul-hamid-achik/glyphrun/cmd/glyph@latest
```

MIT licensed. `glyph init` writes a passing smoke spec. The [Quickstart](/quickstart) walks through it.
