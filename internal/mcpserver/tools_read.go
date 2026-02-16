package mcpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kostyay/httpmon/internal/filter"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerReadTools adds read-only tools to the MCP server.
func (s *Server) registerReadTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_requests",
		Description: "List captured HTTP requests with optional filtering and pagination.",
	}, s.handleListRequests)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_request",
		Description: "Get full details of a captured HTTP request including headers and body.",
	}, s.handleGetRequest)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_requests",
		Description: "Search captured requests by substring match on host+path.",
	}, s.handleSearchRequests)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_request_count",
		Description: "Get the count of captured requests, optionally filtered.",
	}, s.handleGetRequestCount)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "export_har",
		Description: "Export captured requests as a HAR 1.2 JSON document.",
	}, s.handleExportHAR)
}

// --- list_requests ---

type listRequestsInput struct {
	Filter string `json:"filter,omitempty" jsonschema:"advanced filter expression"`
	Offset int    `json:"offset,omitempty" jsonschema:"pagination offset (default 0)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max results (default 50, max 200)"`
}

type flowSummary struct {
	ID          string `json:"id"`
	Method      string `json:"method"`
	StatusCode  int    `json:"status_code"`
	Host        string `json:"host"`
	Path        string `json:"path"`
	DurationMS  int64  `json:"duration_ms"`
	SizeBytes   int64  `json:"size_bytes"`
	StartedAt   string `json:"started_at"`
	State       string `json:"state"`
	ContentType string `json:"content_type"`
	Scheme      string `json:"scheme"`
}

func (s *Server) handleListRequests(
	_ context.Context, _ *mcp.CallToolRequest, in listRequestsInput,
) (*mcp.CallToolResult, any, error) {
	metas, total := s.cfg.Store.List(parseFilter(in.Filter), in.Offset, clampLimit(in.Limit))
	return jsonResult(map[string]any{
		"items": metasToSummaries(metas),
		"total": total,
	}), nil, nil
}

// --- get_request ---

type getRequestInput struct {
	ID          string `json:"id" jsonschema:"flow ID (required)"`
	MaxBodySize int    `json:"max_body_size,omitempty" jsonschema:"max body bytes inline (default 50000)"`
	Dump        bool   `json:"dump,omitempty" jsonschema:"write bodies to temp files instead of inlining"`
}

func (s *Server) handleGetRequest(
	_ context.Context, _ *mcp.CallToolRequest, in getRequestInput,
) (*mcp.CallToolResult, any, error) {
	if in.ID == "" {
		return errorResult("id is required"), nil, nil
	}

	maxBody := in.MaxBodySize
	if maxBody <= 0 {
		maxBody = 50000
	}

	meta, data, err := s.cfg.Store.Get(in.ID)
	if err != nil {
		return errorResult(fmt.Sprintf("flow not found: %s", in.ID)), nil, nil
	}

	result := map[string]any{
		"meta": metaToSummary(*meta),
	}

	if data != nil {
		reqHeaders := headerMap(data.RequestHeaders)
		respHeaders := headerMap(data.ResponseHeaders)
		result["request_headers"] = reqHeaders
		result["response_headers"] = respHeaders

		if in.Dump {
			// Write bodies to temp files (safe: os.CreateTemp always uses os.TempDir).
			if len(data.RequestBody) > 0 {
				if f, err := os.CreateTemp("", in.ID+"-req-*.bin"); err == nil { //#nosec G703 -- f.Name() from os.CreateTemp
					_, _ = f.Write(data.RequestBody)
					_ = f.Close()
					result["request_body_path"] = f.Name()
				}
			}
			if len(data.ResponseBody) > 0 {
				if f, err := os.CreateTemp("", in.ID+"-resp-*.bin"); err == nil { //#nosec G703 -- f.Name() from os.CreateTemp
					_, _ = f.Write(data.ResponseBody)
					_ = f.Close()
					result["response_body_path"] = f.Name()
				}
			}
		} else {
			result["request_body"] = encodeBody(data.RequestBody, maxBody)
			result["response_body"] = encodeBody(data.ResponseBody, maxBody)
		}
	}

	return jsonResult(result), nil, nil
}

// --- search_requests ---

type searchRequestsInput struct {
	Query  string `json:"query" jsonschema:"substring to search in host+path (required)"`
	Offset int    `json:"offset,omitempty" jsonschema:"pagination offset"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max results (default 50, max 200)"`
}

func (s *Server) handleSearchRequests(
	_ context.Context, _ *mcp.CallToolRequest, in searchRequestsInput,
) (*mcp.CallToolResult, any, error) {
	if in.Query == "" {
		return errorResult("query is required"), nil, nil
	}

	metas, total := s.cfg.Store.List(filter.CompileQuick(in.Query), in.Offset, clampLimit(in.Limit))
	return jsonResult(map[string]any{
		"items": metasToSummaries(metas),
		"total": total,
	}), nil, nil
}

// --- get_request_count ---

type getRequestCountInput struct {
	Filter string `json:"filter,omitempty" jsonschema:"filter expression"`
}

func (s *Server) handleGetRequestCount(
	_ context.Context, _ *mcp.CallToolRequest, in getRequestCountInput,
) (*mcp.CallToolResult, any, error) {
	_, total := s.cfg.Store.List(parseFilter(in.Filter), 0, 0)
	return jsonResult(map[string]any{"total": total}), nil, nil
}

