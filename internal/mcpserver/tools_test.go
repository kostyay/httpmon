package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// listResult is the common shape returned by list/search handlers.
type listResult struct {
	Items []flowSummary `json:"items"`
	Total int           `json:"total"`
}

// testServer returns a Server wired to the given mocks.
func testServer(st *mockStore, opts ...func(*Config)) *Server {
	cfg := Config{Store: st}
	for _, o := range opts {
		o(&cfg)
	}
	return &Server{cfg: cfg}
}

func withThrottle(tc *mockThrottle) func(*Config) {
	return func(c *Config) { c.Throttle = tc }
}

func withScripts(sm *mockScripts) func(*Config) {
	return func(c *Config) { c.Scripts = sm }
}

// sampleMetas returns n test flow metas with sequential single-char IDs.
func sampleMetas(n int) []store.FlowMeta {
	metas := make([]store.FlowMeta, n)
	for i := range metas {
		metas[i] = store.FlowMeta{
			ID:        store.FlowID(string(rune('a' + i))),
			Method:    "GET",
			Host:      "example.com",
			Path:      "/path",
			State:     store.StateCompleted,
			StartedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Duration:  100 * time.Millisecond,
			Scheme:    "https",
		}
	}
	return metas
}

// --- handleListRequests ---

func TestHandleListRequests(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("empty_store", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, err := s.handleListRequests(ctx, nil, listRequestsInput{})
		if err != nil {
			t.Fatal(err)
		}
		var out listResult
		unmarshalResult(t, res, &out)
		if out.Total != 0 || len(out.Items) != 0 {
			t.Errorf("expected empty, got total=%d items=%d", out.Total, len(out.Items))
		}
	})

	t.Run("pagination", func(t *testing.T) {
		s := testServer(&mockStore{metas: sampleMetas(5)})
		res, _, _ := s.handleListRequests(ctx, nil, listRequestsInput{Offset: 2, Limit: 2})
		var out listResult
		unmarshalResult(t, res, &out)
		if out.Total != 5 {
			t.Errorf("total = %d, want 5", out.Total)
		}
		if len(out.Items) != 2 {
			t.Errorf("items = %d, want 2", len(out.Items))
		}
	})
}

// --- handleGetRequest ---

func TestHandleGetRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("missing_id", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, _ := s.handleGetRequest(ctx, nil, getRequestInput{})
		if !res.IsError {
			t.Error("expected error result")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, _ := s.handleGetRequest(ctx, nil, getRequestInput{ID: "nope"})
		if !res.IsError {
			t.Error("expected error result")
		}
	})

	t.Run("found_inline", func(t *testing.T) {
		ms := &mockStore{
			metas: []store.FlowMeta{{
				ID: "f1", Method: "POST", Host: "h", Path: "/p",
				State: store.StateCompleted, StartedAt: time.Now(), Scheme: "https",
			}},
			data: map[store.FlowID]*store.FlowData{
				"f1": {
					RequestHeaders:  http.Header{"Content-Type": {"application/json"}},
					RequestBody:     []byte(`{"key":"val"}`),
					ResponseHeaders: http.Header{"X-Resp": {"ok"}},
					ResponseBody:    []byte("resp"),
				},
			},
		}
		s := testServer(ms)
		res, _, err := s.handleGetRequest(ctx, nil, getRequestInput{ID: "f1"})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatal("unexpected error")
		}
		var out map[string]any
		unmarshalResult(t, res, &out)
		if out["request_body"] != `{"key":"val"}` {
			t.Errorf("request_body = %v", out["request_body"])
		}
		if out["response_body"] != "resp" {
			t.Errorf("response_body = %v", out["response_body"])
		}
	})

	t.Run("dump_mode", func(t *testing.T) {
		ms := &mockStore{
			metas: []store.FlowMeta{{ID: "f2", State: store.StateCompleted, StartedAt: time.Now(), Scheme: "https"}},
			data: map[store.FlowID]*store.FlowData{
				"f2": {RequestBody: []byte("req"), ResponseBody: []byte("resp")},
			},
		}
		s := testServer(ms)
		res, _, _ := s.handleGetRequest(ctx, nil, getRequestInput{ID: "f2", Dump: true})
		var out map[string]any
		unmarshalResult(t, res, &out)
		reqPath, _ := out["request_body_path"].(string)
		respPath, _ := out["response_body_path"].(string)
		if reqPath == "" || respPath == "" {
			t.Fatalf("expected temp paths, got req=%q resp=%q", reqPath, respPath)
		}
		os.Remove(reqPath)
		os.Remove(respPath)
	})

	t.Run("body_truncation", func(t *testing.T) {
		ms := &mockStore{
			metas: []store.FlowMeta{{ID: "f3", State: store.StateCompleted, StartedAt: time.Now(), Scheme: "https"}},
			data: map[store.FlowID]*store.FlowData{
				"f3": {ResponseBody: []byte(strings.Repeat("x", 100))},
			},
		}
		s := testServer(ms)
		res, _, _ := s.handleGetRequest(ctx, nil, getRequestInput{ID: "f3", MaxBodySize: 10})
		var out map[string]any
		unmarshalResult(t, res, &out)
		body, _ := out["response_body"].(string)
		if len(body) != 10 {
			t.Errorf("body length = %d, want 10", len(body))
		}
	})
}

