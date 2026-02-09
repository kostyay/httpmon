# feat: Extract testable interfaces for TUI

## Problem

TUI depends on concrete `*store.RingBuffer` and `*proxy.Proxy`. Can't mock for integration tests.

## Interfaces (both in `internal/tui/ports.go`)

```go
type FlowReader interface {
    List(filter store.Filter, offset, limit int) ([]store.FlowMeta, int)
    Get(id store.FlowID) (*store.FlowMeta, *store.FlowData, error)
}

type ProxyInfo interface {
    Addr() string
}
```

`*store.RingBuffer` satisfies `FlowReader`. `*proxy.Proxy` satisfies `ProxyInfo`. Zero changes to store/proxy packages.

## Steps

### 1. Create `internal/tui/ports.go`

Define `FlowReader` and `ProxyInfo` interfaces.

### 2. Update `internal/tui/app.go`

- `App.store` → `FlowReader` (was `*store.RingBuffer`)
- `App.proxy` → `ProxyInfo` (was `*proxy.Proxy`)
- `NewApp(s FlowReader, p ProxyInfo)`
- Drop `proxy` package import
- `proxyAddr()` nil-check works (untyped nil interface = nil)

### 3. Update `internal/tui/app_test.go`

- Add `mockFlowReader` (slice + map, no concurrency needed)
- Add `mockProxyInfo` (returns fixed addr)
- Keep existing tests using `*store.RingBuffer` via `seedStore()` — they still work
- Add new tests with mocks:
  - Empty store → proper empty state
  - Filter by host → only matching flows shown
  - Clear filter → all flows reappear
  - Detail tab switching (1/2 keys)
  - Navigate flows in detail (n/N), boundary checks
  - `proxyAddr()` with nil ProxyInfo → fallback ":8080"

## Files Changed

| File | Action |
|------|--------|
| `internal/tui/ports.go` | **NEW** — FlowReader, ProxyInfo |
| `internal/tui/app.go` | **EDIT** — concrete → interface fields |
| `internal/tui/app_test.go` | **EDIT** — mock helpers + new tests |

## Acceptance Criteria

- [ ] `App` accepts interfaces, not concrete types
- [ ] `tui` package no longer imports `proxy` package
- [ ] `make lint` passes
- [ ] `make test` passes — all existing + new tests
- [ ] No import cycles
