package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	glyphdocs "github.com/abdul-hamid-achik/glyphrun/internal/docs"
	"github.com/abdul-hamid-achik/glyphrun/internal/runner"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

type ServerOptions struct {
	ConfigPath   string
	ArtifactRoot string
	Environment  string
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Result  any            `json:"result,omitempty"`
	Error   *responseError `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func Serve(ctx context.Context, in io.Reader, out io.Writer, opts ServerOptions) error {
	reader := bufio.NewReader(in)
	for {
		payload, err := readMessage(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		var req request
		if err := json.Unmarshal(payload, &req); err != nil {
			if err := writeResponse(out, response{JSONRPC: "2.0", Error: &responseError{Code: -32700, Message: err.Error()}}); err != nil {
				return err
			}
			continue
		}
		if req.ID == nil && strings.HasPrefix(req.Method, "notifications/") {
			continue
		}
		result, rpcErr := handle(ctx, req, opts)
		resp := response{JSONRPC: "2.0", ID: req.ID, Result: result}
		if rpcErr != nil {
			resp.Result = nil
			resp.Error = rpcErr
		}
		if err := writeResponse(out, resp); err != nil {
			return err
		}
	}
}

func handle(ctx context.Context, req request, opts ServerOptions) (any, *responseError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "glyphrun",
				"version": "0.1.0",
			},
		}, nil
	case "tools/list":
		return map[string]any{"tools": tools()}, nil
	case "tools/call":
		var params toolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &responseError{Code: -32602, Message: err.Error()}
		}
		return callTool(ctx, params, opts)
	default:
		return nil, &responseError{Code: -32601, Message: "method not found: " + req.Method}
	}
}

func tools() []map[string]any {
	return []map[string]any{
		tool("glyph_explain", "Describe Glyphrun commands, steps, verifiers, and artifacts.", map[string]any{"type": "object", "properties": map[string]any{}}),
		tool("glyph_docs", "Return focused Glyphrun documentation.", map[string]any{
			"type":       "object",
			"properties": map[string]any{"topic": map[string]any{"type": "string"}},
		}),
		tool("glyph_doctor", "Check local Glyphrun prerequisites.", map[string]any{"type": "object", "properties": map[string]any{}}),
		tool("glyph_spec_verify", "Validate a Glyphrun spec.", map[string]any{
			"type":       "object",
			"required":   []string{"path"},
			"properties": map[string]any{"path": map[string]any{"type": "string"}},
		}),
		tool("glyph_run", "Run a Glyphrun spec.", map[string]any{
			"type":       "object",
			"required":   []string{"path"},
			"properties": map[string]any{"path": map[string]any{"type": "string"}, "updateSnapshots": map[string]any{"type": "boolean"}},
		}),
		tool("glyph_context", "Return agent_context.md for a run or latest.", map[string]any{
			"type":       "object",
			"properties": map[string]any{"run": map[string]any{"type": "string"}},
		}),
		tool("glyph_snapshot_update", "Run a spec and update committed snapshots.", map[string]any{
			"type":       "object",
			"required":   []string{"path"},
			"properties": map[string]any{"path": map[string]any{"type": "string"}},
		}),
		tool("glyph_diff", "Compare two Glyphrun artifact packs.", map[string]any{
			"type":       "object",
			"required":   []string{"runA", "runB"},
			"properties": map[string]any{"runA": map[string]any{"type": "string"}, "runB": map[string]any{"type": "string"}},
		}),
		tool("glyph_spec_scaffold", "Return a starter Glyphrun spec or reusable action.", map[string]any{
			"type":       "object",
			"properties": map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"spec", "action"}}},
		}),
	}
}

func tool(name string, description string, inputSchema map[string]any) map[string]any {
	return map[string]any{"name": name, "description": description, "inputSchema": inputSchema}
}

func callTool(ctx context.Context, params toolCallParams, opts ServerOptions) (any, *responseError) {
	switch params.Name {
	case "glyph_explain":
		return toolText(map[string]any{
			"project":   "glyphrun",
			"binary":    "glyph",
			"commands":  []string{"init", "run", "spec verify", "spec scaffold", "spec scaffold --kind action", "snapshot update", "diff", "context", "docs", "agent", "explain", "doctor", "mcp"},
			"steps":     []string{"press", "type", "paste", "send", "wait", "resize", "snapshot", "use", "when"},
			"verifiers": []string{"screen", "region", "cell", "cursor", "process", "snapshot", "command"},
			"progress":  []string{"auto", "always", "never"},
		})
	case "glyph_docs":
		topic := stringArg(params.Arguments, "topic", "overview")
		return toolText(map[string]any{"topic": topic, "content": docs(topic)})
	case "glyph_doctor":
		rt, err := config.LoadRuntime(".", opts.ConfigPath, opts.Environment)
		if err != nil {
			return toolError(err)
		}
		return toolText(map[string]any{"ok": true, "config": rt.ConfigPath, "artifactRoot": rt.Config.ArtifactRoot})
	case "glyph_spec_verify":
		path := stringArg(params.Arguments, "path", "")
		if path == "" {
			return nil, &responseError{Code: -32602, Message: "path is required"}
		}
		rt, err := config.LoadRuntime(path, opts.ConfigPath, opts.Environment)
		if err != nil {
			return toolError(err)
		}
		parsed, err := spec.ParseFile(path, rt.SpecParseOptions())
		if err != nil {
			return toolError(err)
		}
		return toolText(map[string]any{"valid": true, "name": parsed.Spec.Name, "contractHash": parsed.ContractHash, "steps": len(parsed.Resolved.Steps), "outcomes": len(parsed.Resolved.Outcomes)})
	case "glyph_run":
		path := stringArg(params.Arguments, "path", "")
		if path == "" {
			return nil, &responseError{Code: -32602, Message: "path is required"}
		}
		result, err := runner.RunSpec(ctx, runner.Options{
			SpecPath:        path,
			ConfigPath:      opts.ConfigPath,
			Environment:     opts.Environment,
			ArtifactRoot:    opts.ArtifactRoot,
			UpdateSnapshots: boolArg(params.Arguments, "updateSnapshots", false),
		})
		if err != nil {
			return toolError(err)
		}
		return toolText(result)
	case "glyph_context":
		run := stringArg(params.Arguments, "run", "latest")
		content, err := contextContent(run, opts)
		if err != nil {
			return toolError(err)
		}
		return map[string]any{"content": []map[string]string{{"type": "text", "text": content}}}, nil
	case "glyph_snapshot_update":
		path := stringArg(params.Arguments, "path", "")
		if path == "" {
			return nil, &responseError{Code: -32602, Message: "path is required"}
		}
		result, err := runner.RunSpec(ctx, runner.Options{
			SpecPath:        path,
			ConfigPath:      opts.ConfigPath,
			Environment:     opts.Environment,
			ArtifactRoot:    opts.ArtifactRoot,
			UpdateSnapshots: true,
		})
		if err != nil {
			return toolError(err)
		}
		return toolText(result)
	case "glyph_diff":
		runA := stringArg(params.Arguments, "runA", "")
		runB := stringArg(params.Arguments, "runB", "")
		if runA == "" || runB == "" {
			return nil, &responseError{Code: -32602, Message: "runA and runB are required"}
		}
		rt, err := config.LoadRuntime(".", opts.ConfigPath, opts.Environment)
		if err != nil {
			return toolError(err)
		}
		root := opts.ArtifactRoot
		if root == "" {
			root = rt.Config.ArtifactRoot
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(rt.ProjectRoot, root)
		}
		runADir, err := resolveRunDir(root, runA)
		if err != nil {
			return toolError(err)
		}
		runBDir, err := resolveRunDir(root, runB)
		if err != nil {
			return toolError(err)
		}
		diff, err := artifacts.DiffRunDirs(runADir, runBDir)
		if err != nil {
			return toolError(err)
		}
		return toolText(diff)
	case "glyph_spec_scaffold":
		return map[string]any{"content": []map[string]string{{"type": "text", "text": scaffoldSpec(stringArg(params.Arguments, "kind", "spec"))}}}, nil
	default:
		return nil, &responseError{Code: -32602, Message: "unknown tool: " + params.Name}
	}
}

func scaffoldSpec(kind string) string {
	if kind == "action" {
		return `version: 1
name: wait_for_ready_and_quit

steps:
  - wait:
      screen:
        contains: "ready"
      timeoutMs: 5000
  - snapshot: ready
  - press: "q"
  - wait:
      process:
        exitCode: 0
      timeoutMs: 3000
`
	}
	return `version: 1
name: hello_quits

intent: |
  a user can open the app and quit cleanly.

target:
  cmd: ["./bin/app"]
  cwd: "."

terminal:
  cols: 80
  rows: 24
  profile: xterm-256color

steps:
  - wait:
      screen:
        contains: "ready"
  - press: "q"
  - wait:
      process:
        exitCode: 0

outcomes:
  - id: ready_visible
    description: the app renders its ready state
    verify:
      screen:
        contains: "ready"
`
}

func toolText(value any) (any, *responseError) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return toolError(err)
	}
	return map[string]any{"content": []map[string]string{{"type": "text", "text": string(data)}}}, nil
}

func toolError(err error) (any, *responseError) {
	return map[string]any{"content": []map[string]string{{"type": "text", "text": err.Error()}}, "isError": true}, nil
}

func docs(topic string) string {
	if content, ok := glyphdocs.Content(topic); ok {
		return content
	}
	return "unknown topic: " + topic
}

func contextContent(run string, opts ServerOptions) (string, error) {
	rt, err := config.LoadRuntime(".", opts.ConfigPath, opts.Environment)
	if err != nil {
		return "", err
	}
	root := opts.ArtifactRoot
	if root == "" {
		root = rt.Config.ArtifactRoot
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(rt.ProjectRoot, root)
	}
	runDir, err := resolveContextRunDir(root, run)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(runDir, "agent_context.md"))
	return string(data), err
}

func resolveContextRunDir(root string, run string) (string, error) {
	if run != "" && run != "latest" {
		return resolveRunDir(root, run)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	latest := ""
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "agent_context.md")); err == nil && entry.Name() > latest {
			latest = entry.Name()
		}
	}
	if latest == "" {
		return "", os.ErrNotExist
	}
	return filepath.Join(root, latest), nil
}

func resolveRunDir(root string, run string) (string, error) {
	if run == "" || run == "latest" {
		entries, err := os.ReadDir(root)
		if err != nil {
			return "", err
		}
		latest := ""
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() > latest {
				latest = entry.Name()
			}
		}
		if latest == "" {
			return "", os.ErrNotExist
		}
		return filepath.Join(root, latest), nil
	}
	if filepath.IsAbs(run) {
		return run, nil
	}
	return filepath.Join(root, run), nil
}

func stringArg(args map[string]any, key string, fallback string) string {
	if args == nil {
		return fallback
	}
	if value, ok := args[key].(string); ok {
		return value
	}
	return fallback
}

func boolArg(args map[string]any, key string, fallback bool) bool {
	if args == nil {
		return fallback
	}
	if value, ok := args[key].(bool); ok {
		return value
	}
	return fallback
}

func readMessage(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if strings.HasPrefix(strings.ToLower(line), "content-length:") {
		lengthText := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
		if lengthText == line {
			lengthText = strings.TrimSpace(strings.TrimPrefix(line, "content-length:"))
		}
		length, err := strconv.Atoi(lengthText)
		if err != nil {
			return nil, err
		}
		for {
			header, err := reader.ReadString('\n')
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(header) == "" {
				break
			}
		}
		payload := make([]byte, length)
		_, err = io.ReadFull(reader, payload)
		return payload, err
	}
	return []byte(line), nil
}

func writeResponse(out io.Writer, resp response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Content-Length: %d\r\n\r\n", len(data))
	buf.Write(data)
	_, err = out.Write(buf.Bytes())
	return err
}
