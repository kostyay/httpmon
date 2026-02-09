package proxy

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	mp "github.com/lqqyt2423/go-mitmproxy/proxy"

	"github.com/kostyay/httpmon/internal/store"
)

// Proxy wraps go-mitmproxy and captures traffic into a RingBuffer.
type Proxy struct {
	mp    *mp.Proxy
	store *store.RingBuffer
	addr  string

	// caDir is the directory containing CA cert files.
	caDir string

	// SslInsecure skips TLS verification for upstream servers.
	SslInsecure bool
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

	proxy.AddAddon(newInterceptor(p.store))
	p.mp = proxy
	p.addr = addr
	return nil
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
	if p.mp == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = p.mp.Shutdown(ctx)
}

// Addr returns the listen address passed to Init.
func (p *Proxy) Addr() string {
	return p.addr
}

// CACertPath returns the path to the CA certificate file for user trust.
func (p *Proxy) CACertPath() string {
	return filepath.Join(p.caDir, "mitmproxy-ca-cert.pem")
}
