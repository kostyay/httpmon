# Scripts Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Greasemonkey-style JS scripts that rewrite HTTP requests/responses, managed via a TUI modal.

**Architecture:** YAML-frontmatter `.js` files in `~/.httpmon/scripts/`. Header parser extracts metadata. Glob matcher filters by URL. Engine runs matching scripts in interceptor hooks. TUI modal manages scripts via `ScriptManager` interface.

**Tech Stack:** Go, goja (JS VM), gopkg.in/yaml.v3, Bubble Tea, lipgloss.

**Design doc:** `docs/plans/2026-02-12-scripts-design.md`

---

### Task 1: YAML Header Parser

**Files:**
- Create: `internal/scripting/header.go`
- Create: `internal/scripting/header_test.go`

**Step 1: Write failing tests**

```go
// internal/scripting/header_test.go
package scripting

import (
	"testing"
)

func TestParseHeader_Valid(t *testing.T) {
	src := `// ---
// name: Strip Auth
// match:
//   - "*://api.example.com/*"
//   - "*://staging.example.com/*"
// enabled: true
// ---

function onRequest(ctx) {}
`
	meta, body, err := ParseHeader(src)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name != "Strip Auth" {
		t.Errorf("name = %q, want %q", meta.Name, "Strip Auth")
	}
	if len(meta.Match) != 2 {
		t.Fatalf("match len = %d, want 2", len(meta.Match))
	}
	if meta.Match[0] != "*://api.example.com/*" {
		t.Errorf("match[0] = %q", meta.Match[0])
	}
	if !meta.Enabled {
		t.Error("enabled should be true")
	}
	if body != "\nfunction onRequest(ctx) {}\n" {
		t.Errorf("body = %q", body)
	}
}

func TestParseHeader_DefaultEnabled(t *testing.T) {
	src := `// ---
// name: Test
// match:
//   - "*://*/*"
// ---
function onRequest(ctx) {}
`
	meta, _, err := ParseHeader(src)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.Enabled {
		t.Error("enabled should default to true")
	}
}

func TestParseHeader_NoHeader(t *testing.T) {
	src := `function onRequest(ctx) {}`
	_, _, err := ParseHeader(src)
	if err == nil {
		t.Error("expected error for missing header")
	}
}

func TestParseHeader_MissingName(t *testing.T) {
	src := `// ---
// match:
//   - "*://*/*"
// ---
function onRequest(ctx) {}
`
	_, _, err := ParseHeader(src)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestParseHeader_MissingMatch(t *testing.T) {
	src := `// ---
// name: Test
// ---
function onRequest(ctx) {}
`
	_, _, err := ParseHeader(src)
	if err == nil {
		t.Error("expected error for missing match")
	}
}
```

**Step 2: Run tests, verify they fail**

Run: `go test -v -run TestParseHeader ./internal/scripting/`
Expected: FAIL — `ParseHeader` undefined.

**Step 3: Implement header parser**

```go
// internal/scripting/header.go
package scripting

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ScriptMeta holds parsed YAML frontmatter from a script file.
type ScriptMeta struct {
	Name    string   `yaml:"name"`
	Match   []string `yaml:"match"`
	Enabled *bool    `yaml:"enabled,omitempty"`
}

// ParseHeader extracts YAML frontmatter from a script source.
// Returns the parsed metadata and the JS body after the closing delimiter.
func ParseHeader(source string) (*ScriptMeta, string, error) {
	const delim = "// ---"

	idx := strings.Index(source, delim)
	if idx < 0 {
		return nil, "", fmt.Errorf("missing opening %q delimiter", delim)
	}

	rest := source[idx+len(delim):]
	end := strings.Index(rest, delim)
	if end < 0 {
		return nil, "", fmt.Errorf("missing closing %q delimiter", delim)
	}

	rawYAML := rest[:end]
	body := rest[end+len(delim):]

	// Strip "// " prefix from each line.
	var yamlLines []string
	for _, line := range strings.Split(rawYAML, "\n") {
		line = strings.TrimPrefix(line, "// ")
		line = strings.TrimPrefix(line, "//")
		yamlLines = append(yamlLines, line)
	}
	cleaned := strings.Join(yamlLines, "\n")

	var meta ScriptMeta
	if err := yaml.Unmarshal([]byte(cleaned), &meta); err != nil {
		return nil, "", fmt.Errorf("parse YAML header: %w", err)
	}

	if meta.Name == "" {
		return nil, "", fmt.Errorf("missing required field: name")
	}
	if len(meta.Match) == 0 {
		return nil, "", fmt.Errorf("missing required field: match")
	}

	// Default enabled to true.
	if meta.Enabled == nil {
		t := true
		meta.Enabled = &t
	}

	return &meta, body, nil
}

// IsEnabled returns whether the script is enabled (nil defaults to true).
func (m *ScriptMeta) IsEnabled() bool {
	return m.Enabled == nil || *m.Enabled
}
```

