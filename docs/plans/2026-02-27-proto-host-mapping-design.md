# Proto Host Mapping

Host-specific proto file configuration for protobuf/gRPC-Web decoding.

## Problem

Proto paths are global — all hosts share one `ProtoRegistry`. Different services use different `.proto` files. Need per-host proto resolution.

## Config

Add `proto_hosts` to `~/.httpmon/config.json`:

```json
{
  "proto_paths": ["~/work/shared-protos"],
  "proto_hosts": {
    "api.xx.com": {
      "paths": ["~/work/api/api"]
    },
    "*.internal.io": {
      "paths": ["~/work/internal/protos"],
      "includes": ["~/work/internal/third_party"]
    }
  }
}
```

- Host keys support wildcards (glob matching, reuse `hostfilter` logic)
- `paths`: proto file/dir paths (required)
- `includes`: import search dirs like protoc `-I` (optional)
- Global `proto_paths`/`proto_includes` act as fallback

## Decode Flow

1. Match `meta.Host` against `proto_hosts` keys (wildcard glob)
2. Hit → use that host's compiled `ProtoRegistry`
3. Miss → fall back to global `ProtoRegistry`
4. No global → raw wire decode

## Changes

### `internal/config/config.go`

```go
type ProtoHostConfig struct {
    Paths    []string `json:"paths"`
    Includes []string `json:"includes,omitempty"`
}
```

Add `ProtoHosts map[string]ProtoHostConfig` to `Config`.

### `internal/bodydecoder/decoder.go`

Add `Host string` to `DecoderMetadata`.

New `HostAwareRegistry`: holds map of host-pattern → `*ProtoRegistry` + fallback registry. On decode, matches host → selects registry → delegates to decoder.

### `internal/bodydecoder/protobuf.go`

`RawProtobufDecoder` accepts a registry resolver func instead of single `ProtoReg` field.

### `internal/bodydecoder/grpcweb.go`

Pass host through to inner protobuf decoder.

### `cmd/httpmon/main.go`

Compile global proto paths → fallback registry.
Iterate `cfg.ProtoHosts` → compile per-host registries.
Build `HostAwareRegistry`.

### `internal/tui/detail.go`

Pass `meta.Host` into `DecoderMetadata`.

## Scope

- ~150 LOC net
- No new packages
- Config file only (no TUI editor)
- No CLI flags for host mapping
