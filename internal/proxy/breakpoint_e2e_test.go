package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kostyay/httpmon/internal/breakpoint"
	"github.com/kostyay/httpmon/internal/scripting"
)

func withBreakpoint(
	engine *scripting.Engine, ctrl breakpoint.Controller,
) setupOpt {
	return func(p *Proxy) {
		p.ScriptEngine = engine
		p.BreakpointCtrl = ctrl
	}
}

func TestE2E_RequestBreakpointModifiesBody(t *testing.T) {
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	engine := scripting.New()
	ctrl := breakpoint.NewController()
	require.NoError(t, engine.LoadScript("bp", `
		function onRequest(ctx) { ctx.breakpoint(); }
	`, ""))

	_, _, port := setupProxy(t, withBreakpoint(engine, ctrl))
	client := proxyClient(port)
	sub := ctrl.Subscribe()

	done := make(chan struct{})
	go func() {
		resp, err := client.Post(ts.URL+"/bp-req-body", "text/plain", strings.NewReader("original"))
		if err == nil {
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		close(done)
	}()

	hit := <-sub
	ctrl.Resume(hit.FlowID, breakpoint.BreakpointResume{
		Body: []byte("modified-by-user"),
	})

	<-done
	assert.Equal(t, "modified-by-user", gotBody)
}

func TestE2E_ResponseBreakpointModifiesBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		fmt.Fprint(w, "server-response")
	}))
	defer ts.Close()

	engine := scripting.New()
	ctrl := breakpoint.NewController()
	require.NoError(t, engine.LoadScript("bp", `
		function onResponse(ctx) { ctx.breakpoint(); }
	`, ""))

	_, s, port := setupProxy(t, withBreakpoint(engine, ctrl))
	client := proxyClient(port)
	sub := ctrl.Subscribe()

	done := make(chan struct{})
	go func() {
		resp, err := client.Get(ts.URL + "/bp-resp-body")
		if err == nil {
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		close(done)
	}()

	hit := <-sub
	assert.Equal(t, "server-response", string(hit.Body))
	ctrl.Resume(hit.FlowID, breakpoint.BreakpointResume{
		Body: []byte("user-edited-response"),
	})

	<-done

	_, data := findFlow(t, s, "/bp-resp-body")
	require.NotNil(t, data)
	assert.Equal(t, "user-edited-response", string(data.ResponseBody))
}

func TestE2E_RequestBreakpointModifiesHeaders(t *testing.T) {
	var gotHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Bp-Edited")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	engine := scripting.New()
	ctrl := breakpoint.NewController()
	require.NoError(t, engine.LoadScript("bp", `
		function onRequest(ctx) { ctx.breakpoint(); }
	`, ""))

	_, _, port := setupProxy(t, withBreakpoint(engine, ctrl))
	client := proxyClient(port)
	sub := ctrl.Subscribe()

	done := make(chan struct{})
	go func() {
		resp, err := client.Get(ts.URL + "/bp-req-hdr")
		if err == nil {
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		close(done)
	}()

	hit := <-sub
	ctrl.Resume(hit.FlowID, breakpoint.BreakpointResume{
		Headers: map[string]string{"X-Bp-Edited": "from-breakpoint"},
	})

	<-done
	assert.Equal(t, "from-breakpoint", gotHeader)
}

func TestE2E_ResponseBreakpointModifiesHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	engine := scripting.New()
	ctrl := breakpoint.NewController()
	require.NoError(t, engine.LoadScript("bp", `
		function onResponse(ctx) { ctx.breakpoint(); }
	`, ""))

	_, s, port := setupProxy(t, withBreakpoint(engine, ctrl))
	client := proxyClient(port)
	sub := ctrl.Subscribe()

	done := make(chan struct{})
	go func() {
		resp, err := client.Get(ts.URL + "/bp-resp-hdr")
		if err == nil {
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		close(done)
	}()

	hit := <-sub
	ctrl.Resume(hit.FlowID, breakpoint.BreakpointResume{
		Headers: map[string]string{"X-Resp-Edited": "yes"},
	})

	<-done

	_, data := findFlow(t, s, "/bp-resp-hdr")
	require.NotNil(t, data)
	assert.Equal(t, "yes", data.ResponseHeaders.Get("X-Resp-Edited"))
}

func TestE2E_SkipPassesThroughUnmodified(t *testing.T) {
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	engine := scripting.New()
	ctrl := breakpoint.NewController()
	require.NoError(t, engine.LoadScript("bp", `
		function onRequest(ctx) { ctx.breakpoint(); }
	`, ""))

	_, _, port := setupProxy(t, withBreakpoint(engine, ctrl))
	client := proxyClient(port)
	sub := ctrl.Subscribe()

	done := make(chan struct{})
	go func() {
		resp, err := client.Post(ts.URL+"/bp-skip", "text/plain", strings.NewReader("keep-me"))
		if err == nil {
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		close(done)
	}()

	hit := <-sub
	ctrl.Resume(hit.FlowID, breakpoint.BreakpointResume{Skipped: true})

	<-done
	assert.Equal(t, "keep-me", gotBody)
}

func TestE2E_ScriptContinuesAfterBreakpoint(t *testing.T) {
	var gotHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Post-Bp")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	engine := scripting.New()
	ctrl := breakpoint.NewController()
	require.NoError(t, engine.LoadScript("bp", `
		function onRequest(ctx) {
			ctx.breakpoint();
			ctx.headers["X-Post-Bp"] = "script-continued";
		}
	`, ""))

	_, _, port := setupProxy(t, withBreakpoint(engine, ctrl))
	client := proxyClient(port)
	sub := ctrl.Subscribe()

	done := make(chan struct{})
	go func() {
		resp, err := client.Get(ts.URL + "/bp-continues")
		if err == nil {
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		close(done)
	}()

	hit := <-sub
	ctrl.Resume(hit.FlowID, breakpoint.BreakpointResume{
		Headers: map[string]string{"X-User-Edit": "yes"},
	})

	<-done
	assert.Equal(t, "script-continued", gotHeader)
}

func TestE2E_ConcurrentBreakpoints(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		w.Write(b)
	}))
	defer ts.Close()

	engine := scripting.New()
	ctrl := breakpoint.NewController()
	require.NoError(t, engine.LoadScript("bp", `
		function onRequest(ctx) { ctx.breakpoint(); }
	`, ""))

	_, _, port := setupProxy(t, withBreakpoint(engine, ctrl))
	client := proxyClient(port)
	sub := ctrl.Subscribe()

	const n = 3
	var wg sync.WaitGroup
	responses := make([]string, n)

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := fmt.Sprintf("req-%d", idx)
			resp, err := client.Post(
				fmt.Sprintf("%s/bp-concurrent-%d", ts.URL, idx),
				"text/plain",
				strings.NewReader(body),
			)
			if err == nil {
				b, _ := io.ReadAll(resp.Body)
				responses[idx] = string(b)
				resp.Body.Close()
			}
		}(i)
	}

	// Collect all hits, then resume each with unique body.
	hits := make([]breakpoint.BreakpointHit, 0, n)
	for range n {
		select {
		case h := <-sub:
			hits = append(hits, h)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for breakpoint hits")
		}
	}

	for _, h := range hits {
		ctrl.Resume(h.FlowID, breakpoint.BreakpointResume{
			Body: []byte("edited-" + h.FlowID),
		})
	}

	wg.Wait()

	for i, r := range responses {
		assert.Contains(t, r, "edited-", "response %d should contain edited body", i)
	}
}

