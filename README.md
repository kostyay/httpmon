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
internal/throttle/    Bandwidth throttling
internal/maplocal/    Map remote URLs to local files
internal/scripting/   Lua scripting hooks
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
