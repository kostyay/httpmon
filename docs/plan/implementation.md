# HTTPMon — Development Plan

## Project Overview

A terminal-native HTTP/HTTPS debugging proxy built in Go, inspired by Proxyman. Uses a simplified screen-based TUI (not a 3-pane GUI clone) with keyboard-driven navigation, filesystem-first scripting, and interface-based architecture for future UI decoupling.

### Design Principles

- **Screen-based, not pane-based** — each view owns the full terminal. No simultaneous sidebar/table/inspector.
- **Interface-driven separation** — the TUI consumes the proxy engine through Go interfaces. Phase 1 is in-process; a future gRPC adapter implements the same interfaces.
- **Filesystem-first** — scripts, config, and CA certs live on disk in conventional locations. The TUI is a dashboard, not an editor.
- **Keyboard-native** — vim-style navigation (`j/k/Enter/Esc`). Every action is reachable without a mouse.

---

## Architecture

### Package Layout

```
httpmon/
├── cmd/
│   └── httpmon/
│       └── main.go                  # Entrypoint, wiring, CLI flags
├── internal/
│   ├── engine/                      # Core interfaces (the contract)
│   │   ├── interfaces.go            # ProxyEngine, FlowStore, ScriptManager, FilterEngine
│   │   └── types.go                 # Flow, FlowID, FlowMeta, FlowDetail, Header, etc.
│   ├── proxy/                       # MITM proxy implementation
│   │   ├── proxy.go                 # go-mitmproxy wrapper, implements engine.ProxyEngine
│   │   ├── interceptor.go           # Addon hooks: OnRequest, OnResponse
│   │   ├── cert.go                  # CA generation, leaf cert LRU cache
│   │   └── websocket.go             # WebSocket frame interception
│   ├── store/                       # Flow storage
│   │   ├── ringbuffer.go            # In-memory ring buffer, implements engine.FlowStore
│   │   └── sqlite.go                # SQLite persistence (session save/load)
│   ├── script/                      # JavaScript scripting
│   │   ├── manager.go               # ScriptManager: load, compile, hot-reload
│   │   ├── watcher.go               # fsnotify directory watcher
│   │   ├── runtime.go               # goja VM pool, timeout, sandbox
│   │   └── api.go                   # JS API surface: onRequest/onResponse bridge
│   ├── filter/                      # Filtering
│   │   ├── quick.go                 # Substring/regex quick filter
│   │   ├── cel.go                   # CEL compilation and evaluation
│   │   └── llm.go                   # Ollama client, NL→CEL translation
│   ├── export/                      # Export formatters
│   │   ├── curl.go                  # Flow → cURL string
│   │   └── har.go                   # Flow → HAR JSON
│   ├── maplocal/                    # Map Local middleware
│   │   └── maplocal.go              # URL pattern → local file serving
│   ├── breakpoint/                  # Breakpoint manager
│   │   └── breakpoint.go            # Rule matching, channel-based blocking
│   └── tui/                         # Bubble Tea TUI (replaceable)
│       ├── app.go                   # Root model, screen stack, key dispatch
│       ├── theme.go                 # lipgloss styles, color palette
│       ├── screen_flowlist.go       # Main screen: filter bar + request table
│       ├── screen_detail.go         # Detail view: tabs (Request/Response/Timing/Export)
│       ├── screen_body.go           # Full-screen body viewer with syntax highlighting
│       ├── screen_scripts.go        # Script dashboard (read-only list, toggle, open)
│       ├── screen_help.go           # Help overlay (? key)
│       ├── component_table.go       # Virtualized flow table component
│       ├── component_viewport.go    # Scrollable viewport with search
│       ├── component_statusbar.go   # Bottom status bar
│       ├── component_filterbar.go   # Filter input with mode indicator
│       └── component_notification.go # Transient toast notifications
├── pkg/
│   └── clipboard/                   # Cross-platform clipboard helper
│       └── clipboard.go
├── scripts/                         # Template scripts for `n` (new script) command
│   └── template.js
├── go.mod
└── go.sum
```

### Core Interfaces (`internal/engine/interfaces.go`)

This is the contract that decouples the TUI from the engine. A future gRPC client would implement these same interfaces, making the TUI unaware of whether it talks to an in-process proxy or a remote daemon.

