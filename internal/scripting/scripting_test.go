package scripting

import (
	"net/http"
	"testing"
)

func TestGojaRequestModify(t *testing.T) {
	e := New()
	err := e.LoadScript("test", `
		function onRequest(ctx) {
			ctx.headers["X-Injected"] = "hello";
		}
	`, "")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{"Accept": {"application/json"}},
	}

	e.RunOnRequest(ctx)

	if ctx.Headers.Get("X-Injected") != "hello" {
		t.Error("onRequest should have added X-Injected header")
	}
}

func TestGojaResponseModify(t *testing.T) {
	e := New()
	err := e.LoadScript("test", `
		function onResponse(ctx) {
			ctx.status = 418;
			ctx.headers["X-Custom"] = "modified";
		}
	`, "")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &ResponseContext{
		Status:  200,
		Headers: http.Header{"Content-Type": {"text/html"}},
		Body:    []byte("original"),
	}

	e.RunOnResponse(ctx, "example.com/api")

	if ctx.Status != 418 {
		t.Errorf("status = %d, want 418", ctx.Status)
	}
	if ctx.Headers.Get("X-Custom") != "modified" {
		t.Error("onResponse should have set X-Custom header")
	}
}

func TestGojaBlockRequest(t *testing.T) {
	e := New()
	err := e.LoadScript("test", `
		function onRequest(ctx) {
			ctx.blocked = true;
		}
	`, "")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{},
	}

	e.RunOnRequest(ctx)

	if !ctx.Blocked {
		t.Error("request should be blocked")
	}
}

func TestGojaScriptError(t *testing.T) {
	e := New()
	err := e.LoadScript("test", `
		function onRequest(ctx) {
			throw new Error("intentional");
		}
	`, "")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/api",
		Headers: http.Header{},
	}

	// Should not panic.
	e.RunOnRequest(ctx)

	if len(e.Errors()) == 0 {
		t.Error("should have recorded an error")
	}
}

func TestEngine_LoadFromDir(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "test.js", `// ---
// name: Add Header
// match:
//   - "*://example.com/*"
// enabled: true
// ---
function onRequest(ctx) {
    ctx.headers["X-Script"] = "loaded";
}
`)
	writeTestScript(t, dir, "disabled.js", `// ---
// name: Disabled
// match:
//   - "*://*/*"
// enabled: false
// ---
function onRequest(ctx) {
    ctx.headers["X-Disabled"] = "yes";
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

	if ctx.Headers.Get("X-Script") != "loaded" {
		t.Error("enabled script should run")
	}
	if ctx.Headers.Get("X-Disabled") != "" {
		t.Error("disabled script should not run")
	}
}

func TestEngine_GlobFiltering(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "api-only.js", `// ---
// name: API Only
// match:
//   - "*://api.example.com/*"
// ---
function onRequest(ctx) {
    ctx.headers["X-API"] = "yes";
}
`)

	e := New()
	e.LoadFromDir(dir)

	// Should match.
	ctx1 := &RequestContext{
		Method:  "GET",
		URL:     "https://api.example.com/users",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx1)
	if ctx1.Headers.Get("X-API") != "yes" {
		t.Error("should match api.example.com")
	}

	// Should not match.
	ctx2 := &RequestContext{
		Method:  "GET",
		URL:     "https://other.com/users",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx2)
	if ctx2.Headers.Get("X-API") != "" {
		t.Error("should not match other.com")
	}
}

func TestEngine_Reload(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "a.js", `// ---
// name: Script A
// match:
//   - "*://*/*"
// ---
function onRequest(ctx) {
    ctx.headers["X-A"] = "yes";
}
`)

	e := New()
	e.LoadFromDir(dir)

	infos := e.ScriptInfos()
	if len(infos) != 1 {
		t.Fatalf("expected 1 script info, got %d", len(infos))
	}

	// Add another script file.
	writeTestScript(t, dir, "b.js", `// ---
// name: Script B
// match:
//   - "*://*/*"
// ---
function onRequest(ctx) {}
`)

	e.Reload(dir)
	infos = e.ScriptInfos()
	if len(infos) != 2 {
		t.Fatalf("expected 2 script infos after reload, got %d", len(infos))
	}
}

// --- Integration smoke tests ---

func TestIntegration_FullScriptLifecycle(t *testing.T) {
	dir := t.TempDir()

	// Script that modifies both request and response.
	writeTestScript(t, dir, "rewrite.js", `// ---
// name: Full Rewrite
// match:
//   - "*://api.example.com/*"
// enabled: true
// ---
function onRequest(ctx) {
    ctx.headers["X-Req-Modified"] = "yes";
    ctx.method = "POST";
}

function onResponse(ctx) {
    ctx.headers["X-Resp-Modified"] = "yes";
    ctx.status = 201;
}
`)

	e := New()
	e.LoadFromDir(dir)

	// Test request modification.
	reqCtx := &RequestContext{
		Method:  "GET",
		URL:     "https://api.example.com/users",
		Headers: http.Header{},
	}
	e.RunOnRequest(reqCtx)

	if reqCtx.Method != "POST" {
		t.Errorf("method = %q, want POST", reqCtx.Method)
	}
	if reqCtx.Headers.Get("X-Req-Modified") != "yes" {
		t.Error("request header not modified")
	}
	if reqCtx.Blocked {
		t.Error("should not be blocked")
	}

	// Test response modification.
	respCtx := &ResponseContext{
		Status:  200,
		Headers: http.Header{},
		Body:    []byte("original"),
	}
	e.RunOnResponse(respCtx, "https://api.example.com/users")

	if respCtx.Status != 201 {
		t.Errorf("status = %d, want 201", respCtx.Status)
	}
	if respCtx.Headers.Get("X-Resp-Modified") != "yes" {
		t.Error("response header not modified")
	}
}

func TestIntegration_BlockedRequest(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "blocker.js", `// ---
// name: Block All
// match:
//   - "*://*/*"
// ---
function onRequest(ctx) {
    ctx.blocked = true;
}
`)

	e := New()
	e.LoadFromDir(dir)

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/blocked",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx)

	if !ctx.Blocked {
		t.Error("request should be blocked")
	}
}

func TestIntegration_DisabledScriptSkipped(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "disabled.js", `// ---
// name: Should Not Run
// match:
//   - "*://*/*"
// enabled: false
// ---
function onRequest(ctx) {
    ctx.headers["X-Should-Not-Exist"] = "true";
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

	if ctx.Headers.Get("X-Should-Not-Exist") != "" {
		t.Error("disabled script should not have run")
	}
}

func TestIntegration_NonMatchingURLSkipped(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "targeted.js", `// ---
// name: API Only
// match:
//   - "*://api.example.com/*"
// ---
function onRequest(ctx) {
    ctx.headers["X-API"] = "matched";
}
`)

	e := New()
	e.LoadFromDir(dir)

	ctx := &RequestContext{
		Method:  "GET",
		URL:     "https://other.example.com/test",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx)

	if ctx.Headers.Get("X-API") != "" {
		t.Error("non-matching URL should not trigger script")
	}
}

