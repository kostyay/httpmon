package tui

import "testing"

func TestIsRenderableImage(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"image/gif", true},
		{"image/webp", true},
		{"image/bmp", true},
		{"image/png; charset=utf-8", true},
		{"image/svg+xml", false},
		{"application/json", false},
		{"text/html", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isRenderableImage(tt.ct); got != tt.want {
			t.Errorf("isRenderableImage(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

func TestRenderImageFallback(t *testing.T) {
	_, err := renderImage([]byte{0xFF}, 40, 20)
	if err == nil {
		t.Skip("chafa support compiled in, skipping fallback test")
	}
}
