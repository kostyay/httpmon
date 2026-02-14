package highlight

import (
	"strings"
	"testing"
)

func TestHighlight(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		contentType string
		darkBg      bool
		prettyJSON  bool
		check       func(t *testing.T, out string)
	}{
		{
			name:        "json produces ANSI",
			body:        []byte(`{"key":"value"}`),
			contentType: "application/json",
			darkBg:      true,
			check:       expectANSI,
		},
		{
			name:        "json pretty-print adds newlines",
			body:        []byte(`{"a":1,"b":2}`),
			contentType: "application/json",
			darkBg:      true,
			prettyJSON:  true,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "\n") {
					t.Error("pretty-printed JSON should contain newlines")
				}
			},
		},
		{
			name:        "json raw stays single-line",
			body:        []byte(`{"a":1}`),
			contentType: "application/json",
			darkBg:      true,
			check: func(t *testing.T, out string) {
				t.Helper()
				lines := strings.Split(strings.TrimSpace(out), "\n")
				if len(lines) > 1 {
					t.Errorf("raw JSON should stay single-line, got %d lines", len(lines))
				}
			},
		},
		{
			name:        "html produces ANSI",
			body:        []byte(`<html><body>Hello</body></html>`),
			contentType: "text/html",
			darkBg:      true,
			check:       expectANSI,
		},
		{
			name:        "octet-stream shows binary placeholder",
			body:        []byte("hello world"),
			contentType: "application/octet-stream",
			darkBg:      true,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "binary") {
					t.Errorf("expected binary placeholder, got %q", out)
				}
			},
		},
		{
			name:        "unknown content-type with text body returns plaintext",
			body:        []byte("hello world"),
			contentType: "application/x-custom-thing",
			darkBg:      true,
			check: func(t *testing.T, out string) {
				t.Helper()
				if out != "hello world" {
					t.Errorf("expected plain text, got %q", out)
				}
			},
		},
		{
			name:        "empty body returns empty string",
			body:        []byte{},
			contentType: "application/json",
			darkBg:      true,
			check: func(t *testing.T, out string) {
				t.Helper()
				if out != "" {
					t.Errorf("expected empty string, got %q", out)
				}
			},
		},
		{
			name:        "content-type with charset",
			body:        []byte(`{"k":1}`),
			contentType: "application/json; charset=utf-8",
			darkBg:      true,
			check:       expectANSI,
		},
		{
			name:        "dark theme produces ANSI",
			body:        []byte(`{"k":1}`),
			contentType: "application/json",
			darkBg:      true,
			check:       expectANSI,
		},
		{
			name:        "light theme produces ANSI",
			body:        []byte(`{"k":1}`),
			contentType: "application/json",
			darkBg:      false,
			check:       expectANSI,
		},
		{
			name:        "invalid JSON with pretty-print still highlights",
			body:        []byte(`{not json}`),
			contentType: "application/json",
			darkBg:      true,
			prettyJSON:  true,
			check:       expectANSI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := Highlight(tt.body, tt.contentType, tt.darkBg, tt.prettyJSON)
			tt.check(t, out)
		})
	}
}

func expectANSI(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escape codes, got %q", out)
	}
}

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		contentType string
		want        bool
	}{
		{"image/jpeg", nil, "image/jpeg", true},
		{"image/png", nil, "image/png", true},
		{"image/gif", nil, "image/gif", true},
		{"image/webp", nil, "image/webp", true},
		{"audio/mpeg", nil, "audio/mpeg", true},
		{"video/mp4", nil, "video/mp4", true},
		{"application/octet-stream", nil, "application/octet-stream", true},
		{"application/zip", nil, "application/zip", true},
		{"application/gzip", nil, "application/gzip", true},
		{"application/pdf", nil, "application/pdf", true},
		{"application/wasm", nil, "application/wasm", true},
		{"font/woff2", nil, "font/woff2", true},
		{"json is not binary", nil, "application/json", false},
		{"html is not binary", nil, "text/html", false},
		{"text/plain is not binary", nil, "text/plain", false},
		{"empty content-type with NUL bytes", []byte{0x00, 0x01, 0x02}, "", true},
		{"empty content-type with text", []byte("hello world"), "", false},
		{"JPEG magic bytes override unknown CT", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "application/x-unknown", true},
		{"PNG magic bytes", []byte{0x89, 0x50, 0x4E, 0x47}, "", true},
		{"GIF magic bytes", []byte("GIF89a\x00\x00"), "", true},
		{"PDF magic bytes", []byte("%PDF-1.4 binary\x00content"), "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsBinary(tt.body, tt.contentType)
			if got != tt.want {
				t.Errorf("IsBinary(%q, %q) = %v, want %v", tt.body, tt.contentType, got, tt.want)
			}
		})
	}
}

func TestHighlightBinaryReturnsPlaceholder(t *testing.T) {
	jpegBody := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	out := Highlight(jpegBody, "image/jpeg", true, false)
	if !strings.Contains(out, "binary") {
		t.Errorf("expected binary placeholder, got %q", out)
	}
	if !strings.Contains(out, "image/jpeg") {
		t.Errorf("expected content type in placeholder, got %q", out)
	}
	if !strings.Contains(out, "6 B") {
		t.Errorf("expected size in placeholder, got %q", out)
	}
}

func TestHighlightBinaryNoContentType(t *testing.T) {
	body := []byte{0x00, 0x01, 0x02, 0x03}
	out := Highlight(body, "", true, false)
	if !strings.Contains(out, "binary") {
		t.Errorf("expected binary placeholder for NUL-containing body, got %q", out)
	}
}
