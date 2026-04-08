//go:build chafa

package tui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

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
