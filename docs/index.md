---
layout: home

hero:
  name: Glyphrun
  text: Local-first terminal behavior spec runner
  tagline: Specs declare intent + outcomes as the behavior contract and steps as repairable hints. One tool drives your TUIs from a real PTY.
  actions:
    - theme: brand
      text: Quickstart
      link: /quickstart
    - theme: alt
      text: Authoring Guide
      link: /authoring
    - theme: alt
      text: View on GitHub
      link: https://github.com/abdul-hamid-achik/glyphrun

features:
  - title: intent + outcomes contract
    details: The behavior contract is the durable thing. Steps are repairable hints agents and humans can rewrite without changing what “success” means.
  - title: Real PTY, virtual emulator
    details: Runs your TUI in a real PTY but evaluates screens against a deterministic virtual emulator — so the same spec reproduces byte-for-byte.
  - title: Step types that compose
    details: open, wait, assert, type, keypress, key, run, request, screenshot, batch, controls, script, checkpoint, capture, paste, eval, scroll, hover, click.
  - title: Verifier vocabulary
    details: text, notText, url, network, noFailedRequests, console, count, xlsx, file, and script — typed, schema-validated, never invented on the fly.
  - title: Artifact packs for agents
    details: run.{json,yaml,md}, report.html, agent_context.md, outcomes/*.md, frames/frames.ndjson, screens/, raw/, events.ndjson.
  - title: Repair & flaky detection
    details: Failed runs are analyzed and proposed step rewrites are emitted; flaky detection tracks divergence across repeated runs.
---