// --- export_har ---

type exportHARInput struct {
	Filter     string   `json:"filter,omitempty" jsonschema:"filter expression"`
	RequestIDs []string `json:"request_ids,omitempty" jsonschema:"specific request IDs to export"`
}

func (s *Server) handleExportHAR(
	_ context.Context, _ *mcp.CallToolRequest, in exportHARInput,
) (*mcp.CallToolResult, any, error) {
	var metas []store.FlowMeta

	if len(in.RequestIDs) > 0 {
		for _, id := range in.RequestIDs {
			meta, _, err := s.cfg.Store.Get(id)
			if err == nil {
				metas = append(metas, *meta)
			}
		}
	} else {
		metas, _ = s.cfg.Store.List(parseFilter(in.Filter), 0, 0)
	}

	har := buildHAR(s.cfg.Store, metas)
	harJSON, _ := json.MarshalIndent(har, "", "  ")
	return textResult(string(harJSON)), nil, nil
}

// --- helpers ---

// clampLimit applies default (50) and max (200) bounds to a pagination limit.
func clampLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

// parseFilter compiles a filter expression, returning nil for empty input.
func parseFilter(expr string) store.Filter {
	if expr == "" {
		return nil
	}
	return filter.Compile(expr)
}

func metasToSummaries(metas []store.FlowMeta) []flowSummary {
	items := make([]flowSummary, len(metas))
	for i, m := range metas {
		items[i] = metaToSummary(m)
	}
	return items
}

func metaToSummary(m store.FlowMeta) flowSummary {
	state := "completed"
	switch m.State {
	case store.StateInProgress:
		state = "in_progress"
	case store.StateFailed:
		state = "failed"
	case store.StateBreakpoint:
		state = "breakpoint"
	}

	return flowSummary{
		ID:          m.ID,
		Method:      m.Method,
		StatusCode:  m.StatusCode,
		Host:        m.Host,
		Path:        m.Path,
		DurationMS:  m.Duration.Milliseconds(),
		SizeBytes:   m.SizeBytes,
		StartedAt:   m.StartedAt.Format(time.RFC3339),
		State:       state,
		ContentType: m.ContentType,
		Scheme:      m.Scheme,
	}
}

func headerMap(h http.Header) map[string]string {
	if h == nil {
		return nil
	}
	m := make(map[string]string, len(h))
	for k := range h {
		m[k] = h.Get(k)
	}
	return m
}

func encodeBody(body []byte, maxSize int) string {
	if len(body) == 0 {
		return ""
	}
	if len(body) > maxSize {
		body = body[:maxSize]
	}
	if utf8.Valid(body) {
		return string(body)
	}
	return "base64:" + base64.StdEncoding.EncodeToString(body)
}

func jsonResult(v any) *mcp.CallToolResult {
	data, _ := json.Marshal(v)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}
}

// buildHAR creates a HAR 1.2 document from the given flows.
func buildHAR(reader FlowReader, metas []store.FlowMeta) map[string]any {
	entries := make([]map[string]any, 0, len(metas))
	for _, m := range metas {
		_, data, err := reader.Get(m.ID)
		if err != nil {
			continue
		}

		entry := map[string]any{
			"startedDateTime": m.StartedAt.Format(time.RFC3339Nano),
			"time":            m.Duration.Milliseconds(),
			"request": map[string]any{
				"method":      m.Method,
				"url":         m.Scheme + "://" + m.Host + m.Path,
				"httpVersion": "HTTP/1.1",
				"headers":     harHeaders(data.RequestHeaders),
				"bodySize":    len(data.RequestBody),
			},
			"response": map[string]any{
				"status":      m.StatusCode,
				"statusText":  http.StatusText(m.StatusCode),
				"httpVersion": "HTTP/1.1",
				"headers":     harHeaders(data.ResponseHeaders),
				"content": map[string]any{
					"size":     len(data.ResponseBody),
					"mimeType": m.ContentType,
					"text":     encodeBody(data.ResponseBody, 100000),
				},
				"bodySize": len(data.ResponseBody),
			},
			"timings": map[string]any{
				"send":    0,
				"wait":    m.Duration.Milliseconds(),
				"receive": 0,
			},
		}

		if len(data.RequestBody) > 0 {
			entry["request"].(map[string]any)["postData"] = map[string]any{
				"mimeType": contentTypeFromHeaders(data.RequestHeaders),
				"text":     string(data.RequestBody),
			}
		}

		entries = append(entries, entry)
	}

	return map[string]any{
		"log": map[string]any{
			"version": "1.2",
			"creator": map[string]any{
				"name":    "httpmon",
				"version": "1.0.0",
			},
			"entries": entries,
		},
	}
}

func harHeaders(h http.Header) []map[string]string {
	if h == nil {
		return nil
	}
	out := make([]map[string]string, 0, len(h))
	for k := range h {
		out = append(out, map[string]string{
			"name":  k,
			"value": h.Get(k),
		})
	}
	return out
}

func contentTypeFromHeaders(h http.Header) string {
	if h == nil {
		return ""
	}
	ct := h.Get("Content-Type")
	if ct == "" {
		return "application/octet-stream"
	}
	if idx := strings.Index(ct, ";"); idx > 0 {
		return ct[:idx]
	}
	return ct
}
