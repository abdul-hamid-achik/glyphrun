#!/bin/sh
# Shell transform used by examples/specs/transform_artifact.yml.
#
# Reads $GLYPHRUN_INPUT (the captured report from the previous
# download step), uppercases its body with `tr`, and writes the
# result to $GLYPHRUN_OUTPUT. The trailing `printf` emits a JSON
# evidence line on stdout, matching the Node transform's contract.
#
# The runner pre-creates both the script's cwd and the output's
# parent directory, so the script body only needs to write the file.

set -eu

if [ -z "${GLYPHRUN_INPUT:-}" ] || [ -z "${GLYPHRUN_OUTPUT:-}" ]; then
  echo "transform: GLYPHRUN_INPUT and GLYPHRUN_OUTPUT must be set" >&2
  exit 2
fi

tr '[:lower:]' '[:upper:]' < "$GLYPHRUN_INPUT" > "$GLYPHRUN_OUTPUT"

bytes=$(wc -c < "$GLYPHRUN_OUTPUT" | tr -d ' ')
printf '{"ok":true,"evidence":{"input":"%s","output":"%s","bytes":%s}}\n' \
  "$GLYPHRUN_INPUT" "$GLYPHRUN_OUTPUT" "$bytes"
