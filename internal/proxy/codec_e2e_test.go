package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kostyay/httpmon/internal/bodydecoder"
	"github.com/kostyay/httpmon/internal/e2e/testpb"
	"github.com/kostyay/httpmon/internal/e2e/testpb/testpbconnect"
	"github.com/kostyay/httpmon/internal/scripting"
)

// greeterServer implements the test Greeter service.
type greeterServer struct{}

func (s *greeterServer) SayHello(_ context.Context, req *connect.Request[testpb.HelloRequest]) (*connect.Response[testpb.HelloReply], error) {
	msg := fmt.Sprintf("Hello, %s! You are %d years old.", req.Msg.Name, req.Msg.Age)
	return connect.NewResponse(&testpb.HelloReply{
		Message: msg,
		Success: true,
	}), nil
}

func testProtoFile() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "bodydecoder", "testdata", "test.proto")
}

func withDecoderRegistry(reg *bodydecoder.Registry) setupOpt {
	return func(p *Proxy) { p.DecoderRegistry = reg }
}

func newTestDecoderRegistry(t *testing.T) *bodydecoder.Registry {
	t.Helper()
	protoDec := &bodydecoder.RawProtobufDecoder{}
	protoReg, errs := bodydecoder.LoadProtoFiles([]string{testProtoFile()})
	for _, e := range errs {
		t.Logf("proto warning: %v", e)
	}
	protoDec.ProtoReg = protoReg
	grpcDec := &bodydecoder.GRPCWebDecoder{Proto: protoDec}
	return bodydecoder.NewRegistry(grpcDec, protoDec)
}

// setupGRPCProxy creates a gRPC-Web server + proxy with script engine and decoder.
func setupGRPCProxy(t *testing.T, scriptSource string) (*httptest.Server, *greeterClient, int) {
	t.Helper()

	mux := http.NewServeMux()
	path, handler := testpbconnect.NewGreeterHandler(&greeterServer{})
	mux.Handle(path, handler)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	engine := scripting.New()
	if scriptSource != "" {
		require.NoError(t, engine.LoadScript("codec-test", scriptSource, ""))
	}

	reg := newTestDecoderRegistry(t)
	_, _, port := setupProxy(t, withScriptEngine(engine), withDecoderRegistry(reg))

	return ts, newGreeterClient(t, ts.URL, port), port
}

type greeterClient struct {
	client testpbconnect.GreeterClient
}

func newGreeterClient(t *testing.T, upstreamURL string, proxyPort int) *greeterClient {
	t.Helper()
	c := proxyClient(proxyPort)
	gc := testpbconnect.NewGreeterClient(
		c,
		upstreamURL,
		connect.WithGRPCWeb(),
		connect.WithAcceptCompression("gzip", nil, nil),
	)
	return &greeterClient{client: gc}
}

func (g *greeterClient) sayHello(t *testing.T, name string, age int32) *testpb.HelloReply {
	t.Helper()
	resp, err := g.client.SayHello(context.Background(), connect.NewRequest(&testpb.HelloRequest{
		Name: name,
		Age:  age,
	}))
	require.NoError(t, err)
	return resp.Msg
}

// TestCodecE2E_ScriptModifiesResponse verifies that a script can modify a
// protobuf response body (transparently decoded to JSON, then re-encoded).
func TestCodecE2E_ScriptModifiesResponse(t *testing.T) {
	_, gc, _ := setupGRPCProxy(t, `
		function onResponse(ctx) {
			if (ctx.body && ctx.body.indexOf('"message"') !== -1) {
				var parsed = JSON.parse(ctx.body);
				parsed.message = "MODIFIED";
				ctx.body = JSON.stringify(parsed);
			}
		}
	`)

	reply := gc.sayHello(t, "Alice", 30)
	assert.Equal(t, "MODIFIED", reply.Message)
	assert.True(t, reply.Success)
}

// TestCodecE2E_ScriptModifiesRequest verifies that a script can modify a
// protobuf request body before it reaches the upstream server.
func TestCodecE2E_ScriptModifiesRequest(t *testing.T) {
	_, gc, _ := setupGRPCProxy(t, `
		function onRequest(ctx) {
			if (ctx.body && ctx.body.indexOf('"name"') !== -1) {
				var parsed = JSON.parse(ctx.body);
				parsed.name = "ScriptInjected";
				ctx.body = JSON.stringify(parsed);
			}
		}
	`)

	// The script changes the name to "ScriptInjected", so the upstream
	// echoes it back in the response message.
	reply := gc.sayHello(t, "Original", 25)
	assert.Contains(t, reply.Message, "ScriptInjected")
	assert.NotContains(t, reply.Message, "Original")
}

// TestCodecE2E_NoScript_PassthroughUnchanged verifies that without scripts,
// gRPC-Web traffic passes through unmodified.
func TestCodecE2E_NoScript_PassthroughUnchanged(t *testing.T) {
	_, gc, _ := setupGRPCProxy(t, "")

	reply := gc.sayHello(t, "Passthrough", 99)
	assert.Contains(t, reply.Message, "Passthrough")
	assert.Contains(t, reply.Message, "99")
	assert.True(t, reply.Success)
}

// TestCodecE2E_ScriptDoesNotModify_OriginalBytesPreserved verifies that when
// a script reads but doesn't modify the body, the original wire bytes are forwarded.
func TestCodecE2E_ScriptDoesNotModify_OriginalBytesPreserved(t *testing.T) {
	_, gc, _ := setupGRPCProxy(t, `
		function onResponse(ctx) {
			// Read but don't modify.
			var x = ctx.body;
		}
	`)

	reply := gc.sayHello(t, "ReadOnly", 42)
	assert.Contains(t, reply.Message, "ReadOnly")
	assert.True(t, reply.Success)
}
