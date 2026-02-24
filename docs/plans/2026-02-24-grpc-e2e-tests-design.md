# gRPC-Web E2E Tests Design

## Goal

End-to-end tests for gRPC/protobuf body decoding. Real gRPC-Web server (Connect) → httpmon proxy → store → TUI decode + render.

## Dependencies

- `connectrpc.com/connect` — gRPC-Web server + client
- `protoc-gen-go`, `protoc-gen-connect-go` — code generation (dev tools, not Go deps)

## Generated Code

Source: `internal/bodydecoder/testdata/test.proto` (Greeter service, HelloRequest/HelloReply).
Output: `internal/e2e/testpb/` (committed).

```
internal/e2e/testpb/
  test.pb.go
  testpbconnect/
    test.connect.go
```

## Harness Extension

Extend `newHarness` with variadic `harnessOpt`:

```go
type harnessOpt func(*harnessConfig)
func withBodyDecoder(reg *bodydecoder.Registry) harnessOpt
```

Existing tests unchanged (no opts passed).

`grpcHarness` wraps base harness + Connect Greeter client routed through the proxy with `WithGRPCWeb()`.

## Test Scenarios (`grpc_test.go`)

1. **TestGRPCWebDecodeResponse** — SayHello, response tab shows named JSON fields
2. **TestGRPCWebDecodeRequest** — request tab shows named JSON fields
3. **TestGRPCWebRawToggle** — `r` key toggles between decoded and raw
4. **TestGRPCWebWithoutProtoFiles** — no proto paths → field-number JSON ("1", "2")
5. **TestGRPCWebDecodeError** — garbage bytes with grpc-web content type → status bar error
6. **TestGRPCWebNonProtoUnchanged** — regular JSON GET still works normally

## Makefile

```makefile
generate-proto:
	protoc ...
```

Not wired into `make all`.
