//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- Read-only tool tests ---

func TestMCP_ListRequests(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/echo")
	h.doGet(t, "/binary")
	h.waitForFlows(t, 3)

	result := h.callTool(t, "list_requests", nil)
	text := resultText(result)

	var resp struct {
		Items []json.RawMessage `json:"items"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("expected total=3, got %d", resp.Total)
	}
	if len(resp.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(resp.Items))
	}
}

func TestMCP_GetRequest(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	h.doPost(t, "/echo", `{"key":"value"}`)
	h.waitForFlows(t, 1)

	// Get the flow ID.
	metas, _ := h.store.List(nil, 0, 1)
	if len(metas) == 0 {
		t.Fatal("no flows in store")
	}
	id := metas[0].ID

	result := h.callTool(t, "get_request", map[string]any{"id": id})
	text := resultText(result)

	if !strings.Contains(text, "POST") {
		t.Error("response should contain POST method")
	}
	if !strings.Contains(text, "key") {
		t.Error("response should contain request body content")
	}
}

func TestMCP_GetRequest_BodyTruncation(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	h.doGet(t, "/large")
	h.waitForFlows(t, 1)

	metas, _ := h.store.List(nil, 0, 1)
	id := metas[0].ID

	// Request with small max_body_size.
	result := h.callTool(t, "get_request", map[string]any{
		"id":            id,
		"max_body_size": 100,
	})
	text := resultText(result)

	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}

	body, ok := resp["response_body"].(string)
	if !ok {
		t.Fatal("response_body not a string")
	}
	if len(body) > 110 { // Some overhead for encoding
		t.Errorf("body should be truncated, got len=%d", len(body))
	}
}

func TestMCP_SearchRequests(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/echo")
	h.doGet(t, "/binary")
	h.waitForFlows(t, 3)

	result := h.callTool(t, "search_requests", map[string]any{"query": "json"})
	text := resultText(result)

	var resp struct {
		Items []json.RawMessage `json:"items"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("expected 1 match for 'json', got %d", resp.Total)
	}
}

func TestMCP_GetRequestCount(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/echo")
	h.doGet(t, "/json")
	h.waitForFlows(t, 3)

	// Unfiltered count.
	result := h.callTool(t, "get_request_count", nil)
	text := resultText(result)
	var resp struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("expected total=3, got %d", resp.Total)
	}

	// Filtered count.
	result = h.callTool(t, "get_request_count", map[string]any{"filter": "json"})
	text = resultText(result)
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2 for 'json', got %d", resp.Total)
	}
}

func TestMCP_ExportHAR(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/echo")
	h.waitForFlows(t, 2)

	result := h.callTool(t, "export_har", nil)
	text := resultText(result)

	var har struct {
		Log struct {
			Version string           `json:"version"`
			Entries []json.RawMessage `json:"entries"`
		} `json:"log"`
	}
	if err := json.Unmarshal([]byte(text), &har); err != nil {
		t.Fatalf("parse HAR: %v", err)
	}
	if har.Log.Version != "1.2" {
		t.Errorf("expected HAR version 1.2, got %q", har.Log.Version)
	}
	if len(har.Log.Entries) != 2 {
		t.Errorf("expected 2 HAR entries, got %d", len(har.Log.Entries))
	}
}

// --- Simulation tool tests ---

