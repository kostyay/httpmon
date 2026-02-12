// Package highlight applies syntax highlighting to HTTP body content.
package highlight

import (
	"bytes"
	"encoding/json"
	"mime"
	"strings"

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
	"application/json":                "json",
	"application/ld+json":             "json-ld",
	"application/xml":                 "xml",
	"application/xhtml+xml":           "html",
	"application/javascript":          "javascript",
	"application/x-javascript":        "javascript",
	"application/graphql":             "graphql",
	"application/soap+xml":            "xml",
	"application/atom+xml":            "xml",
	"application/rss+xml":             "xml",
	"application/x-yaml":              "yaml",
	"application/yaml":                "yaml",
	"application/toml":                "toml",
	"application/x-www-form-urlencoded": "http",
	"text/html":                       "html",
	"text/xml":                        "xml",
	"text/css":                        "css",
	"text/javascript":                 "javascript",
	"text/csv":                        "csv",
	"text/yaml":                       "yaml",
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
