//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kostyay/httpmon/internal/procinfo"
	"github.com/kostyay/httpmon/internal/proxy"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/kostyay/httpmon/internal/tui"
)

// harness wires a real upstream server, MITM proxy, store, and TUI app.
type harness struct {
	upstream  *httptest.Server
	proxy     *proxy.Proxy
	store     *store.RingBuffer
	app       *tui.App
	client    *http.Client
	cancel    context.CancelFunc
	proxyAddr string
}

func newHarness(t *testing.T, handler http.Handler) *harness {
	t.Helper()

	upstream := httptest.NewServer(handler)

	s := store.New(100)
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	p := proxy.New(s, t.TempDir())
	p.SslInsecure = true
	p.Resolver = procinfo.New(s)
	if err := p.Init(addr); err != nil {
		upstream.Close()
		t.Fatalf("proxy init: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = p.Serve(ctx) }()

	waitForListener(t, addr)

	app := tui.NewApp(tui.AppConfig{
		Store:     s,
		Proxy:     p,
		CATrusted: true,
		Throttle:  p,
	})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", addr))
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test proxy CA
		},
		Timeout: 10 * time.Second,
	}

	h := &harness{
		upstream:  upstream,
		proxy:     p,
		store:     s,
		app:       app,
		client:    client,
		cancel:    cancel,
		proxyAddr: addr,
	}

	t.Cleanup(func() {
		cancel()
		p.Stop()
		upstream.Close()
	})

	return h
}

// tick sends a tick message to refresh the TUI's flow list.
func (h *harness) tick() {
	h.app.Update(tui.TickMsg(time.Now()))
}

// view returns the ANSI-stripped TUI output.
func (h *harness) view() string {
	v := h.app.View()
	return ansi.Strip(fmt.Sprint(v.Content))
}

// sendKey sends a single character key press.
func (h *harness) sendKey(key string) {
	h.app.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
}

// sendSpecialKey sends a non-character key (Enter, Esc, Tab, etc.).
func (h *harness) sendSpecialKey(code rune) {
	h.app.Update(tea.KeyPressMsg{Code: code})
}

// typeText types each character as a key press.
func (h *harness) typeText(s string) {
	for _, ch := range s {
		h.app.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
}

// doGet makes a GET request through the proxy to the upstream server.
func (h *harness) doGet(t *testing.T, path string) {
	t.Helper()
	resp, err := h.client.Get(h.upstream.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
}

// doPost makes a POST request with the given JSON body.
func (h *harness) doPost(t *testing.T, path, body string) {
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

// waitForText ticks the app repeatedly until the view contains text.
func (h *harness) waitForText(t *testing.T, text string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h.tick()
		if strings.Contains(h.view(), text) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in view:\n%s", text, h.view())
}

// tryWaitForProcess ticks until a flow's Process field is
// populated. Returns true if resolved within timeout.
func (h *harness) tryWaitForProcess(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		h.tick()
		flows, _ := h.store.List(nil, 0, 0)
		for _, f := range flows {
			if f.Process != "" && f.Process != "\u2014" {
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// waitForCondition ticks until fn returns true.
func (h *harness) waitForCondition(t *testing.T, desc string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h.tick()
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for condition: %s\nview:\n%s", desc, h.view())
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func waitForListener(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("proxy never started on %s", addr)
}

// echoHandler returns request method and path in the response body.
func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom", "test-header")
		fmt.Fprintf(w, `{"method":"%s","path":"%s"}`, r.Method, r.URL.Path)
	})
}

// slowHandler waits before responding (for in-progress tests).
func slowHandler(delay time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "slow response")
	})
}

// multiHandler dispatches to sub-handlers by path prefix.
func multiHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom", "test-header")
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, `{"method":"%s","path":"%s","body":%q}`,
			r.Method, r.URL.Path, string(body))
	})
	mux.HandleFunc("/status/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		code := 200
		if len(parts) > 0 {
			fmt.Sscanf(parts[len(parts)-1], "%d", &code)
		}
		w.WriteHeader(code)
		fmt.Fprintf(w, "status %d", code)
	})
	mux.HandleFunc("/binary", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte{0x00, 0x01, 0xFF, 0xFE, 0x89, 0x50, 0x4E, 0x47})
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[1,2,3],"ok":true}`)
	})
	mux.HandleFunc("/large", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write(bytes.Repeat([]byte("x"), 4096))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "ok")
	})
	return mux
}
