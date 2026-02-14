package proxy

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	mp "github.com/lqqyt2423/go-mitmproxy/proxy"
	log "github.com/sirupsen/logrus"

	"github.com/kostyay/httpmon/internal/breakpoint"
	"github.com/kostyay/httpmon/internal/certutil"
	"github.com/kostyay/httpmon/internal/hostfilter"
	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/kostyay/httpmon/internal/store"
)

// Proxy wraps go-mitmproxy and captures traffic into a RingBuffer.
type Proxy struct {
	mp    *mp.Proxy
	store *store.RingBuffer
	addr  string
	intc  *interceptor // kept for runtime throttle changes

	// caDir is the directory containing CA cert files.
	caDir string

	// caCertPath is set after EnsureCA generates/loads the cert.
	caCertPath string

	// logFile holds the open log file handle (closed on Stop).
	logFile *os.File

	// SslInsecure skips TLS verification for upstream servers.
	SslInsecure bool

	// HostFilter controls which hosts are intercepted vs tunneled.
	HostFilter *hostfilter.HostFilter

	// ScriptEngine is optional; enables request/response rewriting via JS scripts.
	ScriptEngine *scripting.Engine

	// ThrottleBPS is initial bandwidth limit in bytes/sec (0 = unlimited).
	ThrottleBPS int64

	// ThrottleLatency is initial per-response latency to add.
	ThrottleLatency time.Duration

	// BreakpointCtrl is optional; enables script breakpoints.
	BreakpointCtrl breakpoint.Controller
}

// New creates a Proxy that writes captured flows into the given store.
// dataDir is the base data directory (CA certs stored under dataDir/).
func New(s *store.RingBuffer, dataDir string) *Proxy {
	return &Proxy{
		store: s,
		caDir: dataDir,
	}
}

const maxBodySize = 5 * 1024 * 1024 // 5 MB

// Init sets up the MITM proxy (CA generation + port validation).
// This must be called before Serve. addr is e.g. ":8080".
func (p *Proxy) Init(addr string) error {
	// Redirect go-mitmproxy's logrus output to a file inside dataDir.
	logPath := filepath.Join(p.caDir, "proxy.log")
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- path derived from internal dataDir
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	p.logFile = lf
	log.SetOutput(lf)

	certPath, err := certutil.EnsureCA(p.caDir)
	if err != nil {
		return fmt.Errorf("ensure CA: %w", err)
	}
	p.caCertPath = certPath

	opts := &mp.Options{
		Addr:              addr,
		StreamLargeBodies: maxBodySize,
		CaRootPath:        p.caDir,
		SslInsecure:       p.SslInsecure,
	}

	proxy, err := mp.NewProxy(opts)
	if err != nil {
		return fmt.Errorf("proxy init: %w", err)
	}

	if p.ScriptEngine != nil && p.BreakpointCtrl != nil {
		p.ScriptEngine.SetBreakpointController(p.BreakpointCtrl)
	}

	p.intc = newInterceptor(interceptorConfig{
		Store:           p.store,
		Engine:          p.ScriptEngine,
		ThrottleBPS:     p.ThrottleBPS,
		ThrottleLatency: p.ThrottleLatency,
	})
	proxy.AddAddon(p.intc)
	p.mp = proxy
	p.addr = addr

	if p.HostFilter != nil {
		proxy.SetShouldInterceptRule(func(req *http.Request) bool {
			return p.HostFilter.ShouldIntercept(req.Host)
		})
	}

	return nil
}

// SetThrottle updates throttle settings at runtime.
func (p *Proxy) SetThrottle(bps int64, latency time.Duration) {
	if p.intc != nil {
		p.intc.SetThrottle(bps, latency)
	}
}

// GetThrottleBPS returns current bandwidth limit.
func (p *Proxy) GetThrottleBPS() int64 {
	if p.intc != nil {
		return p.intc.ThrottleBPS()
	}
	return 0
}

// GetThrottleLatency returns current latency setting.
func (p *Proxy) GetThrottleLatency() time.Duration {
	if p.intc != nil {
		return p.intc.ThrottleLatency()
	}
	return 0
}

// Serve starts the proxy accept loop. Blocks until ctx is cancelled or error.
func (p *Proxy) Serve(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- p.mp.Start()
	}()

	select {
	case <-ctx.Done():
		p.Stop()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// Stop gracefully shuts down the proxy.
func (p *Proxy) Stop() {
	if p.BreakpointCtrl != nil {
		p.BreakpointCtrl.ResumeAll()
	}
	if p.mp == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = p.mp.Shutdown(ctx)
	if p.logFile != nil {
		_ = p.logFile.Close()
	}
}

// Addr returns the listen address passed to Init.
func (p *Proxy) Addr() string {
	return p.addr
}

// CACertPath returns the path to the CA certificate file for user trust.
func (p *Proxy) CACertPath() string {
	return p.caCertPath
}