**Step 4: Add yaml.v3 dependency**

Run: `go get gopkg.in/yaml.v3`

**Step 5: Run tests, verify they pass**

Run: `go test -v -run TestParseHeader ./internal/scripting/`
Expected: PASS

**Step 6: Commit**

```
feat(scripting): add YAML header parser for script frontmatter
```

---

### Task 2: Glob URL Matcher

**Files:**
- Create: `internal/scripting/glob.go`
- Create: `internal/scripting/glob_test.go`

**Step 1: Write failing tests**

```go
// internal/scripting/glob_test.go
package scripting

import "testing"

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		url     string
		want    bool
	}{
		// Exact match
		{"http://example.com/api", "http://example.com/api", true},
		// Wildcard scheme
		{"*://example.com/api", "https://example.com/api", true},
		{"*://example.com/api", "http://example.com/api", true},
		// Wildcard path
		{"*://example.com/*", "https://example.com/foo/bar", true},
		{"*://example.com/*", "https://example.com/", true},
		// Wildcard subdomain
		{"*://*.example.com/*", "https://api.example.com/v1", true},
		{"*://*.example.com/*", "https://example.com/v1", false},
		// No match
		{"*://api.example.com/*", "https://other.com/api", false},
		// Case insensitive
		{"*://API.Example.COM/*", "https://api.example.com/foo", true},
		// Multiple wildcards
		{"*://*.example.com/*/users/*", "https://api.example.com/v1/users/123", true},
		{"*://*.example.com/*/users/*", "https://api.example.com/v1/posts/123", false},
		// Bare host
		{"*://example.com", "https://example.com", true},
		{"*://example.com", "https://example.com/path", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.url, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.url)
			if got != tt.want {
				t.Errorf("GlobMatch(%q, %q) = %v, want %v", tt.pattern, tt.url, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run tests, verify they fail**

Run: `go test -v -run TestGlobMatch ./internal/scripting/`
Expected: FAIL — `GlobMatch` undefined.

**Step 3: Implement glob matcher**

```go
// internal/scripting/glob.go
package scripting

import (
	"path"
	"strings"
)

// GlobMatch checks if a URL matches a glob pattern.
// Pattern format: scheme://host/path where * is a wildcard.
// Matching is case-insensitive.
func GlobMatch(pattern, rawURL string) bool {
	pattern = strings.ToLower(pattern)
	rawURL = strings.ToLower(rawURL)

	// Split into scheme and rest for both pattern and URL.
	pScheme, pRest := splitScheme(pattern)
	uScheme, uRest := splitScheme(rawURL)

	// Match scheme (* matches any).
	if pScheme != "*" && pScheme != uScheme {
		return false
	}

	// Split host and path.
	pHost, pPath := splitHostPath(pRest)
	uHost, uPath := splitHostPath(uRest)

	// Match host using path.Match (supports *).
	if ok, _ := path.Match(pHost, uHost); !ok {
		return false
	}

	// Match path using path.Match.
	if ok, _ := path.Match(pPath, uPath); !ok {
		return false
	}

	return true
}

// splitScheme splits "scheme://rest" into ("scheme", "rest").
func splitScheme(s string) (string, string) {
	if i := strings.Index(s, "://"); i >= 0 {
		return s[:i], s[i+3:]
	}
	return "", s
}

// splitHostPath splits "host/path" into ("host", "/path").
// If no path, returns ("host", "").
func splitHostPath(s string) (string, string) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i:]
	}
	return s, ""
}

