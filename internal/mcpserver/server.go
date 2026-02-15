package mcpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
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
	Port     int // default 9551
}

// Server is the MCP server that exposes httpmon tools to LLM agents.
type Server struct {
	mu      sync.Mutex
	cfg     Config
	srv     *http.Server
	handler *mcp.StreamableHTTPHandler
	running bool
	port    int
}

// DefaultPort is the default MCP server port.
const DefaultPort = 9551

// New creates an MCP server with the given config.
func New(cfg Config) *Server {
	port := cfg.Port
	if port == 0 {
		port = DefaultPort
	}
	return &Server{cfg: cfg, port: port}
}

// Start begins serving on the configured port.
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

	addr := fmt.Sprintf(":%d", s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	// Store actual port (useful if port was 0 for tests).
	s.port = ln.Addr().(*net.TCPAddr).Port

	s.srv = &http.Server{Handler: s.handler}
	s.running = true

	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	go func() {
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			// Log but don't crash.
		}
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
	return s.port
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
