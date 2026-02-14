# httpmon

Terminal-native HTTP/HTTPS debugging proxy. Intercept, inspect, and filter traffic — all from your terminal with vim-style navigation.

Think Proxyman or Charles, but in your terminal.

## Features

- **MITM proxy** — Intercept HTTP and HTTPS traffic with auto-generated CA certificates
- **Live flow list** — Watch requests stream in real-time with color-coded methods and status codes
- **Tree view** — Group flows by host, expand/collapse, focus on a single host
- **Detail inspector** — Headers, syntax-highlighted bodies, collapsible sections, image preview
- **Action menu** — Press `Space` for a context-aware command popup — no memorization needed
- **Quick filter** — `/` to filter by host, path, method, status code, or content type
- **HAR export** — Export all flows or a single flow to HAR format
- **Request tools** — Compose new requests, repeat captured ones, copy as cURL
- **Diff view** — Mark two flows and compare request/response side-by-side
- **Host filtering** — Block or allow hosts at the proxy layer with wildcard patterns
- **Scripting** — JavaScript hooks to modify requests/responses on the fly
- **Breakpoints** — Pause flows mid-flight, edit headers/body in a built-in editor, then resume
- **Bandwidth throttling** — Simulate 3G/4G/WiFi network conditions
- **Map Local** — Serve local files instead of upstream responses via `ctx.respondWith({file})`
- **Keyboard-driven** — Vim-style navigation throughout — no mouse required

## Quick Start

### Install from source

```bash
go install github.com/kostyay/httpmon/cmd/httpmon@latest
```

### Build locally

```bash
git clone https://github.com/kostyay/httpmon.git
cd httpmon
make build
./httpmon
```

### Usage

```bash
httpmon                          # Start proxy on :8080
httpmon --port 9090              # Custom port
httpmon --block "*.ads.com"      # Block hosts matching pattern
httpmon --allow "api.example.*"  # Only intercept matching hosts
httpmon --throttle 3g            # Simulate 3G network (750 kbps)
httpmon --throttle 4g            # Simulate 4G network (4 Mbps)
httpmon --latency 100ms          # Add 100ms latency to responses
httpmon --install-ca             # Install CA cert into system trust store (needs sudo)
httpmon --version                # Print version
```

Then configure your browser or app to use `http://localhost:8080` as its HTTP proxy.

### Trust the CA certificate

The easiest way — run once with sudo:

```bash
sudo httpmon --install-ca
```

