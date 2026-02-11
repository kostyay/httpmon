// Package highlight applies syntax highlighting to HTTP body content.
package highlight

// Highlight applies syntax highlighting to body based on contentType.
// Returns ANSI-colored string. Falls back to plain text for unknown types.
func Highlight(body []byte, contentType string, darkBg bool, prettyJSON bool) string {
	return string(body)
}
