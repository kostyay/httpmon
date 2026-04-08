package tui

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"strings"

	"github.com/blacktop/go-termimg"
	_ "golang.org/x/image/bmp"  // register BMP decoder
	_ "golang.org/x/image/webp" // register WebP decoder
)

var renderableImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
	"image/bmp":  true,
}

func isRenderableImage(contentType string) bool {
	ct := contentType
	if idx := strings.IndexByte(ct, ';'); idx >= 0 {
		ct = ct[:idx]
	}
	return renderableImageTypes[strings.TrimSpace(ct)]
}

// renderImage converts raw image bytes into ANSI art for the terminal.
// width and height are character dimensions of the available viewport.
func renderImage(body []byte, width, height int) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}

	ti := termimg.New(img).
		Width(width).Height(height).
		Protocol(termimg.Halfblocks)
	out, err := ti.Render()
	if err != nil {
		return "", fmt.Errorf("render image: %w", err)
	}
	return out, nil
}
