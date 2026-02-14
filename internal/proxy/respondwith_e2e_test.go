package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kostyay/httpmon/internal/scripting"
)

func TestE2E_RespondWithBody_SkipsUpstream(t *testing.T) {
	upstreamCalled := false
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			upstreamCalled = true
			w.WriteHeader(200)
			fmt.Fprint(w, "from-upstream")
		}))
	defer ts.Close()

	engine := scripting.New()
	require.NoError(t, engine.LoadScript("rw", `
		function onRequest(ctx) {
			ctx.respondWith({
				status: 201,
				body: '{"synthetic":true}',
				headers: {"Content-Type": "application/json"}
			});
		}
	`, ""))

	_, s, port := setupProxy(t, withScriptEngine(engine))
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/rw-body")
	require.NoError(t, err)
	defer resp.Body.Close()

	_, data := findFlow(t, s, "/rw-body")
	require.NotNil(t, data)
	assert.Equal(t, `{"synthetic":true}`, string(data.ResponseBody))
	assert.False(t, upstreamCalled)
}

func TestE2E_RespondWithFile(t *testing.T) {
	upstreamCalled := false
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			upstreamCalled = true
			w.WriteHeader(200)
		}))
	defer ts.Close()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "mock.json"),
		[]byte(`{"mocked":true}`), 0o644))

	engine := scripting.New()
	src := fmt.Sprintf(`
		function onRequest(ctx) {
			ctx.respondWith({file: "%s/mock.json"});
		}
	`, strings.ReplaceAll(dir, "\\", "\\\\"))
	require.NoError(t, engine.LoadScript("rw", src, ""))

	_, s, port := setupProxy(t, withScriptEngine(engine))
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/rw-file")
	require.NoError(t, err)
	defer resp.Body.Close()

	_, data := findFlow(t, s, "/rw-file")
	require.NotNil(t, data)
	assert.Equal(t, `{"mocked":true}`, string(data.ResponseBody))
	assert.False(t, upstreamCalled)
}

func TestE2E_RespondWithOnResponse_ReplacesBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(200)
			fmt.Fprint(w, "original")
		}))
	defer ts.Close()

	engine := scripting.New()
	require.NoError(t, engine.LoadScript("rw", `
		function onResponse(ctx) {
			ctx.respondWith({
				status: 418,
				body: "replaced-response"
			});
		}
	`, ""))

	_, s, port := setupProxy(t, withScriptEngine(engine))
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/rw-resp")
	require.NoError(t, err)
	io.ReadAll(resp.Body)
	resp.Body.Close()

	_, data := findFlow(t, s, "/rw-resp")
	require.NotNil(t, data)
	assert.Equal(t, "replaced-response", string(data.ResponseBody))
}

func TestE2E_RespondWithMissingFile_Passthrough(t *testing.T) {
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.WriteHeader(200)
			fmt.Fprint(w, "upstream-ok")
		}))
	defer ts.Close()

	engine := scripting.New()
	require.NoError(t, engine.LoadScript("rw", `
		function onRequest(ctx) {
			ctx.respondWith({file: "/nonexistent/path.json"});
		}
	`, ""))

	_, _, port := setupProxy(t, withScriptEngine(engine))
	client := proxyClient(port)

	resp, err := client.Post(
		ts.URL+"/rw-missing", "text/plain",
		strings.NewReader("pass-through"))
	require.NoError(t, err)
	io.ReadAll(resp.Body)
	resp.Body.Close()

	assert.Equal(t, "pass-through", gotBody)
}

func TestE2E_ReadFilePlusHeaderModification(t *testing.T) {
	var gotHeader string
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			gotHeader = r.Header.Get("X-From-File")
			w.WriteHeader(200)
			fmt.Fprint(w, "ok")
		}))
	defer ts.Close()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "token.txt"),
		[]byte("secret-token-123"), 0o644))

	engine := scripting.New()
	src := fmt.Sprintf(`
		function onRequest(ctx) {
			var token = ctx.readFile("%s/token.txt");
			if (token !== null) {
				ctx.headers["X-From-File"] = token;
			}
		}
	`, strings.ReplaceAll(dir, "\\", "\\\\"))
	require.NoError(t, engine.LoadScript("rf", src, ""))

	_, _, port := setupProxy(t, withScriptEngine(engine))
	client := proxyClient(port)

	resp, err := client.Get(ts.URL + "/rf-header")
	require.NoError(t, err)
	io.ReadAll(resp.Body)
	resp.Body.Close()

	assert.Equal(t, "secret-token-123", gotHeader)
}
