package proxy

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
	"sync"
	"testing"
	"time"

	mp "github.com/lqqyt2423/go-mitmproxy/proxy"
	uuid "github.com/satori/go.uuid"

	"github.com/kostyay/httpmon/internal/scripting"
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

type setupOpt func(*Proxy)

func withSslInsecure() setupOpt {
	return func(p *Proxy) { p.SslInsecure = true }
}

func setupProxy(t *testing.T, opts ...setupOpt) (*Proxy, *store.RingBuffer, int) {
	t.Helper()
	s := store.New(1000)
	dataDir := t.TempDir()
	p := New(s, dataDir)
	for _, o := range opts {
		o(p)
	}
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

// findFlow polls the store for a flow matching the given path and returns its meta + data.
func findFlow(t *testing.T, s *store.RingBuffer, path string) (store.FlowMeta, *store.FlowData) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		flows, _ := s.List(nil, 0, 0)
		for _, f := range flows {
			if f.Path == path {
				_, d, _ := s.Get(f.ID)
				return f, d
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("flow for %s not found", path)
	return store.FlowMeta{}, nil
}

func TestHTTPSFlowCapture(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"secure":true}`)
	}))
	defer ts.Close()

	_, s, port := setupProxy(t, withSslInsecure())
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/v1/secure")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	meta, _ := findFlow(t, s, "/v1/secure")

	if meta.Scheme != "https" {
		t.Errorf("Scheme = %q, want https", meta.Scheme)
	}
	if meta.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", meta.StatusCode)
	}
	if meta.State != store.StateCompleted {
		t.Errorf("State = %d, want Completed", meta.State)
	}
	if meta.Method != "GET" {
		t.Errorf("Method = %q, want GET", meta.Method)
	}
}

func TestRequestResponseBodyCapture(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		// Echo request body back as response
		w.Write(body)
	}))
	defer ts.Close()

	_, s, port := setupProxy(t)
	client := proxyClient(port)

	reqBody := `{"name":"test"}`
	resp, err := client.Post(ts.URL+"/echo", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	meta, data := findFlow(t, s, "/echo")

	if meta.State != store.StateCompleted {
		t.Errorf("State = %d, want Completed", meta.State)
	}
	if data == nil {
		t.Fatal("expected flow data")
	}
	if string(data.RequestBody) != reqBody {
		t.Errorf("RequestBody = %q, want %q", data.RequestBody, reqBody)
	}
	if string(data.ResponseBody) != reqBody {
		t.Errorf("ResponseBody = %q, want %q", data.ResponseBody, reqBody)
	}
	if ct := data.RequestHeaders.Get("Content-Type"); ct != "application/json" {
		t.Errorf("request Content-Type = %q, want application/json", ct)
	}
	if ct := data.ResponseHeaders.Get("Content-Type"); ct != "application/json" {
		t.Errorf("response Content-Type = %q, want application/json", ct)
	}
}

func TestPOSTMethodCapture(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		fmt.Fprint(w, `{"created":true}`)
	}))
	defer ts.Close()

	_, s, port := setupProxy(t)
	client := proxyClient(port)

	body := `{"item":"widget"}`
	resp, err := client.Post(ts.URL+"/items", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	meta, data := findFlow(t, s, "/items")

	if meta.Method != "POST" {
		t.Errorf("Method = %q, want POST", meta.Method)
	}
	if meta.StatusCode != 201 {
		t.Errorf("StatusCode = %d, want 201", meta.StatusCode)
	}
	if data == nil {
		t.Fatal("expected flow data")
	}
	if string(data.RequestBody) != body {
		t.Errorf("RequestBody = %q, want %q", data.RequestBody, body)
	}
}

func TestServerErrorCapture(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(500)
		fmt.Fprint(w, "internal server error")
	}))
	defer ts.Close()

	_, s, port := setupProxy(t)
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/fail")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	meta, data := findFlow(t, s, "/fail")

	if meta.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", meta.StatusCode)
	}
	if meta.State != store.StateCompleted {
		t.Errorf("State = %d, want Completed (server responded)", meta.State)
	}
	if data == nil {
		t.Fatal("expected flow data")
	}
	if string(data.ResponseBody) != "internal server error" {
		t.Errorf("ResponseBody = %q, want %q", data.ResponseBody, "internal server error")
	}
}

func TestMultipleHeaderValues(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "a=1; Path=/")
		w.Header().Add("Set-Cookie", "b=2; Path=/")
		w.Header().Add("Set-Cookie", "c=3; Path=/")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	_, s, port := setupProxy(t)
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/cookies")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	_, data := findFlow(t, s, "/cookies")

	if data == nil {
		t.Fatal("expected flow data")
	}
	cookies := data.ResponseHeaders.Values("Set-Cookie")
	if len(cookies) != 3 {
		t.Errorf("Set-Cookie count = %d, want 3; values: %v", len(cookies), cookies)
	}
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

func withScriptEngine(e *scripting.Engine) setupOpt {
	return func(p *Proxy) { p.ScriptEngine = e }
}

func TestAddrAndCACertPath(t *testing.T) {
	p, _, _ := setupProxy(t)
	if p.Addr() == "" {
		t.Error("Addr() should be non-empty after Init")
	}
	if p.CACertPath() == "" {
		t.Error("CACertPath() should be non-empty after Init")
	}
}

func TestRequestBodyTruncation(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(s, nil)

	flowID := uuid.NewV4()
	reqURL, _ := url.Parse("http://example.com/bigpost")

	bigBody := make([]byte, maxBodySize+1000)
	for i := range bigBody {
		bigBody[i] = 'B'
	}

	f := &mp.Flow{
		Id: flowID,
		Request: &mp.Request{
			Method: "POST",
			URL:    reqURL,
			Header: http.Header{"Content-Type": {"application/octet-stream"}},
			Body:   bigBody,
		},
	}

	ic.Requestheaders(f)
	ic.Request(f)

	_, data, err := s.Get(store.FlowID(flowID.String()))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if data == nil {
		t.Fatal("expected flow data")
	}
	if len(data.RequestBody) != maxBodySize {
		t.Errorf("request body len = %d, want exactly %d (truncated)", len(data.RequestBody), maxBodySize)
	}
}

func TestScriptRequestMutation(t *testing.T) {
	var gotHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Scripted")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	engine := scripting.New()
	err := engine.LoadScript("mutate-req", `function onRequest(ctx) { ctx.headers["X-Scripted"] = "true"; }`, "")
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}

	_, s, port := setupProxy(t, withScriptEngine(engine))
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/scripted-req")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	findFlow(t, s, "/scripted-req")
	if gotHeader != "true" {
		t.Errorf("upstream X-Scripted = %q, want %q", gotHeader, "true")
	}
}

func TestScriptRequestBlocking(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should not be reached when script blocks")
		w.WriteHeader(200)
	}))
	defer ts.Close()

	engine := scripting.New()
	err := engine.LoadScript("block-req", `function onRequest(ctx) { ctx.blocked = true; }`, "")
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}

	_, _, port := setupProxy(t, withScriptEngine(engine))
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/blocked")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}

func TestScriptResponseMutation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	engine := scripting.New()
	err := engine.LoadScript("mutate-resp", `function onResponse(ctx) { ctx.headers["X-Resp-Script"] = "yes"; }`, "")
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}

	_, s, port := setupProxy(t, withScriptEngine(engine))
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/scripted-resp")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	_, data := findFlow(t, s, "/scripted-resp")
	if data == nil {
		t.Fatal("expected flow data")
	}
	if v := data.ResponseHeaders.Get("X-Resp-Script"); v != "yes" {
		t.Errorf("X-Resp-Script = %q, want %q", v, "yes")
	}
}

func TestMarkFailed(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(s, nil)

	flowID := uuid.NewV4()
	reqURL, _ := url.Parse("http://example.com/fail-test")

	f := &mp.Flow{
		Id: flowID,
		Request: &mp.Request{
			Method: "GET",
			URL:    reqURL,
			Header: http.Header{},
		},
	}

	ic.Requestheaders(f)

	// Response with nil Response field triggers markFailed.
	f.Response = nil
	ic.Response(f)

	meta, _, err := s.Get(store.FlowID(flowID.String()))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if meta.State != store.StateFailed {
		t.Errorf("State = %d, want StateFailed (%d)", meta.State, store.StateFailed)
	}
}
