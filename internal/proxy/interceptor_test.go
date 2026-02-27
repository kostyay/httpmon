package proxy

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	mp "github.com/lqqyt2423/go-mitmproxy/proxy"

	"github.com/kostyay/httpmon/internal/bodydecoder"
	"github.com/kostyay/httpmon/internal/procinfo"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/kostyay/httpmon/internal/throttle"
)

// fakeConn implements net.Conn with a configurable RemoteAddr.
type fakeConn struct {
	net.Conn // embed; only RemoteAddr is called
	addr     net.Addr
}

func (f *fakeConn) RemoteAddr() net.Addr { return f.addr }

type fakeAddr struct{ s string }

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return a.s }

// --- clientPort tests ---

func TestClientPortNilConn(t *testing.T) {
	if got := clientPort(nil); got != 0 {
		t.Errorf("clientPort(nil) = %d, want 0", got)
	}
}

func TestClientPortValidTCP(t *testing.T) {
	c := &fakeConn{addr: fakeAddr{"127.0.0.1:54321"}}
	if got := clientPort(c); got != 54321 {
		t.Errorf("clientPort = %d, want 54321", got)
	}
}

func TestClientPortMalformed(t *testing.T) {
	c := &fakeConn{addr: fakeAddr{"not-a-host-port"}}
	if got := clientPort(c); got != 0 {
		t.Errorf("clientPort(malformed) = %d, want 0", got)
	}
}

func TestClientPortOverflow(t *testing.T) {
	c := &fakeConn{addr: fakeAddr{"127.0.0.1:99999"}}
	if got := clientPort(c); got != 0 {
		t.Errorf("clientPort(overflow) = %d, want 0", got)
	}
}

// --- Requestheaders resolver wiring tests ---

func newTestFlow(conn net.Conn, tls bool) *mp.Flow {
	return &mp.Flow{
		ConnContext: &mp.ConnContext{
			ClientConn: &mp.ClientConn{
				Conn: conn,
				Tls:  tls,
			},
		},
		Request: &mp.Request{
			Method: "GET",
			URL:    &url.URL{Scheme: "http", Host: "example.com", Path: "/test"},
			Header: http.Header{},
		},
	}
}

func TestRequestheadersCallsResolver(t *testing.T) {
	s := store.New(100)
	r := procinfo.New(s)
	ic := newInterceptor(interceptorConfig{Store: s, Resolver: r})

	conn := &fakeConn{addr: fakeAddr{"127.0.0.1:12345"}}
	flow := newTestFlow(conn, false)

	ic.Requestheaders(flow)
	r.Wait()

	// Resolver runs async; after Wait() the store should have Process set.
	flowID := flow.Id.String()
	metas, _ := s.List(nil, 0, 0)
	found := false
	for _, m := range metas {
		if m.ID == flowID {
			if m.Process == "" {
				t.Error("Process should be set after resolver runs")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("flow not found in store")
	}
}

func TestRequestheadersNilResolver(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s})

	conn := &fakeConn{addr: fakeAddr{"127.0.0.1:12345"}}
	flow := newTestFlow(conn, false)

	// Should not panic with nil resolver.
	ic.Requestheaders(flow)

	metas, _ := s.List(nil, 0, 0)
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(metas))
	}
	if metas[0].Process != "" {
		t.Errorf("Process = %q, want empty (no resolver)", metas[0].Process)
	}
}

func TestRequestheadersNilConnContext(t *testing.T) {
	s := store.New(100)
	r := procinfo.New(s)
	ic := newInterceptor(interceptorConfig{Store: s, Resolver: r})

	flow := &mp.Flow{
		Request: &mp.Request{
			Method: "GET",
			URL:    &url.URL{Scheme: "http", Host: "example.com", Path: "/"},
			Header: http.Header{},
		},
	}

	// Should not panic with nil ConnContext.
	ic.Requestheaders(flow)
	r.Wait()

	metas, _ := s.List(nil, 0, 0)
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(metas))
	}
}

func TestRequestheadersZeroPort(t *testing.T) {
	s := store.New(100)
	r := procinfo.New(s)
	ic := newInterceptor(interceptorConfig{Store: s, Resolver: r})

	// Malformed addr → clientPort returns 0 → resolver should not be called.
	conn := &fakeConn{addr: fakeAddr{"bad-addr"}}
	flow := newTestFlow(conn, false)

	ic.Requestheaders(flow)
	r.Wait()

	metas, _ := s.List(nil, 0, 0)
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(metas))
	}
	// With zero port, resolver skips → Process stays empty.
	if metas[0].Process != "" {
		t.Errorf("Process = %q, want empty (zero port)", metas[0].Process)
	}
}

