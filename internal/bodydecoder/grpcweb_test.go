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

func TestGRPCWebDecoder_CanEncode(t *testing.T) {
	d := &GRPCWebDecoder{}
	if !d.CanEncode("application/grpc-web") {
		t.Error("should match grpc-web")
	}
	if d.CanEncode("application/protobuf") {
		t.Error("should not match protobuf")
	}
}

func TestGRPCWebDecoder_Encode(t *testing.T) {
	reg, _ := LoadProtoFiles([]string{"testdata/test.proto"})
	d := &GRPCWebDecoder{Proto: &RawProtobufDecoder{ProtoReg: reg}}

	jsonBody := []byte(`{"name": "Alice", "age": 30}`)
	wire, err := d.Encode(jsonBody, "application/grpc-web", DecoderMetadata{
		RequestPath: "/testpkg.Greeter/SayHello",
		IsRequest:   true,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Verify frame structure: flag(1) + length(4) + payload
	if len(wire) < frameHeaderLen {
		t.Fatalf("wire too short: %d", len(wire))
	}
	if wire[0] != flagData {
		t.Errorf("flag = 0x%02x, want 0x00", wire[0])
	}
	payloadLen := binary.BigEndian.Uint32(wire[1:frameHeaderLen])
	if int(payloadLen) != len(wire)-frameHeaderLen {
		t.Errorf("length field = %d, actual payload = %d", payloadLen, len(wire)-frameHeaderLen)
	}
}

func TestGRPCWebDecoder_EncodeRoundTrip(t *testing.T) {
	reg, _ := LoadProtoFiles([]string{"testdata/test.proto"})
	d := &GRPCWebDecoder{Proto: &RawProtobufDecoder{ProtoReg: reg}}
	meta := DecoderMetadata{RequestPath: "/testpkg.Greeter/SayHello", IsRequest: true}

	jsonBody := []byte(`{"name": "Bob", "age": 25}`)
	wire, err := d.Encode(jsonBody, "application/grpc-web", meta)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Decode the encoded output.
	decoded, _, err := d.Decode(wire, meta)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got["name"] != "Bob" {
		t.Errorf("name = %v", got["name"])
	}
	if got["age"] != float64(25) {
		t.Errorf("age = %v", got["age"])
	}
}

func TestGRPCWebDecoder_Encode_PropagatesError(t *testing.T) {
	// No proto registry → encode should fail.
	d := &GRPCWebDecoder{Proto: &RawProtobufDecoder{}}
	_, err := d.Encode([]byte(`{}`), "application/grpc-web", DecoderMetadata{
		RequestPath: "/testpkg.Greeter/SayHello",
		IsRequest:   true,
	})
	if err == nil {
		t.Fatal("expected error without proto registry")
	}
}
