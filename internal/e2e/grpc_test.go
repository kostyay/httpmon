//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/kostyay/httpmon/internal/bodydecoder"
	"github.com/kostyay/httpmon/internal/e2e/testpb"
	"github.com/kostyay/httpmon/internal/e2e/testpb/testpbconnect"

	tea "charm.land/bubbletea/v2"
)

// greeterServer implements the Greeter Connect service.
type greeterServer struct{}

func (s *greeterServer) SayHello(_ context.Context, req *connect.Request[testpb.HelloRequest]) (*connect.Response[testpb.HelloReply], error) {
	msg := fmt.Sprintf("Hello, %s! You are %d years old.", req.Msg.Name, req.Msg.Age)
	return connect.NewResponse(&testpb.HelloReply{
		Message: msg,
		Success: true,
	}), nil
}

// testProtoDir returns the absolute path to the bodydecoder testdata directory.
func testProtoDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "bodydecoder", "testdata")
}

// grpcHarness extends harness with a gRPC-Web Connect client.
type grpcHarness struct {
	*harness
	greeter testpbconnect.GreeterClient
}

// newGRPCDecoderRegistry builds a body decoder registry for gRPC-Web.
// If protoPaths is non-empty, named field decoding is enabled.
func newGRPCDecoderRegistry(t *testing.T, protoPaths []string) *bodydecoder.Registry {
	t.Helper()
	protoDec := &bodydecoder.RawProtobufDecoder{}
	if len(protoPaths) > 0 {
		protoReg, errs := bodydecoder.LoadProtoFiles(protoPaths)
		for _, e := range errs {
			t.Logf("proto load warning: %v", e)
		}
		protoDec.ResolveReg = bodydecoder.StaticResolver(protoReg)
	}
	grpcDec := &bodydecoder.GRPCWebDecoder{Proto: protoDec}
	return bodydecoder.NewRegistry(grpcDec, protoDec)
}

// newGRPCHarness creates a harness with a real Connect gRPC-Web server and client.
// If protoPaths is non-empty, named field decoding is enabled.
func newGRPCHarness(t *testing.T, protoPaths []string) *grpcHarness {
	t.Helper()

	// Set up Connect gRPC-Web server on a mux.
	mux := http.NewServeMux()
	path, handler := testpbconnect.NewGreeterHandler(&greeterServer{})
	mux.Handle(path, handler)
	// Also serve a plain JSON endpoint for regression test.
	mux.HandleFunc("/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[1,2,3],"ok":true}`)
	})

	h := newHarness(t, mux, withBodyDecoder(newGRPCDecoderRegistry(t, protoPaths)))

	// Create Connect gRPC-Web client routed through the proxy.
	// Disable gzip so responses are uncompressed (our decoder doesn't handle compressed gRPC frames).
	greeter := testpbconnect.NewGreeterClient(
		h.client,
		h.upstream.URL,
		connect.WithGRPCWeb(),
		connect.WithAcceptCompression("gzip", nil, nil),
	)

	return &grpcHarness{harness: h, greeter: greeter}
}

// callSayHello makes a SayHello gRPC-Web call through the proxy.
func (g *grpcHarness) callSayHello(t *testing.T, name string, age int32) {
	t.Helper()
	_, err := g.greeter.SayHello(context.Background(), connect.NewRequest(&testpb.HelloRequest{
		Name: name,
		Age:  age,
	}))
	if err != nil {
		t.Fatalf("SayHello: %v", err)
	}
}

// openResponseTab opens the detail view and switches to the response tab.
func (g *grpcHarness) openResponseTab() {
	g.sendSpecialKey(tea.KeyEnter)
	g.sendKey("2")
	g.tick()
}

// assertNoBinaryFallback fails if the view shows the "[binary content" marker,
// meaning the gRPC decoder did not run.
func (g *grpcHarness) assertNoBinaryFallback(t *testing.T) {
	t.Helper()
	if strings.Contains(g.view(), "[binary content") {
		t.Error("should decode gRPC body, not show [binary content]")
	}
}

func TestGRPCWebDecodeResponse(t *testing.T) {
	t.Parallel()
	g := newGRPCHarness(t, []string{testProtoDir()})

	g.callSayHello(t, "Alice", 30)
	g.waitForText(t, "Greeter")
	g.openResponseTab()

	view := g.view()
	// Named field decode should show field names as JSON.
	if !strings.Contains(view, "message") {
		t.Errorf("response should contain decoded field 'message', got:\n%s", view)
	}
	if !strings.Contains(view, "success") {
		t.Errorf("response should contain decoded field 'success', got:\n%s", view)
	}
	g.assertNoBinaryFallback(t)
}

