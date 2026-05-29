# Security

Glyphrun specs are trusted code. A spec can launch arbitrary local commands through `target.cmd`, `preconditions.commands`, and `verify.command`.

## Rules

- Do not run specs from untrusted sources.
- Glyphrun does not fetch or execute remote specs by URL.
- Artifacts redact common secret patterns before writing text, JSON, YAML, and logs.
- Full environment dumps are avoided. Environment diagnostics are allowlisted.
- Raw PTY logs are for local diagnostics and should not be published without review.

Report security concerns privately to the repository owner.

