# httpmon

Terminal-native HTTP/HTTPS debugging proxy. Intercept, inspect, and filter traffic — all from your terminal with vim-style navigation.

Think Proxyman or Charles, but in your terminal.

## Features

- **MITM proxy** — Intercept HTTP and HTTPS traffic with auto-generated CA certificates
- **Live flow list** — Watch requests stream in real-time with color-coded methods and status codes
- **Detail inspector** — Drill into any request/response: headers, bodies, timing
- **Quick filter** — Type `/` to filter flows by host or path instantly
- **Keyboard-driven** — Vim-style navigation (j/k/Enter/Esc) — no mouse required
- **Body capture** — Request and response bodies captured up to 5MB

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
httpmon                    # Start proxy on :8080
httpmon --port 9090        # Custom port
httpmon --install-ca       # Install CA cert into system trust store (needs sudo)
httpmon --version          # Print version
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

### Flow List

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate up/down |
| `Enter` | Open flow detail |
| `/` | Focus filter bar |
| `Esc` | Clear filter / blur |
| `q` | Quit |

### Detail View

| Key | Action |
|-----|--------|
| `←` / `→` | Switch Request/Response tab |
| `j` / `k` | Scroll content |
| `n` / `N` | Next/previous flow |
| `Esc` | Back to list |

## Architecture

```
cmd/httpmon/       CLI entry point, flag parsing, wiring
internal/proxy/    MITM proxy engine (go-mitmproxy wrapper)
internal/store/    Thread-safe ring buffer for captured flows
internal/filter/   Quick substring filter
internal/tui/      Bubble Tea terminal UI
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