func TestIntegration_ManagerToggleAndReload(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "toggle.js", `// ---
// name: Toggle Test
// match:
//   - "*://*/*"
// enabled: true
// ---
function onRequest(ctx) {
    ctx.headers["X-Toggle"] = "active";
}
`)

	e := New()
	mgr := NewManager(e, dir)
	mgr.Reload()

	// Initially enabled - should run.
	ctx1 := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/test",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx1)
	if ctx1.Headers.Get("X-Toggle") != "active" {
		t.Error("enabled script should run")
	}

	// Toggle to disabled.
	scripts := mgr.Scripts()
	if len(scripts) != 1 {
		t.Fatalf("expected 1 script, got %d", len(scripts))
	}
	if err := mgr.Toggle(scripts[0].FilePath); err != nil {
		t.Fatal(err)
	}

	// Should not run now.
	ctx2 := &RequestContext{
		Method:  "GET",
		URL:     "https://example.com/test",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx2)
	if ctx2.Headers.Get("X-Toggle") != "" {
		t.Error("disabled script should not run after toggle")
	}
}

func TestGojaURLPattern(t *testing.T) {
	e := New()
	err := e.LoadScript("test", `
		function onRequest(ctx) {
			ctx.headers["X-Matched"] = "yes";
		}
	`, "api.example.com/*")
	if err != nil {
		t.Fatal(err)
	}

	// Matching URL.
	ctx1 := &RequestContext{
		Method:  "GET",
		URL:     "https://api.example.com/v1/users",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx1)
	if ctx1.Headers.Get("X-Matched") != "yes" {
		t.Error("should match api.example.com/*")
	}

	// Non-matching URL.
	ctx2 := &RequestContext{
		Method:  "GET",
		URL:     "https://other.com/foo",
		Headers: http.Header{},
	}
	e.RunOnRequest(ctx2)
	if ctx2.Headers.Get("X-Matched") != "" {
		t.Error("should NOT match other.com")
	}
}
