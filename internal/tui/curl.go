package tui

import (
	"encoding/base64"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
)

// formatCurl builds a cURL command from request components.
func formatCurl(method, url string, headers http.Header, body []byte) string {
	var parts []string
	parts = append(parts, "curl")

	if method != "GET" {
		parts = append(parts, "-X", method)
	}

	// Sort headers for deterministic output.
	keys := slices.Sorted(maps.Keys(headers))

	for _, k := range keys {
		for _, v := range headers[k] {
			parts = append(parts, "-H", fmt.Sprintf("'%s: %s'", k, v))
		}
	}

	if len(body) > 0 {
		parts = append(parts, "-d", fmt.Sprintf("'%s'", string(body)))
	}

	parts = append(parts, fmt.Sprintf("'%s'", url))

	return strings.Join(parts, " ")
}

// osc52Sequence returns an OSC 52 escape sequence to set the clipboard.
func osc52Sequence(text string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	return fmt.Sprintf("\033]52;c;%s\a", encoded)
}