// GlobMatchAny returns true if the URL matches any of the patterns.
func GlobMatchAny(patterns []string, rawURL string) bool {
	for _, p := range patterns {
		if GlobMatch(p, rawURL) {
			return true
		}
	}
	return false
}
```

**Step 4: Run tests, verify they pass**

Run: `go test -v -run TestGlobMatch ./internal/scripting/`
Expected: PASS

**Step 5: Commit**

```
feat(scripting): add glob URL matcher for script patterns
```

---

### Task 3: Directory Loader

**Files:**
- Create: `internal/scripting/loader.go`
- Create: `internal/scripting/loader_test.go`

**Step 1: Write failing tests**

```go
// internal/scripting/loader_test.go
package scripting

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadDir_Valid(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "test.js", `// ---
// name: Test Script
// match:
//   - "*://example.com/*"
// enabled: true
// ---
function onRequest(ctx) {}
`)

	scripts, errs := LoadDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(scripts) != 1 {
		t.Fatalf("scripts len = %d, want 1", len(scripts))
	}
	if scripts[0].Meta.Name != "Test Script" {
		t.Errorf("name = %q", scripts[0].Meta.Name)
	}
	if scripts[0].FilePath != filepath.Join(dir, "test.js") {
		t.Errorf("path = %q", scripts[0].FilePath)
	}
}

func TestLoadDir_BadYAML(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "bad.js", `function onRequest(ctx) {}`)

	scripts, errs := LoadDir(dir)
	if len(scripts) != 0 {
		t.Error("should not load script with bad YAML")
	}
	if len(errs) == 0 {
		t.Error("should report error for bad YAML")
	}
}

func TestLoadDir_Empty(t *testing.T) {
	dir := t.TempDir()
	scripts, errs := LoadDir(dir)
	if len(scripts) != 0 || len(errs) != 0 {
		t.Error("empty dir should return empty results")
	}
}

func TestLoadDir_MissingDir(t *testing.T) {
	scripts, errs := LoadDir("/nonexistent/path")
	if len(scripts) != 0 || len(errs) != 0 {
		t.Error("missing dir should return empty results, not error")
	}
}

func TestLoadDir_MultipleScripts(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "a.js", `// ---
// name: Script A
// match:
//   - "*://a.com/*"
// ---
function onRequest(ctx) {}
`)
	writeTestScript(t, dir, "b.js", `// ---
// name: Script B
// match:
//   - "*://b.com/*"
// enabled: false
// ---
function onRequest(ctx) {}
`)

	scripts, errs := LoadDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(scripts) != 2 {
		t.Fatalf("scripts len = %d, want 2", len(scripts))
	}
}

func TestToggleEnabled(t *testing.T) {
	dir := t.TempDir()
	p := writeTestScript(t, dir, "test.js", `// ---
// name: Test
// match:
//   - "*://example.com/*"
// enabled: true
// ---
function onRequest(ctx) {}
`)

	if err := ToggleEnabled(p); err != nil {
		t.Fatal(err)
	}

	// Re-read and check.
	data, _ := os.ReadFile(p)
	meta, _, err := ParseHeader(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if meta.IsEnabled() {
		t.Error("should be disabled after toggle")
	}

	// Toggle back.
	if err := ToggleEnabled(p); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(p)
	meta, _, err = ParseHeader(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if !meta.IsEnabled() {
		t.Error("should be enabled after second toggle")
	}
}
```

**Step 2: Run tests, verify fail**

Run: `go test -v -run "TestLoadDir|TestToggleEnabled" ./internal/scripting/`
Expected: FAIL — undefined functions.

**Step 3: Implement loader**

```go
// internal/scripting/loader.go
package scripting

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ScriptFile represents a loaded script file with metadata.
type ScriptFile struct {
	Meta     *ScriptMeta
	Source   string // JS body (after header)
	FilePath string
	Error    string // non-empty if script failed to load
}

// LoadDir reads all .js files from dir and parses their headers.
// Returns successfully loaded scripts and per-file errors.
// Missing dir returns empty results (not an error).
func LoadDir(dir string) ([]ScriptFile, []ScriptFile) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil // missing dir is fine
	}

	var scripts, errs []ScriptFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(p) // #nosec G304 -- user script dir
		if err != nil {
			errs = append(errs, ScriptFile{FilePath: p, Error: err.Error()})
			continue
		}

		meta, body, err := ParseHeader(string(data))
		if err != nil {
			errs = append(errs, ScriptFile{
				FilePath: p,
				Error:    err.Error(),
				Meta:     &ScriptMeta{Name: filenameToName(e.Name())},
			})
			continue
		}

		scripts = append(scripts, ScriptFile{
			Meta:     meta,
			Source:   body,
			FilePath: p,
		})
	}
	return scripts, errs
}

