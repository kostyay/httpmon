package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
	"github.com/kostyay/httpmon/internal/store"
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
func setupGRPCProxy(t *testing.T, scriptSource string) (*httptest.Server, *greeterClient, *store.RingBuffer, int) {
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
	_, s, port := setupProxy(t, withScriptEngine(engine), withDecoderRegistry(reg))

	return ts, newGreeterClient(t, ts.URL, port), s, port
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
	_, gc, _, _ := setupGRPCProxy(t, `
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
	_, gc, _, _ := setupGRPCProxy(t, `
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
	_, gc, _, _ := setupGRPCProxy(t, "")

	reply := gc.sayHello(t, "Passthrough", 99)
	assert.Contains(t, reply.Message, "Passthrough")
	assert.Contains(t, reply.Message, "99")
	assert.True(t, reply.Success)
}

// TestCodecE2E_ScriptDoesNotModify_OriginalBytesPreserved verifies that when
// a script reads but doesn't modify the body, the original wire bytes are forwarded.
func TestCodecE2E_ScriptDoesNotModify_OriginalBytesPreserved(t *testing.T) {
	_, gc, _, _ := setupGRPCProxy(t, `
		function onResponse(ctx) {
			// Read but don't modify.
			var x = ctx.body;
		}
	`)

	reply := gc.sayHello(t, "ReadOnly", 42)
	assert.Contains(t, reply.Message, "ReadOnly")
	assert.True(t, reply.Success)
}

// TestCodecE2E_RespondWithOnGRPCWeb verifies that ctx.respondWith on a gRPC-Web
// request skips upstream and returns a synthetic plain response.
func TestCodecE2E_RespondWithOnGRPCWeb(t *testing.T) {
	upstreamCalled := false
	mux := http.NewServeMux()
	path, handler := testpbconnect.NewGreeterHandler(&greeterServer{})
	mux.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		handler.ServeHTTP(w, r)
	}))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	engine := scripting.New()
	require.NoError(t, engine.LoadScript("rw", `
		function onRequest(ctx) {
			ctx.respondWith({
				status: 299,
				body: '{"synthetic":"yes"}',
				headers: {"Content-Type": "application/json"}
			});
		}
	`, ""))

	reg := newTestDecoderRegistry(t)
	_, s, port := setupProxy(t, withScriptEngine(engine), withDecoderRegistry(reg))
	client := proxyClient(port)

	// POST with gRPC-Web content type to trigger codec path.
	req, err := http.NewRequest("POST",
		ts.URL+"/testpkg.Greeter/SayHello",
		bytes.NewReader([]byte{0, 0, 0, 0, 0})) // minimal gRPC frame (empty)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/grpc-web+proto")

	resp, err := client.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	assert.Equal(t, 299, resp.StatusCode)
	assert.Equal(t, `{"synthetic":"yes"}`, string(body))
	assert.False(t, upstreamCalled)

	_, data := findFlow(t, s, "/testpkg.Greeter/SayHello")
	require.NotNil(t, data)
	assert.Equal(t, `{"synthetic":"yes"}`, string(data.ResponseBody))
}

// TestCodecE2E_ModifyBothRequestAndResponse verifies that scripts can modify
// both the request and response in the same flow.
func TestCodecE2E_ModifyBothRequestAndResponse(t *testing.T) {
	_, gc, _, _ := setupGRPCProxy(t, `
		function onRequest(ctx) {
			if (ctx.body && ctx.body.indexOf('"name"') !== -1) {
				var parsed = JSON.parse(ctx.body);
				parsed.name = "DualMod";
				ctx.body = JSON.stringify(parsed);
			}
		}
		function onResponse(ctx) {
			if (ctx.body && ctx.body.indexOf('"message"') !== -1) {
				var parsed = JSON.parse(ctx.body);
				parsed.message = "BOTH_MODIFIED";
				ctx.body = JSON.stringify(parsed);
			}
		}
	`)

	reply := gc.sayHello(t, "Original", 10)
	assert.Equal(t, "BOTH_MODIFIED", reply.Message)
	assert.True(t, reply.Success)
}

