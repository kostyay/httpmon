# gRPC/gRPC-Web Transparent Encode/Decode for Scripts

## Goal

Scripts see JSON for all body types — protobuf, gRPC-Web.
Modifications auto-re-encode to wire format before forwarding.
No script API changes. Same ergonomics as HTTP/JSON editing.

## Scope

- gRPC-Web (`application/grpc-web`, `application/grpc-web+proto`)
- Raw protobuf (`application/protobuf`, `application/x-protobuf`, `application/grpc`, `application/grpc+proto`)
- Encode requires `.proto` files loaded. Without protos: decode to JSON for display, modifications skipped with warning log.
- Connect protocol: out of scope for v1. Content-type matching handles it naturally if added later.
- Native gRPC (HTTP/2): out of scope.

## Section 1: Encoder Interface

Separate `Encoder` interface — `Decoder` stays untouched. Types that can encode implement both.

```go
// bodydecoder/encoder.go
type Encoder interface {
    CanEncode(contentType string) bool
    Encode(jsonBody []byte, contentType string, meta *DecoderMetadata) ([]byte, error)
}

var ErrNoEncoder = errors.New("no encoder for content type")
```

`Registry.Encode()` iterates decoders, type-asserts to `Encoder`:

```go
func (r *Registry) Encode(body []byte, ct string, meta *DecoderMetadata) ([]byte, error) {
    for _, d := range r.decoders {
        if enc, ok := d.(Encoder); ok && enc.CanEncode(ct) {
            return enc.Encode(body, ct, meta)
        }
    }
    return nil, ErrNoEncoder
}
```

### RawProtobufDecoder.Encode()

- `CanEncode()` delegates to `CanDecode()` (same content types)
- Looks up method via `ProtoRegistry.LookupMethod(meta.RequestPath)`
- No match → `ErrNoEncoder`
- `protojson.Unmarshal(jsonBody, dynamicMsg)` → `proto.Marshal(dynamicMsg)` → wire bytes

### ProtoRegistry.EncodeNamed()

- Uses `meta.IsRequest` to pick input type vs output type from method descriptor
- Creates `dynamicpb.NewMessage(msgDesc)`
- `protojson.Unmarshal` → `proto.Marshal`

### GRPCWebDecoder.Encode()

- `CanEncode()` delegates to `CanDecode()`
- Delegates payload to `RawProtobufDecoder.Encode()`
- Wraps in gRPC-Web data frame: `0x00` + uint32 big-endian length + payload

## Section 2: Interceptor Integration

Extract a helper to avoid duplicating decode→run→encode logic:

```go
func (i *interceptor) runScriptsWithCodec(
    body []byte, contentType string, meta bodydecoder.DecoderMetadata,
    run func(body []byte) (modified []byte, changed bool),
) []byte {
    decoded, _, err := i.decoderReg.Decode(body, contentType, &meta)
    if err != nil {
        // Can't decode — run scripts with raw body
        result, _ := run(body)
        return result
    }
    // Scripts see JSON
    result, changed := run(decoded)
    if !changed {
        return body // unchanged — return original wire bytes
    }
    // Re-encode
    encoded, err := i.decoderReg.Encode(result, contentType, &meta)
    if err != nil {
        log.Warn("encode failed, using original body", "err", err)
        return body
    }
    return encoded
}
```

Called from both `Request()` and `Response()` wrapping the existing script execution.

### Key invariant

Original content-type header **never changed**. Decode/encode invisible to upstream/downstream. Only the script sees JSON.

## Section 3: DecoderMetadata Plumbing

- `RequestPath`: already available from `f.Request.URL.Path`
- `IsRequest`: set `true` in request hook, `false` in response hook
- Response path carries forward request URL from `mp.Flow`

## Section 4: Error Handling

- Decode failure → pass raw body, no error to script
- Encode failure (no proto, no method match) → warn log, use original body
- Encode failure (malformed JSON) → error log, use original body
- **Never corrupt wire traffic.** Any encode error → fall back to original bytes.

## Section 5: Files Changed

1. `internal/bodydecoder/encoder.go` — New file: `Encoder` interface, `ErrNoEncoder`
2. `internal/bodydecoder/decoder.go` — Add `Registry.Encode()` with type-assertion loop
3. `internal/bodydecoder/protobuf.go` — Add `CanEncode()`/`Encode()`, `ProtoRegistry.EncodeNamed()`
4. `internal/bodydecoder/grpcweb.go` — Add `CanEncode()`/`Encode()` with frame wrapping
5. `internal/proxy/interceptor.go` — Add `runScriptsWithCodec()` helper, wire into `Request()` and `Response()`

## Resolved Questions

1. **Connect protocol** — Out of scope v1. Content-type matching handles it if added later. JSON mode needs no codec.
2. **Streaming** — Single-frame only. Multi-frame encode is a separate problem.
3. **Body size** — Non-issue. Re-encoded protobuf smaller than JSON intermediate. 5MB cap on wire body.
