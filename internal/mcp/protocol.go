package mcp

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/version"
)

const (
	protocolLegacy2024 = "2024-11-05"
	protocolLegacy2025 = "2025-11-25"
	protocolModern     = "2026-07-28"

	errUnsupportedProtocol = -32022
)

var supportedProtocols = []string{protocolModern, protocolLegacy2025, protocolLegacy2024}

func serverInfo() map[string]any {
	v := version.Version
	if v == "" {
		v = "dev"
	}
	return map[string]any{"name": "glyphrun", "version": v}
}

func toolCapabilities() map[string]any {
	return map[string]any{
		"listChanged": false,
		"filtering":   true,
	}
}

func discoverResult() map[string]any {
	return map[string]any{
		"resultType":        "complete",
		"supportedVersions": supportedProtocols,
		"capabilities": map[string]any{
			"tools": toolCapabilities(),
		},
		"instructions": "Glyphrun MCP. Prefer glyph_search_tools to find a tool by intent, then glyph_run / glyph_context. Do not edit intent or outcomes silently.",
		"_meta": map[string]any{
			"io.modelcontextprotocol/serverInfo": serverInfo(),
		},
		"ttlMs":      3600000,
		"cacheScope": "public",
	}
}

func initializeResult(requested string) map[string]any {
	chosen := protocolLegacy2025
	if supportsProtocol(requested) {
		chosen = requested
	}
	return map[string]any{
		"protocolVersion": chosen,
		"capabilities": map[string]any{
			"tools": toolCapabilities(),
		},
		"serverInfo":   serverInfo(),
		"instructions": "Use glyph_search_tools when you need a tool by capability instead of loading the full catalog.",
	}
}

func supportsProtocol(v string) bool {
	for _, s := range supportedProtocols {
		if s == v {
			return true
		}
	}
	return false
}

func requestedProtocol(req request) string {
	var envelope struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Meta            map[string]any `json:"_meta"`
	}
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &envelope)
	}
	if envelope.ProtocolVersion != "" {
		return envelope.ProtocolVersion
	}
	if envelope.Meta != nil {
		if v, ok := envelope.Meta["io.modelcontextprotocol/protocolVersion"].(string); ok {
			return v
		}
	}
	return ""
}

func unsupportedProtocolError(requested string) *responseError {
	data, _ := json.Marshal(map[string]any{"supported": supportedProtocols, "requested": requested})
	return &responseError{
		Code:    errUnsupportedProtocol,
		Message: "Unsupported protocol version",
		Data:    json.RawMessage(data),
	}
}

func listToolsResult(params json.RawMessage) map[string]any {
	var p struct {
		Cursor string `json:"cursor"`
		Query  string `json:"query"`
	}
	_ = json.Unmarshal(params, &p)
	all := tools()
	if q := strings.TrimSpace(p.Query); q != "" {
		all = filterTools(all, q)
	}
	page, next := pageTools(all, p.Cursor, 0)
	out := map[string]any{
		"resultType": "complete",
		"tools":      page,
		"ttlMs":      300000,
		"cacheScope": "public",
	}
	if next != "" {
		out["nextCursor"] = next
	}
	return out
}

func pageTools(all []map[string]any, cursor string, pageSize int) ([]map[string]any, string) {
	start := 0
	if cursor != "" {
		raw, err := base64.StdEncoding.DecodeString(cursor)
		if err == nil {
			if n, err := strconv.Atoi(string(raw)); err == nil && n >= 0 {
				start = n
			}
		}
	}
	if start > len(all) {
		start = len(all)
	}
	if pageSize <= 0 || cursor == "" {
		return all[start:], ""
	}
	end := start + pageSize
	if end >= len(all) {
		return all[start:], ""
	}
	next := base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(end)))
	return all[start:end], next
}

func filterTools(all []map[string]any, query string) []map[string]any {
	q := strings.ToLower(query)
	out := make([]map[string]any, 0)
	for _, t := range all {
		blob := strings.ToLower(strings.Join([]string{
			stringField(t, "name"),
			stringField(t, "title"),
			stringField(t, "description"),
		}, " "))
		if strings.Contains(blob, q) {
			out = append(out, t)
		}
	}
	return out
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func searchTools(query string, limit int) []map[string]any {
	if limit <= 0 {
		limit = 8
	}
	q := strings.ToLower(strings.TrimSpace(query))
	type scored struct {
		tool  map[string]any
		score int
	}
	var ranked []scored
	for _, t := range tools() {
		name := strings.ToLower(stringField(t, "name"))
		title := strings.ToLower(stringField(t, "title"))
		desc := strings.ToLower(stringField(t, "description"))
		score := 0
		if name == q || strings.TrimPrefix(name, "glyph_") == q {
			score = 100
		} else if strings.Contains(name, q) {
			score = 50
		} else if strings.Contains(title, q) {
			score = 20
		} else if strings.Contains(desc, q) {
			score = 10
		} else {
			continue
		}
		ranked = append(ranked, scored{t, score})
	}
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].score > ranked[i].score {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]map[string]any, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, map[string]any{
			"name":        r.tool["name"],
			"title":       r.tool["title"],
			"description": r.tool["description"],
			"inputSchema": r.tool["inputSchema"],
		})
	}
	return out
}