func TestGRPCWebDecodeRequest(t *testing.T) {
	t.Parallel()
	g := newGRPCHarness(t, []string{testProtoDir()})

	g.callSayHello(t, "Bob", 25)
	g.waitForText(t, "Greeter")

	// Open detail view — request tab is default (tab 0).
	g.sendSpecialKey(tea.KeyEnter)
	g.tick()

	view := g.view()
	if !strings.Contains(view, "name") {
		t.Errorf("request should contain decoded field 'name', got:\n%s", view)
	}
	if !strings.Contains(view, "Bob") {
		t.Errorf("request should contain value 'Bob', got:\n%s", view)
	}
	g.assertNoBinaryFallback(t)
}

func TestGRPCWebRawToggle(t *testing.T) {
	t.Parallel()
	g := newGRPCHarness(t, []string{testProtoDir()})

	g.callSayHello(t, "Charlie", 40)
	g.waitForText(t, "Greeter")
	g.openResponseTab()

	// Pretty mode — should show decoded JSON.
	prettyView := g.view()
	if !strings.Contains(prettyView, "message") {
		t.Fatal("pretty mode should show decoded field names")
	}

	// Toggle to raw mode.
	g.sendKey("p")
	g.tick()
	rawView := g.view()

	// Raw mode shows the raw bytes through the decoder, which may still decode
	// but without pretty-printing, or show the original binary.
	if prettyView == rawView {
		t.Error("raw toggle should change the view")
	}

	// Toggle back.
	g.sendKey("p")
	g.tick()
	backView := g.view()
	if !strings.Contains(backView, "message") {
		t.Error("toggling back should restore decoded view")
	}
}

func TestGRPCWebWithoutProtoFiles(t *testing.T) {
	t.Parallel()
	// No proto paths — raw wire-format decode.
	g := newGRPCHarness(t, nil)

	g.callSayHello(t, "Dave", 35)
	g.waitForText(t, "Greeter")
	g.openResponseTab()

	view := g.view()
	// Without proto files, fields show as numbers, not names.
	// Field 1 = message (string), field 2 = success (bool).
	if !strings.Contains(view, `"1"`) && !strings.Contains(view, `"2"`) {
		t.Errorf("raw wire decode should show field numbers, got:\n%s", view)
	}
	// Should NOT show named fields.
	if strings.Contains(view, `"message"`) {
		t.Error("without proto files, should NOT show named field 'message'")
	}
	g.assertNoBinaryFallback(t)
}

func TestGRPCWebDecodeError(t *testing.T) {
	t.Parallel()

	// Custom upstream that returns grpc-web content type with garbage bytes.
	garbage := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/grpc-web+proto")
		w.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF}) //nolint:errcheck
	})

	h := newHarness(t, garbage, withBodyDecoder(newGRPCDecoderRegistry(t, nil)))
	h.doGet(t, "/bad-grpc")
	h.waitForText(t, "/bad-grpc")

	h.sendSpecialKey(tea.KeyEnter)
	h.sendKey("2")
	h.tick()

	view := h.view()
	// Status bar should show a proto decode error.
	if !strings.Contains(view, "proto:") && !strings.Contains(view, "grpc-web") && !strings.Contains(view, "truncated") {
		t.Errorf("should show decode error in status bar, got:\n%s", view)
	}
}

func TestGRPCWebNonProtoUnchanged(t *testing.T) {
	t.Parallel()
	g := newGRPCHarness(t, []string{testProtoDir()})

	// Make a regular JSON GET — should render normally.
	g.doGet(t, "/json")
	g.waitForText(t, "/json")
	g.openResponseTab()

	view := g.view()
	if !strings.Contains(view, "items") {
		t.Errorf("JSON response should render normally, got:\n%s", view)
	}
	if !strings.Contains(view, "ok") {
		t.Errorf("JSON response should contain 'ok', got:\n%s", view)
	}
}

func TestGRPCWebInvalidProtoPath(t *testing.T) {
	t.Parallel()
	// Invalid proto path — decoder falls back to raw wire decode.
	g := newGRPCHarness(t, []string{"/nonexistent/proto/path"})

	g.callSayHello(t, "Eve", 28)
	g.waitForText(t, "Greeter")
	g.openResponseTab()

	// Should still decode (raw wire format), not crash or show binary.
	g.assertNoBinaryFallback(t)
	// Without valid protos, no named fields.
	if strings.Contains(g.view(), `"message"`) {
		t.Error("invalid proto path should not produce named fields")
	}
}
