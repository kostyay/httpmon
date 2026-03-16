package tui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

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

func TestRenderImage(t *testing.T) {
	// Create a small 4x4 red PNG.
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	red := color.RGBA{R: 255, A: 255}
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, red)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	out, err := renderImage(buf.Bytes(), 40, 20)
	if err != nil {
		t.Fatalf("renderImage: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("renderImage returned empty string")
	}
}