// --- handleSearchRequests ---

func TestHandleSearchRequests(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("empty_query", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, _ := s.handleSearchRequests(ctx, nil, searchRequestsInput{})
		if !res.IsError {
			t.Error("expected error for empty query")
		}
	})

	t.Run("matches", func(t *testing.T) {
		ms := &mockStore{metas: []store.FlowMeta{
			{ID: "1", Host: "api.example.com", Path: "/users", State: store.StateCompleted, StartedAt: time.Now(), Scheme: "https"},
			{ID: "2", Host: "other.com", Path: "/foo", State: store.StateCompleted, StartedAt: time.Now(), Scheme: "https"},
		}}
		s := testServer(ms)
		res, _, _ := s.handleSearchRequests(ctx, nil, searchRequestsInput{Query: "example"})
		var out listResult
		unmarshalResult(t, res, &out)
		if out.Total != 1 {
			t.Errorf("total = %d, want 1", out.Total)
		}
	})
}

// --- handleGetRequestCount ---

func TestHandleGetRequestCount(t *testing.T) {
	t.Parallel()
	s := testServer(&mockStore{metas: sampleMetas(3)})

	res, _, _ := s.handleGetRequestCount(context.Background(), nil, getRequestCountInput{})
	var out struct{ Total int }
	unmarshalResult(t, res, &out)
	if out.Total != 3 {
		t.Errorf("total = %d, want 3", out.Total)
	}
}

// --- handleExportHAR ---

func TestHandleExportHAR(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("by_filter", func(t *testing.T) {
		ms := &mockStore{
			metas: []store.FlowMeta{{
				ID: "h1", Method: "GET", Host: "example.test", Path: "/page",
				StatusCode: 200, State: store.StateCompleted, StartedAt: time.Now(),
				Scheme: "https", ContentType: "text/html",
			}},
			data: map[store.FlowID]*store.FlowData{
				"h1": {ResponseBody: []byte("ok")},
			},
		}
		s := testServer(ms)
		res, _, _ := s.handleExportHAR(ctx, nil, exportHARInput{})
		text := res.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, `"version": "1.2"`) {
			t.Error("HAR missing version")
		}
		if !strings.Contains(text, "https://example.test/page") {
			t.Errorf("HAR missing entry URL, got: %s", text[:min(len(text), 500)])
		}
	})

	t.Run("by_request_ids", func(t *testing.T) {
		ms := &mockStore{
			metas: []store.FlowMeta{
				{ID: "a", Host: "a", Path: "/", State: store.StateCompleted, StartedAt: time.Now(), Scheme: "https"},
				{ID: "b", Host: "b", Path: "/", State: store.StateCompleted, StartedAt: time.Now(), Scheme: "https"},
			},
			data: map[store.FlowID]*store.FlowData{
				"a": {}, "b": {},
			},
		}
		s := testServer(ms)
		res, _, _ := s.handleExportHAR(ctx, nil, exportHARInput{RequestIDs: []string{"a"}})
		text := res.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, "https://a/") {
			t.Error("expected entry for a")
		}
		if strings.Contains(text, "https://b/") {
			t.Error("should not contain b")
		}
	})

	t.Run("skips_missing_ids", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, _ := s.handleExportHAR(ctx, nil, exportHARInput{RequestIDs: []string{"gone"}})
		text := res.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, `"entries": []`) {
			t.Error("expected empty entries")
		}
	})
}

// --- handleSetThrottle ---

