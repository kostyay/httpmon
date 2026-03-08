package proxy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kostyay/httpmon/internal/scripting"
)

// gzipBytes compresses data using gzip.
func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// gzipServer returns a test server that responds with gzip-compressed payload.
func gzipServer(t *testing.T, payload string) *httptest.Server {
	t.Helper()
	compressed := gzipBytes(t, []byte(payload))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(200)
		w.Write(compressed)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// TestE2E_GzipContentEncoding verifies that gzip-compressed responses are
// forwarded to the client with consistent Content-Encoding and body.
// Without the fix, the proxy forwards the decoded (decompressed) body but
// leaves the Content-Encoding: gzip header intact, causing browsers to fail
// with "Content Encoding Error".
func TestE2E_GzipContentEncoding(t *testing.T) {
	t.Run("no-op engine", func(t *testing.T) {
		const payload = `{"message":"hello from gzip"}`
		ts := gzipServer(t, payload)

		engine := scripting.New()
		_, s, port := setupProxy(t, withScriptEngine(engine))
		client := proxyClient(port)

		resp, err := client.Get(ts.URL + "/gzip-test")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, payload, string(body))
		assert.Equal(t, 200, resp.StatusCode)

		meta, data := findFlow(t, s, "/gzip-test")
		assert.Equal(t, 200, meta.StatusCode)
		require.NotNil(t, data)
		assert.Equal(t, payload, string(data.ResponseBody))
	})

	t.Run("script reads headers only", func(t *testing.T) {
		const payload = `{"data":"compressed"}`
		ts := gzipServer(t, payload)

		engine := scripting.New()
		err := engine.LoadScript("noop-resp", `
			function onResponse(ctx) {
				ctx.headers["X-Seen"] = "true";
			}
		`, "")
		require.NoError(t, err)

		_, s, port := setupProxy(t, withScriptEngine(engine))
		client := proxyClient(port)

		resp, err := client.Get(ts.URL + "/gzip-script")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, payload, string(body))

		meta, data := findFlow(t, s, "/gzip-script")
		assert.Equal(t, 200, meta.StatusCode)
		require.NotNil(t, data)
		assert.Equal(t, payload, string(data.ResponseBody))
	})

	t.Run("no engine passthrough", func(t *testing.T) {
		const payload = `{"plain":"pass-through"}`
		ts := gzipServer(t, payload)

		_, s, port := setupProxy(t)
		client := proxyClient(port)

		resp, err := client.Get(ts.URL + "/gzip-noengine")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, payload, string(body))

		meta, data := findFlow(t, s, "/gzip-noengine")
		assert.Equal(t, 200, meta.StatusCode)
		require.NotNil(t, data)
		assert.Equal(t, payload, string(data.ResponseBody))
	})

	t.Run("script modifies body", func(t *testing.T) {
		const original = `{"value":"original"}`
		const modified = `{"value":"modified"}`
		ts := gzipServer(t, original)

		engine := scripting.New()
		err := engine.LoadScript("modify-body", fmt.Sprintf(`
			function onResponse(ctx) {
				ctx.body = %q;
			}
		`, modified), "")
		require.NoError(t, err)

		_, s, port := setupProxy(t, withScriptEngine(engine))
		client := proxyClient(port)

		resp, err := client.Get(ts.URL + "/gzip-modify")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, modified, string(body))

		meta, data := findFlow(t, s, "/gzip-modify")
		assert.Equal(t, 200, meta.StatusCode)
		require.NotNil(t, data)
		assert.Equal(t, modified, string(data.ResponseBody))
	})
}
