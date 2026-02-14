# Breakpoints Design

Breakpoints let users pause HTTP flows mid-flight, inspect and modify headers/body in a built-in editor, then resume. Implemented as a `ctx.breakpoint()` function available in any script.

## Script API

Any script calls `ctx.breakpoint()` in `onRequest` or `onResponse`. The proxy goroutine blocks until the user resumes.

```js
function onRequest(ctx) {
  if (ctx.body.includes("sensitive")) {
    ctx.breakpoint();
    ctx.headers["X-Inspected"] = "true";
  }
}

function onResponse(ctx) {
  if (ctx.status === 500) {
    ctx.breakpoint();
  }
}
```

After resume, `ctx.headers` and `ctx.body` reflect user edits. Script continues executing from the line after `ctx.breakpoint()`.

Multiple `ctx.breakpoint()` calls in one script: each is an independent pause/resume cycle.

If `BreakpointController` is nil, `ctx.breakpoint()` is a no-op.

## BreakpointController (proxy layer)

Frontend-agnostic. Lives in `internal/proxy/` or `internal/breakpoint/`.

```go
type BreakpointHit struct {
    FlowID  uint64
    Phase   Phase // Request or Response
    Headers map[string]string
    Body    []byte
    Meta    FlowMeta // read-only context
}

type BreakpointResume struct {
    Headers map[string]string
    Body    []byte
    Skipped bool
}

type BreakpointController interface {
    Subscribe() <-chan BreakpointHit
    Resume(flowID uint64, resp BreakpointResume)
    Pending() []BreakpointHit
    ResumeAll()
}
```

When JS calls `ctx.breakpoint()`, the scripting engine calls `Pause(hit)` on the controller. The controller notifies subscribers and blocks until `Resume()` is called.

Per-hit channel (buffered size 1). Each flow runs in its own goroutine — blocking one doesn't affect others.

## Scripting Engine Changes

Minimal. Add `breakpoint` method to `RequestContext` and `ResponseContext`.

When called:
1. Extract current headers + body from context
2. Call `BreakpointController.Pause(hit)` — blocks
3. Receive `BreakpointResume`
4. If not skipped, write modified headers/body back to JS context
5. Return to JS

Controller injected at engine startup. No changes to script loading, metadata, or lifecycle.

## TUI Integration

New file: `internal/tui/breakpoint.go`. TUI is a thin subscriber.

### Status bar

- Hit counter: total breakpoints triggered this session (`BP: 12`)
- Pending: currently paused flows (`⏸ 2`)

Both update on tick.

### Keyboard flow

- `B` from list view → breakpoint queue (paused flows: method, host, path, phase)
- Select flow → editor view
- Editor: two panes — headers (top), body (bottom)
- `Tab` → switch panes
- Body has syntax highlighting based on content-type
- `Enter` / `Ctrl+S` → resume with modifications
- `Esc` → skip (resume unmodified), back to queue
- `E` → system editor escape hatch via existing `openInEditor()`
- Empty queue → back to list view

### App state

```go
showBreakpointQueue  bool
breakpointCursor     int
breakpointEditor     breakpointEditorModel
editingBreakpoint    *BreakpointHit
breakpointHitCount   int
```

`breakpointEditorModel`: two `textarea.Model` (headers, body) with `focusedPane` toggle.

Menu item: "Breakpoints (B)" in `listMenuItems()`, conditional on controller != nil.

## Shutdown & Cleanup

- TUI exit: `ResumeAll()` — all blocked goroutines resume unmodified
- Proxy shutdown: `ResumeAll()` before closing listener
- No persistence: breakpoint state is ephemeral. Logic lives in scripts (already persisted as `.js` files)
- Paused flows show `StateBreakpoint` in list view. Returns to `StateInProgress` on resume.

## Testing

### Unit Tests

**BreakpointController (`internal/breakpoint/breakpoint_test.go`):**
- Pause blocks until Resume is called
- Resume with modified headers/body returns correct data
- Resume with Skipped=true returns original data
- Multiple concurrent pauses (different flows) — each resumes independently
- Pending() returns only currently paused flows
- ResumeAll() unblocks all paused flows with skip
- Subscribe() delivers hits to subscriber channel
- Resume on unknown flowID is a no-op (no panic)

**Scripting engine (`internal/scripting/engine_test.go`):**
- `ctx.breakpoint()` in onRequest pauses and resumes with modified body
- `ctx.breakpoint()` in onResponse pauses and resumes with modified body
- Script continues executing after breakpoint resume (verify post-breakpoint code runs)
- Modified headers/body from resume are visible in ctx after return
- `ctx.breakpoint()` with nil controller is a no-op
- Multiple breakpoint calls in one script — each pauses independently
- Breakpoint in onRequest + script modifies headers after resume — both edits apply

**TUI breakpoint view (`internal/tui/breakpoint_test.go`):**
- Breakpoint queue renders paused flows list
- Selecting a flow opens editor with correct headers/body
- Tab switches focus between headers and body panes
- Esc skips (resumes unmodified)
- Enter/Ctrl+S resumes with editor contents
- Status bar shows hit count and pending count
- Empty queue returns to list view

### E2E Tests

**E2E (`internal/proxy/breakpoint_e2e_test.go`):**

Full proxy pipeline tests using a real HTTP server, proxy, and scripting engine. No TUI — exercise the controller directly.

- **Request breakpoint modifies body:** Script with `ctx.breakpoint()` in onRequest. Start request, controller receives hit, resume with modified body. Upstream server receives modified body.
- **Response breakpoint modifies body:** Script with `ctx.breakpoint()` in onResponse. Make request, controller receives hit, resume with modified body. Client receives modified body.
- **Request breakpoint modifies headers:** Resume with added/changed headers. Upstream sees modified headers.
- **Response breakpoint modifies headers:** Resume with added/changed headers. Client sees modified headers.
- **Skip breakpoint (no modification):** Resume with Skipped=true. Request/response passes through unmodified.
- **Script continues after breakpoint:** Script adds header after `ctx.breakpoint()`. Verify both user edits and script modifications are present.
- **Multiple concurrent breakpoints:** Two requests hit breakpoints simultaneously. Resume each independently. Both complete correctly.
- **Breakpoint on body content match:** Script checks `ctx.body.includes("token")`. Only matching requests pause.
- **ResumeAll on shutdown:** Multiple paused flows. Call ResumeAll. All requests complete unmodified.
- **No breakpoint when condition not met:** Script has conditional `ctx.breakpoint()`. Non-matching requests pass through without pause.

## Decisions

| Decision | Choice |
|----------|--------|
| Script integration | `ctx.breakpoint()` in any script, no new script type |
| Ownership | `BreakpointController` in proxy layer, frontend-agnostic |
| Editor | Built-in textarea with syntax highlighting, system editor via `E` |
| Editable fields | Headers + body, Tab to switch panes |
| Timeout | None — auto-resume all on shutdown |
| Post-resume | Script continues with modified context |
| Persistence | None — ephemeral state |