func TestHandleSetThrottle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("nil_throttle", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, _ := s.handleSetThrottle(ctx, nil, setThrottleInput{BPS: 100})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("preset_3g", func(t *testing.T) {
		tc := &mockThrottle{}
		s := testServer(&mockStore{}, withThrottle(tc))
		res, _, _ := s.handleSetThrottle(ctx, nil, setThrottleInput{Preset: "3g"})
		if res.IsError {
			t.Fatal("unexpected error")
		}
		if tc.bps != 93750 {
			t.Errorf("bps = %d, want 93750", tc.bps)
		}
	})

	t.Run("unknown_preset", func(t *testing.T) {
		tc := &mockThrottle{}
		s := testServer(&mockStore{}, withThrottle(tc))
		res, _, _ := s.handleSetThrottle(ctx, nil, setThrottleInput{Preset: "5g"})
		if !res.IsError {
			t.Error("expected error for unknown preset")
		}
	})

	t.Run("custom_bps_latency", func(t *testing.T) {
		tc := &mockThrottle{}
		s := testServer(&mockStore{}, withThrottle(tc))
		res, _, _ := s.handleSetThrottle(ctx, nil, setThrottleInput{BPS: 500, LatencyMS: 100})
		if res.IsError {
			t.Fatal("unexpected error")
		}
		if tc.bps != 500 {
			t.Errorf("bps = %d", tc.bps)
		}
		if tc.latency != 100*time.Millisecond {
			t.Errorf("latency = %v", tc.latency)
		}
	})
}

// --- handleGetThrottle ---

func TestHandleGetThrottle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("nil_throttle", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, _ := s.handleGetThrottle(ctx, nil, getThrottleInput{})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("active", func(t *testing.T) {
		s := testServer(&mockStore{}, withThrottle(&mockThrottle{bps: 1000}))
		res, _, _ := s.handleGetThrottle(ctx, nil, getThrottleInput{})
		var out map[string]any
		unmarshalResult(t, res, &out)
		if out["active"] != true {
			t.Error("expected active=true")
		}
	})

	t.Run("inactive", func(t *testing.T) {
		s := testServer(&mockStore{}, withThrottle(&mockThrottle{bps: 0}))
		res, _, _ := s.handleGetThrottle(ctx, nil, getThrottleInput{})
		var out map[string]any
		unmarshalResult(t, res, &out)
		if out["active"] != false {
			t.Error("expected active=false")
		}
	})
}

// --- handleMockResponse ---

