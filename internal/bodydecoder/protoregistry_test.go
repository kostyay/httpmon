package bodydecoder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProtoFiles_SingleFile(t *testing.T) {
	reg, errs := LoadProtoFiles([]string{"testdata/test.proto"})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !reg.HasMethods() {
		t.Fatal("expected methods to be loaded")
	}
	m, ok := reg.LookupMethod("/testpkg.Greeter/SayHello")
	if !ok {
		t.Fatal("SayHello not found")
	}
	if string(m.Input().FullName()) != "testpkg.HelloRequest" {
		t.Errorf("input = %s", m.Input().FullName())
	}
	if string(m.Output().FullName()) != "testpkg.HelloReply" {
		t.Errorf("output = %s", m.Output().FullName())
	}
}

func TestLoadProtoFiles_Directory(t *testing.T) {
	reg, errs := LoadProtoFiles([]string{"testdata"})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !reg.HasMethods() {
		t.Fatal("expected methods from directory scan")
	}
}

func TestLoadProtoFiles_BadPath(t *testing.T) {
	reg, errs := LoadProtoFiles([]string{"/nonexistent/path"})
	if len(errs) == 0 {
		t.Error("expected error for bad path")
	}
	// Registry should still be usable (empty).
	if reg.HasMethods() {
		t.Error("expected no methods")
	}
}

func TestLoadProtoFiles_EmptyPaths(t *testing.T) {
	reg, errs := LoadProtoFiles(nil)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if reg.HasMethods() {
		t.Error("expected no methods")
	}
}

func TestLoadProtoFiles_InvalidProto(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.proto"), []byte("this is not proto"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errs := LoadProtoFiles([]string{dir})
	if len(errs) == 0 {
		t.Error("expected errors for invalid proto syntax")
	}
}

func TestLookupMethod_PrefixedPath(t *testing.T) {
	reg, errs := LoadProtoFiles([]string{"testdata/test.proto"})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	// Path with arbitrary prefix.
	m, ok := reg.LookupMethod("/api/v1/testpkg.Greeter/SayHello")
	if !ok {
		t.Fatal("should match with prefix")
	}
	if string(m.Name()) != "SayHello" {
		t.Errorf("method name = %s", m.Name())
	}
}

func TestLookupMethod_NoMatch(t *testing.T) {
	reg, _ := LoadProtoFiles([]string{"testdata/test.proto"})
	_, ok := reg.LookupMethod("/unknown.Service/Method")
	if ok {
		t.Error("should not match unknown service")
	}
}

func TestLookupMethod_EmptyPath(t *testing.T) {
	reg, _ := LoadProtoFiles([]string{"testdata/test.proto"})
	_, ok := reg.LookupMethod("")
	if ok {
		t.Error("should not match empty path")
	}
}

func TestDecodeNamed_Request(t *testing.T) {
	reg, errs := LoadProtoFiles([]string{"testdata/test.proto"})
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}

	// Build a HelloRequest: name="Alice", age=30
	body := buildProto(
		bytesField(1, []byte("Alice")),
		varintField(2, 30),
	)

	decoded, err := reg.DecodeNamed(body, "/testpkg.Greeter/SayHello", true)
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
	if got["age"] != float64(30) {
		t.Errorf("age = %v", got["age"])
	}
}

func TestDecodeNamed_Response(t *testing.T) {
	reg, errs := LoadProtoFiles([]string{"testdata/test.proto"})
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}

	// Build a HelloReply: message="Hi!", success=true
	body := buildProto(
		bytesField(1, []byte("Hi!")),
		varintField(2, 1),
	)

	decoded, err := reg.DecodeNamed(body, "/testpkg.Greeter/SayHello", false)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got["message"] != "Hi!" {
		t.Errorf("message = %v", got["message"])
	}
	if got["success"] != true {
		t.Errorf("success = %v", got["success"])
	}
}

func TestRawProtobufDecoder_NamedFallbackToRaw(t *testing.T) {
	reg, _ := LoadProtoFiles([]string{"testdata/test.proto"})
	d := &RawProtobufDecoder{ProtoReg: reg}

	// Unknown path: should fall back to raw wire decode.
	body := buildProto(varintField(1, 42))
	decoded, _, err := d.Decode(body, DecoderMetadata{RequestPath: "/unknown.Svc/Method"})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Raw decode uses field numbers.
	if !strings.Contains(decoded, `"1"`) {
		t.Errorf("expected raw field number in output: %s", decoded)
	}
}

func TestRawProtobufDecoder_NamedDecode(t *testing.T) {
	reg, _ := LoadProtoFiles([]string{"testdata/test.proto"})
	d := &RawProtobufDecoder{ProtoReg: reg}

	body := buildProto(
		bytesField(1, []byte("Bob")),
		varintField(2, 25),
	)
	decoded, ct, err := d.Decode(body, DecoderMetadata{
		RequestPath: "/testpkg.Greeter/SayHello",
		IsRequest:   true,
	})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("content type = %q", ct)
	}
	// Named decode uses field names.
	if !strings.Contains(decoded, `"name"`) {
		t.Errorf("expected named field: %s", decoded)
	}
}

func TestExtractGRPCPath(t *testing.T) {
	tests := []struct {
		path    string
		service string
		method  string
	}{
		{"/pkg.Svc/Method", "pkg.Svc", "Method"},
		{"/api/v1/pkg.Svc/Method", "pkg.Svc", "Method"},
		{"/Method", "", ""},
		{"", "", ""},
		{"/", "", ""},
		{"//", "", ""},
	}
	for _, tt := range tests {
		svc, method := extractGRPCPath(tt.path)
		if svc != tt.service || method != tt.method {
			t.Errorf("extractGRPCPath(%q) = (%q, %q), want (%q, %q)",
				tt.path, svc, method, tt.service, tt.method)
		}
	}
}
