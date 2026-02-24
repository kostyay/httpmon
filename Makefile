.PHONY: all fmt lint test e2e build clean security release generate-proto

all: lint test build

fmt:
	goimports -w .

lint:
	golangci-lint run

test:
	go test -race -coverprofile=coverage.out ./...

e2e:
	go test -v -tags e2e -count=1 -timeout 120s ./internal/e2e/

build:
	go build -o httpmon ./cmd/httpmon

clean:
	rm -f httpmon coverage.out

release: lint test security
	@git fetch --tags origin && \
	if [ -n "$(VERSION)" ]; then TAG=$(VERSION); \
	else LATEST=$$(git tag -l 'v*' --sort=-v:refname | sed -n '1p'); \
	  if [ -z "$$LATEST" ]; then TAG=v0.1.0; \
	  else V=$${LATEST#v}; P=$${V##*.}; TAG=v$${V%.*}.$$((P+1)); fi; \
	fi && \
	if [ "$(CONFIRM)" = "y" ]; then ans=y; else printf "Release $$TAG? [y/N] " && read ans; fi && [ "$$ans" = y ] && \
	git tag "$$TAG" && git push origin "$$TAG" && \
	GITHUB_TOKEN=$$(gh auth token) goreleaser release --clean

# Requires: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#           go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
#           go install github.com/bufbuild/buf/cmd/buf@latest
generate-proto:
	buf generate \
		--template '{"version":"v2","plugins":[{"remote":"buf.build/protocolbuffers/go","out":"internal/e2e/testpb","opt":"paths=source_relative"},{"remote":"buf.build/connectrpc/go","out":"internal/e2e/testpb","opt":"paths=source_relative"}]}' \
		internal/bodydecoder/testdata/test.proto

security:
	@echo "==> gosec"
	@command -v gosec >/dev/null 2>&1 && gosec -exclude-dir=internal/e2e/testpb ./... || echo "gosec not installed (go install github.com/securego/gosec/v2/cmd/gosec@latest)"
	@echo "==> govulncheck"
	@command -v govulncheck >/dev/null 2>&1 && govulncheck ./... || echo "govulncheck not installed (go install golang.org/x/vuln/cmd/govulncheck@latest)"
	@echo "==> gitleaks"
	@command -v gitleaks >/dev/null 2>&1 && gitleaks detect --source . --verbose || echo "gitleaks not installed (brew install gitleaks)"
	@echo "==> trufflehog"
	@command -v trufflehog >/dev/null 2>&1 && trufflehog git file://. --only-verified --fail || echo "trufflehog not installed (brew install trufflehog)"