func TestHandleMockResponse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("nil_scripts", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, _ := s.handleMockResponse(ctx, nil, mockResponseInput{MatchPattern: "*.example.com/*"})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("missing_pattern", func(t *testing.T) {
		s := testServer(&mockStore{}, withScripts(&mockScripts{dir: t.TempDir()}))
		res, _, _ := s.handleMockResponse(ctx, nil, mockResponseInput{})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("creates_script", func(t *testing.T) {
		dir := t.TempDir()
		sm := &mockScripts{
			dir:     dir,
			scripts: []scripting.ScriptInfo{{ID: "mock1", FilePath: "placeholder"}},
		}
		s := testServer(&mockStore{}, withScripts(sm))
		res, _, _ := s.handleMockResponse(ctx, nil, mockResponseInput{
			MatchPattern: "*.example.com/*",
			Status:       201,
			Headers:      map[string]string{"X-Custom": "val"},
			Body:         "mock body",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %v", res.Content)
		}
		if sm.reloaded != 1 {
			t.Error("Reload not called")
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) == 0 {
			t.Fatal("no script file created")
		}
		content, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
		if !strings.Contains(string(content), "respondWith") {
			t.Error("script missing respondWith call")
		}
		if !strings.Contains(string(content), "201") {
			t.Error("script missing status 201")
		}
	})
}

// --- handleListScripts ---

func TestHandleListScripts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("nil_scripts", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, _ := s.handleListScripts(ctx, nil, listScriptsInput{})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("returns_summaries", func(t *testing.T) {
		sm := &mockScripts{scripts: []scripting.ScriptInfo{
			{ID: "s1", Name: "test", Matches: []string{"*"}, Enabled: true,
				Categories: []scripting.ScriptCategory{scripting.CategoryScript}},
		}}
		s := testServer(&mockStore{}, withScripts(sm))
		res, _, _ := s.handleListScripts(ctx, nil, listScriptsInput{})
		var out []scriptSummary
		unmarshalResult(t, res, &out)
		if len(out) != 1 || out[0].ScriptID != "s1" {
			t.Errorf("got %+v", out)
		}
	})
}

// --- handleCreateScript ---

func TestHandleCreateScript(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("nil_scripts", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, _ := s.handleCreateScript(ctx, nil, createScriptInput{Name: "x", MatchPatterns: []string{"*"}, Code: "x"})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("missing_name", func(t *testing.T) {
		s := testServer(&mockStore{}, withScripts(&mockScripts{dir: t.TempDir()}))
		res, _, _ := s.handleCreateScript(ctx, nil, createScriptInput{MatchPatterns: []string{"*"}, Code: "x"})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("missing_patterns", func(t *testing.T) {
		s := testServer(&mockStore{}, withScripts(&mockScripts{dir: t.TempDir()}))
		res, _, _ := s.handleCreateScript(ctx, nil, createScriptInput{Name: "n", Code: "x"})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("missing_code", func(t *testing.T) {
		s := testServer(&mockStore{}, withScripts(&mockScripts{dir: t.TempDir()}))
		res, _, _ := s.handleCreateScript(ctx, nil, createScriptInput{Name: "n", MatchPatterns: []string{"*"}})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("creates_file", func(t *testing.T) {
		dir := t.TempDir()
		sm := &mockScripts{dir: dir, scripts: []scripting.ScriptInfo{
			{ID: "new1", FilePath: "will-match-after-reload"},
		}}
		s := testServer(&mockStore{}, withScripts(sm))
		res, _, _ := s.handleCreateScript(ctx, nil, createScriptInput{
			Name: "my-script", MatchPatterns: []string{"*.com/*"}, Code: "function onRequest(ctx) {}", Enabled: true,
		})
		if res.IsError {
			t.Fatal("unexpected error")
		}
		if sm.reloaded != 1 {
			t.Error("Reload not called")
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) == 0 {
			t.Fatal("no file created")
		}
		data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
		if !strings.Contains(string(data), "my-script") {
			t.Error("missing script name in header")
		}
	})
}

// --- handleGetScript ---

func TestHandleGetScript(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("missing_id", func(t *testing.T) {
		s := testServer(&mockStore{}, withScripts(&mockScripts{}))
		res, _, _ := s.handleGetScript(ctx, nil, getScriptInput{})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		s := testServer(&mockStore{}, withScripts(&mockScripts{}))
		res, _, _ := s.handleGetScript(ctx, nil, getScriptInput{ScriptID: "nope"})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("returns_source", func(t *testing.T) {
		dir := t.TempDir()
		scriptContent := `// ---
// name: test-script
// match:
//   - "*.com/*"
// enabled: true
// ---

function onRequest(ctx) { ctx.respondWith({status: 200}); }
`
		path := filepath.Join(dir, "test.js")
		os.WriteFile(path, []byte(scriptContent), 0o644)

		sm := &mockScripts{scripts: []scripting.ScriptInfo{
			{ID: "s1", Name: "test-script", FilePath: path, Enabled: true},
		}}
		s := testServer(&mockStore{}, withScripts(sm))
		res, _, _ := s.handleGetScript(ctx, nil, getScriptInput{ScriptID: "s1"})
		if res.IsError {
			t.Fatal("unexpected error")
		}
		var out map[string]any
		unmarshalResult(t, res, &out)
		if out["name"] != "test-script" {
			t.Errorf("name = %v", out["name"])
		}
		source, _ := out["source"].(string)
		if !strings.Contains(source, "respondWith") {
			t.Error("source missing body")
		}
	})
}

// --- handleToggleScript ---

func TestHandleToggleScript(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("nil_scripts", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, _ := s.handleToggleScript(ctx, nil, toggleScriptInput{ScriptID: "x"})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("missing_id", func(t *testing.T) {
		s := testServer(&mockStore{}, withScripts(&mockScripts{}))
		res, _, _ := s.handleToggleScript(ctx, nil, toggleScriptInput{})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		s := testServer(&mockStore{}, withScripts(&mockScripts{}))
		res, _, _ := s.handleToggleScript(ctx, nil, toggleScriptInput{ScriptID: "nope"})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("toggles", func(t *testing.T) {
		dir := t.TempDir()
		scriptContent := `// ---
// name: toggler
// match:
//   - "*"
// enabled: false
// ---

function onRequest(ctx) {}
`
		path := filepath.Join(dir, "toggler.js")
		os.WriteFile(path, []byte(scriptContent), 0o644)

		sm := &mockScripts{scripts: []scripting.ScriptInfo{
			{ID: "t1", Name: "toggler", FilePath: path, Enabled: true},
		}}
		s := testServer(&mockStore{}, withScripts(sm))
		res, _, _ := s.handleToggleScript(ctx, nil, toggleScriptInput{ScriptID: "t1"})
		if res.IsError {
			t.Fatal("unexpected error")
		}
		if sm.toggled != path {
			t.Errorf("toggled = %q, want %q", sm.toggled, path)
		}
	})
}

// --- handleDeleteScript ---

func TestHandleDeleteScript(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("nil_scripts", func(t *testing.T) {
		s := testServer(&mockStore{})
		res, _, _ := s.handleDeleteScript(ctx, nil, deleteScriptInput{ScriptID: "x"})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("missing_id", func(t *testing.T) {
		s := testServer(&mockStore{}, withScripts(&mockScripts{}))
		res, _, _ := s.handleDeleteScript(ctx, nil, deleteScriptInput{})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		s := testServer(&mockStore{}, withScripts(&mockScripts{}))
		res, _, _ := s.handleDeleteScript(ctx, nil, deleteScriptInput{ScriptID: "nope"})
		if !res.IsError {
			t.Error("expected error")
		}
	})

	t.Run("deletes", func(t *testing.T) {
		sm := &mockScripts{scripts: []scripting.ScriptInfo{
			{ID: "d1", Name: "deleteme", FilePath: "/tmp/deleteme.js"},
		}}
		s := testServer(&mockStore{}, withScripts(sm))
		res, _, _ := s.handleDeleteScript(ctx, nil, deleteScriptInput{ScriptID: "d1"})
		if res.IsError {
			t.Fatal("unexpected error")
		}
		if sm.deleted != "/tmp/deleteme.js" {
			t.Errorf("deleted = %q", sm.deleted)
		}
		var out map[string]any
		unmarshalResult(t, res, &out)
		if out["deleted"] != true {
			t.Error("expected deleted=true")
		}
	})
}

// --- buildHAR ---

func TestBuildHAR(t *testing.T) {
	t.Parallel()

	t.Run("structure", func(t *testing.T) {
		ms := &mockStore{
			metas: []store.FlowMeta{{
				ID: "h1", Method: "POST", Host: "api.test", Path: "/data",
				StatusCode: 201, State: store.StateCompleted,
				StartedAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
				Duration: 50 * time.Millisecond, Scheme: "https", ContentType: "application/json",
			}},
			data: map[store.FlowID]*store.FlowData{
				"h1": {
					RequestHeaders:  http.Header{"Content-Type": {"application/json"}},
					RequestBody:     []byte(`{"a":1}`),
					ResponseHeaders: http.Header{"X-Custom": {"val"}},
					ResponseBody:    []byte(`{"b":2}`),
				},
			},
		}

		har := buildHAR(ms, ms.metas)
		log, _ := har["log"].(map[string]any)
		if log["version"] != "1.2" {
			t.Error("missing version")
		}
		entries, _ := log["entries"].([]map[string]any)
		if len(entries) != 1 {
			t.Fatalf("entries = %d", len(entries))
		}
		req, _ := entries[0]["request"].(map[string]any)
		if req["method"] != "POST" {
			t.Error("method mismatch")
		}
		if req["url"] != "https://api.test/data" {
			t.Errorf("url = %v", req["url"])
		}
		if req["postData"] == nil {
			t.Error("postData missing for POST with body")
		}
	})

	t.Run("skips_missing", func(t *testing.T) {
		ms := &mockStore{}
		metas := []store.FlowMeta{{ID: "gone", Scheme: "https", Host: "x", Path: "/"}}
		har := buildHAR(ms, metas)
		log, _ := har["log"].(map[string]any)
		entries, _ := log["entries"].([]map[string]any)
		if len(entries) != 0 {
			t.Error("should skip missing")
		}
	})
}

// --- test helpers ---

// unmarshalResult extracts the JSON text from a CallToolResult and decodes it into dst.
func unmarshalResult(t *testing.T, res *mcp.CallToolResult, dst any) {
	t.Helper()
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var raw struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal result wrapper: %v", err)
	}
	if len(raw.Content) == 0 {
		t.Fatal("no content in result")
	}
	if err := json.Unmarshal([]byte(raw.Content[0].Text), dst); err != nil {
		t.Fatalf("unmarshal content text: %v\ntext: %s", err, raw.Content[0].Text)
	}
}
