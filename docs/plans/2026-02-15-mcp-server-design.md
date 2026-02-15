# MCP Server for httpmon

**Date:** 2026-02-15
**Status:** Draft

## Overview

Add an MCP (Model Context Protocol) server to httpmon so LLMs can programmatically debug, inspect, and manipulate HTTP traffic. The MCP server runs as a peer to the TUI — both consume the same store/proxy/scripting interfaces.

## Architecture

```
Browser → Proxy → Store
                    ↕
              ┌─────┴─────┐
              TUI         MCP Server
              (human)     (LLM agent)
```

MCP server is **not** layered on the TUI. Both are independent consumers of existing port interfaces: `FlowReader`, `ProxyInfo`, `ScriptManager`, `ThrottleController`.

## Package Structure

```
internal/mcpserver/
  server.go        — Server struct, Start/Stop, tool registration
  tools_read.go    — list_requests, get_request, search_requests, get_request_count, export_har
  tools_sim.go     — set_throttle, get_throttle, replay_request, mock_response
  tools_script.go  — list_scripts, create_script, get_script, toggle_script, delete_script
```

## Dependencies

```go
type Config struct {
    Store    FlowReader
    Proxy    ProxyInfo
    Scripts  ScriptManager
    Throttle ThrottleController
    Port     int // default 9551
}
```

Same interfaces from `tui/ports.go`. No new abstraction layer.

## Transport

SSE/HTTP on a configurable port. Runs alongside TUI.

## Lifecycle

- **Disabled by default.**
- Enabled via TUI settings screen (runtime toggle) or `--mcp` / `--mcp-port N` CLI flags.
- `Server.Start(ctx)` / `Server.Stop()` callable at runtime.
- `Server.Running() bool` for TUI display state.

### CLI Flags

| Flag | Behavior |
|---|---|
| `--mcp` | Start MCP server on default port (9551) |
| `--mcp-port N` | Start on port N (implies `--mcp`) |
| _(omitted)_ | MCP server disabled; toggleable from TUI |

### TUI Integration

- Settings screen entry: **MCP Server: OFF** → toggle → **MCP Server: ON (:9551)**
- Port shown in status bar when active.
- `tui.AppConfig` gets `MCP *mcpserver.Server` field.

## SDK

[github.com/modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) — official Go MCP SDK.

## Tools (14 total)

### Read-only (5)

**`list_requests`**
- Params: `filter` (string, optional, advanced filter syntax), `offset` (int, default 0), `limit` (int, default 50, max 200)
- Returns: array of `{ id, method, status_code, host, path, duration_ms, size_bytes, started_at, state, content_type, scheme }`
- Impl: `store.List()` with `filter.Compile()`

**`get_request`**
- Params: `id` (string, required), `max_body_size` (int, default 50000), `dump_path` (string, optional)
- Returns: full meta + request headers/body + response headers/body
- Body: inline as string (text) or base64 (binary), capped at 50KB default
- If `dump_path` set: writes full bodies to files, returns file paths instead
- Impl: `store.Get()`

**`search_requests`**
- Params: `query` (string, required), `offset` (int), `limit` (int)
- Returns: same shape as `list_requests`
- Impl: `filter.CompileQuick()`

**`get_request_count`**
- Params: `filter` (string, optional)
- Returns: `{ total: int }`
- Impl: `store.List()` reading total count

**`export_har`**
- Params: `filter` (string, optional), `request_ids` (string array, optional)
- Returns: HAR 1.2 JSON string
- Impl: existing `har.Export()`

### Simulation (4)

**`set_throttle`**
- Params: `bps` (int64, 0=disable), `latency_ms` (int, 0=disable), `preset` (string, optional — `3g`/`4g`/`wifi`)
- `preset` overrides `bps`; `latency_ms` applied on top
- Returns: `{ bps: int64, latency_ms: int }`

**`get_throttle`**
- No params
- Returns: `{ bps: int64, latency_ms: int, active: bool }`

**`replay_request`**
- Params (two modes):
  - Replay existing: `request_id` (string)
  - Compose new: `method`, `url`, `headers` (map), `body` (string)
