package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/kostyay/httpmon/internal/store"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func setupProxy(t *testing.T) (*Proxy, *store.RingBuffer, int) {
	t.Helper()
	s := store.New(1000)
	dataDir := t.TempDir()
	p := New(s, dataDir)
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if err := p.Init(addr); err != nil {
		t.Fatalf("Init: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		p.Stop()
	})
	go p.Serve(ctx)

	// Wait for proxy to be ready
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return p, s, port
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("proxy did not start in time")
	return nil, nil, 0
}

func proxyClient(port int) *http.Client {
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	return &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}
}

func TestHTTPFlowCapture(t *testing.T) {
	// Start an HTTP test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer ts.Close()

	_, s, port := setupProxy(t)
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/v1/test")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	// Give interceptor time to process
	time.Sleep(200 * time.Millisecond)

	if s.Len() == 0 {
		t.Fatal("expected at least 1 flow in store")
	}

	flows, _ := s.List(nil, 0, 0)
	found := false
	for _, f := range flows {
		if f.Path == "/v1/test" && f.Method == "GET" {
			found = true
			if f.StatusCode != 200 {
				t.Errorf("StatusCode = %d, want 200", f.StatusCode)
			}
			if f.State != store.StateCompleted {
				t.Errorf("State = %d, want Completed", f.State)
			}
			if f.Duration == 0 {
				t.Error("Duration should be > 0")
			}
			break
		}
	}
	if !found {
		t.Error("flow for /v1/test not found in store")
	}
}

func TestConcurrentRequests(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	_, s, port := setupProxy(t)
	client := proxyClient(port)

	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := client.Get(fmt.Sprintf("%s/req/%d", ts.URL, i))
			if err != nil {
				return
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}(i)
	}
	wg.Wait()

	time.Sleep(500 * time.Millisecond)

	if s.Len() < n/2 {
		t.Errorf("expected at least %d flows, got %d", n/2, s.Len())
	}
}

func TestBodyTruncation(t *testing.T) {
	// Create a response body larger than maxBodySize
	bigBody := make([]byte, maxBodySize+1000)
	for i := range bigBody {
		bigBody[i] = 'A'
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		w.Write(bigBody)
	}))
	defer ts.Close()

	_, s, port := setupProxy(t)
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/big")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	time.Sleep(300 * time.Millisecond)

	flows, _ := s.List(nil, 0, 0)
	for _, f := range flows {
		if f.Path == "/big" {
			_, data, err := s.Get(f.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if data == nil {
				t.Fatal("expected data for /big flow")
			}
			if len(data.ResponseBody) > maxBodySize {
				t.Errorf("response body len = %d, want <= %d", len(data.ResponseBody), maxBodySize)
			}
			return
		}
	}
	t.Error("flow for /big not found")
}

func TestInitPortZero(t *testing.T) {
	s := store.New(10)
	p := New(s, t.TempDir())
	err := p.Init("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Init with port 0: %v", err)
	}
	p.Stop()
}
