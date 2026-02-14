# Changelog

## feat/throttle-maplocal

Bandwidth throttling and map-local file serving gain full TUI modals with
keyboard-driven preset selection (3G/4G/WiFi) and rule management. The proxy
interceptor now correctly completes MapLocal flows during the Request phase,
fixing a bug where locally-served responses stayed in-progress indefinitely.

23 end-to-end tests exercise the full stack (real HTTP server → MITM proxy →
store → TUI) across five categories: capture, filtering, views, actions, and
proxy features. All tests run in parallel with per-test isolation. README
updated with scripting, throttling, and map-local documentation.

## feat/tree-view-flow-list (#7)

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

## feat/phase1-core-pipeline-tui (#3)

Full HTTP/HTTPS debugging proxy with terminal UI (#3). Captures request/response
flows through a go-mitmproxy MITM interceptor into a thread-safe ring buffer
store, with real-time display via Bubble Tea TUI featuring list/detail views,
keyboard navigation (j/k/enter/esc), and live `/` filtering. E2E test suite
covers HTTP and HTTPS interception, POST body capture, header preservation,
error status codes, and concurrent request handling with race detection.

New `--install-ca` flag automates CA certificate trust on macOS and Linux,
replacing manual platform-specific commands. Runs `security add-trusted-cert`
on darwin and `update-ca-certificates` on linux; just `sudo httpmon --install-ca`.

## feat/project-scaffold (#1)

Full project scaffolding with security-first CI/CD pipeline. Go 1.25 module
with golangci-lint, goreleaser for cross-platform releases, and a three-layer
security gate: gosec (static analysis), govulncheck (dependency vulnerabilities),
and trivy (filesystem scan). Pre-commit hooks enforce gitleaks and trufflehog
secret scanning on every commit.
