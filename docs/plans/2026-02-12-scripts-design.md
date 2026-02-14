# Scripts: Greasemonkey-Style Request/Response Rewriting

## Overview

User-authored JS scripts that intercept and rewrite HTTP requests/responses flowing through the proxy. Scripts live as `.js` files in `~/.httpmon/scripts/`, use YAML frontmatter for metadata, and are managed via a TUI modal.

## Script Format

```javascript
// ---
// name: Strip Auth Headers
// match:
//   - "*://api.example.com/*"
//   - "*://staging.example.com/v2/*"
// enabled: true
// ---

function onRequest(ctx) {
  delete ctx.headers["Authorization"];
}

function onResponse(ctx) {
  ctx.headers["X-Debug"] = "rewritten";
}
```

- YAML between `// ---` markers, each line stripped of `// ` prefix
- `name` — required, display name (fallback: filename)
- `match` — glob patterns: `*` = wildcard segment, `*://` = any scheme, `*.example.com` = subdomains
- `enabled` — `true`/`false`, defaults `true` if omitted
- `onRequest(ctx)` — ctx: `method`, `url`, `headers`, `body`, `blocked`
- `onResponse(ctx)` — ctx: `status`, `headers`, `body`

## TUI Modal

Toggled via `S` key or action menu "Scripts":

```
┌─ Scripts ──────────────────────────────────────┐
│  ✓  Strip Auth Headers    *://api.example.com/*│
│  ✓  Add Debug Header      *://*/debug/*        │
│  ✗  Block Analytics       *://analytics.*/*    │
│                                                │
│  n:new  e:edit  space:toggle  d:delete  esc    │
└────────────────────────────────────────────────┘
```

- `✓`/`✗` enabled/disabled
- Multiple `@match`: show first + `+N more`
- `space` toggles `enabled:` in file
- `e` opens in `$EDITOR`
- `n` creates from template, opens in `$EDITOR`
- `d` deletes file (with confirmation)
- Re-scan directory on modal open

## Data Flow

```
Browser → proxy → interceptor.Request()
  → build RequestContext from mp.Flow
  → engine.RunOnRequest(url, ctx)
    → each enabled script where glob matches:
       → execute onRequest(ctx) in JS VM
  → if ctx.blocked: return 403, skip upstream
  → else: apply mutations back to mp.Flow → forward upstream

Upstream responds → interceptor.Response()
  → build ResponseContext
  → engine.RunOnResponse(url, ctx)
  → apply mutations back → return to browser
```

## Error Handling

- JS runtime error → log, skip script, continue others
- Script errors shown as `⚠` in modal
- Bad YAML → script not loaded, shown as errored
- Missing `name` → filename as fallback

## New/Modified Files

| File | What |
|------|------|
| `internal/scripting/header.go` | YAML header parser |
| `internal/scripting/header_test.go` | Header parsing tests |
| `internal/scripting/glob.go` | Glob URL matcher |
| `internal/scripting/glob_test.go` | Glob matching tests |
| `internal/scripting/loader.go` | Directory scanner |
| `internal/scripting/loader_test.go` | Loader tests |
| `internal/scripting/scripting.go` | Modify: ScriptMeta, filter by glob+enabled |
| `internal/scripting/scripting_test.go` | Modify: integration tests |
| `internal/proxy/interceptor.go` | Wire engine into Request()/Response() |
| `internal/tui/scripts.go` | Scripts modal |
| `internal/tui/app.go` | Add showScripts, S keybinding |
| `internal/tui/menu.go` | Add "Scripts" menu item |
| `cmd/httpmon/main.go` | Init engine, load scripts, pass to proxy+tui |
| `internal/tui/ports.go` | Add ScriptManager interface |

## ScriptManager Interface

```go
type ScriptManager interface {
    Scripts() []ScriptInfo
    Toggle(name string) error
    Delete(name string) error
    NewScriptPath() string
    ScriptDir() string
    Reload()
}
```

TUI depends on interface, not concrete engine.