- Routes through proxy via `http.ProxyURL` (same as TUI repeat/compose)
- Returns: `{ status_code: int, request_id: string }`

**`mock_response`**
- Params: `match_pattern` (string, glob), `status` (int), `headers` (map, optional), `body` (string)
- Convenience: generates script via `scripting.Manager.QuickAddMapLocal()`
- Returns: `{ script_path: string, name: string }`

### Script Management (5)

**`list_scripts`**
- No params
- Returns: array of `{ name, file_path, match_patterns, enabled, category }`
- Category: `script` / `mock` / `breakpoint`

**`create_script`**
- Params: `name` (string), `match_patterns` (string array), `code` (string — JS source), `enabled` (bool, default true)
- Generates YAML header, writes to scripts dir, reloads engine
- Returns: `{ file_path: string }`

**`get_script`**
- Params: `file_path` (string)
- Returns: `{ name, match_patterns, enabled, category, source }`

**`toggle_script`**
- Params: `file_path` (string)
- Returns: `{ enabled: bool }`

**`delete_script`**
- Params: `file_path` (string)
- Returns: `{ deleted: bool }`

## E2E Tests

Reuse existing e2e harness infrastructure. Refactor `harness_test.go` to extract shared setup (upstream + proxy + store + HTTP client + handlers) into a base harness. MCP tests extend with MCP server + client session.

```
internal/e2e/
  harness_test.go       — base harness: upstream + proxy + store + doGet/doPost + handlers (refactored from existing)
  tui_harness_test.go   — TUI extension: App + sendKey/tick/view helpers (extracted from current harness)
  mcp_harness_test.go   — MCP extension: starts MCP server on free port, MCP SDK client session
  capture_test.go       — TUI tests (unchanged, use TUI harness)
  actions_test.go       — TUI tests (unchanged)
  views_test.go         — TUI tests (unchanged)
  mcp_test.go           — MCP tests (use MCP harness, call tools, assert on responses)
```

Same `//go:build e2e` tag. ~60% of harness code shared (setup, traffic generation, wait helpers).

### Test Cases (`internal/e2e/mcp_test.go`)

**Read-only:**
- `TestMCP_ListRequests` — send requests through proxy, call `list_requests`, verify count and fields
- `TestMCP_GetRequest` — send POST with body, call `get_request`, verify headers + body returned
- `TestMCP_GetRequest_BodyTruncation` — send large body, verify 50KB cap, then use `dump_path` and verify files written
- `TestMCP_SearchRequests` — send to multiple paths, `search_requests` for substring, verify filtered results
- `TestMCP_GetRequestCount` — send N requests, verify `get_request_count` returns N (with and without filter)
- `TestMCP_ExportHAR` — send requests, call `export_har`, parse JSON, verify HAR structure

**Simulation:**
- `TestMCP_Throttle` — `set_throttle` with 3g preset, verify `get_throttle` returns settings, send request, verify slower than baseline
- `TestMCP_ReplayRequest` — send request, call `replay_request` with its ID, verify new request appears in store
- `TestMCP_ReplayCompose` — call `replay_request` with method/url/headers/body, verify request captured
- `TestMCP_MockResponse` — call `mock_response`, send matching request through proxy, verify synthetic response

**Scripts:**
- `TestMCP_CreateAndListScript` — `create_script`, verify `list_scripts` includes it
- `TestMCP_ToggleScript` — create script, `toggle_script`, verify enabled flips
- `TestMCP_DeleteScript` — create script, `delete_script`, verify removed from `list_scripts`
- `TestMCP_GetScript` — create script, `get_script`, verify source returned
- `TestMCP_MockViaScript` — `mock_response`, verify shows as `mock` category in `list_scripts`

**Lifecycle:**
- `TestMCP_ServerStartStop` — start server, verify tools callable, stop, verify connection closed

## Future (v2+)

- **Breakpoint tools** — pause/resume flows, edit in-flight headers/body via MCP
- **Stdio transport** — headless mode for direct Claude Code integration
- **Flow subscriptions** — real-time notifications when new requests match a pattern
- **Diff tool** — compare two requests via MCP
