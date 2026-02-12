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
			name:        "unknown content-type returns plaintext",
			body:        []byte("hello world"),
			contentType: "application/octet-stream",
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
