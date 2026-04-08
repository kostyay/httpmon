package tui

import "strings"

// renderableImageTypes lists MIME types that chafa can render.
var renderableImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
	"image/bmp":  true,
}

// isRenderableImage reports whether the content type is an image chafa can display.
func isRenderableImage(contentType string) bool {
	ct := contentType
	if idx := strings.IndexByte(ct, ';'); idx >= 0 {
		ct = ct[:idx]
	}
	return renderableImageTypes[strings.TrimSpace(ct)]
}
