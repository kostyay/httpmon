package scripting

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dop251/goja"

	"github.com/kostyay/httpmon/internal/breakpoint"
	"github.com/kostyay/httpmon/internal/store"
)

// RequestContext is passed to JS onRequest handlers.
type RequestContext struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
	Blocked bool
	Meta    store.FlowMeta

	Responded       bool
	ResponseStatus  int
	ResponseHeaders map[string]string
	ResponseBody    []byte
}

// ResponseContext is passed to JS onResponse handlers.
type ResponseContext struct {
	Status  int
	Headers http.Header
	Body    []byte
	Meta    store.FlowMeta

	Responded       bool
	ResponseStatus  int
	ResponseHeaders map[string]string
	ResponseBody    []byte
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
	mu             sync.Mutex
	scripts        []script
	errors         []string
	breakpointCtrl breakpoint.Controller
}

// New creates a scripting engine.
func New() *Engine {
	return &Engine{}
}

// SetBreakpointController injects the breakpoint controller.
func (e *Engine) SetBreakpointController(ctrl breakpoint.Controller) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.breakpointCtrl = ctrl
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
	_ = jsCtx.Set("headers", headersToJS(vm, ctx.Headers))

	if len(ctx.Body) > 0 {
		_ = jsCtx.Set("body", string(ctx.Body))
	}

	e.injectBreakpoint(vm, jsCtx, ctx.Meta, breakpoint.PhaseRequest)
	injectRespondWith(vm, jsCtx, true)
	injectReadFile(vm, jsCtx, s.filePath)

	_, err = fn(goja.Undefined(), jsCtx)
	if isGojaInterrupt(err) {
		err = nil
	}
	if err != nil {
		e.recordError(s.name, err.Error())
		return
	}

	readBackRespondWith(jsCtx, &ctx.Responded,
		&ctx.ResponseStatus, &ctx.ResponseHeaders, &ctx.ResponseBody)
	if ctx.Responded {
		return
	}

	if v := jsCtx.Get("blocked"); v != nil {
		ctx.Blocked = v.ToBoolean()
	}
	if v := jsCtx.Get("method"); v != nil {
		ctx.Method = v.String()
	}
	if v := jsCtx.Get("url"); v != nil {
		ctx.URL = v.String()
	}
	if v := jsCtx.Get("body"); v != nil && !goja.IsUndefined(v) {
		ctx.Body = []byte(v.String())
	}

	readBackHeaders(vm, jsCtx, ctx.Headers)
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
	_ = jsCtx.Set("headers", headersToJS(vm, ctx.Headers))

	e.injectBreakpoint(vm, jsCtx, ctx.Meta, breakpoint.PhaseResponse)
	injectRespondWith(vm, jsCtx, false)
	injectReadFile(vm, jsCtx, s.filePath)

	_, err = fn(goja.Undefined(), jsCtx)
	if err != nil {
		e.recordError(s.name, err.Error())
		return
	}

	readBackRespondWith(jsCtx, &ctx.Responded,
		&ctx.ResponseStatus, &ctx.ResponseHeaders, &ctx.ResponseBody)
	if ctx.Responded {
		return
	}

	if v := jsCtx.Get("status"); v != nil {
		ctx.Status = int(v.ToInteger())
	}
	if v := jsCtx.Get("body"); v != nil && !goja.IsUndefined(v) {
		ctx.Body = []byte(v.String())
	}

	readBackHeaders(vm, jsCtx, ctx.Headers)
}

