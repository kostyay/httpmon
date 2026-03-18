# Changelog


## feat/linux-browse-support

Added Linux support for the `--browse` flag (#37), extending URL opening and system proxy integration beyond macOS. The `internal/browse` package now uses platform-specific build tags (`//go:build darwin` and `//go:build linux`) with `xdg-open` for Linux browser launching and a documented no-op for Linux system proxy configuration due to desktop environment variability. macOS-specific proxy setup via `networksetup` remains unchanged, maintaining the existing behavior for auto-configuring httpmon as the system proxy and restoring original settings on exit.

## [0.1.13](https://github.com/kostyay/httpmon/pull/36) - 2026-03-18

New `--browse <url>` flag opens the given URL in the default browser with
httpmon's proxy automatically configured as the macOS system proxy (#36), with
the TUI filter pre-populated to show only relevant traffic. Protobuf decoding
gains per-host registry configuration in `~/.httpmon/config.json` and automatic
buf.lock dependency resolution for proto imports (#33, #34). The TUI rendering
layer migrates from `fmt.Sprintf` + `WriteString` to direct `fmt.Fprintf`,
reducing allocations (#35). Scripting is consolidated to core request/response
interception — advanced APIs (`ctx.respondWith()`, `ctx.breakpoint()`,
`ctx.readFile()`) and per-host proto registry in scripts are removed in favor of
the JSON-based configuration approach. Content-encoding headers are now correctly
stripped when the proxy applies body decompression (#34).

## [0.1.12](https://github.com/kostyay/httpmon/pull/31) - 2026-02-27

Body decoder internals refactored with a new `Encoder` interface for
gRPC-Web/protobuf round-trip encoding and a simplified `Decode()` path via
extracted `matchesContentType` helper (#31, #32).

## [0.1.11](https://github.com/kostyay/httpmon/pull/26) - 2026-02-24

Protobuf and gRPC-Web bodies are now decoded into human-readable text in the
detail view (#26). A new `--proto-path` flag loads `.proto` files for named
message decoding; without it, raw wire-format decoding is always available. The
`BodyDecoderRegistry` interface threads through the TUI port layer, and a
`renderOpts` refactor replaces six positional render arguments with a single
struct. Protobuf/gRPC MIME types are removed from the binary blocklist so they
flow through the decoder pipeline. A gRPC-Web frame parser fix resolves a gosec
G115 integer overflow, and a new E2E test exercises a full Connect RPC
round-trip with proto decoding.

## [0.1.10](https://github.com/kostyay/httpmon/pull/22) - 2026-02-16

Persistent configuration via `~/.httpmon/config.json` replaces pure CLI-flag
defaults (#22). A new `internal/config` package handles Load/Save with automatic
default backfill for forward-compatible configs, CLI flag overrides via
`flag.Visit`, and transparent MCP token migration from the legacy `mcp-token`
file. A TUI settings screen (P key) exposes all seven fields — ProxyPort,
MCPEnabled, MCPAddr, BufferSize, ThrottlePreset, ListMode, TreeGroupBy — with
bool/enum toggle and text editing, auto-saving on close. Ten unit tests cover
open/close, navigation, field toggling, text editing with cancel, disk
persistence, view rendering, and menu integration. Go upgraded from 1.25.3 to
1.26.0 alongside golangci-lint v2.9.0, resolving the race detector toolchain
mismatch.

## [0.1.9](https://github.com/kostyay/httpmon/pull/20) - 2026-02-16

An MCP (Model Context Protocol) server lets LLM agents debug HTTP traffic
programmatically alongside the TUI. Fourteen tools span read-only inspection
(`list_requests`, `search_requests`, `export_har`), traffic simulation
(`replay_request`, `mock_response`, `set_throttle`), and script management
(`create_script`, `toggle_script`, `delete_script`). Bearer token auth with a
crypto-random 32-byte hex token protects the localhost-only endpoint. Scripts
use opaque IDs instead of file paths, and all gosec findings are resolved.

## [0.1.8](https://github.com/kostyay/httpmon/pull/19) - 2026-02-16

Each proxied request now shows which OS process initiated it (#19). A new
`internal/procinfo` package resolves PID and process name asynchronously via
gopsutil, with a bounded semaphore, PID→name cache, and graceful fallback to
em dash when permissions are insufficient. The list view gains a PROCESS column,
tree view cycles through flat → host → process grouping via `t`, and the detail
card displays PID and full command line. Supported on macOS and Linux.

## [0.1.7](https://github.com/kostyay/httpmon/pull/14) - 2026-02-15

Homebrew distribution via `brew tap kostyay/tap && brew install httpmon` is now
configured through goreleaser's `homebrew_casks` integration (#13). Each release
auto-generates a cask with platform-specific binaries and SHA256 checksums,
pushed to the `kostyay/homebrew-tap` repo. Project branding added with logo,
favicon, and README header image.

## [0.1.6](https://github.com/kostyay/httpmon/pull/10) - 2026-02-15

Scripts gain three new primitives: `ctx.respondWith()` for synthetic responses,
`ctx.readFile()` for local file access, and `ctx.breakpoint()` for interactive
pause/edit/resume (#10). The `internal/maplocal` package is deleted entirely —
map-local is now just `ctx.respondWith({file: "path"})` in a script. A new
`BreakpointController` manages pause/resume with per-flow channels and a TUI
editor featuring dual headers/body panes with syntax highlighting. Scripts are
auto-categorized as `[Breakpoint]`, `[Map Local]`, or `[Script]` via static
source analysis, and a quick-add helper (`m` key) generates map-local
boilerplate from a URL pattern and file path.

## [0.1.5](https://github.com/kostyay/httpmon/pull/7) - 2026-02-14

JavaScript scripting engine with YAML-frontmatter script files, glob-based URL
matching, and `onRequest`/`onResponse` hooks that can modify headers, bodies,
and status codes in-flight. Scripts live in `~/.config/httpmon/scripts/`, support
hot-reload via `ScriptManager`, and are toggled/created/deleted through a new TUI
modal (Ctrl+S). Integration tests cover the full script lifecycle.

Massive TUI expansion with a discoverable MC-style action menu (#7). Pressing
Space opens a context-aware popup showing available commands for the current
view — list items like Export HAR, Compose Request, Mark for Diff; detail items
like Copy cURL, Repeat Request, Open in Editor — eliminating the need to
memorize 30+ keybindings.

Bandwidth throttling and map-local file serving gain full TUI modals with
keyboard-driven preset selection (3G/4G/WiFi) and rule management (#8). The
proxy interceptor now correctly completes MapLocal flows during the Request
phase, fixing a bug where locally-served responses stayed in-progress
indefinitely. Flow list gains tree view with host grouping, expand/collapse, and
focus mode. Detail view adds image preview, syntax-highlighted bodies,
collapsible sections, in-view search with match navigation, and external editor
support. New CLI flags `--block` and `--allow` enable wildcard-based host
filtering at the proxy layer.

All gosec security findings resolved (#9): file permissions tightened from 0644
to 0600, integer overflow guards added with `math.MaxInt32` clamping, unchecked
`Close()` and `Remove()` calls addressed, and `#nosec` annotations added with
justification comments for intentional patterns. 23 end-to-end tests exercise
the full stack (real HTTP server → MITM proxy → store → TUI) across five
categories: capture, filtering, views, actions, and proxy features.

Supporting packages round out the feature set: HAR export, request diff, request
repeat, cURL copy (OSC 52 clipboard), request composer, and advanced filter
expressions.

## [0.1.1](https://github.com/kostyay/httpmon/pull/3) - 2026-02-09

Full HTTP/HTTPS debugging proxy with terminal UI (#3). Captures request/response
flows through a go-mitmproxy MITM interceptor into a thread-safe ring buffer
store, with real-time display via Bubble Tea TUI featuring list/detail views,
keyboard navigation (j/k/enter/esc), and live `/` filtering. E2E test suite
covers HTTP and HTTPS interception, POST body capture, header preservation,
error status codes, and concurrent request handling with race detection.

New `--install-ca` flag automates CA certificate trust on macOS and Linux,
replacing manual platform-specific commands. Runs `security add-trusted-cert`
on darwin and `update-ca-certificates` on linux; just `sudo httpmon --install-ca`.

## [0.1.0](https://github.com/kostyay/httpmon/pull/1) - 2026-02-09

Full project scaffolding with security-first CI/CD pipeline. Go 1.25 module
with golangci-lint, goreleaser for cross-platform releases, and a three-layer
security gate: gosec (static analysis), govulncheck (dependency vulnerabilities),
and trivy (filesystem scan). Pre-commit hooks enforce gitleaks and trufflehog
secret scanning on every commit.
