package tui

import (
	"fmt"
	"testing"
	"time"
)

func TestShortContentType(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"application/json", "json"},
		{"application/xml", "xml"},
		{"text/xml", "xml"},
		{"text/html", "html"},
		{"text/plain", "text"},
		{"text/css", "css"},
		{"application/javascript", "js"},
		{"text/javascript", "js"},
		{"application/octet-stream", "binary"},
		{"multipart/form-data", "multipart"},
		{"application/x-www-form-urlencoded", "form"},
		{"application/grpc", "grpc"},
		{"application/pdf", "pdf"},
		{"application/protobuf", "protobuf"},
		{"application/x-protobuf", "protobuf"},
		{"image/png", "png"},
		{"font/woff2", "woff2"},
		{"application/json; charset=utf-8", "json"},
		{"text/html; charset=iso-8859-1", "html"},
		{"video/mp4", "video/mp4"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := shortContentType(tt.in)
			if got != tt.want {
				t.Errorf("shortContentType(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTruncPad(t *testing.T) {
	tests := []struct {
		s    string
		w    int
		want string
	}{
		{"", 0, ""},
		{"", 5, "     "},
		{"ab", 5, "ab   "},
		{"abcde", 5, "abcde"},
		{"abcdef", 5, "abcd\u2026"},
		{"a", 1, "a"},
		{"ab", 1, "a"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_w%d", tt.s, tt.w), func(t *testing.T) {
			got := truncPad(tt.s, tt.w)
			if got != tt.want {
				t.Errorf("truncPad(%q, %d) = %q, want %q", tt.s, tt.w, got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		b    int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1048576, "1.0M"},
		{2621440, "2.5M"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatSize(tt.b)
			if got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.b, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Microsecond, "500\u00b5s"},
		{45 * time.Millisecond, "45ms"},
		{1500 * time.Millisecond, "1.5s"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		w    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := truncate(tt.s, tt.w)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.w, got, tt.want)
			}
		})
	}
}