// TestCodecE2E_AddField verifies that a script can add a new field to the request.
func TestCodecE2E_AddField(t *testing.T) {
	_, gc, _, _ := setupGRPCProxy(t, `
		function onRequest(ctx) {
			if (ctx.body) {
				var parsed = JSON.parse(ctx.body);
				parsed.age = 99;
				ctx.body = JSON.stringify(parsed);
			}
		}
	`)

	// Client sends age=0, script overrides to 99.
	reply := gc.sayHello(t, "Alice", 0)
	assert.Contains(t, reply.Message, "99")
}

// TestCodecE2E_RemoveField verifies that deleting a field from the request JSON
// results in the proto default value (0 for int32).
func TestCodecE2E_RemoveField(t *testing.T) {
	_, gc, _, _ := setupGRPCProxy(t, `
		function onRequest(ctx) {
			if (ctx.body) {
				var parsed = JSON.parse(ctx.body);
				delete parsed.age;
				ctx.body = JSON.stringify(parsed);
			}
		}
	`)

	// Client sends age=55, script deletes it → upstream sees default 0.
	reply := gc.sayHello(t, "Bob", 55)
	assert.Contains(t, reply.Message, "0 years old")
	assert.NotContains(t, reply.Message, "55")
}

// TestCodecE2E_StoreCapturesReencodedBody verifies that after response modification,
// the store's FlowData.ResponseBody contains re-encoded gRPC-Web wire bytes.
func TestCodecE2E_StoreCapturesReencodedBody(t *testing.T) {
	_, gc, s, _ := setupGRPCProxy(t, `
		function onResponse(ctx) {
			if (ctx.body && ctx.body.indexOf('"message"') !== -1) {
				var parsed = JSON.parse(ctx.body);
				parsed.message = "STORED";
				ctx.body = JSON.stringify(parsed);
			}
		}
	`)

	reply := gc.sayHello(t, "StoreTest", 1)
	require.Equal(t, "STORED", reply.Message)

	_, data := findFlow(t, s, "/testpkg.Greeter/SayHello")
	require.NotNil(t, data)

	// The stored body should be gRPC-Web wire bytes (starts with 0x00 frame flag),
	// not raw JSON.
	require.True(t, len(data.ResponseBody) > 5, "response body too short")
	assert.Equal(t, byte(0x00), data.ResponseBody[0], "expected gRPC data frame flag")
	// Verify it's NOT raw JSON text.
	assert.NotContains(t, string(data.ResponseBody), `"message"`)
}

// TestCodecE2E_ScriptSetsEmptyBody verifies that setting the body to an empty JSON
// object produces a valid (empty) proto message without errors.
func TestCodecE2E_ScriptSetsEmptyBody(t *testing.T) {
	_, gc, _, _ := setupGRPCProxy(t, `
		function onResponse(ctx) {
			ctx.body = "{}";
		}
	`)

	reply := gc.sayHello(t, "EmptyTest", 10)
	// All fields should be zero-valued defaults.
	assert.Equal(t, "", reply.Message)
	assert.False(t, reply.Success)
}

// TestCodecE2E_UndefinedFieldIgnored verifies that adding a field not in the proto
// schema is silently ignored during re-encoding, and the response is still valid.
func TestCodecE2E_UndefinedFieldIgnored(t *testing.T) {
	_, gc, _, _ := setupGRPCProxy(t, `
		function onResponse(ctx) {
			if (ctx.body) {
				var parsed = JSON.parse(ctx.body);
				parsed.unknownField = "should be ignored";
				parsed.anotherUnknown = 12345;
				ctx.body = JSON.stringify(parsed);
			}
		}
	`)

	reply := gc.sayHello(t, "UnknownField", 7)
	// Known fields should still be intact.
	assert.Contains(t, reply.Message, "UnknownField")
	assert.Contains(t, reply.Message, "7")
	assert.True(t, reply.Success)
}