// ToggleEnabled flips the enabled field in a script file's YAML header.
func ToggleEnabled(path string) error {
	data, err := os.ReadFile(path) // #nosec G304 -- user script file
	if err != nil {
		return err
	}

	meta, _, err := ParseHeader(string(data))
	if err != nil {
		return err
	}

	current := meta.IsEnabled()
	newVal := "false"
	if !current {
		newVal = "true"
	}

	content := string(data)
	// Replace or insert enabled line.
	if strings.Contains(content, "// enabled:") {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "// enabled:") {
				lines[i] = "// enabled: " + newVal
				break
			}
		}
		content = strings.Join(lines, "\n")
	} else {
		// Insert before closing delimiter.
		content = strings.Replace(content, "// ---\n\n", "// enabled: "+newVal+"\n// ---\n\n", 1)
	}

	return os.WriteFile(path, []byte(content), 0o644) // #nosec G306
}

// NewScriptTemplate returns the content for a new script file.
func NewScriptTemplate() string {
	return `// ---
// name: New Script
// match:
//   - "*://*/*"
// enabled: true
// ---

function onRequest(ctx) {
  // ctx.method, ctx.url, ctx.headers, ctx.body, ctx.blocked
}

function onResponse(ctx) {
  // ctx.status, ctx.headers, ctx.body
}
`
}

// filenameToName converts "my-script.js" to "my-script".
func filenameToName(name string) string {
	return strings.TrimSuffix(name, ".js")
}

// EnsureScriptDir creates the scripts directory if it doesn't exist.
func EnsureScriptDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// DeleteScript removes a script file.
func DeleteScript(path string) error {
	return os.Remove(path)
}

// CreateNewScript creates a new script from template, returns its path.
func CreateNewScript(dir string) (string, error) {
	if err := EnsureScriptDir(dir); err != nil {
		return "", fmt.Errorf("ensure dir: %w", err)
	}
	f, err := os.CreateTemp(dir, "script-*.js")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(NewScriptTemplate()); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	return f.Name(), nil
}
```

**Step 4: Run tests, verify pass**

Run: `go test -v -run "TestLoadDir|TestToggleEnabled" ./internal/scripting/`
Expected: PASS

**Step 5: Commit**

```
feat(scripting): add directory loader, toggle, and script templates
```

---

### Task 4: Refactor Engine to Use ScriptFile + Glob

**Files:**
- Modify: `internal/scripting/scripting.go`
- Modify: `internal/scripting/scripting_test.go`

**Step 1: Write new integration tests**

Add to `scripting_test.go`:

```go
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
```

**Step 2: Run tests, verify fail**

Run: `go test -v -run "TestEngine_LoadFromDir|TestEngine_GlobFiltering" ./internal/scripting/`
Expected: FAIL — `LoadFromDir` undefined.

**Step 3: Refactor engine**

Update `scripting.go`:
- Add `LoadFromDir(dir string)` method that calls `LoadDir` then `LoadScript` for each enabled file
- Update `script` struct to include `Meta *ScriptMeta`
- Replace `matchURL` calls with `GlobMatchAny(s.Meta.Match, url)` for scripts loaded via dir
- Keep backward compat: old `LoadScript(name, source, urlPattern)` still works for existing tests
- Add `ScriptInfos() []ScriptInfo` method for TUI
- Add `ScriptInfo` struct: `Name, Matches []string, Enabled bool, FilePath string, Error string`

**Step 4: Run ALL scripting tests**

Run: `go test -v ./internal/scripting/`
Expected: ALL PASS (old + new tests).

**Step 5: Commit**

```
refactor(scripting): integrate ScriptFile and glob matching into engine
```

---

### Task 5: Wire Engine into Interceptor

**Files:**
- Modify: `internal/proxy/interceptor.go`
- Modify: `internal/proxy/proxy.go`

**Step 1: Update interceptor to accept engine**

Modify `interceptor` struct to hold `*scripting.Engine`. In `Request()` hook, after capturing request data, build `RequestContext`, call `engine.RunOnRequest()`, apply mutations back to `mp.Flow`. If `ctx.Blocked`, set `f.Response` to a 403.

In `Response()` hook, build `ResponseContext`, call `engine.RunOnResponse()`, apply mutations back.

**Step 2: Update `proxy.go`**

- Add `ScriptEngine *scripting.Engine` field to `Proxy`
- Pass engine to `newInterceptor(s, engine)`
- In `Init()`, pass `p.ScriptEngine` to interceptor

**Step 3: Run existing tests**

Run: `go test ./internal/proxy/ ./internal/scripting/`
Expected: PASS

**Step 4: Commit**

```
feat(proxy): wire scripting engine into request/response interceptor hooks
```

---

### Task 6: ScriptManager Interface + TUI Ports

**Files:**
- Modify: `internal/tui/ports.go`
- Create: `internal/scripting/manager.go`

**Step 1: Add interface to ports.go**

```go
// ScriptInfo describes a script for TUI display.
type ScriptInfo struct {
	Name     string
	Matches  []string
	Enabled  bool
	FilePath string
	Error    string
}