func TestRequestheadersSetsScheme(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s})

	conn := &fakeConn{addr: fakeAddr{"127.0.0.1:1111"}}
	flow := newTestFlow(conn, true)

	ic.Requestheaders(flow)

	metas, _ := s.List(nil, 0, 0)
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(metas))
	}
	if metas[0].Scheme != "https" {
		t.Errorf("Scheme = %q, want https", metas[0].Scheme)
	}
}

func TestStreamResponseModifierNoThrottle(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s})

	in := strings.NewReader("hello")
	out := ic.StreamResponseModifier(nil, in)

	if out != in {
		t.Error("should return original reader when no throttle")
	}
}

func TestStreamResponseModifierWithThrottle(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{
		Store:       s,
		ThrottleBPS: throttle.PresetBandwidth("wifi"),
	})

	in := strings.NewReader("hello world")
	out := ic.StreamResponseModifier(nil, in)

	if out == in {
		t.Error("should wrap reader when throttle active")
	}

	data, err := io.ReadAll(out)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("data = %q, want %q", string(data), "hello world")
	}
}

func TestStreamResponseModifierLatencyOnly(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{
		Store:           s,
		ThrottleLatency: 1 * time.Millisecond,
	})

	in := strings.NewReader("test")
	out := ic.StreamResponseModifier(nil, in)

	if out == in {
		t.Error("should wrap reader when latency > 0")
	}
}

func TestResponseheadersUpdatesStore(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s})

	conn := &fakeConn{addr: fakeAddr{"127.0.0.1:22222"}}
	flow := newTestFlow(conn, false)
	ic.Requestheaders(flow)

	flow.Response = &mp.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": {"application/json"}},
	}
	ic.Responseheaders(flow)

	flowID := flow.Id.String()
	metas, _ := s.List(nil, 0, 0)
	for _, m := range metas {
		if m.ID == flowID {
			if m.StatusCode != 200 {
				t.Errorf("StatusCode = %d, want 200", m.StatusCode)
			}
			if m.ContentType != "application/json" {
				t.Errorf("ContentType = %q, want application/json", m.ContentType)
			}
			return
		}
	}
	t.Error("flow not found in store")
}

func TestResponseheadersNilResponse(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s})

	conn := &fakeConn{addr: fakeAddr{"127.0.0.1:33333"}}
	flow := newTestFlow(conn, false)
	ic.Requestheaders(flow)

	flow.Response = nil
	ic.Responseheaders(flow) // must not panic

	flowID := flow.Id.String()
	metas, _ := s.List(nil, 0, 0)
	for _, m := range metas {
		if m.ID == flowID {
			if m.StatusCode != 0 {
				t.Errorf("StatusCode = %d, want 0 (unchanged)", m.StatusCode)
			}
			return
		}
	}
	t.Error("flow not found in store")
}

// --- runScriptsWithCodec tests ---

// fakeEncoder implements both bodydecoder.Decoder and bodydecoder.Encoder.
type fakeEncoder struct {
	canMatch    bool
	decoded     string
	encoded     []byte
	decodeErr   error
	encodeErr   error
	encodeCalls int
}

func (f *fakeEncoder) CanDecode(string) bool { return f.canMatch }
func (f *fakeEncoder) Decode(_ []byte, _ bodydecoder.DecoderMetadata) (string, string, error) {
	return f.decoded, "application/json", f.decodeErr
}
func (f *fakeEncoder) CanEncode(string) bool { return f.canMatch }
func (f *fakeEncoder) Encode(_ []byte, _ string, _ bodydecoder.DecoderMetadata) ([]byte, error) {
	f.encodeCalls++
	return f.encoded, f.encodeErr
}

