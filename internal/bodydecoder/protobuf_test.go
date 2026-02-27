package bodydecoder

import (
	"encoding/json"
	"math"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestRawProtobufDecoder_CanDecode(t *testing.T) {
	d := &RawProtobufDecoder{}
	yes := []string{
		"application/protobuf",
		"application/x-protobuf",
		"application/x-google-protobuf",
		"application/grpc",
		"application/grpc+proto",
		"application/protobuf; charset=utf-8",
		"Application/Protobuf",
	}
	for _, ct := range yes {
		if !d.CanDecode(ct) {
			t.Errorf("should match %q", ct)
		}
	}
	no := []string{
		"application/json",
		"text/html",
		"application/grpc-web",
	}
	for _, ct := range no {
		if d.CanDecode(ct) {
			t.Errorf("should not match %q", ct)
		}
	}
}

// buildProto encodes fields into protobuf wire format.
func buildProto(fields ...func([]byte) []byte) []byte {
	var b []byte
	for _, f := range fields {
		b = f(b)
	}
	return b
}

func varintField(num protowire.Number, val uint64) func([]byte) []byte {
	return func(b []byte) []byte {
		b = protowire.AppendTag(b, num, protowire.VarintType)
		b = protowire.AppendVarint(b, val)
		return b
	}
}

func bytesField(num protowire.Number, val []byte) func([]byte) []byte {
	return func(b []byte) []byte {
		b = protowire.AppendTag(b, num, protowire.BytesType)
		b = protowire.AppendBytes(b, val)
		return b
	}
}

func fixed32Field(num protowire.Number, val uint32) func([]byte) []byte {
	return func(b []byte) []byte {
		b = protowire.AppendTag(b, num, protowire.Fixed32Type)
		b = protowire.AppendFixed32(b, val)
		return b
	}
}

func fixed64Field(num protowire.Number, val uint64) func([]byte) []byte {
	return func(b []byte) []byte {
		b = protowire.AppendTag(b, num, protowire.Fixed64Type)
		b = protowire.AppendFixed64(b, val)
		return b
	}
}

func TestRawProtobufDecoder_Varint(t *testing.T) {
	body := buildProto(varintField(1, 42))
	d := &RawProtobufDecoder{}
	decoded, ct, err := d.Decode(body, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("content type = %q", ct)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if v, ok := got["1"]; !ok || v != float64(42) {
		t.Errorf("field 1 = %v", got["1"])
	}
}

func TestRawProtobufDecoder_StringField(t *testing.T) {
	body := buildProto(bytesField(2, []byte("hello world")))
	d := &RawProtobufDecoder{}
	decoded, _, err := d.Decode(body, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if got["2"] != "hello world" {
		t.Errorf("field 2 = %v", got["2"])
	}
}

func TestRawProtobufDecoder_NestedMessage(t *testing.T) {
	inner := buildProto(varintField(1, 99))
	body := buildProto(
		varintField(1, 1),
		bytesField(2, inner),
	)
	d := &RawProtobufDecoder{}
	decoded, _, err := d.Decode(body, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	nested, ok := got["2"].(map[string]any)
	if !ok {
		t.Fatalf("field 2 is not object: %T", got["2"])
	}
	if nested["1"] != float64(99) {
		t.Errorf("nested field 1 = %v", nested["1"])
	}
}

func TestRawProtobufDecoder_RepeatedField(t *testing.T) {
	body := buildProto(
		varintField(1, 10),
		varintField(1, 20),
		varintField(1, 30),
	)
	d := &RawProtobufDecoder{}
	decoded, _, err := d.Decode(body, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	arr, ok := got["1"].([]any)
	if !ok {
		t.Fatalf("field 1 should be array, got %T", got["1"])
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr))
	}
}

func TestRawProtobufDecoder_Fixed32Float(t *testing.T) {
	val := math.Float32bits(3.14)
	body := buildProto(fixed32Field(1, val))
	d := &RawProtobufDecoder{}
	decoded, _, err := d.Decode(body, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	f, ok := got["1"].(float64)
	if !ok {
		t.Fatalf("field 1 type = %T", got["1"])
	}
	if math.Abs(f-3.14) > 0.01 {
		t.Errorf("field 1 = %v, want ~3.14", f)
	}
}

func TestRawProtobufDecoder_Fixed64Float(t *testing.T) {
	val := math.Float64bits(2.718281828)
	body := buildProto(fixed64Field(1, val))
	d := &RawProtobufDecoder{}
	decoded, _, err := d.Decode(body, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	f, ok := got["1"].(float64)
	if !ok {
		t.Fatalf("field 1 type = %T", got["1"])
	}
	if math.Abs(f-2.718281828) > 0.0001 {
		t.Errorf("field 1 = %v, want ~2.718", f)
	}
}

func TestRawProtobufDecoder_EmptyBody(t *testing.T) {
	d := &RawProtobufDecoder{}
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

func TestRawProtobufDecoder_InvalidData(t *testing.T) {
	d := &RawProtobufDecoder{}
	// 0xFF is not a valid protobuf tag.
	_, _, err := d.Decode([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01}, DecoderMetadata{})
	if err == nil {
		t.Error("expected error for invalid protobuf data")
	}
}

func TestRawProtobufDecoder_MultipleFieldTypes(t *testing.T) {
	body := buildProto(
		varintField(1, 42),
		bytesField(2, []byte("test")),
		fixed32Field(3, 100),
	)
	d := &RawProtobufDecoder{}
	decoded, _, err := d.Decode(body, DecoderMetadata{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if got["1"] != float64(42) {
		t.Errorf("field 1 = %v", got["1"])
	}
	if got["2"] != "test" {
		t.Errorf("field 2 = %v", got["2"])
	}
}

func TestRawProtobufDecoder_CanEncode(t *testing.T) {
	d := &RawProtobufDecoder{}
	// CanEncode should match same content types as CanDecode.
	for _, ct := range []string{"application/protobuf", "application/grpc+proto"} {
		if !d.CanEncode(ct) {
			t.Errorf("CanEncode should match %q", ct)
		}
	}
	for _, ct := range []string{"application/json", "application/grpc-web"} {
		if d.CanEncode(ct) {
			t.Errorf("CanEncode should not match %q", ct)
		}
	}
}

func TestRawProtobufDecoder_Encode(t *testing.T) {
	reg, _ := LoadProtoFiles([]string{"testdata/test.proto"})
	d := &RawProtobufDecoder{ProtoReg: reg}

	jsonBody := []byte(`{"name": "Alice", "age": 30}`)
	wire, err := d.Encode(jsonBody, "application/protobuf", DecoderMetadata{
		RequestPath: "/testpkg.Greeter/SayHello",
		IsRequest:   true,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Verify by decoding back.
	decoded, _, err := d.Decode(wire, DecoderMetadata{
		RequestPath: "/testpkg.Greeter/SayHello",
		IsRequest:   true,
	})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got["name"] != "Alice" {
		t.Errorf("name = %v", got["name"])
	}
}

func TestRawProtobufDecoder_Encode_NoProto(t *testing.T) {
	d := &RawProtobufDecoder{} // no ProtoReg
	_, err := d.Encode([]byte(`{}`), "application/protobuf", DecoderMetadata{
		RequestPath: "/testpkg.Greeter/SayHello",
		IsRequest:   true,
	})
	if err == nil {
		t.Fatal("expected error without proto registry")
	}
}

func TestRawProtobufDecoder_Encode_NoMethodMatch(t *testing.T) {
	reg, _ := LoadProtoFiles([]string{"testdata/test.proto"})
	d := &RawProtobufDecoder{ProtoReg: reg}

	_, err := d.Encode([]byte(`{}`), "application/protobuf", DecoderMetadata{
		RequestPath: "/unknown.Svc/Method",
		IsRequest:   true,
	})
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
}

func TestStripParams(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"application/protobuf", "application/protobuf"},
		{"application/protobuf; charset=utf-8", "application/protobuf"},
		{"Application/Protobuf", "application/protobuf"},
		{" TEXT/HTML ; foo=bar ", "text/html"},
	}
	for _, tt := range tests {
		got := stripParams(tt.input)
		if got != tt.want {
			t.Errorf("stripParams(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
