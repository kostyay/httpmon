package scripting

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

// RequestContext is passed to JS onRequest handlers.
type RequestContext struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
	Blocked bool
}

// ResponseContext is passed to JS onResponse handlers.
type ResponseContext struct {
	Status  int
	Headers http.Header
	Body    []byte
}

type script struct {
	name       string
	source     string
	urlPattern string      // legacy: used by LoadScript()
	meta       *ScriptMeta // used by LoadFromDir()
	filePath   string      // set for dir-loaded scripts
	fromDir    bool        // true if loaded via LoadFromDir/Reload
}

// Engine manages loaded JS scripts and runs them via goja.
type Engine struct {
	mu      sync.Mutex
	scripts []script
	errors  []string
}

// New creates a scripting engine.
func New() *Engine {
	return &Engine{}
}

// LoadScript adds a script with an optional URL pattern filter.
func (e *Engine) LoadScript(name, source, urlPattern string) error {
	// Validate by compiling once.
	_, err := goja.Compile(name, source, false)
	if err != nil {
		return fmt.Errorf("compile %s: %w", name, err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scripts = append(e.scripts, script{name: name, source: source, urlPattern: urlPattern})
	return nil
}

// Errors returns recorded runtime errors.
func (e *Engine) Errors() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.errors...)
}

// RunOnRequest executes all scripts' onRequest against the context.
func (e *Engine) RunOnRequest(ctx *RequestContext) {
	e.mu.Lock()
	scripts := append([]script(nil), e.scripts...)
	e.mu.Unlock()

	for _, s := range scripts {
		if !s.matchesURL(ctx.URL) {
			continue
		}
		e.runOnRequest(s, ctx)
	}
}

// RunOnResponse executes all scripts' onResponse against the context.
func (e *Engine) RunOnResponse(ctx *ResponseContext, hostPath string) {
	e.mu.Lock()
	scripts := append([]script(nil), e.scripts...)
	e.mu.Unlock()

	for _, s := range scripts {
		if !s.matchesURL(hostPath) {
			continue
		}
		e.runOnResponse(s, ctx)
	}
}

func (e *Engine) runOnRequest(s script, ctx *RequestContext) {
	defer func() {
		if r := recover(); r != nil {
			e.recordError(s.name, fmt.Sprintf("panic: %v", r))
		}
	}()

	vm := goja.New()
	_, err := vm.RunString(s.source)
	if err != nil {
		e.recordError(s.name, err.Error())
		return
	}

	fn, ok := goja.AssertFunction(vm.Get("onRequest"))
	if !ok {
		return // Script doesn't export onRequest.
	}

	jsCtx := vm.NewObject()
	_ = jsCtx.Set("method", ctx.Method)
	_ = jsCtx.Set("url", ctx.URL)
	_ = jsCtx.Set("blocked", false)

	// Convert headers to plain object.
	hdrs := vm.NewObject()
	for k := range ctx.Headers {
		_ = hdrs.Set(k, ctx.Headers.Get(k))
	}
	_ = jsCtx.Set("headers", hdrs)

	if len(ctx.Body) > 0 {
		_ = jsCtx.Set("body", string(ctx.Body))
	}

	_, err = fn(goja.Undefined(), jsCtx)
	if err != nil {
		e.recordError(s.name, err.Error())
		return
	}

	// Read back modified values.
	if v := jsCtx.Get("blocked"); v != nil {
		ctx.Blocked = v.ToBoolean()
	}
	if v := jsCtx.Get("method"); v != nil {
		ctx.Method = v.String()
	}
	if v := jsCtx.Get("url"); v != nil {
		ctx.URL = v.String()
	}

	// Read back headers.
	hObj := jsCtx.Get("headers")
	if hObj != nil && !goja.IsUndefined(hObj) && !goja.IsNull(hObj) {
		obj := hObj.ToObject(vm)
		for _, key := range obj.Keys() {
			val := obj.Get(key)
			if val != nil {
				ctx.Headers.Set(key, val.String())
			}
		}
	}
}

