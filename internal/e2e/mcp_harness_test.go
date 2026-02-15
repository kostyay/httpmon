//go:build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kostyay/httpmon/internal/mcpserver"
	"github.com/kostyay/httpmon/internal/proxy"
	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpHarness extends the base harness with MCP server + client.
type mcpHarness struct {
	upstream  *httptest.Server
	proxy     *proxy.Proxy
	store     *store.RingBuffer
	client    *http.Client
	mcpSrv    *mcpserver.Server
	session   *mcp.ClientSession
	cancel    context.CancelFunc
	proxyAddr string
	scriptsDir string
}

func newMCPHarness(t *testing.T, handler http.Handler) *mcpHarness {
	t.Helper()

	upstream := httptest.NewServer(handler)

	s := store.New(100)
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	p := proxy.New(s, t.TempDir())
	p.SslInsecure = true

	scriptsDir := filepath.Join(t.TempDir(), "scripts")
	engine := scripting.New()
	engine.LoadFromDir(scriptsDir)
	p.ScriptEngine = engine
	mgr := scripting.NewManager(engine, scriptsDir)

	if err := p.Init(addr); err != nil {
		upstream.Close()
		t.Fatalf("proxy init: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = p.Serve(ctx) }()

	waitForListener(t, addr)

	// Start MCP server on a free port.
	mcpPort := freePort(t)
	mcpSrv := mcpserver.New(mcpserver.Config{
		Store:    s,
		Proxy:    p,
		Scripts:  mgr,
		Throttle: p,
		Port:     mcpPort,
	})
	if err := mcpSrv.Start(ctx); err != nil {
		cancel()
		p.Stop()
		upstream.Close()
		t.Fatalf("mcp server start: %v", err)
	}

	// Wait for MCP server to be listening.
	mcpAddr := fmt.Sprintf("127.0.0.1:%d", mcpSrv.Port())
	waitForListener(t, mcpAddr)

	// Connect MCP client.
	mcpClient := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "1.0.0"},
		nil,
	)
	transport := &mcp.StreamableClientTransport{
		Endpoint: fmt.Sprintf("http://%s", mcpAddr),
	}

	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		cancel()
		mcpSrv.Stop()
		p.Stop()
		upstream.Close()
		t.Fatalf("mcp client connect: %v", err)
	}

	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", addr))
	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test proxy CA
		},
		Timeout: 10 * time.Second,
	}

	h := &mcpHarness{
		upstream:   upstream,
		proxy:      p,
		store:      s,
		client:     httpClient,
		mcpSrv:     mcpSrv,
		session:    session,
		cancel:     cancel,
		proxyAddr:  addr,
		scriptsDir: scriptsDir,
	}

	t.Cleanup(func() {
		session.Close()
		cancel()
		mcpSrv.Stop()
		p.Stop()
		upstream.Close()
	})

	return h
}

// doGet makes a GET through the proxy.
func (h *mcpHarness) doGet(t *testing.T, path string) {
	t.Helper()
	resp, err := h.client.Get(h.upstream.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
}

// doPost makes a POST through the proxy.
func (h *mcpHarness) doPost(t *testing.T, path, body string) {
	t.Helper()
	resp, err := h.client.Post(
		h.upstream.URL+path,
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
}

// callTool is a shorthand for session.CallTool.
func (h *mcpHarness) callTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := h.session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("callTool %s: %v", name, err)
	}
	return result
}

// resultText extracts the text from the first content block.
func resultText(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	if tc, ok := result.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

// waitForFlows waits until the store has at least n flows.
func (h *mcpHarness) waitForFlows(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if h.store.Len() >= n {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d flows (have %d)", n, h.store.Len())
}

func freePortMCP(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}
