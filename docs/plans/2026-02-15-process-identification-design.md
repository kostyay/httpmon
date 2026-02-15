# Process Identification Design

## Summary

Add process identification to httpmon. Resolve which OS process made each
proxied request. Display process name in list view column, support process-based
tree grouping, and show PID + cmdline in detail card.

## Decisions

- Platforms: macOS + Linux
- Library: `github.com/shirou/gopsutil/v3` (already used in netmon)
- Resolution: async background goroutine per connection
- Fallback display: `—` when process unknown
- Tree mode: `t` cycles `flat → host-tree → process-tree → flat`

---

## 1. Process Resolver (`internal/procinfo/`)

New package. Single file `resolver.go` + tests.

```go
type Resolver struct {
    store    *store.RingBuffer
    pidCache sync.Map // int32 → string (PID → process name)
}

func New(store *store.RingBuffer) *Resolver

// Resolve spawns goroutine: port → PID → process name → store.Update.
func (r *Resolver) Resolve(flowID string, clientPort uint16)
```

Resolution flow:
1. `net.Connections("tcp")` → find entry where `Laddr.Port == clientPort`
2. `process.NewProcess(pid).Name()` → get process name (cached by PID)
3. `process.NewProcess(pid).Cmdline()` → get full command line
4. `store.Update(flowID, ...)` → set `Process`, `ProcessPID`, `ProcessCmdline`

PID→name cache: cleared periodically (PIDs can be reused). Port→PID not cached
(ports reuse fast).

---

## 2. Data Model Changes

### FlowMeta (store/types.go)

```go
type FlowMeta struct {
    // ... existing fields ...
    Process string // process name ("—" if unknown)
}
```

### FlowData (store/types.go)

```go
type FlowData struct {
    // ... existing fields ...
    ProcessPID     int32  // 0 if unknown
    ProcessCmdline string // full command line, empty if unknown
}
```

Rationale: Process name in FlowMeta (lightweight, needed for list view).
PID + cmdline in FlowData (heavier, only needed for detail card).

---

## 3. Interceptor Changes

`interceptor` gets `resolver *procinfo.Resolver` field. Set via
`interceptorConfig`.

In `Requestheaders`, after `store.Add(meta)`:

```go
if i.resolver != nil && f.ConnContext != nil {
    if addr, ok := f.ConnContext.ClientConn.Conn.RemoteAddr().(*net.TCPAddr); ok {
        i.resolver.Resolve(meta.ID, uint16(addr.Port))
    }
}
```

---

## 4. TUI — List View Process Column

Flat mode layout:

```
METHOD  STATUS  HOST             PROCESS    PATH            SIZE  TIME
GET     200     api.example.com  curl       /v1/users       1.2K  45ms
```

- Column between HOST and PATH
- Dynamic width, capped at 15 chars
- Shows `—` when process unknown

