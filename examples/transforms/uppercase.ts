// Transform script for examples/specs/transform_artifact.yml.
//
// Reads $GLYPHRUN_INPUT (a report file written by the reporter app),
// uppercases the body, and writes the result to $GLYPHRUN_OUTPUT. The
// fixtures bag is exposed via $GLYPHRUN_FIXTURES_JSON (Node only — shell
// scripts can also use it if they parse the JSON).
//
// The `assign: report` step that follows this transform references the
// output path via ${artifacts.<assign>.path} from a Node verifier (the
// shell equivalent lives in the spec's `command:` outcome).

import { readFile, writeFile } from "node:fs/promises";

const inputPath = process.env["GLYPHRUN_INPUT"];
const outputPath = process.env["GLYPHRUN_OUTPUT"];

if (!inputPath || !outputPath) {
  console.error("transform: GLYPHRUN_INPUT and GLYPHRUN_OUTPUT must be set");
  process.exit(2);
}

const body = await readFile(inputPath, "utf8");
const upper = body.toUpperCase();
await writeFile(outputPath, upper, "utf8");

console.log(JSON.stringify({ ok: true, evidence: { input: inputPath, output: outputPath, bytes: upper.length } }));
