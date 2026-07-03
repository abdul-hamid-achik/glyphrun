package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/abdul-hamid-achik/glyphrun/internal/affected"
	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	glyphdocs "github.com/abdul-hamid-achik/glyphrun/internal/docs"
	"github.com/abdul-hamid-achik/glyphrun/internal/render"
	"github.com/abdul-hamid-achik/glyphrun/internal/repair"
	"github.com/abdul-hamid-achik/glyphrun/internal/runner"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
		tool("glyph_run", "Run a Glyphrun spec. Set monitor to capture process telemetry of the spawned target via the monitor CLI.", map[string]any{
			"type":     "object",
			"required": []string{"path"},
			"properties": map[string]any{
				"path":            map[string]any{"type": "string"},
				"updateSnapshots": map[string]any{"type": "boolean"},
				"monitor":         map[string]any{"type": "string", "description": "path to the monitor binary; enables process-telemetry sampling -> diagnostics/process.{md,json}"},
				"monitorProfile":  map[string]any{"type": "string", "enum": []string{"heap", "cpu", "goroutine", "sample"}, "description": "capture an end-of-run process profile (use with monitor)"},
				"monitorInterval": map[string]any{"type": "string", "description": "sample interval as a Go duration, e.g. 250ms (use with monitor)"},
			},
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
		tool("glyph_spec_scaffold", "Return a starter Glyphrun spec or reusable action. coversSymbol (spec kind only) binds the stub to the code symbol it exercises, so glyph affected-specs can select it.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind":         map[string]any{"type": "string", "enum": []string{"spec", "action"}},
				"coversSymbol": map[string]any{"type": "string"},
			},
		}),
		tool("glyph_render", "Render a run's final screen to a deterministic SVG and return it.", map[string]any{
			"type":       "object",
			"properties": map[string]any{"run": map[string]any{"type": "string"}, "screen": map[string]any{"type": "string"}},
		}),
		tool("glyph_repair", "Propose step repairs for a spec's failed run (never touches the contract).", map[string]any{
			"type":       "object",
			"required":   []string{"path"},
			"properties": map[string]any{"path": map[string]any{"type": "string"}, "run": map[string]any{"type": "string"}, "write": map[string]any{"type": "boolean"}},
		}),
		tool("glyph_affected_specs", "Select the specs a git change can hit: shells out to `codemap review --json`, intersects each spec's coversSymbol against the changed symbols + blast radius, and returns the minimal spec set (run those via glyph_run). One of since/staged selects the diff scope; neither means the working tree.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"paths":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "spec files/dirs to scan (default: [\".\"])"},
				"since":   map[string]any{"type": "string", "description": "review changes since this git ref"},
				"staged":  map[string]any{"type": "boolean", "description": "review only staged changes"},
				"codemap": map[string]any{"type": "string", "description": "path to the codemap binary (default: codemap on $PATH)"},
				"depth":   map[string]any{"type": "integer", "minimum": 0, "description": "max blast-radius hops (default 3)"},
			},
		}),
		tool("glyph_clean", "Prune old run directories from the artifact root. Without flags, applies the project retention.keepRuns (default 3) and, when retention.archive is configured, archives pruned dirs to the external command (e.g. fcheap) before deleting them. Set all=true to wipe every run dir; noArchive=true to delete locally without archiving.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"keep":      map[string]any{"type": "integer", "minimum": 0, "description": "keep the N newest runs (overrides retention.keepRuns for this call; 0 disables pruning)"},
				"all":       map[string]any{"type": "boolean", "description": "remove every run directory under the artifact root"},
				"noArchive": map[string]any{"type": "boolean", "description": "delete pruned run dirs locally without archiving them first"},
			},
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
			"steps":     []string{"press", "type", "paste", "send", "mouse", "wait", "resize", "snapshot", "use", "download", "transform", "monitor", "batch", "when"},
			"verifiers": []string{"screen", "region", "cell", "cursor", "process", "snapshot", "command", "file", "script", "count", "link", "metrics"},
			"commands":  []string{"init", "run", "run --monitor <path>", "spec verify", "spec scaffold", "spec scaffold --kind action", "spec scaffold --coversSymbol <sym>", "affected-specs --since <ref>", "snapshot update", "diff", "context", "render", "repair", "docs", "agent", "explain", "doctor", "list", "clean", "clean --no-archive", "mcp"},
			"namedArtifacts": map[string]any{
				"placeholders": []string{"${artifacts.<name>.path}", "${artifacts.<name>.relativePath}"},
				"stepKinds":    []string{"download", "transform"},
				"example":      "see examples/specs/download_artifact.yml and examples/specs/transform_artifact.yml",
			},
			"compositeSteps": map[string]any{
				"batch":        "queue multiple press/type/paste/send sub-steps into one PTY write so transient state survives",
				"trailingWait": "an optional trailing wait: in a batch is the only synchronization point",
			},
			"ciIntegration": map[string]any{
				"junit": "--junit <file> on `glyph run` writes a JUnit XML report consumable by GitHub Actions, GitLab, Jenkins, Buildkite",
			},
			"progress": []string{"auto", "always", "never"},
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
		runOpts := runner.Options{
			SpecPath:        path,
			ConfigPath:      opts.ConfigPath,
			Environment:     opts.Environment,
			ArtifactRoot:    opts.ArtifactRoot,
			UpdateSnapshots: boolArg(params.Arguments, "updateSnapshots", false),
		}
		if bin := stringArg(params.Arguments, "monitor", ""); bin != "" {
			interval := 250 * time.Millisecond
			if d, err := time.ParseDuration(stringArg(params.Arguments, "monitorInterval", "")); err == nil && d > 0 {
				interval = d
			}
			runOpts.Procmon = &runner.ProcmonConfig{Bin: bin, Interval: interval, Profile: stringArg(params.Arguments, "monitorProfile", "")}
		}
		result, err := runner.RunSpec(ctx, runOpts)
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
		return map[string]any{"content": []map[string]string{{"type": "text", "text": scaffoldSpec(stringArg(params.Arguments, "kind", "spec"), stringArg(params.Arguments, "coversSymbol", ""))}}}, nil
	case "glyph_render":
		run := stringArg(params.Arguments, "run", "latest")
		screen := stringArg(params.Arguments, "screen", "final")
		svg, err := renderScreen(run, screen, opts)
		if err != nil {
			return toolError(err)
		}
		return map[string]any{"content": []map[string]string{{"type": "text", "text": svg}}}, nil
	case "glyph_repair":
		path := stringArg(params.Arguments, "path", "")
		if path == "" {
			return nil, &responseError{Code: -32602, Message: "path is required"}
		}
		result, err := repairSpec(path, stringArg(params.Arguments, "run", "latest"), boolArg(params.Arguments, "write", false), opts)
		if err != nil {
			return toolError(err)
		}
		return toolText(result)
	case "glyph_affected_specs":
		since := stringArg(params.Arguments, "since", "")
		staged := boolArg(params.Arguments, "staged", false)
		if since != "" && staged {
			return nil, &responseError{Code: -32602, Message: "affected-specs: pass at most one of since/staged"}
		}
		mode := "working"
		if staged {
			mode = "staged"
		} else if since != "" {
			mode = "since"
		}
		paths := stringSliceArg(params.Arguments, "paths")
		if len(paths) == 0 {
			paths = []string{"."}
		}
		rows, err := affected.LoadSpecs(paths, opts.ConfigPath, opts.Environment)
		if err != nil {
			return toolError(err)
		}
		depth := intArg(params.Arguments, "depth", 3)
		review, err := affected.RunReview(stringArg(params.Arguments, "codemap", "codemap"), mode, since, depth)
		if err != nil {
			return toolError(err)
		}
		report := affected.Select(rows, review)
		report.SchemaVersion = 1
		report.Mode = mode
		report.Since = since
		return toolText(report)
	case "glyph_clean":
		root, err := artifactRoot(opts)
		if err != nil {
			return toolError(err)
		}
		rt, err := config.LoadRuntime(".", opts.ConfigPath, opts.Environment)
		if err != nil {
			return toolError(err)
		}
		if boolArg(params.Arguments, "all", false) {
			report, err := artifacts.CleanAll(root)
			if err != nil {
				return toolError(err)
			}
			return toolText(report)
		}
		effectiveKeep := intArg(params.Arguments, "keep", 0)
		if _, ok := params.Arguments["keep"]; !ok {
			effectiveKeep = rt.Config.Retention.KeepRuns
		}
		if effectiveKeep < 0 {
			return toolError(fmt.Errorf("keep must be >= 0 (got %d)", effectiveKeep))
		}
		archive := artifacts.ArchiveConfig{}
		if !boolArg(params.Arguments, "noArchive", false) {
			archive = artifacts.ArchiveConfig{
				Enabled: rt.Config.Retention.Archive.Enabled,
				Command: rt.Config.Retention.Archive.Command,
				Args:    rt.Config.Retention.Archive.Args,
			}
			if d, perr := artifacts.ParseArchiveTimeout(rt.Config.Retention.Archive.Timeout); perr == nil {
				archive.Timeout = d
			}
		}
		report, err := artifacts.PruneRuns(root, effectiveKeep, archive)
		if err != nil {
			return toolError(err)
		}
		return toolText(report)
	default:
		return nil, &responseError{Code: -32602, Message: "unknown tool: " + params.Name}
	}
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

