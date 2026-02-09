.PHONY: all fmt lint test build clean security

all: lint test build

fmt:
	goimports -w .

lint:
	golangci-lint run

test:
	go test -race -coverprofile=coverage.out ./...

build:
	go build -o httpmon ./cmd/httpmon

clean:
	rm -f httpmon coverage.out

security:
	@echo "==> gosec"
	@command -v gosec >/dev/null 2>&1 && gosec ./... || echo "gosec not installed (go install github.com/securego/gosec/v2/cmd/gosec@latest)"
	@echo "==> govulncheck"
	@command -v govulncheck >/dev/null 2>&1 && govulncheck ./... || echo "govulncheck not installed (go install golang.org/x/vuln/cmd/govulncheck@latest)"
	@echo "==> gitleaks"
	@command -v gitleaks >/dev/null 2>&1 && gitleaks detect --source . --verbose || echo "gitleaks not installed (brew install gitleaks)"
	@echo "==> trufflehog"
	@command -v trufflehog >/dev/null 2>&1 && trufflehog git file://. --only-verified --fail || echo "trufflehog not installed (brew install trufflehog)"
