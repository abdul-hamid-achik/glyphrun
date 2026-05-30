package docs

import "sort"

var byTopic = map[string]string{
	"overview": `# Glyphrun Overview

Glyphrun runs YAML or JSON terminal behavior specs against a target command in a real PTY. Assertions read a virtual terminal screen, not raw ANSI bytes.

Start with ` + "`glyph agent --format md`" + ` for the agent workflow, or ` + "`glyph docs topics --format md`" + ` to list focused docs.
`,
	"quickstart": `# Quickstart

1. Run ` + "`glyph doctor --format md`" + `.
2. Create a spec with ` + "`glyph spec scaffold > specs/smoke.yml`" + `.
3. Validate it with ` + "`glyph spec verify specs/smoke.yml --format json`" + `.
4. Run it with ` + "`glyph run specs/smoke.yml --format md --progress auto`" + `.
5. Inspect failures with ` + "`glyph context latest --format md`" + `.
`,
	"authoring": `# Authoring

Separate behavior contracts from repairable steps. Keep user intent in ` + "`intent`" + `, stable expectations in ` + "`outcomes`" + `, and navigation/input hints in ` + "`steps`" + `.

Run ` + "`glyph spec verify <spec> --format json`" + ` before running a spec. Use ` + "`glyph spec verify <spec> --stamp`" + ` only when the expected behavior intentionally changed.

Good specs assert user-visible behavior. Avoid coupling outcomes to implementation details, timing artifacts, or raw ANSI bytes.
`,
	"snippets": `# Reusable Actions

Create reusable terminal step snippets with ` + "`glyph spec scaffold --kind action`" + `. Import them from specs with ` + "`imports`" + ` and call them with ` + "`use`" + `.

Use ` + "`when`" + ` on a step to run it only when a verifier is currently true. This is useful for optional TUI prompts, warnings, login walls, and other state that may or may not appear.

Use trusted ` + "`command`" + ` verifiers for Bash checks such as ` + "`test -x ./bin/app`" + `.
`,
	"steps": `# Steps

Supported v1 steps: ` + "`press`" + `, ` + "`type`" + `, ` + "`paste`" + `, ` + "`send`" + `, ` + "`wait`" + `, ` + "`resize`" + `, ` + "`snapshot`" + `, and imported ` + "`use`" + ` actions.

Every step can include a ` + "`when`" + ` guard that uses the same verifier shape as an outcome. Prefer ` + "`wait`" + ` steps that synchronize on visible screen or process state. Use ` + "`snapshot`" + ` to capture named terminal states in the artifact pack.

` + "`paste`" + ` sends bracketed paste delimiters only after the target enables terminal mode ` + "`?2004`" + `; otherwise it writes literal text.
`,
	"verifiers": `# Verifiers

Supported v1 verifiers: ` + "`screen`" + `, ` + "`region`" + `, ` + "`cell`" + `, ` + "`cursor`" + `, ` + "`process`" + `, ` + "`snapshot`" + `, and trusted ` + "`command`" + `.

Screen verifiers support ` + "`contains`" + `, ` + "`notContains`" + `, and ` + "`regex`" + `. Cell verifiers can check characters and style attributes. Process verifiers can check exit state and exit code.

Outcomes can set ` + "`timeoutMs`" + ` and ` + "`normalize`" + ` when a single assertion needs longer polling or custom volatile-text cleanup.
`,
	"artifacts": `# Artifacts

Each run writes ` + "`run.json`" + `, ` + "`run.yaml`" + `, ` + "`run.md`" + `, ` + "`agent_context.md`" + `, ` + "`events.ndjson`" + `, ` + "`spec.resolved.yml`" + `, final screens, frames, raw logs, snapshots, outcomes, and diagnostics.

Start with ` + "`run.md`" + ` for a human summary, ` + "`run.json`" + ` for automation, ` + "`agent_context.md`" + ` for agent debugging, ` + "`diagnostics/environment.md`" + ` for runtime context, and ` + "`screens/final.txt`" + ` for the normalized terminal state.
`,
	"agents": `# Agents

Call ` + "`glyph agent --format md`" + ` or ` + "`glyph explain --format json`" + ` before editing specs.

Recommended loop:

1. ` + "`glyph spec verify <spec> --format json`" + `
2. ` + "`glyph run <spec> --format json`" + `
3. ` + "`glyph context latest --format md`" + ` after a failure
4. inspect ` + "`diagnostics/failure.md`" + `, ` + "`screens/final.txt`" + `, and ` + "`frames/frames.ndjson`" + `

Do not edit ` + "`intent`" + ` or ` + "`outcomes`" + ` without surfacing the contract change. Repair ` + "`steps`" + ` when the route through the terminal UI changed.
`,
	"mcp": `# MCP

Run ` + "`glyph mcp`" + ` to start the stdio MCP server. The current server exposes tools for explain, docs, doctor, spec verification, spec scaffolding, runs, snapshot updates, diffs, and context lookup.
`,
	"configuration": `# Configuration

Glyphrun reads ` + "`glyphrun.config.yml`" + ` by walking up from the spec path. Defaults include ` + "`.glyphrun/runs`" + ` artifacts and an xterm-256color terminal profile.

Use config for shared terminal defaults, artifact behavior, variables, and redaction rules. Use ` + "`glyph doctor --format md`" + ` to confirm the active config and artifact root.

` + "`target.timeoutMs`" + ` wraps the whole target session after the PTY starts and exits with code ` + "`3`" + ` when it expires.
`,
	"troubleshooting": `# Troubleshooting

Use ` + "`glyph context latest --format md`" + ` after a failure. Inspect ` + "`screens/final.txt`" + `, ` + "`raw/pty.raw.log`" + `, ` + "`frames/frames.ndjson`" + `, and ` + "`diagnostics/failure.md`" + `.

Use ` + "`glyph run <spec> --format md --progress always`" + ` for live step/outcome progress during long TUI runs. Progress is written to stderr.
`,
	"topics": `# Docs Topics

- overview
- quickstart
- authoring
- snippets
- steps
- verifiers
- artifacts
- agents
- mcp
- configuration
- troubleshooting
- topics
`,
}

func Content(topic string) (string, bool) {
	content, ok := byTopic[topic]
	return content, ok
}

func Topics() []string {
	topics := make([]string, 0, len(byTopic))
	for topic := range byTopic {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	return topics
}