This generates the CA cert (if it doesn't exist yet) and adds it to your system trust store.
Supports macOS and Linux.

<details>
<summary>Manual installation</summary>

**macOS:**

```bash
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain ~/.httpmon/mitmproxy-ca-cert.pem
```

**Linux:**

```bash
sudo cp ~/.httpmon/mitmproxy-ca-cert.pem /usr/local/share/ca-certificates/httpmon.crt
sudo update-ca-certificates
```

</details>

## Scripting

Scripts are JavaScript files stored in `~/.httpmon/scripts/`. Each script has a YAML frontmatter header and exports `onRequest` and/or `onResponse` hooks.

### Script format

```javascript
// ---
// name: Add Auth Header
// match:
//   - "*://api.example.com/*"
// enabled: true
// ---

function onRequest(ctx) {
  ctx.headers["Authorization"] = "Bearer my-token";
}

function onResponse(ctx) {
  // ctx.status, ctx.headers, ctx.body
}
```

### Header fields

| Field | Description |
|-------|-------------|
| `name` | Display name (required) |
| `match` | URL patterns to match — `*` wildcards supported (required) |
| `enabled` | `true` or `false` — defaults to `true` if omitted |

### Hooks

**`onRequest(ctx)`** — runs before the request is sent upstream.

| Field | Type | Description |
|-------|------|-------------|
| `ctx.method` | string | HTTP method (read/write) |
| `ctx.url` | string | Full URL (read/write) |
| `ctx.headers` | object | Request headers (read/write) |
| `ctx.body` | string | Request body (read) |
| `ctx.blocked` | bool | Set to `true` to block the request |
| `ctx.respondWith(opts)` | function | Send a synthetic response (see below) |
| `ctx.readFile(path)` | function | Read a local file relative to script dir |
| `ctx.breakpoint()` | function | Pause for interactive editing |

**`onResponse(ctx)`** — runs before the response reaches the client.

| Field | Type | Description |
|-------|------|-------------|
| `ctx.status` | int | Status code (read/write) |
| `ctx.headers` | object | Response headers (read/write) |
| `ctx.body` | string | Response body (read) |
| `ctx.respondWith(opts)` | function | Replace the response |
| `ctx.readFile(path)` | function | Read a local file relative to script dir |
| `ctx.breakpoint()` | function | Pause for interactive editing |

### Script actions

**`ctx.respondWith(opts)`** — send a synthetic response. In `onRequest`, this short-circuits the upstream request. In `onResponse`, it replaces the response.

```javascript
// Return a JSON body
ctx.respondWith({status: 200, body: '{"ok": true}', headers: {"Content-Type": "application/json"}});

// Serve a local file (content-type inferred from extension)
ctx.respondWith({file: "./fixtures/users.json"});
```

**`ctx.readFile(path)`** — read a local file as a string. Returns `null` if the file is missing. Paths resolve relative to the script file's directory.

```javascript
let data = ctx.readFile("./fixtures/users.json");
```

**`ctx.breakpoint()`** — pause the flow for interactive editing. The TUI shows a built-in editor with headers and body panes. After resume, `ctx.headers` and `ctx.body` reflect the user's edits.

```javascript
function onRequest(ctx) {
  if (ctx.url.includes("/api/sensitive")) {
    ctx.breakpoint();
  }
}
```

### Managing scripts

Press `S` in the TUI to open the scripts manager. From there:

- **Space** — Toggle a script on/off
- **n** — Create a new script from template
- **m** — Quick-add a map-local script (URL pattern + file path)
- **e** — Edit a script in your `$EDITOR`
- **d** — Delete a script (with confirmation)

Scripts are categorized automatically: `[Breakpoint]`, `[Map Local]`, or `[Script]` badges appear in the list based on which APIs the script uses.

## Throttle

Simulate slow network conditions. Throttling applies to all response bodies.

### Presets

| Preset | Bandwidth | Latency |
|--------|-----------|---------|
| `3g` | 750 kbps (93,750 B/s) | 100ms |
| `4g` | 4 Mbps (500,000 B/s) | 50ms |
| `wifi` | 30 Mbps (3,750,000 B/s) | 5ms |

### CLI

```bash
httpmon --throttle 3g             # Apply 3G preset
httpmon --throttle wifi --latency 200ms  # WiFi bandwidth + custom latency
```

### TUI

Press `T` to open the throttle modal. Select a preset with `j`/`k` and press `Enter` to apply. The active preset shows in the status bar.

## Map Local

Serve local files instead of fetching from upstream using `ctx.respondWith({file})` in a script.

```javascript
// ---
// name: Mock Users API
// match:
//   - "*://api.example.com/users*"
// ---
function onRequest(ctx) {
  ctx.respondWith({file: "./fixtures/users.json"});
}
```

Use `m` in the scripts manager (`S`) to quickly create a map-local script from a URL pattern and file path.

## Breakpoints

Pause HTTP flows mid-flight, inspect and edit headers/body in a built-in editor, then resume.

Add `ctx.breakpoint()` to any script:

```javascript
// ---
// name: Debug API
// match:
//   - "*://api.example.com/*"
// ---
function onRequest(ctx) {
  ctx.breakpoint();
}
```

Press `B` to open the breakpoint queue. Select a paused flow to edit:

- **Tab** — Switch between headers and body panes
- **Enter** / **Ctrl+S** — Resume with modifications
- **Esc** — Skip (resume unmodified)
- **E** — Open in external editor

All paused flows auto-resume on exit.

## Keyboard Shortcuts

Press `?` anywhere for the full help overlay, or `Space` for a context-aware action menu.

### Flow List

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate up/down |
| `Enter` | Open flow detail |
| `t` | Toggle flat/tree view |
| `f` | Focus host (tree mode) |
| `l` / `h` | Expand/collapse host (tree mode) |
| `/` | Focus filter bar |
| `Space` | Open action menu |
| `x` | Export HAR |
| `C` | Compose request |
| `d` | Mark for diff |
| `q` | Quit |

### Detail View

| Key | Action |
|-----|--------|
| `1` / `2` | Request/Response tab |
| `j` / `k` | Scroll content |
| `n` / `N` | Next/previous flow |
| `p` | Toggle pretty/raw |
| `e` | Open body in external editor |
| `i` | Toggle image preview |
| `g` / `h` / `b` | Collapse general/headers/body |
| `/` | Search in content |
| `Space` | Open action menu |
| `x` | Export HAR |
| `c` | Copy as cURL |
| `r` | Repeat request |
| `Esc` | Back to list |

### Global

| Key | Action |
|-----|--------|
| `S` | Scripts manager |
| `T` | Throttle settings |
| `B` | Breakpoint queue |
| `?` | Help overlay |

## Architecture

```
cmd/httpmon/          CLI entry point, flag parsing, wiring
internal/proxy/       MITM proxy engine (go-mitmproxy wrapper)
internal/store/       Thread-safe ring buffer for captured flows
internal/tui/         Bubble Tea terminal UI (list, detail, menu, compose, diff, export)
internal/filter/      Quick filter + advanced filter expressions
internal/hostfilter/  Wildcard-based host block/allow at proxy layer
internal/har/         HAR 1.2 export
internal/diff/        Flow diff engine
internal/highlight/   Syntax highlighting for response/request bodies
internal/certutil/    CA certificate generation and system trust installation
internal/throttle/    Bandwidth throttling (wraps io.Reader with rate limiting)
internal/breakpoint/  Breakpoint controller (pause/resume/subscribe)
internal/scripting/   JavaScript scripting engine (goja runtime, respondWith, readFile, breakpoint)
```

Built with [go-mitmproxy](https://github.com/lqqyt2423/go-mitmproxy) and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Contributing

```bash
make all        # lint + test + build
make test       # tests with race detection
make lint       # golangci-lint
make security   # gosec, govulncheck, gitleaks, trufflehog
```

## License

[MIT](LICENSE)