```go
package engine

import "context"

// ProxyEngine controls the proxy lifecycle.
type ProxyEngine interface {
    Start(ctx context.Context, addr string) error
    Stop() error
    Addr() string // Listening address
    CACertPath() string
}

// FlowStore provides access to captured flows.
type FlowStore interface {
    // Subscribe returns a channel that emits new FlowMeta as they arrive.
    // Closing the context cancels the subscription.
    Subscribe(ctx context.Context) <-chan FlowEvent

    // List returns flows matching the filter. If filter is nil, returns all.
    // Supports cursor-based pagination via offset/limit.
    List(filter Filter, offset, limit int) ([]FlowMeta, int, error)

    // Get returns full detail for a single flow (headers + body).
    Get(id FlowID) (*FlowDetail, error)

    // Count returns total and filtered counts.
    Count(filter Filter) (total int, filtered int)
}

// FlowEvent is emitted on the subscription channel.
type FlowEvent struct {
    Type EventType // FlowStarted, FlowUpdated, FlowCompleted
    Flow FlowMeta
}

// Filter is the compiled filter applied to flows.
// nil means "match all".
type Filter interface {
    Match(flow *FlowMeta) bool
    String() string // Human-readable representation
}

// FilterEngine compiles filter expressions.
type FilterEngine interface {
    // CompileQuick compiles a simple substring/glob filter.
    CompileQuick(input string) (Filter, error)

    // CompileCEL compiles a CEL expression string.
    CompileCEL(expr string) (Filter, error)

    // TranslateNL uses the LLM to translate natural language to CEL,
    // then compiles it. Returns the filter and the generated CEL string.
    TranslateNL(ctx context.Context, prompt string) (Filter, string, error)
}

// ScriptManager manages the lifecycle of user scripts.
type ScriptManager interface {
    // Start begins watching the script directory.
    Start(ctx context.Context) error
    Stop() error

    // Scripts returns the current list of loaded scripts.
    Scripts() []ScriptInfo

    // SetEnabled toggles a script on/off by filename.
    SetEnabled(filename string, enabled bool) error

    // OnRequest/OnResponse are called by the interceptor.
    // They run all matching enabled scripts.
    OnRequest(flow *FlowDetail) error
    OnResponse(flow *FlowDetail) error

    // Subscribe to script change notifications (load, error, toggle).
    Subscribe(ctx context.Context) <-chan ScriptEvent
}

// ExportService formats flows for export.
type ExportService interface {
    ToCurl(flow *FlowDetail) (string, error)
    ToHAR(flows []*FlowDetail) ([]byte, error)
}
```

---

## Phase 1 — Core Pipeline & Basic TUI

**Goal:** Proxy intercepts HTTP/HTTPS traffic and streams flows to a working TUI with a flow list and detail view. No scripting, no smart filters yet — just see traffic.

**Status:** 🟡 In progress — core pipeline working (proxy, store, filter, TUI list+detail). Missing: body full-screen view, help screen, subscribe/purge on store, advanced filter modes, cURL copy.

**Coverage:** store 98% · filter 100% · proxy 75% · tui 82%

**Duration:** ~4 weeks

### Task 1.1 — Project Scaffolding ✅

Set up the Go module, directory structure, and build tooling.

- [x] `go mod init` with module path
- [x] Create the full directory structure from the package layout above
- [x] Set up `cmd/httpmon/main.go` with CLI flag parsing (`--port`, `--data-dir`, `--buffer-size`, `--version`)
- [x] Add a `Makefile` with targets: `build`, `test`, `lint`, `security`, `all`
- [x] Add `.golangci-lint.yml` configuration
- [ ] ~Add `go.work` if using workspace mode during dev~ — not needed

**Dependencies to add:**
```
github.com/lqqyt2423/go-mitmproxy
github.com/charmbracelet/bubbletea
github.com/charmbracelet/lipgloss
github.com/charmbracelet/bubbles
```

### Task 1.2 — Define Core Types ✅ (partial — simplified)

Implemented in `internal/store/types.go` (not `internal/engine/`). Simplified: no separate engine package, types live in store.

- [x] `FlowID` — string type
- [x] `FlowMeta` — `ID, Method, StatusCode, Host, Path, Duration, SizeBytes, StartedAt, State, ContentType, Scheme`
- [x] `FlowState` — enum: `StateInProgress`, `StateCompleted`, `StateFailed`
- [x] `FlowData` — `RequestHeaders, RequestBody, ResponseHeaders, ResponseBody` (using `http.Header` directly, no custom Header type)
- [ ] `FlowDetail` — not implemented; `FlowMeta` + `FlowData` used separately
- [ ] `Header` custom type — skipped; using `net/http.Header` directly
- [ ] `EventType` / `FlowEvent` — not implemented (polling via tick instead of subscribe)
- [ ] `ScriptInfo` / `ScriptEvent` — Phase 2
- [x] `Filter` interface — defined in `store/types.go`

### Task 1.3 — Define Core Interfaces ⏭️ (deferred)

No separate `internal/engine/` package. Interfaces defined locally where needed:
- [x] `Filter` interface — `store/types.go`
- [x] `FlowReader` / `ProxyInfo` — `tui/ports.go` (TUI-local interfaces for dependency injection)
- [ ] `ProxyEngine` formal interface — not needed yet (concrete `*proxy.Proxy` used directly)
- [ ] `FlowStore` formal interface — not needed yet
- [ ] `FilterEngine` / `ScriptManager` / `ExportService` — Phase 2+

