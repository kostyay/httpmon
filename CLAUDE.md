# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

*Never* commit to main/master directly, always create a side branch. Code is merged via pull request.

## Build Commands

```bash
make all        # Full pipeline: lint -> test -> build
make lint       # Run golangci-lint
make test       # Run tests with race detection and coverage
make build      # Build binary to ./httpmon
make security   # Run gosec, govulncheck, gitleaks, trufflehog
```

Run a single test:
```bash
go test -v -run TestName ./internal/package/
```

## Architecture

Terminal-native HTTP/HTTPS debugging proxy. MITM intercept → ring buffer store → Bubble Tea TUI.

### Data Flow

```
Browser → proxy.Proxy (go-mitmproxy) → interceptor addon → store.RingBuffer → tui.App
```

The interceptor (`internal/proxy/interceptor.go`) captures requests/responses and writes `FlowMeta` + `FlowData` into the store. The TUI polls the store on a tick to render the flow list.

### Key Packages

- **cmd/httpmon** — CLI flags, wiring, signal handling. Inits proxy (generates CA), starts TUI.
- **internal/proxy** — Wraps go-mitmproxy. `Proxy.Init()` generates CA certs + sets up MITM. `Proxy.Serve()` runs accept loop.
- **internal/store** — `RingBuffer`: thread-safe circular buffer. Stores `FlowMeta` (list view) separately from `FlowData` (headers/bodies). Evicts oldest on overflow.
- **internal/tui** — Bubble Tea app with list view and detail view. Communicates with store/proxy through `FlowReader` and `ProxyInfo` interfaces (defined in `ports.go`).
- **internal/filter** — `QuickFilter`: case-insensitive substring match on host+path. Implements `store.Filter` interface.
- **internal/certutil** — `Install()`: adds CA cert to system trust store (darwin/linux).

### Design Patterns

- **Port interfaces** (`tui/ports.go`): TUI depends on `FlowReader` and `ProxyInfo` interfaces, not concrete types. This enables testing with mocks.
- **Meta/Data split**: `FlowMeta` (lightweight, always in memory) vs `FlowData` (headers + bodies, loaded on demand). List view only needs Meta; detail view fetches Data.
- **Store.Filter interface**: filtering is pushed into the store's `List()` call, not done in the TUI layer.