// injectBreakpoint adds ctx.breakpoint() to the JS context.
// No-op when controller is nil.
func (e *Engine) injectBreakpoint(
	vm *goja.Runtime, jsCtx *goja.Object,
	meta store.FlowMeta, phase breakpoint.Phase,
) {
	ctrl := e.breakpointCtrl
	if ctrl == nil {
		_ = jsCtx.Set("breakpoint", func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		return
	}

	_ = jsCtx.Set("breakpoint", func(call goja.FunctionCall) goja.Value {
		headers := jsHeadersToMap(vm, jsCtx)
		var body []byte
		if v := jsCtx.Get("body"); v != nil && !goja.IsUndefined(v) {
			body = []byte(v.String())
		}

		hit := breakpoint.BreakpointHit{
			FlowID:  meta.ID,
			Phase:   phase,
			Headers: headers,
			Body:    body,
			Meta:    meta,
		}

		resp := ctrl.Pause(hit)
		if resp.Skipped {
			return goja.Undefined()
		}

		for k, v := range resp.Headers {
			hObj := jsCtx.Get("headers")
			if hObj != nil && !goja.IsUndefined(hObj) {
				_ = hObj.ToObject(vm).Set(k, v)
			}
		}
		if resp.Body != nil {
			_ = jsCtx.Set("body", string(resp.Body))
		}
		return goja.Undefined()
	})
}

// injectRespondWith adds ctx.respondWith(opts) to the JS context.
// In onRequest (interrupt=true), it halts script execution via goja interrupt.
func injectRespondWith(
	vm *goja.Runtime, jsCtx *goja.Object, interrupt bool,
) {
	_ = jsCtx.Set("respondWith", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		opts := call.Arguments[0].ToObject(vm)

		status := 200
		if v := opts.Get("status"); v != nil && !goja.IsUndefined(v) {
			status = int(v.ToInteger())
		}

		var body string
		var headers map[string]string

		if f := opts.Get("file"); f != nil && !goja.IsUndefined(f) {
			path := f.String()
			data, err := safeReadFile(scriptDirFromCtx(jsCtx), path)
			if err != nil {
				return goja.Undefined()
			}
			body = string(data)
			ct := mime.TypeByExtension(filepath.Ext(path))
			if ct == "" {
				ct = "application/octet-stream"
			}
			headers = map[string]string{"Content-Type": ct}
		} else {
			if b := opts.Get("body"); b != nil && !goja.IsUndefined(b) {
				body = b.String()
			}
			if h := opts.Get("headers"); h != nil && !goja.IsUndefined(h) {
				headers = make(map[string]string)
				hObj := h.ToObject(vm)
				for _, k := range hObj.Keys() {
					if val := hObj.Get(k); val != nil {
						headers[k] = val.String()
					}
				}
			}
		}

		_ = jsCtx.Set("_responded", true)
		_ = jsCtx.Set("_responseStatus", status)
		_ = jsCtx.Set("_responseBody", body)

		if headers != nil {
			hObj := vm.NewObject()
			for k, v := range headers {
				_ = hObj.Set(k, v)
			}
			_ = jsCtx.Set("_responseHeaders", hObj)
		}

		if interrupt {
			vm.Interrupt("respondWith")
		}
		return goja.Undefined()
	})
}

// scriptDirFromCtx returns the script directory stored in the JS context.
func scriptDirFromCtx(jsCtx *goja.Object) string {
	if v := jsCtx.Get("_scriptDir"); v != nil && !goja.IsUndefined(v) {
		return v.String()
	}
	return ""
}