// renderScreen resolves a run and renders its final screen (or a named
// snapshot) to a deterministic SVG, mirroring `glyph render`.
func renderScreen(run, screen string, opts ServerOptions) (string, error) {
	root, err := artifactRoot(opts)
	if err != nil {
		return "", err
	}
	runDir, err := resolveRunDir(root, run)
	if err != nil {
		return "", err
	}
	var rel string
	if screen == "" || screen == "final" {
		rel = filepath.Join("screens", "final.json")
	} else {
		rel = filepath.Join("snapshots", artifacts.SafeName(screen)+".json")
	}
	data, err := os.ReadFile(filepath.Join(runDir, rel))
	if err != nil {
		return "", err
	}
	var snapshot terminal.ScreenSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return "", err
	}
	return render.SnapshotSVG(snapshot, render.DefaultOptions()), nil
}

// repairSpec mirrors `glyph repair`: it analyzes a spec's failed run and
// proposes (optionally applies) step repairs that never touch the contract.
func repairSpec(path, run string, write bool, opts ServerOptions) (any, error) {
	rt, err := config.LoadRuntime(path, opts.ConfigPath, opts.Environment)
	if err != nil {
		return nil, err
	}
	parseOpts := rt.SpecParseOptions()
	parseOpts.AllowHashMismatch = true
	parsed, err := spec.ParseFile(path, parseOpts)
	if err != nil {
		return nil, err
	}
	root, err := artifactRoot(opts)
	if err != nil {
		return nil, err
	}
	var runDir string
	if run != "" && run != "latest" {
		runDir, err = resolveRunDir(root, run)
	} else {
		runDir, err = repair.LatestRunDirForSpec(root, parsed.Resolved.Name)
	}
	if err != nil {
		return nil, err
	}
	proposals := repair.Analyze(runDir, parsed.Resolved.Steps)
	if write {
		for i := range proposals {
			if proposals[i].Proposed == "" || proposals[i].Current == "" {
				continue
			}
			if err := repair.Apply(parsed.Path, proposals[i]); err != nil {
				return nil, err
			}
			proposals[i].Applied = true
		}
	}
	return map[string]any{
		"spec":      parsed.Resolved.Name,
		"run":       filepath.Base(runDir),
		"proposals": proposals,
		"applied":   write,
	}, nil
}

// artifactRoot resolves the absolute artifact root from server options + config.
func artifactRoot(opts ServerOptions) (string, error) {
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
	return root, nil
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

func intArg(args map[string]any, key string, fallback int) int {
	if args == nil {
		return fallback
	}
	// JSON numbers unmarshal into float64; accept int and float64.
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return fallback
}

func stringSliceArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	arr, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
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

// writeResponse writes a JSON-RPC response as a single JSON object followed by
// a newline. This is the newline-delimited JSON-RPC framing that stdio MCP
// clients (Claude Code, Codex, OpenCode, Claude Desktop) expect. The input
// side (readMessage) still accepts Content-Length-framed requests for
// backwards compatibility, but the output is line-delimited only because that
// is what the MCP spec defines for stdio transport and what every known
// consumer of `glyph mcp` uses.
func writeResponse(out io.Writer, resp response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = out.Write(data)
	return err
}

func scaffoldSpec(kind, coversSymbol string) string {
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
	cs := ""
	if strings.TrimSpace(coversSymbol) != "" {
		cs = "coversSymbol: " + strings.TrimSpace(coversSymbol) + "\n"
	}
	return `version: 1
name: hello_quits
` + cs + `
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