func TestE2E_BreakpointOnBodyContentMatch(t *testing.T) {
	var gotBodies []string
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotBodies = append(gotBodies, string(b))
		mu.Unlock()
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	engine := scripting.New()
	ctrl := breakpoint.NewController()
	require.NoError(t, engine.LoadScript("bp", `
		function onRequest(ctx) {
			if (ctx.body && ctx.body.indexOf("token") >= 0) {
				ctx.breakpoint();
			}
		}
	`, ""))

	_, _, port := setupProxy(t, withBreakpoint(engine, ctrl))
	client := proxyClient(port)
	sub := ctrl.Subscribe()

	// Request without token — should pass through without breakpoint.
	resp, err := client.Post(ts.URL+"/bp-match-no", "text/plain", strings.NewReader("no-match"))
	require.NoError(t, err)
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Request with token — should hit breakpoint.
	done := make(chan struct{})
	go func() {
		resp, err := client.Post(ts.URL+"/bp-match-yes", "text/plain", strings.NewReader("has-token-here"))
		if err == nil {
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		close(done)
	}()

	select {
	case hit := <-sub:
		assert.Contains(t, string(hit.Body), "token")
		ctrl.Resume(hit.FlowID, breakpoint.BreakpointResume{Skipped: true})
	case <-time.After(3 * time.Second):
		t.Fatal("expected breakpoint for token request")
	}

	<-done

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, gotBodies, "no-match", "non-matching request should pass through")
}

func TestE2E_ResumeAllOnShutdown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	engine := scripting.New()
	ctrl := breakpoint.NewController()
	require.NoError(t, engine.LoadScript("bp", `
		function onRequest(ctx) { ctx.breakpoint(); }
	`, ""))

	_, _, port := setupProxy(t, withBreakpoint(engine, ctrl))
	client := proxyClient(port)

	const n = 2
	var wg sync.WaitGroup

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := client.Get(fmt.Sprintf("%s/bp-shutdown-%d", ts.URL, idx))
			if err == nil {
				io.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}(i)
	}

	require.Eventually(t, func() bool {
		return len(ctrl.Pending()) == n
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.ResumeAll()
	wg.Wait()

	assert.Empty(t, ctrl.Pending())
}

func TestE2E_ConditionalBreakpointNonMatchingPassThrough(t *testing.T) {
	var gotHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Original")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	engine := scripting.New()
	ctrl := breakpoint.NewController()
	require.NoError(t, engine.LoadScript("bp", `
		function onRequest(ctx) {
			if (ctx.url.indexOf("pause-me") >= 0) {
				ctx.breakpoint();
			}
		}
	`, ""))

	_, _, port := setupProxy(t, withBreakpoint(engine, ctrl))
	client := proxyClient(port)

	req, _ := http.NewRequest("GET", ts.URL+"/no-pause", nil)
	req.Header.Set("X-Original", "present")
	resp, err := client.Do(req)
	require.NoError(t, err)
	io.ReadAll(resp.Body)
	resp.Body.Close()

	assert.Equal(t, "present", gotHeader)
	assert.Empty(t, ctrl.Pending())
}