// safeReadFile reads a file scoped under rootDir using os.Root
// to prevent directory traversal (G304).
func safeReadFile(rootDir, path string) ([]byte, error) {
	if filepath.IsAbs(path) {
		rootDir = filepath.Dir(path)
		path = filepath.Base(path)
	} else if rootDir == "" {
		rootDir = "."
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// injectReadFile adds ctx.readFile(path) to the JS context.
func injectReadFile(
	vm *goja.Runtime, jsCtx *goja.Object, scriptFilePath string,
) {
	if scriptFilePath != "" {
		_ = jsCtx.Set("_scriptDir", filepath.Dir(scriptFilePath))
	}

	_ = jsCtx.Set("readFile", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		path := call.Arguments[0].String()
		data, err := safeReadFile(scriptDirFromCtx(jsCtx), path)
		if err != nil {
			return goja.Null()
		}
		return vm.ToValue(string(data))
	})
}

// readBackRespondWith reads the responded flag and response fields from JS.
func readBackRespondWith(
	jsCtx *goja.Object,
	responded *bool, status *int,
	headers *map[string]string, body *[]byte,
) {
	v := jsCtx.Get("_responded")
	if v == nil || goja.IsUndefined(v) || !v.ToBoolean() {
		return
	}
	*responded = true

	if s := jsCtx.Get("_responseStatus"); s != nil && !goja.IsUndefined(s) {
		*status = int(s.ToInteger())
	}
	if b := jsCtx.Get("_responseBody"); b != nil && !goja.IsUndefined(b) {
		*body = []byte(b.String())
	}
	if h := jsCtx.Get("_responseHeaders"); h != nil && !goja.IsUndefined(h) {
		obj := h.Export()
		if m, ok := obj.(map[string]any); ok {
			result := make(map[string]string, len(m))
			for k, val := range m {
				result[k] = fmt.Sprintf("%v", val)
			}
			*headers = result
		}
	}
}

// isGojaInterrupt returns true if the error is a goja interrupt.
func isGojaInterrupt(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*goja.InterruptedError)
	return ok
}

// jsHeadersToMap reads the JS headers object as map[string]string.
func jsHeadersToMap(
	vm *goja.Runtime, jsCtx *goja.Object,
) map[string]string {
	result := make(map[string]string)
	hObj := jsCtx.Get("headers")
	if hObj == nil || goja.IsUndefined(hObj) || goja.IsNull(hObj) {
		return result
	}
	obj := hObj.ToObject(vm)
	for _, key := range obj.Keys() {
		if val := obj.Get(key); val != nil {
			result[key] = val.String()
		}
	}
	return result
}

// headersToJS converts http.Header to a goja object (single-value per key).
func headersToJS(vm *goja.Runtime, h http.Header) *goja.Object {
	obj := vm.NewObject()
	for k := range h {
		_ = obj.Set(k, h.Get(k))
	}
	return obj
}

// readBackHeaders copies JS header object modifications back into http.Header.
func readBackHeaders(vm *goja.Runtime, jsCtx *goja.Object, h http.Header) {
	hObj := jsCtx.Get("headers")
	if hObj == nil || goja.IsUndefined(hObj) || goja.IsNull(hObj) {
		return
	}
	obj := hObj.ToObject(vm)
	for _, key := range obj.Keys() {
		if val := obj.Get(key); val != nil {
			h.Set(key, val.String())
		}
	}
}

func (e *Engine) recordError(script, msg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errors = append(e.errors, fmt.Sprintf("[%s] %s", script, msg))
}

// matchesURL returns true when the script should run for the given URL.
func (s script) matchesURL(url string) bool {
	if s.meta != nil {
		return GlobMatchAny(s.meta.Match, url)
	}
	if s.urlPattern == "" {
		return true
	}
	return matchURL(s.urlPattern, url)
}

// ScriptCategory classifies scripts by detected API usage.
type ScriptCategory string

const (
	CategoryScript     ScriptCategory = "Script"
	CategoryMapLocal   ScriptCategory = "Map Local"
	CategoryBreakpoint ScriptCategory = "Breakpoint"
)

// DetectCategories returns categories based on static source analysis.
func DetectCategories(source string) []ScriptCategory {
	var cats []ScriptCategory
	if strings.Contains(source, "ctx.breakpoint()") ||
		strings.Contains(source, "ctx.breakpoint(") {
		cats = append(cats, CategoryBreakpoint)
	}
	if strings.Contains(source, "ctx.respondWith(") {
		cats = append(cats, CategoryMapLocal)
	}
	if len(cats) == 0 {
		cats = append(cats, CategoryScript)
	}
	return cats
}

// ScriptInfo describes a loaded script for TUI display.
type ScriptInfo struct {
	ID         string
	Name       string
	Matches    []string
	Enabled    bool
	FilePath   string
	Error      string
	Categories []ScriptCategory
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
			Name:       s.name,
			FilePath:   s.filePath,
			Enabled:    true,
			Categories: DetectCategories(s.source),
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
