package bodydecoder

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestGRPCWebDecoder_CanDecode(t *testing.T) {
	d := &GRPCWebDecoder{}
	yes := []string{
		"application/grpc-web",
		"application/grpc-web+proto",
		"application/grpc-web; charset=utf-8",
	}
	for _, ct := range yes {
		if !d.CanDecode(ct) {
			t.Errorf("should match %q", ct)
		}
	}
	no := []string{
		"application/protobuf",
		"application/grpc",
		"text/plain",
	}
	for _, ct := range no {
		if d.CanDecode(ct) {
			t.Errorf("should not match %q", ct)
		}
	}
}

// buildFrame constructs a gRPC-Web frame: 1-byte flag + 4-byte length + payload.
func buildFrame(flag byte, payload []byte) []byte {
	frame := make([]byte, frameHeaderLen+len(payload))
	frame[0] = flag
	binary.BigEndian.PutUint32(frame[1:frameHeaderLen], uint32(len(payload)))
	copy(frame[frameHeaderLen:], payload)
	return frame
}

func TestGRPCWebDecoder_SingleDataFrame(t *testing.T) {
	proto := buildProto(varintField(1, 42))
	body := buildFrame(flagData, proto)

	d := &GRPCWebDecoder{Proto: &RawProtobufDecoder{}}
	decoded, ct, err := d.Decode(body, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("content type = %q", ct)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got["1"] != float64(42) {
		t.Errorf("field 1 = %v", got["1"])
	}
}

func TestGRPCWebDecoder_MultipleDataFrames(t *testing.T) {
	// Two data frames whose payloads concatenate into one protobuf message.
	var part1, part2 []byte
	part1 = protowire.AppendTag(part1, 1, protowire.VarintType)
	part1 = protowire.AppendVarint(part1, 10)
	part2 = protowire.AppendTag(part2, 2, protowire.VarintType)
	part2 = protowire.AppendVarint(part2, 20)

	body := append(buildFrame(flagData, part1), buildFrame(flagData, part2)...)

	d := &GRPCWebDecoder{Proto: &RawProtobufDecoder{}}
	decoded, _, err := d.Decode(body, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got["1"] != float64(10) || got["2"] != float64(20) {
		t.Errorf("fields = %v", got)
	}
}

func TestGRPCWebDecoder_TrailerFrameSkipped(t *testing.T) {
	proto := buildProto(varintField(1, 99))
	body := append(buildFrame(flagData, proto), buildFrame(flagTrailers, []byte("grpc-status:0\r\n"))...)

	d := &GRPCWebDecoder{Proto: &RawProtobufDecoder{}}
	decoded, _, err := d.Decode(body, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Trailer should not affect output.
	if strings.Contains(decoded, "grpc-status") {
		t.Error("trailer content leaked into output")
	}
}

func TestGRPCWebDecoder_CompressedFrame(t *testing.T) {
	fakePayload := []byte("compressed-data")
	body := buildFrame(flagCompressed, fakePayload)

	d := &GRPCWebDecoder{Proto: &RawProtobufDecoder{}}
	decoded, _, err := d.Decode(body, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(decoded, "[compressed gRPC payload,") {
		t.Errorf("missing compressed note: %q", decoded)
	}
}

func TestGRPCWebDecoder_TruncatedFrame(t *testing.T) {
	// Build a valid protobuf payload that we'll claim is truncated.
	pb := buildProto(varintField(1, 7))

	// Frame header declares 100 bytes, but only len(pb) bytes follow.
	frame := make([]byte, frameHeaderLen+len(pb))
	frame[0] = flagData
	binary.BigEndian.PutUint32(frame[1:frameHeaderLen], 100)
	copy(frame[frameHeaderLen:], pb)

	d := &GRPCWebDecoder{Proto: &RawProtobufDecoder{}}
	decoded, _, err := d.Decode(frame, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(decoded, "truncated") {
		t.Errorf("missing truncation note: %q", decoded)
	}
	// Should still decode the available protobuf data.
	if !strings.Contains(decoded, "\"1\"") {
		t.Errorf("missing decoded field: %q", decoded)
	}
}

func TestGRPCWebDecoder_EmptyBody(t *testing.T) {
	d := &GRPCWebDecoder{Proto: &RawProtobufDecoder{}}
	decoded, ct, err := d.Decode([]byte{}, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("content type = %q", ct)
	}
	if decoded != "{}" {
		t.Errorf("expected empty object, got %q", decoded)
	}
}

func TestGRPCWebDecoder_TruncatedHeader(t *testing.T) {
	// Only 3 bytes — not enough for a frame header.
	_, _, err := extractDataFrames([]byte{0x00, 0x01, 0x02})
	if err == nil {
		t.Error("expected error for truncated header")
	}
}