func TestMCP_Throttle(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	// Set throttle.
	result := h.callTool(t, "set_throttle", map[string]any{"preset": "3g"})
	text := resultText(result)
	if !strings.Contains(text, "93750") {
		t.Errorf("expected 3g bps in response, got: %s", text)
	}

	// Get throttle.
	result = h.callTool(t, "get_throttle", nil)
	text = resultText(result)
	var resp struct {
		BPS    int64 `json:"bps"`
		Active bool  `json:"active"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !resp.Active {
		t.Error("throttle should be active")
	}
	if resp.BPS != 93750 {
		t.Errorf("expected bps=93750, got %d", resp.BPS)
	}

	// Disable throttle.
	h.callTool(t, "set_throttle", map[string]any{"bps": 0})
	result = h.callTool(t, "get_throttle", nil)
	text = resultText(result)
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Active {
		t.Error("throttle should be inactive after disabling")
	}
}

func TestMCP_ReplayRequest(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	h.doGet(t, "/echo")
	h.waitForFlows(t, 1)

	metas, _ := h.store.List(nil, 0, 1)
	id := metas[0].ID

	// Replay by ID.
	result := h.callTool(t, "replay_request", map[string]any{"request_id": id})
	text := resultText(result)
	if result.IsError {
		t.Fatalf("replay error: %s", text)
	}
	if !strings.Contains(text, "status_code") {
		t.Errorf("expected status_code in response, got: %s", text)
	}

	// Verify new request appeared.
	h.waitForFlows(t, 2)
}

func TestMCP_ReplayCompose(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	result := h.callTool(t, "replay_request", map[string]any{
		"method":  "POST",
		"url":     h.upstream.URL + "/echo",
		"headers": map[string]any{"X-Test": "composed"},
		"body":    `{"composed":true}`,
	})
	text := resultText(result)
	if result.IsError {
		t.Fatalf("compose error: %s", text)
	}

	h.waitForFlows(t, 1)
	metas, _ := h.store.List(nil, 0, 1)
	if metas[0].Method != "POST" {
		t.Errorf("expected POST, got %s", metas[0].Method)
	}
}

// --- Script tool tests ---

func TestMCP_CreateAndListScript(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	// Create script.
	result := h.callTool(t, "create_script", map[string]any{
		"name":           "test-script",
		"match_patterns": []any{"*"},
		"code":           `function onRequest(ctx) { /* noop */ }`,
		"enabled":        true,
	})
	text := resultText(result)
	if result.IsError {
		t.Fatalf("create error: %s", text)
	}
	if !strings.Contains(text, "file_path") {
		t.Errorf("expected file_path in response, got: %s", text)
	}

	// List scripts.
	result = h.callTool(t, "list_scripts", nil)
	text = resultText(result)
	if !strings.Contains(text, "test-script") {
		t.Errorf("list should contain test-script, got: %s", text)
	}
}

func TestMCP_ToggleScript(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	// Create script.
	result := h.callTool(t, "create_script", map[string]any{
		"name":           "toggle-test",
		"match_patterns": []any{"*"},
		"code":           `function onRequest(ctx) {}`,
		"enabled":        true,
	})
	var createResp struct {
		FilePath string `json:"file_path"`
	}
	json.Unmarshal([]byte(resultText(result)), &createResp)

	// Toggle off.
	result = h.callTool(t, "toggle_script", map[string]any{
		"file_path": createResp.FilePath,
	})
	text := resultText(result)
	var toggleResp struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(text), &toggleResp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if toggleResp.Enabled {
		t.Error("script should be disabled after toggle")
	}
}

func TestMCP_DeleteScript(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	result := h.callTool(t, "create_script", map[string]any{
		"name":           "delete-me",
		"match_patterns": []any{"*"},
		"code":           `function onRequest(ctx) {}`,
		"enabled":        true,
	})
	var createResp struct {
		FilePath string `json:"file_path"`
	}
	json.Unmarshal([]byte(resultText(result)), &createResp)

	// Delete.
	result = h.callTool(t, "delete_script", map[string]any{
		"file_path": createResp.FilePath,
	})
	text := resultText(result)
	if !strings.Contains(text, `"deleted":true`) {
		t.Errorf("expected deleted:true, got: %s", text)
	}

	// Verify removed from list.
	result = h.callTool(t, "list_scripts", nil)
	text = resultText(result)
	if strings.Contains(text, "delete-me") {
		t.Error("deleted script should not appear in list")
	}
}

func TestMCP_GetScript(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	jsCode := `function onRequest(ctx) { /* custom code */ }`
	result := h.callTool(t, "create_script", map[string]any{
		"name":           "source-test",
		"match_patterns": []any{"*.example.com"},
		"code":           jsCode,
		"enabled":        true,
	})
	var createResp struct {
		FilePath string `json:"file_path"`
	}
	json.Unmarshal([]byte(resultText(result)), &createResp)

	// Get script.
	result = h.callTool(t, "get_script", map[string]any{
		"file_path": createResp.FilePath,
	})
	text := resultText(result)
	if !strings.Contains(text, "source-test") {
		t.Error("should contain script name")
	}
	if !strings.Contains(text, "custom code") {
		t.Error("should contain script source")
	}
}

// --- Lifecycle tests ---

func TestMCP_ServerStartStop(t *testing.T) {
	t.Parallel()
	h := newMCPHarness(t, multiHandler())

	// Server should be running.
	if !h.mcpSrv.Running() {
		t.Error("MCP server should be running")
	}

	// Tools should be callable.
	result := h.callTool(t, "get_request_count", nil)
	if result.IsError {
		t.Error("tool call should succeed while server is running")
	}
}
