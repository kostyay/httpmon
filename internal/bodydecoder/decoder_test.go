package bodydecoder

import (
	"errors"
	"testing"
)

// stubDecoder matches a fixed content type and returns canned output.
type stubDecoder struct {
	match             string
	decoded           string
	resultContentType string
	err               error
}

func (s *stubDecoder) CanDecode(contentType string) bool {
	return contentType == s.match
}

func (s *stubDecoder) Decode(_ []byte, _ DecoderMetadata) (string, string, error) {
	return s.decoded, s.resultContentType, s.err
}

func TestRegistry_FirstMatchWins(t *testing.T) {
	first := &stubDecoder{match: "application/protobuf", decoded: "first", resultContentType: "application/json"}
	second := &stubDecoder{match: "application/protobuf", decoded: "second", resultContentType: "application/json"}

	reg := NewRegistry(first, second)
	got, ct, err := reg.Decode(nil, "application/protobuf", DecoderMetadata{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "first" {
		t.Errorf("expected first decoder to win, got %q", got)
	}
	if ct != "application/json" {
		t.Errorf("unexpected resultContentType %q", ct)
	}
}

func TestRegistry_RoutesToCorrectDecoder(t *testing.T) {
	proto := &stubDecoder{match: "application/protobuf", decoded: "proto-decoded", resultContentType: "application/json"}
	grpc := &stubDecoder{match: "application/grpc-web", decoded: "grpc-decoded", resultContentType: "application/json"}

	reg := NewRegistry(proto, grpc)

	got, _, err := reg.Decode(nil, "application/grpc-web", DecoderMetadata{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "grpc-decoded" {
		t.Errorf("expected grpc decoder, got %q", got)
	}
}

func TestRegistry_NoMatch_ReturnsErrNoDecoder(t *testing.T) {
	proto := &stubDecoder{match: "application/protobuf", decoded: "proto"}

	reg := NewRegistry(proto)
	_, _, err := reg.Decode(nil, "text/html", DecoderMetadata{})
	if !errors.Is(err, ErrNoDecoder) {
		t.Fatalf("expected ErrNoDecoder, got %v", err)
	}
}

func TestRegistry_Empty_ReturnsErrNoDecoder(t *testing.T) {
	reg := NewRegistry()
	_, _, err := reg.Decode(nil, "application/json", DecoderMetadata{})
	if !errors.Is(err, ErrNoDecoder) {
		t.Fatalf("expected ErrNoDecoder, got %v", err)
	}
}

func TestRegistry_DecoderError_Propagated(t *testing.T) {
	decoderErr := errors.New("decode failed")
	bad := &stubDecoder{match: "application/protobuf", err: decoderErr}

	reg := NewRegistry(bad)
	_, _, err := reg.Decode([]byte("data"), "application/protobuf", DecoderMetadata{})
	if !errors.Is(err, decoderErr) {
		t.Fatalf("expected decoder error, got %v", err)
	}
}

func TestRegistry_MetadataPassedThrough(t *testing.T) {
	var captured DecoderMetadata
	capturing := &capturingDecoder{match: "application/protobuf", onDecode: func(meta DecoderMetadata) {
		captured = meta
	}}

	reg := NewRegistry(capturing)
	meta := DecoderMetadata{RequestPath: "/pkg.Svc/Method", IsRequest: true}
	_, _, _ = reg.Decode(nil, "application/protobuf", meta)

	if captured.RequestPath != "/pkg.Svc/Method" {
		t.Errorf("RequestPath not passed through: %q", captured.RequestPath)
	}
	if !captured.IsRequest {
		t.Error("IsRequest not passed through")
	}
}

// capturingDecoder records the metadata it receives.
type capturingDecoder struct {
	match    string
	onDecode func(DecoderMetadata)
}

func (c *capturingDecoder) CanDecode(contentType string) bool {
	return contentType == c.match
}

func (c *capturingDecoder) Decode(_ []byte, meta DecoderMetadata) (string, string, error) {
	if c.onDecode != nil {
		c.onDecode(meta)
	}
	return "", "", nil
}
