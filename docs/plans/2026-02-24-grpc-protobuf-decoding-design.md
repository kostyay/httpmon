# gRPC/Protobuf Body Decoding

## Summary

Decode protobuf and gRPC-Web bodies in the response viewer. Raw wire-format decode by default; full named-field decode when `.proto` files are supplied. Pluggable body decoder architecture for future format support.

## Proto Path Configuration

**Config file** (`~/.httpmon/config.json`):
```json
{
  "proto_paths": ["/home/user/protos", "/home/user/api/service.proto"]
}
```

**CLI flag** (repeated, merges with config):
```
httpmon --proto-path ./protos --proto-path ./other/service.proto
```

**Path resolution:**
- File path: load that single `.proto`
- Directory path: recursively glob all `*.proto`
- CLI paths appended after config paths; all searched together
- All directories act as import roots (like `protoc -I`)

**Startup behavior:**
- Parse proto files into descriptor registry using `bufbuild/protocompile`
- Invalid path or syntax error: warn in TUI status bar, skip, continue
- No hot-reload; restart to pick up changes

**Config struct field:** `ProtoPaths []string \`json:"proto_paths"\``

## Body Decoder Architecture

New package: `internal/bodydecoder`

```
internal/bodydecoder/
├── decoder.go        # Registry + Decoder interface
├── protobuf.go       # Wire-format + proto-file decode
└── grpcweb.go        # gRPC-Web frame stripping → delegates to protobuf
```

Note: JSON pretty-print stays in `highlight.go` — no need to move it into the registry.

### Interface

```go
type Decoder interface {
    CanDecode(contentType string) bool
    Decode(body []byte, metadata DecoderMetadata) (decoded string, resultContentType string, err error)
}

type DecoderMetadata struct {
    RequestPath string // e.g. /api/v1/package.Service/Method
    IsRequest   bool   // true = request body, false = response body
}
```

`resultContentType` tells the highlight pipeline which lexer to use. Convention: protobuf decoder returns `"application/json"` so Chroma highlights as JSON. Future decoders may return other MIME types.

### Registry

```go
func NewRegistry(protoPaths []string) (*Registry, []error)
func (r *Registry) Decode(body []byte, contentType string, meta DecoderMetadata) (string, string, error)
```

Ordered decoder list; first `CanDecode` match wins. Returns `ErrNoDecoder` if no decoder matches — caller falls through to existing highlight behavior.

### Content Type Routing

| Content Type | Decoder | Notes |
|---|---|---|
| `application/grpc-web`, `application/grpc-web+proto` | grpcweb → protobuf | Strip 5-byte frame headers first |
| `application/grpc`, `application/grpc+proto` | protobuf | gRPC over HTTP (direct wire format) |
| `application/protobuf`, `application/x-protobuf`, `application/x-google-protobuf` | protobuf | Plain protobuf wire format |

## Protobuf Decoding

### Message Type Resolution

1. **Has gRPC path** (request path contains `/{package.Service}/{Method}`): Extract service+method from last two path segments. Look up input (request) or output (response) message type from service descriptor.
2. **No gRPC path** (plain protobuf): Skip named decode. Go straight to raw wire decode.
3. **Raw wire decode** (fallback, always available): Iterate varint-tagged fields → JSON like `{"1": 42, "2": "base64data"}`.

Rationale for skipping "try all descriptors" on plain protobuf: wire format is too permissive — almost any byte sequence parses as *some* valid message, producing false positives.

### Dependencies

- `bufbuild/protocompile` — parse `.proto` files into linked descriptors
- `google.golang.org/protobuf/types/dynamicpb` — dynamic message construction
- `google.golang.org/protobuf/encoding/protojson` — marshal to JSON
- `google.golang.org/protobuf/encoding/protowire` — raw wire-format decode

## gRPC-Web Frame Handling

- 1-byte flags + 4-byte big-endian length per frame
- `0x00` = data frame, `0x01` = compressed data frame, `0x80` = trailers frame
- Extract and concatenate data frame payloads
- Pass concatenated payload to protobuf decoder
- **Compressed frames** (flag `0x01`): not supported initially. Display `[compressed gRPC payload, N bytes]` and fall back to raw hex. Can add gzip decompression later.
- **Truncated frames** (length > remaining body): decode what's available, note truncation in output.
- **Trailer frames**: skip (metadata, not message data).

## Integration

### highlight.go Changes

**Remove protobuf/gRPC content types from `binaryMIMETypes`.** Currently these are hardcoded as binary and `Highlight()` returns `[binary content: ...]` before any decoder runs. Must remove:
- `application/x-protobuf`, `application/protobuf`, `application/x-google-protobuf`
- `application/grpc`, `application/grpc+proto`
- `application/grpc-web`, `application/grpc-web+proto`

**Skip NUL-byte and UTF-8 heuristics** when a decoder has successfully decoded the body (decoded output is valid UTF-8 JSON, so heuristics won't trigger on the *output* — but the check order matters).

**New call flow:**

```
renderBody() in detail.go
  ├── call registry.Decode(body, contentType, metadata)
  │   ├── success: use decoded string + resultContentType for highlighting
  │   └── ErrNoDecoder: fall through to existing Highlight() path
  └── call highlight.Highlight(decodedBody, effectiveContentType, ...)
```

The decoder is called **before** `Highlight()`, not inside it. This keeps `Highlight()` focused on syntax highlighting and avoids plumbing `DecoderMetadata` through it.

### DecoderMetadata Plumbing

`renderBody()` in `detail.go` currently receives `(label, body, contentType, darkBg, prettyJSON)`. It needs to also receive `DecoderMetadata`. The metadata is available in the detail view — `FlowMeta.Path` for request path, and which tab is active (request vs response) for `IsRequest`.

Updated signature:
```go
func (a *App) renderBody(label string, body []byte, contentType string, darkBg bool, prettyJSON bool, meta bodydecoder.DecoderMetadata) string
```

### Dependency Injection

`bodydecoder.Registry` created in `cmd/httpmon/main.go`, stored as new field on `tui.AppConfig` struct. `nil` registry is valid (no decoders, all content falls through to existing behavior).

### Proto path merging

```go
paths := append(cfg.ProtoPaths, cliProtoPaths...)
```

Config file paths first, CLI paths appended.

### Startup errors

`NewRegistry()` returns `[]error` for any proto files that failed to parse. These are collected and displayed as warnings in the TUI status bar on first render. Registry still functions with whatever protos loaded successfully.

## Future Extensibility

Adding a new body format:
1. Create `internal/bodydecoder/newformat.go`
2. Implement `Decoder` interface
3. Register in `NewRegistry()`