// ScriptManager exposes script operations to the TUI.
type ScriptManager interface {
	Scripts() []ScriptInfo
	Toggle(filePath string) error
	Delete(filePath string) error
	CreateNew() (string, error)
	ScriptDir() string
	Reload()
}
```

**Step 2: Implement Manager wrapping Engine**

```go
// internal/scripting/manager.go
package scripting

// Manager implements tui.ScriptManager wrapping the Engine.
type Manager struct {
	engine *Engine
	dir    string
}

func NewManager(engine *Engine, dir string) *Manager {
	return &Manager{engine: engine, dir: dir}
}
```

Implement all methods: `Scripts()`, `Toggle()`, `Delete()`, `CreateNew()`, `ScriptDir()`, `Reload()`.

**Step 3: Run tests**

Run: `go test ./internal/scripting/ ./internal/tui/`
Expected: PASS

**Step 4: Commit**

```
feat(scripting): add ScriptManager interface and implementation
```

---

### Task 7: TUI Scripts Modal

**Files:**
- Create: `internal/tui/scripts.go`
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/menu.go`
- Modify: `internal/tui/help.go`

**Step 1: Add scripts state to App**

In `app.go`:
- Add `scripts ScriptManager` field
- Add `showScripts bool`, `scriptsCursor int`, `scriptsList []ScriptInfo`, `scriptsConfirmDelete bool`
- Update `NewApp` to accept `ScriptManager` (can be nil)
- Route `S` key to open scripts modal
- In `Update()`, add `showScripts` routing before other modals
- In `View()`, add `showScripts` rendering

**Step 2: Implement scripts modal**

Create `scripts.go` with:
- `initScripts()` — reload scripts, reset cursor
- `updateScripts(msg)` — handle keys: j/k navigate, space toggle, e edit, n new, d delete (with confirm), esc close
- `viewScripts()` — render table: `✓/✗ | name | first match pattern (+N more)`

The modal uses the same centered popup pattern as `viewMenu()`.

**Step 3: Add to menus**

In `menu.go`:
- Add `{label: "Scripts", key: "S"}` to both `listMenuItems()` and `detailMenuItems()`

In `help.go`:
- Add `{"S", "Scripts manager"}` to Global group

**Step 4: Update main.go**

In `cmd/httpmon/main.go`:
- Create `scripting.Engine`, load scripts dir (`~/.httpmon/scripts/`)
- Create `scripting.Manager`
- Pass engine to `proxy.Proxy`
- Pass manager to `tui.NewApp`

**Step 5: Build and verify**

Run: `make all`
Expected: lint + test + build all pass.

**Step 6: Commit**

```
feat(tui): add scripts management modal with create/edit/toggle/delete
```

---

### Task 8: Integration Smoke Test

**Files:**
- Modify: `internal/scripting/scripting_test.go`

**Step 1: Write end-to-end test**

Test the full flow: create temp dir with script → init engine → load dir → build request context → run → verify mutation. Already partially covered by Task 4 tests but add a response rewriting test too.

**Step 2: Run full gate**

Run: `make all`
Expected: ALL green.

**Step 3: Commit**

```
test(scripting): add integration smoke tests for full script lifecycle
```

---

## Unresolved Questions

None — all decisions made during brainstorming.
