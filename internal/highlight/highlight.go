// Package highlight applies syntax highlighting to HTTP body content.
package highlight

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Highlight applies syntax highlighting to body based on contentType.
// Returns ANSI-colored string. Falls back to plain text for unknown types.
func Highlight(body []byte, contentType string, darkBg bool, prettyJSON bool) string {
	if len(body) == 0 {
		return ""
	}

	if IsBinary(body, contentType) {
		ct := contentType
		if ct == "" {
			ct = "unknown"
		}
		return fmt.Sprintf("[binary content: %s, %s]", ct, formatBinarySize(len(body)))
	}

	mediaType, _, _ := mime.ParseMediaType(contentType)

	lexerName := lexerForMIME(mediaType)
	if lexerName == "" {
		return string(body)
	}
	lexer := lexers.Get(lexerName)
	if lexer == nil {
		return string(body)
	}

	if prettyJSON && isJSON(mediaType) {
		if pretty, err := prettyPrintJSON(body); err == nil {
			body = pretty
		}
	}

	styleName := "monokai"
	if !darkBg {
		styleName = "github"
	}

	iter, err := lexer.Tokenise(nil, string(body))
	if err != nil {
		return string(body)
	}

	var buf bytes.Buffer
	if err := formatters.Get("terminal256").Format(&buf, styles.Get(styleName), iter); err != nil {
		return string(body)
	}

	return strings.TrimRight(buf.String(), "\n")
}

func isJSON(mediaType string) bool {
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func prettyPrintJSON(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var mimeToLexer = map[string]string{
	"application/json":                  "json",
	"application/ld+json":               "json-ld",
	"application/xml":                   "xml",
	"application/xhtml+xml":             "html",
	"application/javascript":            "javascript",
	"application/x-javascript":          "javascript",
	"application/graphql":               "graphql",
	"application/soap+xml":              "xml",
	"application/atom+xml":              "xml",
	"application/rss+xml":               "xml",
	"application/x-yaml":                "yaml",
	"application/yaml":                  "yaml",
	"application/toml":                  "toml",
	"application/x-www-form-urlencoded": "http",
	"text/html":                         "html",
	"text/xml":                          "xml",
	"text/css":                          "css",
	"text/javascript":                   "javascript",
	"text/csv":                          "csv",
	"text/yaml":                         "yaml",
}

// binaryMIMEPrefixes are MIME type prefixes that are always binary.
var binaryMIMEPrefixes = []string{"image/", "audio/", "video/", "font/"}

// binaryMIMETypes are specific MIME types that are always binary.
var binaryMIMETypes = map[string]bool{
	"application/octet-stream":      true,
	"application/zip":               true,
	"application/gzip":              true,
	"application/x-gzip":            true,
	"application/x-tar":             true,
	"application/x-bzip2":           true,
	"application/x-7z-compressed":   true,
	"application/x-rar-compressed":  true,
	"application/pdf":               true,
	"application/wasm":              true,
	"application/x-shockwave-flash": true,
	"application/x-protobuf":        true,
	"application/protobuf":          true,
	"application/x-google-protobuf": true,
	"application/grpc":              true,
	"application/grpc+proto":        true,
	"application/vnd.ms-fontobject": true,
	"application/x-font-ttf":        true,
	"application/x-font-opentype":   true,
	"application/x-executable":      true,
	"application/x-mach-binary":     true,
	"application/vnd.apple.pkpass":  true,
	"application/vnd.ms-excel":      true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
}

// magicHeaders maps file magic bytes to format names.
var magicHeaders = []struct {
	magic []byte
	name  string
}{
	{[]byte{0xFF, 0xD8, 0xFF}, "JPEG"},
	{[]byte{0x89, 0x50, 0x4E, 0x47}, "PNG"},
	{[]byte("GIF87a"), "GIF"},
	{[]byte("GIF89a"), "GIF"},
	{[]byte{0x52, 0x49, 0x46, 0x46}, "RIFF (WebP/AVI)"},
	{[]byte{0x50, 0x4B, 0x03, 0x04}, "ZIP"},
	{[]byte{0x1F, 0x8B}, "gzip"},
	{[]byte("%PDF"), "PDF"},
	{[]byte{0x7F, 0x45, 0x4C, 0x46}, "ELF"},
	{[]byte{0x00, 0x61, 0x73, 0x6D}, "WebAssembly"},
}

// IsBinary reports whether the content is binary based on content-type and body inspection.
func IsBinary(body []byte, contentType string) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)

	// Check MIME prefixes.
	for _, prefix := range binaryMIMEPrefixes {
		if strings.HasPrefix(mediaType, prefix) {
			return true
		}
	}

	// Check explicit binary MIME types.
	if binaryMIMETypes[mediaType] {
		return true
	}

	// Inspect body bytes.
	if len(body) > 0 {
		// Check magic bytes.
		for _, m := range magicHeaders {
			if bytes.HasPrefix(body, m.magic) {
				return true
			}
		}
		// NUL byte check (strong binary indicator).
		if bytes.ContainsRune(body, 0) {
			return true
		}
		// Invalid UTF-8 check.
		if !utf8.Valid(body) {
			return true
		}
	}

	return false
}

// formatBinarySize returns a human-readable size string.
func formatBinarySize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func lexerForMIME(mediaType string) string {
	if name, ok := mimeToLexer[mediaType]; ok {
		return name
	}
	// Handle +json / +xml suffixes (e.g. application/vnd.api+json)
	if strings.HasSuffix(mediaType, "+json") {
		return "json"
	}
	if strings.HasSuffix(mediaType, "+xml") {
		return "xml"
	}
	return ""
}
