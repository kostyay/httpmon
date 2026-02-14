package scripting

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kostyay/httpmon/internal/breakpoint"
	"github.com/kostyay/httpmon/internal/store"
)

func TestBreakpointOnRequestPausesAndResumes(t *testing.T) {
	e := New()
	ctrl := breakpoint.NewController()
	e.SetBreakpointController(ctrl)

	err := e.LoadScript("bp", `
		function onRequest(ctx) {
			ctx.breakpoint();
		}
	`, "")
	require.NoError(t, err)

	sub := ctrl.Subscribe()

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{"Accept": {"text/html"}},
		Body:    []byte("original body"),
		Meta:    store.FlowMeta{ID: "flow-1", Method: "GET", Host: "example.com"},
	}

	done := make(chan struct{})
	go func() {
		e.RunOnRequest(ctx)
		close(done)
	}()

	select {
	case hit := <-sub:
		assert.Equal(t, "flow-1", hit.FlowID)
		assert.Equal(t, breakpoint.PhaseRequest, hit.Phase)
		assert.Equal(t, "original body", string(hit.Body))

		ctrl.Resume("flow-1", breakpoint.BreakpointResume{
			Headers: map[string]string{"X-Modified": "true"},
			Body:    []byte("modified body"),
		})
	case <-time.After(2 * time.Second):
		t.Fatal("breakpoint hit not received")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnRequest did not return after resume")
	}

	assert.Equal(t, "modified body", string(ctx.Body))
	assert.Equal(t, "true", ctx.Headers.Get("X-Modified"))
}

func TestBreakpointOnResponsePausesAndResumes(t *testing.T) {
	e := New()
	ctrl := breakpoint.NewController()
	e.SetBreakpointController(ctrl)

	err := e.LoadScript("bp", `
		function onResponse(ctx) {
			ctx.breakpoint();
		}
	`, "")
	require.NoError(t, err)

	sub := ctrl.Subscribe()

	ctx := &ResponseContext{
		Status:  200,
		Headers: http.Header{"Content-Type": {"application/json"}},
		Body:    []byte(`{"key":"value"}`),
		Meta:    store.FlowMeta{ID: "flow-2", Method: "GET", Host: "example.com"},
	}

	done := make(chan struct{})
	go func() {
		e.RunOnResponse(ctx, "https://example.com/api")
		close(done)
	}()

	select {
	case hit := <-sub:
		assert.Equal(t, "flow-2", hit.FlowID)
		assert.Equal(t, breakpoint.PhaseResponse, hit.Phase)
		ctrl.Resume("flow-2", breakpoint.BreakpointResume{
			Body: []byte("new body"),
		})
	case <-time.After(2 * time.Second):
		t.Fatal("breakpoint hit not received")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnResponse did not return after resume")
	}

	assert.Equal(t, "new body", string(ctx.Body))
}

func TestBreakpointScriptContinuesAfterResume(t *testing.T) {
	e := New()
	ctrl := breakpoint.NewController()
	e.SetBreakpointController(ctrl)

	err := e.LoadScript("bp", `
		function onRequest(ctx) {
			ctx.breakpoint();
			ctx.headers["X-Post-BP"] = "continued";
		}
	`, "")
	require.NoError(t, err)

	sub := ctrl.Subscribe()

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{},
		Meta:    store.FlowMeta{ID: "flow-3"},
	}

	done := make(chan struct{})
	go func() {
		e.RunOnRequest(ctx)
		close(done)
	}()

	hit := <-sub
	ctrl.Resume(hit.FlowID, breakpoint.BreakpointResume{
		Headers: map[string]string{"X-BP-Edit": "yes"},
		Body:    []byte("bp body"),
	})

	<-done

	assert.Equal(t, "yes", ctx.Headers.Get("X-BP-Edit"))
	assert.Equal(t, "continued", ctx.Headers.Get("X-Post-BP"))
}

func TestBreakpointModifiedHeadersVisibleInCtx(t *testing.T) {
	e := New()
	ctrl := breakpoint.NewController()
	e.SetBreakpointController(ctrl)

	err := e.LoadScript("bp", `
		function onRequest(ctx) {
			ctx.breakpoint();
			if (ctx.headers["X-Injected"] === "by-user") {
				ctx.headers["X-Confirmed"] = "true";
			}
		}
	`, "")
	require.NoError(t, err)

	sub := ctrl.Subscribe()

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{},
		Meta:    store.FlowMeta{ID: "flow-4"},
	}

	done := make(chan struct{})
	go func() {
		e.RunOnRequest(ctx)
		close(done)
	}()

	<-sub
	ctrl.Resume("flow-4", breakpoint.BreakpointResume{
		Headers: map[string]string{"X-Injected": "by-user"},
	})

	<-done

	assert.Equal(t, "true", ctx.Headers.Get("X-Confirmed"))
}

func TestBreakpointNilControllerIsNoop(t *testing.T) {
	e := New()

	err := e.LoadScript("bp", `
		function onRequest(ctx) {
			ctx.breakpoint();
			ctx.headers["X-Ran"] = "yes";
		}
	`, "")
	require.NoError(t, err)

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{},
	}

	e.RunOnRequest(ctx)

	assert.Equal(t, "yes", ctx.Headers.Get("X-Ran"))
}

func TestBreakpointMultipleCallsIndependent(t *testing.T) {
	e := New()
	ctrl := breakpoint.NewController()
	e.SetBreakpointController(ctrl)

	err := e.LoadScript("bp", `
		function onRequest(ctx) {
			ctx.breakpoint();
			ctx.headers["X-After-First"] = ctx.body;
			ctx.breakpoint();
			ctx.headers["X-After-Second"] = ctx.body;
		}
	`, "")
	require.NoError(t, err)

	sub := ctrl.Subscribe()

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{},
		Body:    []byte("initial"),
		Meta:    store.FlowMeta{ID: "flow-5"},
	}

	done := make(chan struct{})
	go func() {
		e.RunOnRequest(ctx)
		close(done)
	}()

	<-sub
	ctrl.Resume("flow-5", breakpoint.BreakpointResume{Body: []byte("first-edit")})

	<-sub
	ctrl.Resume("flow-5", breakpoint.BreakpointResume{Body: []byte("second-edit")})

	<-done

	assert.Equal(t, "first-edit", ctx.Headers.Get("X-After-First"))
	assert.Equal(t, "second-edit", ctx.Headers.Get("X-After-Second"))
}

func TestBreakpointSkipReturnsOriginal(t *testing.T) {
	e := New()
	ctrl := breakpoint.NewController()
	e.SetBreakpointController(ctrl)

	err := e.LoadScript("bp", `
		function onRequest(ctx) {
			ctx.breakpoint();
		}
	`, "")
	require.NoError(t, err)

	sub := ctrl.Subscribe()

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{"Original": {"yes"}},
		Body:    []byte("original"),
		Meta:    store.FlowMeta{ID: "flow-6"},
	}

	done := make(chan struct{})
	go func() {
		e.RunOnRequest(ctx)
		close(done)
	}()

	<-sub
	ctrl.Resume("flow-6", breakpoint.BreakpointResume{Skipped: true})

	<-done

	assert.Equal(t, "original", string(ctx.Body))
	assert.Equal(t, "yes", ctx.Headers.Get("Original"))
}