### Task 1.4 — Ring Buffer Flow Store ✅

Implemented in `internal/store/ringbuffer.go`. Test coverage: 98%.

- [x] Thread-safe ring buffer with configurable capacity (mutex-guarded)
- [x] `Add(FlowMeta)` — appends, evicts oldest at capacity
- [x] `Update(FlowID, fn func(*FlowMeta))` — atomic update via callback
- [x] `Get(FlowID) (*FlowMeta, *FlowData, error)` — returns meta + data separately
- [x] `SetData(FlowID, *FlowData)` — stores request/response data in `map[FlowID]*FlowData`
- [x] `List(filter, offset, limit)` — newest-first, filtered, paginated
- [x] `Len()` — returns count
- [ ] `Count(filter)` — not implemented (total returned by `List`)
- [ ] `Subscribe(ctx)` — not implemented (TUI polls via tick)
- [ ] `Purge()` — not implemented
- [ ] Benchmarks — not written yet

### Task 1.5 — MITM Proxy Engine ✅

Implemented in `internal/proxy/proxy.go` + `interceptor.go`. Test coverage: 75%.

- [x] go-mitmproxy wrapper with `Init(addr)` / `Serve(ctx)` / `Stop()`
- [x] `CACertPath()` — returns path to generated CA cert
- [x] CA generation delegated to go-mitmproxy (`CaRootPath` option)
- [ ] Custom `cert.go` — not needed; go-mitmproxy handles CA + leaf certs internally
- [x] `interceptor.go` — addon bridging go-mitmproxy to `store.RingBuffer`:
  - [x] `Requestheaders` → create `FlowMeta` (InProgress), detect http/https scheme
  - [x] `Request` → capture request body + headers via `store.SetData()`
  - [x] `Responseheaders` → update status code + content-type
  - [x] `Response` → capture response body, merge with request data, mark Completed
  - [x] `markFailed()` for error flows
- [x] Panic recovery in every hook (`defer recover`)
- [x] 5MB body size limit (`maxBodySize`)
- [x] `SslInsecure` option for upstream TLS skip (test support)

Integration tests:
- [x] HTTP flow capture with metadata verification
- [x] HTTPS flow capture (TLS interception via `httptest.NewTLSServer`)
- [x] Concurrent requests (50 goroutines, race detector)
- [x] Body truncation at maxBodySize
- [x] POST method + request body capture
- [x] Request/response body + header content verification
- [x] Server error (500) capture
- [x] Multiple header values (Set-Cookie)
- [x] Port 0 initialization

### Task 1.6 — Quick Filter ✅ (basic)

Implemented in `internal/filter/quick.go`. Test coverage: 100%.

- [x] `QuickFilter` struct implementing `store.Filter`
- [x] `Match(flow *FlowMeta) bool` — case-insensitive substring on host+path
- [x] `CompileQuick(input string) *QuickFilter` — returns nil for empty input
- [ ] Method prefix (`POST `) — not implemented
- [ ] Status prefix (`4xx`, `404`) — not implemented
- [ ] Negation (`!stripe`) — not implemented
- [ ] `String()` — not implemented

Tests cover: empty input, case-insensitive match, host+path matching.

### Task 1.7 — TUI Foundation: App Shell ✅ (simplified)

Implemented in `internal/tui/app.go`. No screen stack — uses `showDetail` bool toggle instead. Test coverage: 82%.

- [ ] `Screen` interface — not implemented; single `App` model handles both views
- [x] `App` struct — root Bubble Tea model with `FlowReader`/`ProxyInfo` interfaces (`tui/ports.go`)
- [x] `width, height` — terminal dimensions from `tea.WindowSizeMsg`
- [x] `ctrl+c` / `q` — quit
- [x] `Enter` — toggle detail view
- [x] `Esc` — back to list
- [x] Tick-based polling (200ms) for flow updates
- [ ] Screen stack (push/pop) — not implemented
- [ ] Notification toasts — not implemented
- [ ] `?` help screen — not implemented

### Task 1.8 — TUI Theme ✅ (basic)

Implemented in `internal/tui/styles.go`.

- [x] Method color-coding (`methodStyle()` — green/yellow/red/blue/cyan)
- [x] Status code color-coding (`statusStyle()` — green 2xx, yellow 3xx, red 4xx/5xx)
- [x] Selected row highlight style
- [x] Header/label styles
- [x] Tab styling (active/inactive)
- [ ] Full `Theme` struct — not implemented; styles are package-level functions
- [ ] Adaptive colors — not implemented
- [ ] Notification/toast styling — not implemented

### Task 1.9 — Flow List Screen ✅ (core)

Implemented in `internal/tui/list.go` (inline in `App`, not a separate screen).

