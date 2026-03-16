package proxy

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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

// TestE2E_LargeGzipBodyNotTruncated verifies that a response whose decoded
// size exceeds maxBodySize is forwarded in full to the client.
// The script engine is always active in the real binary even with no scripts
// loaded, so this exercises the engine code-path that previously sent a
// truncated body to the browser.
func TestE2E_LargeGzipBodyNotTruncated(t *testing.T) {
	// Build a body larger than maxBodySize when decompressed.  Repetitive
	// content so gzip shrinks it well below the 5 MB streaming threshold.
	unit := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	decoded := bytes.Repeat(unit, (maxBodySize/len(unit))+100)
	require.Greater(t, len(decoded), maxBodySize, "test setup: decoded body must exceed maxBodySize")

	ts := gzipServer(t, string(decoded))

	engine := scripting.New() // no scripts – same as real httpmon default
	_, s, port := setupProxy(t, withScriptEngine(engine))
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test only
		},
		Timeout: 30 * time.Second, // large body — be generous on slow CI
	}

	resp, err := client.Get(ts.URL + "/large-gzip")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)
	// Client must receive the FULL decoded body, not a 5 MB slice of it.
	assert.Equal(t, len(decoded), len(body), "client received truncated body")
	assert.Equal(t, decoded, body, "client body content mismatch")

	// Ring-buffer stored copy is allowed to be capped for TUI display.
	meta, data := findFlow(t, s, "/large-gzip")
	assert.Equal(t, 200, meta.StatusCode)
	require.NotNil(t, data)
	assert.LessOrEqual(t, len(data.ResponseBody), maxBodySize, "stored body must be capped at maxBodySize")
	// SizeBytes reflects actual (full) decoded size.
	assert.Equal(t, int64(len(decoded)), meta.SizeBytes)
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
