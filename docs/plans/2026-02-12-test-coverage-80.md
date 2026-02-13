# Test Coverage → 80% Plan

Target: proxy (63→80%) and tui (73→80%).

## 1. proxy package

Extend existing integration harness (`setupProxy` + `proxyClient`).

| Test | Covers |
|------|--------|
| Request body >5MB truncated to maxBodySize | `Request` body truncation |
| Script engine mutates request URL/headers/method → upstream sees changes | `Request` scripting path |
| Script blocks request → 403 returned, flow stored | `Request` blocking path |
| Script engine rewrites response status/headers → store reflects | `Response` scripting path |
| Response body >5MB truncated | `Response` body truncation |
| Backend resets connection → StateFailed in store | `markFailed` |
| `Addr()` / `CACertPath()` non-empty after Init | Simple getters |

## 2. tui — pure functions (list.go, styles.go)

Table-driven tests, no App needed.

- `shortContentType`: json, html, css, js, image/png, font/woff2, grpc, form, multipart, param stripping, unknown passthrough, empty
- `truncPad`: w=0, w=1, shorter (padded), exact, longer (truncated with ellipsis)
- `formatSize`: 0B, bytes, KB, MB
- `formatDuration`: µs, ms, seconds
- `statusStyle`: 200, 301, 404, 500, 100
- `truncate`: shorter, exact, longer

## 3. tui — stateful components

Use existing `newTestApp` + mock harness. Add `mockScriptManager`.

### compose
- Esc → `showCompose=false`
- Tab → cycles method through composeMethods
- Ctrl+J/K → cycles composeFocus 0→1→2→0
- Verify focused textinput after focus change

### scripts (needs mockScriptManager)
- `initScripts`: sets showScripts, cursor=0, calls Reload
- j/k: moves cursor
- space: calls Toggle
- d: sets confirmDelete; y: calls Delete
- esc/S: closes
- `viewScripts`: renders empty state + populated list

### diffview
- `viewDiff`: set showDiff+diffContent → output contains header + content

### export
- Esc closes
- Enter with flows → calls doExportHAR (mock store, temp file)

### updateDetailContent
- Get error → "no longer available"
- Normal render path
- Image preview toggle on image content-type

### dispatchMenuAction
- "Export HAR", "Export single HAR", "Compose" branches