- [x] **Filter bar**: `textinput.Model`, `/` to focus, `Esc` to blur, placeholder text
- [x] Filter recompiles on every keystroke via `filter.CompileQuick()`
- [ ] Filter mode indicator (`[quick]`) — not implemented
- [x] **Flow table**: method/status/host/path/duration/size columns
- [x] Color-coded methods and status codes
- [x] Selected row highlight
- [x] `j`/`k` navigation
- [ ] `g`/`G` top/bottom — not implemented
- [x] Tick-based polling (200ms) for updates
- [x] **Status bar**: flow count + proxy address
- [x] `Enter` — open detail view
- [x] `/` — focus filter
- [ ] `Space` multi-select — not implemented
- [ ] `c` copy cURL — not implemented
- [ ] `D` clear flows — not implemented
- [ ] `p` pause/resume — not implemented
- [x] Data flow: `store.List(filter, 0, 0)` on each tick

### Task 1.10 — Detail View Screen ✅ (basic)

Implemented in `internal/tui/detail.go` (inline in `App`, not a separate screen).

- [x] Request/Response tab toggle (tab 0 / tab 1)
- [x] Tab bar with active/inactive styling
- [x] Scrollable viewport (`bubbles/viewport`)
- [x] Headers display (key: value pairs)
- [x] Body display (raw text)
- [x] `Esc` — back to list
- [x] `Left`/`Right` — switch request/response tabs
- [x] `j`/`k` — scroll viewport
- [x] `n`/`N` — next/previous flow
- [ ] Top bar with method/status/URL — not implemented
- [ ] Timing tab — not implemented
- [ ] Export tab — not implemented
- [ ] `b` body full-screen — not implemented
- [ ] `c` copy cURL — not implemented
- [ ] `y` copy value — not implemented

### Task 1.11 — Body Full-Screen View ❌ (not started)

### Task 1.12 — Help Screen ❌ (not started)

### Task 1.13 — Wiring & Main ✅

Implemented in `cmd/httpmon/main.go`.

- [x] CLI flags: `--port` (8080), `--data-dir` (~/.httpmon), `--buffer-size` (10000), `--version`
- [ ] `--scripts-dir` — Phase 2
- [ ] `--verbose` — not implemented
- [x] `store.New(bufSize)` + `proxy.New(s, dataDir)` + `proxy.Init(addr)`
- [x] Proxy started in goroutine
- [x] `tui.NewApp(s, p)` with interface injection
- [x] `tea.WithAltScreen()` + `tea.WithMouseCellMotion()`
- [x] Graceful shutdown: cancel ctx → `p.Stop()` → drain proxy error
- [x] CA cert path printed to stderr on startup
- [x] Signal handling (`os.Interrupt`)

### Task 1.14 — Integration Testing ✅

- [x] E2E proxy tests: HTTP flow capture, HTTPS interception, concurrent requests, body truncation, POST capture, body/header content, server errors, multiple header values
- [ ] TUI headless mode tests — not implemented (no teatest)
- [ ] Resize test — not implemented
- [x] Filter tests: empty, case-insensitive, host+path (in `filter/quick_test.go`)
- [x] Navigation tests: j/k bounds, Enter/Esc detail toggle, / filter focus/blur (in `tui/app_test.go`)

---

## Phase 2 — Scripting & Filesystem Watcher

**Goal:** Users can write JavaScript scripts that modify requests/responses. Scripts live on disk, are hot-reloaded, and managed via a TUI dashboard.

**Duration:** ~3 weeks

### Task 2.1 — Goja Script Runtime

Implement `internal/script/runtime.go`.

- [ ] `Runtime` struct wrapping a `goja.Runtime` instance
- [ ] `Compile(source string) (*goja.Program, error)` — parse and compile JS source
- [ ] `Execute(program, flowDetail) (*FlowDetail, error)`:
  - Create a new `goja.Runtime` per execution (isolation between scripts)
  - Map `FlowDetail` → JS objects using `goja.FieldNameMapper` (TitleCase → camelCase)
  - Expose `JSRequest` and `JSResponse` objects matching the Proxyman API
  - Call `onRequest(context, url, request)` or `onResponse(context, url, request, response)`
  - Read back modified objects and apply changes to `FlowDetail`
  - Implement 100ms timeout via `time.AfterFunc` + `vm.Interrupt("timeout")`
- [ ] Body handling:
  - Expose body as string for text content types
  - Impose 1MB limit for scriptable bodies
  - Detect and expose JSON bodies as pre-parsed objects for ergonomic access (`request.body.field`)
- [ ] Sandbox security:
  - No `require()`, no filesystem access, no network access
  - Only expose the request/response API and standard JS builtins
  - Log a warning if a script attempts forbidden operations

### Task 2.2 — Script Header Parsing

Implement header comment convention parsing.

