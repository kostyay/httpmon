package scripting

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondWithBodyStatusHeaders(t *testing.T) {
	e := New()
	require.NoError(t, e.LoadScript("rw", `
		function onRequest(ctx) {
			ctx.respondWith({
				status: 201,
				body: '{"ok":true}',
				headers: {"Content-Type": "application/json"}
			});
		}
	`, ""))

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx)

	assert.True(t, ctx.Responded)
	assert.Equal(t, 201, ctx.ResponseStatus)
	assert.Equal(t, `{"ok":true}`, string(ctx.ResponseBody))
	assert.Equal(t, "application/json", ctx.ResponseHeaders["Content-Type"])
}

func TestRespondWithDefaultStatus(t *testing.T) {
	e := New()
	require.NoError(t, e.LoadScript("rw", `
		function onRequest(ctx) {
			ctx.respondWith({body: "hello"});
		}
	`, ""))

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx)

	assert.True(t, ctx.Responded)
	assert.Equal(t, 200, ctx.ResponseStatus)
	assert.Equal(t, "hello", string(ctx.ResponseBody))
}

func TestRespondWithFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "mock.json"), []byte(`{"users":[]}`), 0o644))

	writeTestScript(t, dir, "serve.js", `// ---
// name: Serve File
// match:
//   - "*://*/*"
// ---
function onRequest(ctx) {
    ctx.respondWith({file: "./mock.json"});
}
`)

	e := New()
	e.LoadFromDir(dir)

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx)

	assert.True(t, ctx.Responded)
	assert.Equal(t, `{"users":[]}`, string(ctx.ResponseBody))
	assert.Equal(t, "application/json", ctx.ResponseHeaders["Content-Type"])
}

func TestRespondWithFileMissing(t *testing.T) {
	e := New()
	require.NoError(t, e.LoadScript("rw", `
		function onRequest(ctx) {
			ctx.respondWith({file: "/nonexistent/file.json"});
		}
	`, ""))

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx)

	assert.False(t, ctx.Responded)
}

func TestRespondWithInterruptsOnRequest(t *testing.T) {
	e := New()
	require.NoError(t, e.LoadScript("rw", `
		function onRequest(ctx) {
			ctx.respondWith({body: "short-circuit"});
			ctx.headers["X-After"] = "should-not-run";
		}
	`, ""))

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx)

	assert.True(t, ctx.Responded)
	assert.Equal(t, "short-circuit", string(ctx.ResponseBody))
	assert.Empty(t, ctx.Headers.Get("X-After"))
}

func TestRespondWithNoInterruptOnResponse(t *testing.T) {
	e := New()
	require.NoError(t, e.LoadScript("rw", `
		function onResponse(ctx) {
			ctx.respondWith({body: "replaced", status: 418});
			ctx.headers["X-After"] = "still-runs";
		}
	`, ""))

	ctx := &ResponseContext{
		Status:  200,
		Headers: http.Header{},
		Body:    []byte("original"),
	}
	e.RunOnResponse(ctx, "example.com/api")

	assert.True(t, ctx.Responded)
	assert.Equal(t, 418, ctx.ResponseStatus)
	assert.Equal(t, "replaced", string(ctx.ResponseBody))
}

func TestReadFileReturnsContents(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "data.txt"), []byte("file-contents"), 0o644))

	writeTestScript(t, dir, "reader.js", `// ---
// name: Reader
// match:
//   - "*://*/*"
// ---
function onRequest(ctx) {
    var data = ctx.readFile("./data.txt");
    ctx.headers["X-Data"] = data;
}
`)

	e := New()
	e.LoadFromDir(dir)

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/test",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx)

	assert.Equal(t, "file-contents", ctx.Headers.Get("X-Data"))
}

func TestReadFileMissingReturnsNull(t *testing.T) {
	e := New()
	require.NoError(t, e.LoadScript("rw", `
		function onRequest(ctx) {
			var data = ctx.readFile("/nonexistent/file.txt");
			if (data === null) {
				ctx.headers["X-Result"] = "null";
			}
		}
	`, ""))

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/test",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx)

	assert.Equal(t, "null", ctx.Headers.Get("X-Result"))
}

func TestReadFilePathResolution(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "fixtures")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(sub, "mock.json"), []byte(`{"test":1}`), 0o644))

	writeTestScript(t, dir, "resolver.js", `// ---
// name: Resolver
// match:
//   - "*://*/*"
// ---
function onRequest(ctx) {
    var data = ctx.readFile("./fixtures/mock.json");
    ctx.headers["X-Data"] = data;
}
`)

	e := New()
	e.LoadFromDir(dir)

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/test",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx)

	assert.Equal(t, `{"test":1}`, ctx.Headers.Get("X-Data"))
}
