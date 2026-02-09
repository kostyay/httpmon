# Changelog

## feat/phase1-core-pipeline-tui

Full HTTP/HTTPS debugging proxy with terminal UI (#3). Captures request/response
flows through a go-mitmproxy MITM interceptor into a thread-safe ring buffer
store, with real-time display via Bubble Tea TUI featuring list/detail views,
keyboard navigation (j/k/enter/esc), and live `/` filtering. E2E test suite
covers HTTP and HTTPS interception, POST body capture, header preservation,
error status codes, and concurrent request handling with race detection.

New `--install-ca` flag automates CA certificate trust on macOS and Linux,
replacing manual platform-specific commands. Runs `security add-trusted-cert`
on darwin and `update-ca-certificates` on linux; just `sudo httpmon --install-ca`.

## feat/project-scaffold

Full project scaffolding with security-first CI/CD pipeline. Go 1.25 module
with golangci-lint, goreleaser for cross-platform releases, and a three-layer
security gate: gosec (static analysis), govulncheck (dependency vulnerabilities),
and trivy (filesystem scan). Pre-commit hooks enforce gitleaks and trufflehog
secret scanning on every commit.