- [ ] Parse `// @match <CEL expression>` from the first 10 lines of a `.js` file
- [ ] Parse `// @on request|response|both` (default: `both`)
- [ ] Parse `// @priority <int>` for execution ordering (default: `0`, lower = first)
- [ ] If `@match` is absent, script applies to all traffic
- [ ] Compile the `@match` expression using the CEL filter engine (reuse from `internal/filter/cel.go`)
- [ ] Return a `ScriptHeader` struct: `MatchExpr string, MatchFilter Filter, Phase string, Priority int`

### Task 2.3 — Script Manager

Implement `internal/script/manager.go`.

- [ ] `Manager` struct implementing `engine.ScriptManager`
- [ ] `Start(ctx)`:
  - Ensure the scripts directory exists (create if absent)
  - Do an initial scan: load all `.js` files
  - Start the fsnotify watcher (Task 2.4)
- [ ] `loadScript(path string)`:
  - Read file contents
  - Parse header comments (Task 2.2)
  - Compile with goja
  - If compilation fails, mark script as errored but keep it in the list (so the TUI shows the error)
  - Store in an internal `map[string]*LoadedScript`
- [ ] `OnRequest(flow)` / `OnResponse(flow)`:
  - Iterate scripts sorted by priority
  - For each enabled script where `phase` matches and `matchFilter.Match(flow)` is true:
    - Execute the script via Runtime (Task 2.1)
    - Apply modifications back to the flow
    - If script errors, log and continue to next script (don't abort the chain)
- [ ] `Scripts()` — return a snapshot of `[]ScriptInfo` for the TUI
- [ ] `SetEnabled(filename, enabled)` — toggle state; persist to `config.yaml` in the scripts dir
- [ ] `Subscribe(ctx)` — channel of `ScriptEvent` for TUI notifications
- [ ] Enabled/disabled convention:
  - Primary: maintained in `config.yaml` (a simple `disabled: [filename1, filename2]` list)
  - Fallback visual hint: files prefixed with `_` start disabled on first load

### Task 2.4 — Filesystem Watcher

Implement `internal/script/watcher.go`.

- [ ] Use `github.com/fsnotify/fsnotify` to watch the scripts directory
- [ ] Handle events:
  - `Create` / `Write` → call `loadScript(path)`, emit `ScriptEvent{Loaded}`
  - `Remove` → remove from internal map, emit `ScriptEvent{Removed}`
  - `Rename` → treat as Remove + Create
- [ ] Debounce writes: editors like VS Code trigger multiple write events per save. Use a 200ms debounce per filename
- [ ] Ignore non-`.js` files (e.g., `.swp`, `~` backup files, `.yaml`)
- [ ] On reload, if the new version fails to compile, keep the old version active and emit `ScriptEvent{Errored}` with the error message

### Task 2.5 — Wire Scripts into Interceptor

Update `internal/proxy/interceptor.go` to call the `ScriptManager`.

- [ ] In the `Request` hook, after capturing the body:
  - Call `scriptManager.OnRequest(flowDetail)`
  - If the script modified headers/body, update the `proxy.Flow` with the new values
- [ ] In the `Response` hook, after capturing the body:
  - Call `scriptManager.OnResponse(flowDetail)`
  - Apply modifications to the response
- [ ] If `ScriptManager` is nil (Phase 1 compat), skip gracefully

### Task 2.6 — Scripts TUI Screen

Implement `internal/tui/screen_scripts.go` — the read-only script dashboard.

- [ ] Header: `Scripts` + script directory path
- [ ] Table of scripts:
  - `[x]` / `[ ]` enabled indicator
  - Filename
  - Match expression (or "all requests")
  - Status: "ok", "error: <message>", "loading..."
- [ ] Key bindings:
  - `Space` — toggle enabled/disabled
  - `e` — open script in `$EDITOR` (shell out via `tea.ExecProcess`)
  - `n` — create new script from template, open in `$EDITOR`
  - `o` — open scripts directory in file manager
  - `Esc` — back to flow list
- [ ] Subscribe to `ScriptManager.Subscribe()` for live updates
- [ ] Show a notification toast when a script is reloaded ("✓ stripe-mock.js reloaded")
- [ ] Show error inline when a script fails to compile

### Task 2.7 — Script Template

Create `scripts/template.js` bundled with the binary.

```javascript
// @match req.host.contains("example.com")
// @on both

function onRequest(context, url, request) {
  // Modify request before it reaches the server
  // request.headers["X-Custom"] = "value";
  return request;
}

function onResponse(context, url, request, response) {
  // Modify response before it reaches the client
  // response.statusCode = 200;
  // response.body = JSON.stringify({ mock: true });
  return response;
}
```

- [ ] Embed template via `//go:embed`
- [ ] The `n` command in the scripts screen copies this to `new-script.js` (or prompts for a name), then opens it

---

## Phase 3 — Intelligent Filtering

**Goal:** CEL-based filtering and LLM-powered natural language → CEL translation.

**Duration:** ~2 weeks

### Task 3.1 — CEL Filter Engine

Implement `internal/filter/cel.go`.

- [ ] Define the CEL environment with variables matching `FlowMeta` fields:
  ```
  req.method   (string)
  req.host     (string)
  req.path     (string)
  req.scheme   (string)
  res.code     (int)
  res.size     (int)
  flow.duration_ms (int)
  flow.state   (string)
  ```
- [ ] `CompileCEL(expr string) (Filter, error)`:
  - Compile the CEL expression using `cel-go`
  - Return a `CELFilter` struct that caches the compiled `cel.Program`
  - `Match()` creates an activation from the `FlowMeta` fields and evaluates
- [ ] Common expressions to test:
  - `res.code == 404`
  - `req.host == "api.stripe.com" && req.method == "POST"`
  - `res.code >= 400 && res.code < 500`
  - `req.path.contains("/api/v2")`
  - `flow.duration_ms > 1000`
- [ ] Performance benchmark: evaluate 10,000 flows against a compiled expression (target: <10ms)

### Task 3.2 — LLM Smart Filter (Ollama)

Implement `internal/filter/llm.go`.

- [ ] Ollama HTTP client:
  - `POST http://localhost:11434/api/generate`
  - Configurable endpoint via `--ollama-url` flag
  - Model selection via `--ollama-model` flag (default: `phi-3`)
- [ ] Prompt template (embedded via `//go:embed`):
  ```
  You are a filter translator. Convert the user's natural language request into a CEL expression.
  
  Available fields:
  - req.method (string): HTTP method (GET, POST, PUT, DELETE, etc.)
  - req.host (string): hostname (e.g., "api.stripe.com")
  - req.path (string): URL path (e.g., "/v1/charges")
  - res.code (int): HTTP status code (e.g., 200, 404)
  - res.size (int): response body size in bytes
  - flow.duration_ms (int): request duration in milliseconds
  
  String functions: .contains(), .startsWith(), .endsWith()
  
  Examples:
  - "show me errors" → res.code >= 400
  - "POST requests to stripe" → req.method == "POST" && req.host.contains("stripe")
  - "slow requests" → flow.duration_ms > 1000
  - "large responses" → res.size > 100000
  - "everything except images" → !req.path.endsWith(".png") && !req.path.endsWith(".jpg")
  
  Output ONLY the CEL expression. No explanation.
  
  User: {input}
  ```
- [ ] `TranslateNL(ctx, prompt)`:
  - Send prompt to Ollama
  - Parse response — strip markdown fences if present, trim whitespace
  - Compile result as CEL
  - If compilation fails, attempt one retry with error feedback: "Previous output `{expr}` failed with: `{error}`. Fix it."
  - Return the compiled filter and the CEL string (so the TUI can show what was generated)
  - Timeout: 5s context deadline
- [ ] Graceful degradation: if Ollama is unreachable, return an error that the TUI shows as "Smart filter unavailable — Ollama not running"

### Task 3.3 — Update Filter Bar for Multi-Mode

Update `internal/tui/component_filterbar.go`.

- [ ] Three filter modes, indicated by prefix:
  - (no prefix) — quick filter (substring match)
  - `:` prefix — CEL expression (e.g., `:res.code >= 400`)
  - `?` prefix — smart filter / natural language (e.g., `?show me slow POST requests`)
- [ ] Mode indicator badge next to the input: `[quick]`, `[CEL]`, `[smart]`
- [ ] For quick and CEL modes: compile on every keystroke (debounced 100ms). Show inline error if invalid CEL
- [ ] For smart mode: trigger LLM on `Enter` only (not on every keystroke). Show a spinner while waiting. Display the generated CEL expression below the filter bar
- [ ] Persist last-used filter across screen transitions (don't lose filter when drilling into a flow and back)

---

## Phase 4 — Export & Persistence

**Goal:** Export flows as cURL/HAR. Save and load sessions via SQLite.

**Duration:** ~2 weeks

### Task 4.1 — cURL Export

Implement `internal/export/curl.go`.

- [ ] `ToCurl(flow *FlowDetail) (string, error)`:
  - Reconstruct a complete `curl` command from the flow
  - Include: method (`-X`), URL, all headers (`-H`), body (`-d` / `--data-binary`)
  - Handle special cases: `GET` omits `-X`, compressed bodies use `--compressed`, binary bodies use `--data-binary @-`
  - Properly shell-escape header values and body content
  - Support both single-line and multi-line (backslash-continued) output

### Task 4.2 — HAR Export

Implement `internal/export/har.go`.

- [ ] `ToHAR(flows []*FlowDetail) ([]byte, error)`:
  - Generate a valid HAR 1.2 JSON document
  - Map each flow to a HAR `entry`: `request`, `response`, `timings`, `startedDateTime`
  - Include `log.creator` with app name and version
  - Validate output against the HAR schema
- [ ] Support exporting all flows or a selected subset

### Task 4.3 — SQLite Session Storage

Implement `internal/store/sqlite.go`.

- [ ] Use `modernc.org/sqlite` (pure Go, no CGO)
- [ ] Schema:
  ```sql
  CREATE TABLE sessions (
      id TEXT PRIMARY KEY,
      name TEXT,
      created_at TIMESTAMP,
      flow_count INTEGER
  );
  
  CREATE TABLE flows (
      id TEXT PRIMARY KEY,
      session_id TEXT REFERENCES sessions(id),
      method TEXT,
      host TEXT,
      path TEXT,
      status_code INTEGER,
      duration_ms INTEGER,
      size_bytes INTEGER,
      started_at TIMESTAMP,
      request_headers BLOB,   -- JSON encoded
      request_body BLOB,
      response_headers BLOB,  -- JSON encoded
      response_body BLOB,
      protocol TEXT,
      remote_addr TEXT
  );
  
  CREATE INDEX idx_flows_session ON flows(session_id);
  CREATE INDEX idx_flows_host ON flows(host);
  ```
- [ ] `SaveSession(name string, store FlowStore) error` — dump current ring buffer to SQLite
- [ ] `LoadSession(path string) ([]FlowDetail, error)` — load a saved `.httpmon` file
- [ ] `ListSessions(dir string) ([]SessionInfo, error)` — list saved sessions in the data directory
- [ ] Session files stored as `~/.httpmon/sessions/<name>-<timestamp>.db`

### Task 4.4 — Export Keybindings in TUI

- [ ] Flow list: `c` copies cURL for selected flow, `E` opens export submenu
- [ ] Detail view export tab: render cURL, `c` to copy, `h` to export as HAR
- [ ] Session management: `:save <name>` command in the filter bar (colon-prefix commands), `:load` to list and load sessions

---

## Phase 5 — Breakpoints & Map Local

**Goal:** Pause requests for inspection/modification. Serve local files instead of upstream responses.

**Duration:** ~3 weeks

### Task 5.1 — Breakpoint Manager

Implement `internal/breakpoint/breakpoint.go`.

- [ ] `Manager` struct:
  - `rules []BreakpointRule` — each rule has a `Filter` and a `Phase` (request/response/both)
  - `pending map[FlowID]chan *FlowDetail` — blocked flows awaiting user action
- [ ] `Matches(flow *FlowMeta) bool` — check if any rule matches
- [ ] `Block(flowID FlowID, flow *FlowDetail) *FlowDetail`:
  - Create a buffered channel (size 1)
  - Store in `pending` map
  - Emit a `BreakpointHitEvent` for the TUI
  - Block on the channel (with a configurable max timeout, default 5 min)
  - Return the (possibly modified) `FlowDetail` from the channel
- [ ] `Resume(flowID FlowID, modified *FlowDetail)` — send modified flow on the channel, unblocking the goroutine
- [ ] `Drop(flowID FlowID)` — close the channel, causing the proxy to return a 502
- [ ] Expose via `engine.BreakpointManager` interface

### Task 5.2 — Breakpoint TUI

- [ ] Breakpoint indicator on the flow list: paused flows show a ⏸ icon and highlighted row
- [ ] When a breakpoint fires, show a notification toast and auto-select the paused flow
- [ ] In detail view for a paused flow, show editable fields:
  - Headers: inline edit with `e` on a header row
  - Body: `e` opens `$EDITOR` with the body content as a temp file; on save, read it back
  - Status code (response breakpoints): editable field
- [ ] Action bar for paused flows: `r` resume, `d` drop, `e` edit
- [ ] Breakpoint rule management: `B` from flow list opens a breakpoint rules screen (similar to scripts screen)

### Task 5.3 — Map Local

Implement `internal/maplocal/maplocal.go`.

- [ ] `MapLocal` struct:
  - `rules []MapRule` — each has a `URLPattern` (glob or regex) and a `LocalPath` (file or directory)
  - Loaded from `~/.config/httpmon/maplocal.yaml`
- [ ] `Match(flow *FlowMeta) *MapRule` — check if any rule matches the request URL
- [ ] `Serve(rule *MapRule, flow *FlowDetail) *FlowDetail`:
  - Read the local file
  - Set response body, infer content-type from extension
  - Set status 200
  - Return the modified flow (the proxy skips the upstream request)
- [ ] Hot-reload: watch `maplocal.yaml` with fsnotify
- [ ] TUI: `M` from flow list opens the map local rules screen; `e` opens the YAML in `$EDITOR`

---

## Phase 6 — Polish & Advanced Features

**Goal:** Production-quality UX, performance tuning, and stretch features.

**Duration:** ~2 weeks

### Task 6.1 — Diff View (P2)

- [ ] In flow list, `Space` marks flows for selection (multi-select mode)
- [ ] With exactly 2 flows selected, `d` opens the diff view
- [ ] Use `github.com/sergi/go-diff` for text diffing
- [ ] Render side-by-side or unified diff with green (insertions) and red (deletions) via lipgloss
- [ ] Tabs to diff: Headers, Body, URL/Query params

### Task 6.2 — Mouse Support

- [ ] Click on a flow row to select it, double-click to drill in
- [ ] Click on tabs to switch tabs
- [ ] Scroll wheel in viewports
- [ ] Click filter bar to focus it

### Task 6.3 — Clipboard Integration

Implement `pkg/clipboard/clipboard.go`.

- [ ] Use OSC 52 escape sequence for terminal clipboard (works over SSH)
- [ ] Fallback to `pbcopy` (macOS), `xclip` (Linux), `clip.exe` (WSL)
- [ ] Show notification toast on successful copy

### Task 6.4 — Performance Tuning

- [ ] Profile TUI rendering at 500+ flows/sec with `pprof`
- [ ] Optimize ring buffer for zero-allocation list iteration
- [ ] Implement body streaming for large payloads (>5MB) — don't load into memory, stream from temp file
- [ ] Benchmark and optimize chroma syntax highlighting (cache highlighted output per flow body)
- [ ] Add `--cpuprofile` and `--memprofile` flags for debugging

### Task 6.5 — Configuration File

- [ ] Support `~/.config/httpmon/config.yaml` for persistent settings:
  ```yaml
  proxy:
    port: 8080
    buffer_size: 10000
  
  scripts:
    dir: ~/.config/httpmon/scripts
  
  filter:
    ollama_url: http://localhost:11434
    ollama_model: phi-3
  
  tui:
    theme: dark  # or "light"
    vim_keys: true
    mouse: true
  ```
- [ ] CLI flags override config file values
- [ ] Environment variables override both: `HTTPMON_PORT`, `HTTPMON_SCRIPTS_DIR`, etc.
- [ ] Priority: env vars > CLI flags > config file > defaults

---

## Future — gRPC Decoupling (Post-MVP)

**Goal:** Extract the engine into a standalone daemon, communicate via gRPC. This enables a Swift native app, web UI, or CLI client.

### Approach

- [ ] Define `traffic.proto` based on the existing Go interfaces
- [ ] Implement a `grpc.FlowStoreClient` that wraps the gRPC stub and implements `engine.FlowStore`
- [ ] Implement a `grpc.ScriptManagerClient` that wraps the stub and implements `engine.ScriptManager`
- [ ] The TUI code requires **zero changes** — it already depends on interfaces, not implementations
- [ ] Add a `--headless` mode to the binary that starts only the proxy + gRPC server (no TUI)
- [ ] Add a `--remote <addr>` flag that connects the TUI to a remote gRPC endpoint instead of starting a local proxy

This is the payoff of the interface-based separation in Phase 1: the TUI's `AppModel` accepts interfaces, and whether those interfaces are backed by in-process Go calls or gRPC stubs is a wiring decision made in `main.go`.

---

## Dependencies Summary

| Dependency | Purpose | Phase |
|---|---|---|
| `lqqyt2423/go-mitmproxy` | MITM proxy engine | 1 |
| `charmbracelet/bubbletea` | TUI framework (Elm architecture) | 1 |
| `charmbracelet/lipgloss` | Terminal styling | 1 |
| `charmbracelet/bubbles` | TUI components (textinput, viewport, table) | 1 |
| `alecthomas/chroma` | Syntax highlighting | 1 |
| `google/uuid` | Flow ID generation | 1 |
| `dop251/goja` | Embedded JavaScript runtime | 2 |
| `fsnotify/fsnotify` | Filesystem watcher for script hot-reload | 2 |
| `google/cel-go` | CEL expression engine for filters | 3 |
| `modernc.org/sqlite` | Pure-Go SQLite for session persistence | 4 |
| `sergi/go-diff` | Text diffing for diff view | 6 |

## Testing Strategy

| Layer | Approach |
|---|---|
| `engine/types` | Unit tests for all helper methods |
| `store/ringbuffer` | Unit + benchmark tests. Concurrent read/write fuzzing |
| `proxy` | Integration tests with real HTTP/HTTPS requests through the proxy |
| `filter/quick` | Table-driven unit tests for every match variant |
| `filter/cel` | Table-driven tests with known CEL expressions |
| `filter/llm` | Mock Ollama HTTP server for deterministic testing |
| `script/runtime` | Unit tests with known JS scripts, verify flow modifications |
| `script/manager` | Integration tests with temp directories and file manipulation |
| `export` | Snapshot tests — compare output against golden files |
| `tui` | Screen-level tests using `bubbletea/teatest` for headless model testing |