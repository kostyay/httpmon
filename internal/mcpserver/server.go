package mcpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FlowReader is the read-only port for accessing captured flows.
type FlowReader interface {
	List(filter store.Filter, offset, limit int) ([]store.FlowMeta, int)
	Get(id store.FlowID) (*store.FlowMeta, *store.FlowData, error)
}

// ProxyInfo exposes proxy status.
type ProxyInfo interface {
	Addr() string
}

// ScriptManager exposes script operations.
type ScriptManager interface {
	Scripts() []scripting.ScriptInfo
	ScriptByID(id string) (scripting.ScriptInfo, bool)
	Toggle(filePath string) error
	Delete(filePath string) error
	CreateNew() (string, error)
	QuickAddMapLocal(pattern, localPath string) (string, error)
	ScriptDir() string
	Reload()
}

// ThrottleController manages bandwidth throttling.
type ThrottleController interface {
	SetThrottle(bps int64, latency time.Duration)
	GetThrottleBPS() int64
	GetThrottleLatency() time.Duration
}

// Config holds dependencies for the MCP server.
type Config struct {
	Store    FlowReader
	Proxy    ProxyInfo
	Scripts  ScriptManager
	Throttle ThrottleController
	Addr     string // listen address (default "127.0.0.1:9551")
	Token    string // bearer token for auth
}

// Server is the MCP server that exposes httpmon tools to LLM agents.
type Server struct {
	mu      sync.Mutex
	cfg     Config
	srv     *http.Server
	handler *mcp.StreamableHTTPHandler
	running bool
	addr    string
}

// DefaultAddr is the default MCP server listen address.
const DefaultAddr = "127.0.0.1:9551"

// New creates an MCP server with the given config.
func New(cfg Config) *Server {
	addr := cfg.Addr
	if addr == "" {
		addr = DefaultAddr
	}
	return &Server{cfg: cfg, addr: addr}
}

// Start begins serving on the configured address.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("mcp server already running")
	}

	mcpServer := mcp.NewServer(
		&mcp.Implementation{Name: "httpmon", Version: "1.0.0"},
		nil,
	)

	s.registerTools(mcpServer)

	s.handler = mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return mcpServer },
		nil,
	)

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}

	// Store actual addr (useful if port was 0 for tests).
	s.addr = ln.Addr().String()

	var handler http.Handler = s.handler
	handler = bearerAuthMiddleware(s.cfg.Token, handler)

	s.srv = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.running = true

	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	go func() {
		_ = s.srv.Serve(ln) // ErrServerClosed on graceful shutdown; other errors are non-recoverable.
	}()

	return nil
}

// Stop shuts down the MCP server.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
	s.running = false
}

// Running reports whether the server is active.
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Port returns the actual port the server is listening on.
func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, portStr, err := net.SplitHostPort(s.addr)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return port
}

// Addr returns the full listen address.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// registerTools adds all MCP tools to the server.
func (s *Server) registerTools(srv *mcp.Server) {
	s.registerReadTools(srv)
	if s.cfg.Throttle != nil || s.cfg.Proxy != nil {
		s.registerSimTools(srv)
	}
	if s.cfg.Scripts != nil {
		s.registerScriptTools(srv)
	}
}
