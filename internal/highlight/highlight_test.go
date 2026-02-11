package highlight

import (
	"strings"
	"testing"
)

func TestJSONHighlighted(t *testing.T) {
	out := Highlight([]byte(`{"key":"value"}`), "application/json", true, false)
	if !strings.Contains(out, "\x1b[") {
		t.Error("JSON output should contain ANSI escape codes")
	}
}

func TestJSONPrettyPrint(t *testing.T) {
	out := Highlight([]byte(`{"a":1,"b":2}`), "application/json", true, true)
	if !strings.Contains(out, "\n") {
		t.Error("pretty-printed JSON should contain newlines")
	}
}

func TestJSONPrettyPrintDisabled(t *testing.T) {
	out := Highlight([]byte(`{"a":1}`), "application/json", true, false)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > 1 {
		t.Errorf("raw JSON should stay single-line, got %d lines", len(lines))
	}
}

func TestHTMLHighlighted(t *testing.T) {
	out := Highlight([]byte(`<html><body>Hello</body></html>`), "text/html", true, false)
	if !strings.Contains(out, "\x1b[") {
		t.Error("HTML output should contain ANSI escape codes")
	}
}

func TestUnknownContentTypePlaintext(t *testing.T) {
	out := Highlight([]byte("hello world"), "application/octet-stream", true, false)
	if out != "hello world" {
		t.Errorf("unknown content-type should return plain text, got %q", out)
	}
}

func TestEmptyBody(t *testing.T) {
	out := Highlight([]byte{}, "application/json", true, false)
	if out != "" {
		t.Errorf("empty body should return empty string, got %q", out)
	}
}

func TestContentTypeWithCharset(t *testing.T) {
	out := Highlight([]byte(`{"k":1}`), "application/json; charset=utf-8", true, false)
	if !strings.Contains(out, "\x1b[") {
		t.Error("should handle content-type with charset param")
	}
}

func TestDarkVsLightTheme(t *testing.T) {
	dark := Highlight([]byte(`{"k":1}`), "application/json", true, false)
	light := Highlight([]byte(`{"k":1}`), "application/json", false, false)
	if !strings.Contains(dark, "\x1b[") {
		t.Error("dark theme should produce ANSI")
	}
	if !strings.Contains(light, "\x1b[") {
		t.Error("light theme should produce ANSI")
	}
}

func TestInvalidJSONPrettyPrintFallback(t *testing.T) {
	out := Highlight([]byte(`{not json}`), "application/json", true, true)
	if !strings.Contains(out, "\x1b[") {
		t.Error("invalid JSON should still get highlighted")
	}
}