Host-tree mode: process column in expanded flow rows, not host headers.
Process-tree mode: HOST column shown, PROCESS column hidden (it's the group key).

---

## 5. TUI — Detail Card

Add "Process" section to detail view when process info is available:

```
── Process ──────────────────────
Name:     curl
PID:      12345
Cmdline:  curl -X GET https://api.example.com/v1/users
```

Only shown when `ProcessPID != 0`. Placed after the request URL section,
before request headers.

---

## 6. Generic Tree Grouping

Replace `hostGroup` with generic `flowGroup`:

```go
type flowGroup struct {
    Key    string
    Flows  []store.FlowMeta
    Newest time.Time
}

// buildGroups groups flows by key extractor, sorted by most recent.
func buildGroups(flows []store.FlowMeta, keyFn func(store.FlowMeta) string) []flowGroup

// flattenTree converts groups into cursor-addressable rows.
func flattenTree(groups []flowGroup, expanded map[string]bool) []treeRow

// flattenFocus returns rows for a single focused group.
func flattenFocus(groups []flowGroup, key string) []treeRow
```

Key extractors:

```go
func hostKey(f store.FlowMeta) string    { return f.Host }
func processKey(f store.FlowMeta) string { return f.Process }
```

`treeRow.Host` field renamed to `treeRow.GroupKey`.

---

## 7. Tree Mode Cycle

```go
type TreeMode int
const (
    TreeModeFlat TreeMode = iota
    TreeModeHost
    TreeModeProcess
)
```

`t` key: `flat → host → process → flat`

Status bar: `t:flat`, `t:host`, `t:proc`

Focus mode (`f`) works in both host-tree and process-tree modes.
`l`/`h` expand/collapse works on group headers regardless of mode.

---

## 8. Wiring (cmd/httpmon)

1. Create `procinfo.Resolver` with store reference
2. Pass resolver to `interceptorConfig`
3. No CLI flags needed — process resolution is always on when available

---

## Files Changed

| File | Change |
|------|--------|
| `internal/procinfo/resolver.go` | **NEW** — process resolver |
| `internal/procinfo/resolver_test.go` | **NEW** — tests |
| `internal/store/types.go` | Add `Process` to FlowMeta, PID+cmdline to FlowData |
| `internal/proxy/interceptor.go` | Add resolver field, call in Requestheaders |
| `internal/proxy/proxy.go` | Wire resolver through config |
| `internal/tui/tree.go` | Generic `flowGroup` + `buildGroups(keyFn)` |
| `internal/tui/tree_test.go` | Update tests for generic grouping |
| `internal/tui/list.go` | Add PROCESS column rendering |
| `internal/tui/app.go` | TreeMode cycle, process-tree rendering |
| `internal/tui/detail.go` | Add Process section to detail card |
| `internal/tui/help.go` | Update help text for tree modes |
| `cmd/httpmon/main.go` | Create resolver, wire into proxy |
| `go.mod` | Add `gopsutil/v3` dependency |

---

## 9. Testing

### Unit Tests

**`internal/procinfo/resolver_test.go`:**
- Test resolution with mock store — verify FlowMeta.Process and FlowData fields
  are set after Resolve completes.
- Test PID cache hit — verify process.NewProcess not called twice for same PID.
- Test unknown process — verify `—` fallback when process lookup fails.
- Test port not found — verify graceful handling when no connection matches.

**`internal/tui/tree_test.go`:**
- Update all existing tests from `hostGroup` → `flowGroup` with `hostKey`.
- Add parallel tests with `processKey` extractor.
- Test `buildGroups` with mixed known/unknown process names.
- Test `flattenTree`/`flattenFocus` with process groups.
- Test tree mode cycle state transitions: flat → host → process → flat.

**`internal/tui/list_test.go`:**
- Test PROCESS column rendering in flat mode.
- Test `—` display for unknown process.
- Test column width capping at 15 chars.
- Test process column hidden in process-tree mode.
- Test HOST column shown in process-tree mode.

**`internal/tui/detail_test.go`:**
- Test Process section renders when PID present.
- Test Process section absent when PID is 0.
- Test cmdline truncation for long command lines.

### E2E Tests (update existing)

**`internal/e2e/harness_test.go`:**
- Wire `procinfo.Resolver` into harness's proxy setup.
- Add `waitForProcess` helper: ticks until FlowMeta.Process != "".

**`internal/e2e/capture_test.go`:**
- `TestE2E_FlatView_GETRequest`: after waiting for text, also verify process
  column shows a non-empty process name (the test binary itself, e.g. "e2e.test"
  or similar).
- `TestE2E_DetailView_RequestBody`: enter detail view, verify Process section
  shows PID and process name.

**`internal/e2e/views_test.go`:**
- Add `TestE2E_ProcessTreeMode`: send requests, press `t` twice to enter
  process-tree, verify grouping by process name.
- Add `TestE2E_ProcessTreeFocus`: enter process-tree, press `f` to focus a
  process group.

**New `internal/e2e/process_test.go`:**
- `TestE2E_ProcessResolution`: send request, wait for process name to appear
  in FlowMeta, verify it matches the test binary name.
- `TestE2E_ProcessInDetailCard`: send request, open detail card, verify PID
  and cmdline are displayed.
- `TestE2E_ProcessTreeGrouping`: send requests from same test binary, verify
  all flows group under one process node.

---

## Resolved Questions

1. **Permissions**: Log a warning (once) if process resolution fails N times
   consecutively (e.g., 5). Suggests running with `sudo` or granting permissions.
   Still fall back to `—` per-flow.
2. **Goroutine limit**: Use a semaphore (buffered channel, size ~10) to cap
   concurrent resolution goroutines. Excess requests queued, not dropped.
