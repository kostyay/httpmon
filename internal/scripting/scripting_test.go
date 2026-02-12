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
