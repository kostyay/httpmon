//go:build chafa

package tui

import (
	"fmt"
	"math"
	"os"

	chafa "github.com/ploMP4/chafa-go"
	_ "golang.org/x/image/bmp"  // register BMP decoder
	_ "golang.org/x/image/webp" // register WebP decoder
)

// renderImage converts raw image bytes into ANSI art for the terminal.
// width and height are character dimensions of the available viewport.
func renderImage(body []byte, width, height int) (string, error) {
	f, err := os.CreateTemp("", "httpmon-img-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)

	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	pixels, imgW, imgH, err := chafa.Load(tmpPath)
	if err != nil {
		return "", fmt.Errorf("chafa load: %w", err)
	}

	// Canvas cell is roughly 2:1 (tall:wide), so halve height for aspect ratio.
	canvasW := int32(min(width, math.MaxInt32))    // #nosec G115 -- clamped to MaxInt32
	canvasH := int32(min(height/2, math.MaxInt32)) // #nosec G115 -- clamped to MaxInt32
	if canvasW < 1 {
		canvasW = 1
	}
	if canvasH < 1 {
		canvasH = 1
	}

	// Let chafa compute optimal geometry preserving aspect ratio.
	chafa.CalcCanvasGeometry(imgW, imgH, &canvasW, &canvasH,
		0.5, // font ratio ~2:1
		false, false)

	symbolMap := chafa.SymbolMapNew()
	defer chafa.SymbolMapUnref(symbolMap)
	chafa.SymbolMapAddByTags(symbolMap, chafa.CHAFA_SYMBOL_TAG_BLOCK|chafa.CHAFA_SYMBOL_TAG_BORDER)

	config := chafa.CanvasConfigNew()
	defer chafa.CanvasConfigUnref(config)
	chafa.CanvasConfigSetGeometry(config, canvasW, canvasH)
	chafa.CanvasConfigSetSymbolMap(config, symbolMap)

	canvas := chafa.CanvasNew(config)
	defer chafa.CanvasUnRef(canvas)

	rowStride := imgW * 4 // RGBA = 4 bytes per pixel
	chafa.CanvasDrawAllPixels(canvas, chafa.CHAFA_PIXEL_RGBA8_UNASSOCIATED,
		pixels, imgW, imgH, rowStride)

	gs := chafa.CanvasPrint(canvas, nil)
	return gs.String(), nil
}
