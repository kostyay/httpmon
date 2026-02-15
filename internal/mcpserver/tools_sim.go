package mcpserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kostyay/httpmon/internal/throttle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerSimTools adds simulation tools to the MCP server.
func (s *Server) registerSimTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "set_throttle",
		Description: "Set bandwidth throttling and/or added latency. Use preset (3g/4g/wifi) or custom bps.",
	}, s.handleSetThrottle)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_throttle",
		Description: "Get current throttle settings.",
	}, s.handleGetThrottle)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "replay_request",
		Description: "Replay an existing request by ID, or compose a new request to send through the proxy.",
	}, s.handleReplayRequest)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "mock_response",
		Description: "Create a mock response rule that intercepts matching requests and returns a synthetic response.",
	}, s.handleMockResponse)
}

// --- set_throttle ---

type setThrottleInput struct {
	BPS       int64  `json:"bps,omitempty" jsonschema:"bytes per second (0 to disable)"`
	LatencyMS int    `json:"latency_ms,omitempty" jsonschema:"added latency in ms (0 to disable)"`
	Preset    string `json:"preset,omitempty" jsonschema:"throttle preset: 3g, 4g, wifi"`
}

func (s *Server) handleSetThrottle(
	_ context.Context, _ *mcp.CallToolRequest, in setThrottleInput,
) (*mcp.CallToolResult, any, error) {
	if s.cfg.Throttle == nil {
		return errorResult("throttle not available"), nil, nil
	}

	bps := in.BPS
	if in.Preset != "" {
		presetBPS := throttle.PresetBandwidth(in.Preset)
		if presetBPS == 0 {
			return errorResult(fmt.Sprintf("unknown preset: %q", in.Preset)), nil, nil
		}
		bps = presetBPS
	}

	latency := time.Duration(in.LatencyMS) * time.Millisecond
	s.cfg.Throttle.SetThrottle(bps, latency)

	return jsonResult(map[string]any{
		"bps":        bps,
		"latency_ms": in.LatencyMS,
	}), nil, nil
}

// --- get_throttle ---

type getThrottleInput struct{}

func (s *Server) handleGetThrottle(
	_ context.Context, _ *mcp.CallToolRequest, _ getThrottleInput,
) (*mcp.CallToolResult, any, error) {
	if s.cfg.Throttle == nil {
		return errorResult("throttle not available"), nil, nil
	}

	bps := s.cfg.Throttle.GetThrottleBPS()
	latency := s.cfg.Throttle.GetThrottleLatency()

	return jsonResult(map[string]any{
		"bps":        bps,
		"latency_ms": latency.Milliseconds(),
		"active":     bps > 0 || latency > 0,
	}), nil, nil
}

// --- replay_request ---

type replayRequestInput struct {
	// Replay existing
	RequestID string `json:"request_id,omitempty" jsonschema:"ID of existing request to replay"`
	// Compose new
	Method  string            `json:"method,omitempty" jsonschema:"HTTP method for composed request"`
	URL     string            `json:"url,omitempty" jsonschema:"full URL for composed request"`
	Headers map[string]string `json:"headers,omitempty" jsonschema:"headers for composed request"`
	Body    string            `json:"body,omitempty" jsonschema:"body for composed request"`
}

func (s *Server) handleReplayRequest(
	_ context.Context, _ *mcp.CallToolRequest, in replayRequestInput,
) (*mcp.CallToolResult, any, error) {
	var method, targetURL string
	var headers map[string]string
	var body string

	if in.RequestID != "" {
		// Replay existing request.
		meta, data, err := s.cfg.Store.Get(in.RequestID)
		if err != nil {
			return errorResult(fmt.Sprintf("request not found: %s", in.RequestID)), nil, nil
		}
		method = meta.Method
		targetURL = meta.Scheme + "://" + meta.Host + meta.Path
		if data != nil {
			headers = headerMap(data.RequestHeaders)
			body = string(data.RequestBody)
		}
	} else if in.URL != "" {
		method = in.Method
		if method == "" {
			method = "GET"
		}
		targetURL = in.URL
		headers = in.Headers
		body = in.Body
	} else {
		return errorResult("either request_id or url is required"), nil, nil
	}

	// Route through the proxy.
	proxyURL, _ := url.Parse("http://" + s.cfg.Proxy.Addr())
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // proxy MITM CA
		},
		Timeout: 30 * time.Second,
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, targetURL, bodyReader)
	if err != nil {
		return errorResult(fmt.Sprintf("build request: %v", err)), nil, nil
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return errorResult(fmt.Sprintf("send request: %v", err)), nil, nil
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return jsonResult(map[string]any{
		"status_code": resp.StatusCode,
	}), nil, nil
}

// --- mock_response ---

type mockResponseInput struct {
	MatchPattern string            `json:"match_pattern" jsonschema:"URL glob pattern to match (required)"`
	Status       int               `json:"status,omitempty" jsonschema:"response status code (default 200)"`
	Headers      map[string]string `json:"headers,omitempty" jsonschema:"response headers"`
	Body         string            `json:"body,omitempty" jsonschema:"response body"`
}

func (s *Server) handleMockResponse(
	_ context.Context, _ *mcp.CallToolRequest, in mockResponseInput,
) (*mcp.CallToolResult, any, error) {
	if s.cfg.Scripts == nil {
		return errorResult("scripts not available"), nil, nil
	}
	if in.MatchPattern == "" {
		return errorResult("match_pattern is required"), nil, nil
	}

	status := in.Status
	if status == 0 {
		status = 200
	}

	// Build inline JS that uses respondWith.
	headersJS := ""
	if len(in.Headers) > 0 {
		pairs := make([]string, 0, len(in.Headers))
		for k, v := range in.Headers {
			pairs = append(pairs, fmt.Sprintf("    %q: %q", k, v))
		}
		headersJS = fmt.Sprintf(",\n  headers: {\n%s\n  }", strings.Join(pairs, ",\n"))
	}

	bodyJS := ""
	if in.Body != "" {
		bodyJS = fmt.Sprintf(",\n  body: %q", in.Body)
	}

	source := fmt.Sprintf(`function onRequest(ctx) {
  ctx.respondWith({
    status: %d%s%s
  });
}
`, status, headersJS, bodyJS)

	// Write via script manager's CreateNew + write manually since
	// QuickAddMapLocal is for file-based mocks. We'll create the script directly.
	dir := s.cfg.Scripts.ScriptDir()
	path, err := createMockScript(dir, in.MatchPattern, source)
	if err != nil {
		return errorResult(fmt.Sprintf("create mock: %v", err)), nil, nil
	}
	s.cfg.Scripts.Reload()

	return jsonResult(map[string]any{
		"script_path": path,
		"name":        "mock",
	}), nil, nil
}

// createMockScript writes a mock script with header to the scripts directory.
func createMockScript(dir, pattern, jsSource string) (string, error) {
	if err := ensureDir(dir); err != nil {
		return "", err
	}

	content := fmt.Sprintf(`// ---
// name: mock-%s
// match:
//   - "%s"
// enabled: true
// ---

%s`, slugify(pattern), pattern, jsSource)

	f, err := createTempFile(dir, "mock-*.js")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	return path, nil
}

func slugify(s string) string {
	r := strings.NewReplacer(
		"*://", "", "http://", "", "https://", "",
		"*", "", "/", "-", ".", "-",
	)
	out := strings.Trim(r.Replace(s), "-")
	if len(out) > 30 {
		out = out[:30]
	}
	if out == "" {
		out = "rule"
	}
	return out
}