func (e *Engine) runOnResponse(s script, ctx *ResponseContext) {
	defer func() {
		if r := recover(); r != nil {
			e.recordError(s.name, fmt.Sprintf("panic: %v", r))
		}
	}()

	vm := goja.New()
	_, err := vm.RunString(s.source)
	if err != nil {
		e.recordError(s.name, err.Error())
		return
	}

	fn, ok := goja.AssertFunction(vm.Get("onResponse"))
	if !ok {
		return
	}

	jsCtx := vm.NewObject()
	_ = jsCtx.Set("status", ctx.Status)
	_ = jsCtx.Set("body", string(ctx.Body))

	hdrs := vm.NewObject()
	for k := range ctx.Headers {
		_ = hdrs.Set(k, ctx.Headers.Get(k))
	}
	_ = jsCtx.Set("headers", hdrs)

	_, err = fn(goja.Undefined(), jsCtx)
	if err != nil {
		e.recordError(s.name, err.Error())
		return
	}

	// Read back.
	if v := jsCtx.Get("status"); v != nil {
		ctx.Status = int(v.ToInteger())
	}

	hObj := jsCtx.Get("headers")
	if hObj != nil && !goja.IsUndefined(hObj) && !goja.IsNull(hObj) {
		obj := hObj.ToObject(vm)
		for _, key := range obj.Keys() {
			val := obj.Get(key)
			if val != nil {
				ctx.Headers.Set(key, val.String())
			}
		}
	}
}

func (e *Engine) recordError(script, msg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errors = append(e.errors, fmt.Sprintf("[%s] %s", script, msg))
}

// matchesURL returns true when the script should run for the given URL.
func (s *script) matchesURL(url string) bool {
	if s.meta != nil {
		return GlobMatchAny(s.meta.Match, url)
	}
	if s.urlPattern == "" {
		return true
	}
	return matchURL(s.urlPattern, url)
}

// ScriptInfo describes a loaded script for TUI display.
type ScriptInfo struct {
	Name     string
	Matches  []string
	Enabled  bool
	FilePath string
	Error    string
}

// ScriptInfos returns info about all dir-loaded scripts (including errors).
func (e *Engine) ScriptInfos() []ScriptInfo {
	e.mu.Lock()
	defer e.mu.Unlock()

	var infos []ScriptInfo
	for _, s := range e.scripts {
		if !s.fromDir {
			continue
		}
		info := ScriptInfo{
			Name:     s.name,
			FilePath: s.filePath,
			Enabled:  true,
		}
		if s.meta != nil {
			info.Matches = s.meta.Match
			info.Enabled = s.meta.IsEnabled()
		}
		infos = append(infos, info)
	}
	return infos
}

// LoadFromDir loads all scripts from dir via LoadDir.
// Enabled scripts are added to the engine; disabled scripts are tracked for ScriptInfos.
func (e *Engine) LoadFromDir(dir string) {
	scripts, errs := LoadDir(dir)

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, sf := range scripts {
		s := script{
			name:     sf.Meta.Name,
			source:   sf.Source,
			meta:     sf.Meta,
			filePath: sf.FilePath,
			fromDir:  true,
		}
		if sf.Meta.IsEnabled() {
			e.scripts = append(e.scripts, s)
		}
	}

	// Track errored scripts for ScriptInfos display.
	for _, sf := range errs {
		e.errors = append(e.errors, fmt.Sprintf("[%s] %s", sf.Meta.Name, sf.Error))
	}
}

// Reload clears dir-loaded scripts and re-loads from dir.
// Manually loaded scripts (via LoadScript) are kept intact.
func (e *Engine) Reload(dir string) {
	e.mu.Lock()
	var kept []script
	for _, s := range e.scripts {
		if !s.fromDir {
			kept = append(kept, s)
		}
	}
	e.scripts = kept
	e.mu.Unlock()

	e.LoadFromDir(dir)
}

// matchURL checks if a URL matches a pattern (supports * wildcards).
func matchURL(pattern, rawURL string) bool {
	// Extract host+path from URL.
	hostPath := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		hostPath = u.Host + u.Path
	}

	pattern = strings.ToLower(pattern)
	hostPath = strings.ToLower(hostPath)

	if !strings.Contains(pattern, "*") {
		return strings.Contains(hostPath, pattern)
	}

	parts := strings.Split(pattern, "*")
	pos := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(hostPath[pos:], part)
		if idx < 0 {
			return false
		}
		pos += idx + len(part)
	}
	return true
}
