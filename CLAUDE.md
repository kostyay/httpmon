# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

*Never* commit to main/master directly, always create a side branch. Code is merged via pull request.

## Build Commands

```bash
make all        # Full pipeline: lint -> test -> build
make lint       # Run golangci-lint
make test       # Run tests with race detection and coverage
make build      # Build binary to ./httpmon
make security   # Run gosec, govulncheck, gitleaks, trufflehog
```

Run a single test:
```bash
go test -v -run TestName ./internal/package/
```

## Architecture

Terminal-native HTTP/HTTPS debugging proxy. Screen-based TUI with keyboard-driven navigation.

### Key Packages

- **cmd/httpmon** - Entry point, CLI flags, wiring

### Design Patterns

- Dependency injection via interfaces for testing
- Interface-driven separation: TUI consumes proxy engine through Go interfaces
