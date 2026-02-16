# Changelog

## feat/process-identification

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

## [feat/script-actions](https://github.com/kostyay/httpmon/pull/10) - 2026-02-14

Scripts gain three new primitives: `ctx.respondWith()` for synthetic responses,
`ctx.readFile()` for local file access, and `ctx.breakpoint()` for interactive
pause/edit/resume (#10). The `internal/maplocal` package is deleted entirely —
map-local is now just `ctx.respondWith({file: "path"})` in a script. A new
`BreakpointController` manages pause/resume with per-flow channels and a TUI
editor featuring dual headers/body panes with syntax highlighting. Scripts are
auto-categorized as `[Breakpoint]`, `[Map Local]`, or `[Script]` via static
source analysis, and a quick-add helper (`m` key) generates map-local
boilerplate from a URL pattern and file path.

## [fix/gosec-security-issues](https://github.com/kostyay/httpmon/pull/9) - 2026-02-14

Resolve all gosec security findings (#9). File permissions tightened from 0644
to 0600, integer overflow guards added with `math.MaxInt32` clamping, unchecked
`Close()` and `Remove()` calls addressed, and `#nosec` annotations added with
justification comments for intentional patterns (MITM TLS, user-selected editor
commands, known-dir file reads).

## [feat/throttle-maplocal](https://github.com/kostyay/httpmon/pull/8) - 2026-02-14

Bandwidth throttling and map-local file serving gain full TUI modals with
keyboard-driven preset selection (3G/4G/WiFi) and rule management (#8). The
proxy interceptor now correctly completes MapLocal flows during the Request
phase, fixing a bug where locally-served responses stayed in-progress
indefinitely.

23 end-to-end tests exercise the full stack (real HTTP server → MITM proxy →
store → TUI) across five categories: capture, filtering, views, actions, and
proxy features. All tests run in parallel with per-test isolation. README
updated with scripting, throttling, and map-local documentation.

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

Flow list gains tree view with host grouping, expand/collapse, and focus mode.
Detail view adds image preview, syntax-highlighted bodies, collapsible sections,
in-view search with match navigation, and external editor support. New CLI flags
`--block` and `--allow` enable wildcard-based host filtering at the proxy layer.

Supporting packages round out the feature set: HAR export, request diff, request
repeat, cURL copy (OSC 52 clipboard), request composer, throttling, map-local
file serving, and advanced filter expressions.

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