func TestRunScriptsWithCodec_DecodesForScript(t *testing.T) {
	enc := &fakeEncoder{canMatch: true, decoded: `{"name":"Alice"}`, encoded: []byte("wire")}
	reg := bodydecoder.NewRegistry(enc)
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s, DecoderRegistry: reg})

	meta := bodydecoder.DecoderMetadata{RequestPath: "/pkg.Svc/Method", IsRequest: true}
	result := ic.runScriptsWithCodec([]byte("proto-wire"), "application/protobuf", meta,
		func(body []byte) ([]byte, bool) {
			// Script sees decoded JSON.
			if string(body) != `{"name":"Alice"}` {
				t.Errorf("script saw %q, want decoded JSON", body)
			}
			return []byte(`{"name":"Bob"}`), true
		},
	)
	if string(result) != "wire" {
		t.Errorf("result = %q, want re-encoded wire", result)
	}
	if enc.encodeCalls != 1 {
		t.Errorf("encode called %d times, want 1", enc.encodeCalls)
	}
}

func TestRunScriptsWithCodec_UnchangedBody(t *testing.T) {
	enc := &fakeEncoder{canMatch: true, decoded: `{"name":"Alice"}`, encoded: []byte("wire")}
	reg := bodydecoder.NewRegistry(enc)
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s, DecoderRegistry: reg})

	original := []byte("proto-wire")
	meta := bodydecoder.DecoderMetadata{}
	result := ic.runScriptsWithCodec(original, "application/protobuf", meta,
		func(body []byte) ([]byte, bool) {
			return body, false // unchanged
		},
	)
	// Should return original, not re-encode.
	if string(result) != "proto-wire" {
		t.Errorf("result = %q, want original", result)
	}
	if enc.encodeCalls != 0 {
		t.Errorf("encode called %d times, want 0 (body unchanged)", enc.encodeCalls)
	}
}

func TestRunScriptsWithCodec_DecodeFails_PassesRaw(t *testing.T) {
	enc := &fakeEncoder{canMatch: true, decodeErr: bodydecoder.ErrNoDecoder}
	reg := bodydecoder.NewRegistry(enc)
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s, DecoderRegistry: reg})

	meta := bodydecoder.DecoderMetadata{}
	result := ic.runScriptsWithCodec([]byte("raw-body"), "text/plain", meta,
		func(body []byte) ([]byte, bool) {
			if string(body) != "raw-body" {
				t.Errorf("script saw %q, want raw", body)
			}
			return []byte("modified-raw"), true
		},
	)
	if string(result) != "modified-raw" {
		t.Errorf("result = %q, want modified-raw", result)
	}
}

func TestRunScriptsWithCodec_EncodeFails_FallsBack(t *testing.T) {
	enc := &fakeEncoder{canMatch: true, decoded: `{"name":"Alice"}`, encodeErr: bodydecoder.ErrNoEncoder}
	reg := bodydecoder.NewRegistry(enc)
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s, DecoderRegistry: reg})

	original := []byte("proto-wire")
	meta := bodydecoder.DecoderMetadata{}
	result := ic.runScriptsWithCodec(original, "application/protobuf", meta,
		func(body []byte) ([]byte, bool) {
			return []byte(`{"name":"Bob"}`), true
		},
	)
	// Encode failed → fall back to original.
	if string(result) != "proto-wire" {
		t.Errorf("result = %q, want original (encode fallback)", result)
	}
}

func TestRunScriptsWithCodec_NilRegistry(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s}) // no registry

	meta := bodydecoder.DecoderMetadata{}
	result := ic.runScriptsWithCodec([]byte("raw"), "application/protobuf", meta,
		func(body []byte) ([]byte, bool) {
			return []byte("modified"), true
		},
	)
	if string(result) != "modified" {
		t.Errorf("result = %q, want modified (nil registry passthrough)", result)
	}
}

func TestSetThrottleRuntimeChange(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s})

	if ic.ThrottleBPS() != 0 {
		t.Errorf("initial BPS = %d, want 0", ic.ThrottleBPS())
	}

	bps3g := throttle.PresetBandwidth("3g")
	ic.SetThrottle(bps3g, 100*time.Millisecond)

	if ic.ThrottleBPS() != bps3g {
		t.Errorf("BPS = %d, want %d", ic.ThrottleBPS(), bps3g)
	}
	if ic.ThrottleLatency() != 100*time.Millisecond {
		t.Errorf("Latency = %v, want 100ms", ic.ThrottleLatency())
	}

	// After runtime change, StreamResponseModifier should wrap.
	in := strings.NewReader("data")
	out := ic.StreamResponseModifier(nil, in)
	if out == in {
		t.Error("should wrap after SetThrottle")
	}
}
