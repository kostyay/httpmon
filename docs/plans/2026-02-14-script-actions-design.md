# Script Actions Design

Unify breakpoints, map-local, and synthetic responses as script-level primitives. Delete `internal/maplocal` package.

## Script API

Three methods on JS context, available in both `onRequest` and `onResponse`:

```js
// Synthetic response — short-circuits upstream in onRequest, replaces response in onResponse
ctx.respondWith({
  status: 200,           // optional, defaults to 200
  body: '{"ok": true}',  // string
  headers: {             // optional
    "Content-Type": "application/json"
  }
})

// File variant — reads file, infers content-type from extension
ctx.respondWith({
  file: "./fixtures/users.json",
  status: 200  // optional, defaults to 200
})

// Read file contents as string (null if missing). Relative to script dir.
let data = ctx.readFile("./fixtures/users.json")

// Pause for interactive editing (existing breakpoint design)
ctx.breakpoint()
```

### Semantics

- **onRequest + respondWith**: sets `Responded=true`, engine interrupts script execution. Interceptor builds `f.Response`, skips upstream.
- **onResponse + respondWith**: replaces response in-place. Script continues executing.
- **respondWith({file})** with missing file: no-op (Responded stays false, request passes through).
- **readFile** with missing file: returns `null`.
- File paths resolve relative to the script file's directory.

### Go context fields

`RequestContext` and `ResponseContext` gain:

```go
Responded       bool
ResponseStatus  int
ResponseHeaders map[string]string
ResponseBody    []byte
```

## Interceptor Changes

### Request flow

```
1. Run scripts (onRequest)
2. If ctx.Responded → build f.Response from ctx fields, skip upstream
```

Replaces current hardcoded MapLocal check + script execution.

### Response flow

```
1. Run scripts (onResponse)
2. If ctx.Responded → replace f.Response from ctx fields
   else → use modified headers/body/status as today
```

### Engine changes

`runOnRequest` and `runOnResponse` register three JS functions on the context:

- `ctx.respondWith(opts)` — sets responded flag + response fields on Go context
- `ctx.readFile(path)` — returns file contents as string, resolves relative to script's `filePath`
- `ctx.breakpoint()` — calls `BreakpointController.Pause()` (per breakpoint design)

After JS returns, engine reads back `Responded`, `ResponseStatus`, `ResponseHeaders`, `ResponseBody`.

In onRequest, `respondWith` sets a goja interrupt to halt further script execution. In onResponse, no interrupt.

## Deletions

- `internal/maplocal/` — entire package
- `internal/tui/maplocal.go`, `maplocal_test.go`
- MapLocal references in `interceptor.go`, `ports.go`, `app.go`, `main.go`
- MapLocal CLI flags, menu items

## Script Categorization

### Categories

```go
type ScriptCategory string

const (
    CategoryScript     ScriptCategory = "Script"
    CategoryMapLocal   ScriptCategory = "Map Local"
    CategoryBreakpoint ScriptCategory = "Breakpoint"
)
```

`ScriptInfo` gains `Categories []ScriptCategory`.

### Detection

Static source analysis. Scan for all matches (multi-valued):

- Source contains `ctx.breakpoint()` → Breakpoint
- Source contains `ctx.respondWith(` → Map Local
- Neither → Script (default, only when no other category matches)

### TUI display

```
[Breakpoint][Map Local]  debug-api.js    */api/*
[Map Local]              mock-users.js   */api/users*
[Script]                 add-header.js   *
```

### Quick-add helper

TUI action from scripts list view. Prompts for URL pattern + local file path. Generates boilerplate:

```js
// ---
// name: mock-<pattern-slug>
// match: ["<pattern>"]
// ---
function onRequest(ctx) {
  ctx.respondWith({file: "<path>"});
}
```

Triggers engine reload.

## Testing

### Unit tests (scripting)

- `ctx.respondWith({body, status, headers})` in onRequest sets Responded + fields
- `ctx.respondWith({file})` reads file, infers content-type
- `ctx.respondWith({file: "missing"})` is no-op
- `ctx.readFile(path)` returns contents as string
- `ctx.readFile("missing")` returns null
- File paths resolve relative to script directory
- `respondWith` in onRequest halts further script execution
- `respondWith` in onResponse does not halt execution
- Category detection: breakpoint source → Breakpoint
- Category detection: respondWith source → Map Local
- Category detection: both → both categories
- Category detection: neither → Script only

### Interceptor tests

- onRequest with Responded=true builds f.Response, skips upstream
- onResponse with Responded=true replaces f.Response
- Responded=false unchanged (existing behavior)

### E2E tests

- respondWith({body}) in onRequest → client gets synthetic response, upstream never called
- respondWith({file}) in onRequest → client gets file contents with correct content-type
- respondWith in onResponse → client gets replaced response
- readFile + header modification → both apply
- Missing file fallthrough → request reaches upstream

Delete and rewrite maplocal e2e tests as script-based equivalents.

## Decisions

| Decision | Choice |
|----------|--------|
| maplocal package | Delete, no migration |
| Script primitives | `ctx.respondWith(opts)`, `ctx.readFile(path)`, `ctx.breakpoint()` |
| respondWith scope | onRequest (short-circuit) and onResponse (replace) |
| respondWith({file}) | Reads file, infers content-type from extension |
| readFile missing file | Returns null |
| respondWith({file}) missing | No-op, passes through |
| File path resolution | Relative to script file directory |
| onRequest after respondWith | Halt (goja interrupt) |
| onResponse after respondWith | Continue |
| Script categories | Multi-valued list |
| Category detection | Static source analysis |
| Quick-add TUI | Generates boilerplate .js |
| MapLocal TUI | Deleted |
