package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type editorFinishedMsg struct{ err error }

// resolveEditor returns the user's preferred editor ($VISUAL, $EDITOR, or vi).
func resolveEditor() string {
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	return "vi"
}

// contentTypeToExt maps a MIME content type to a file extension.
func contentTypeToExt(ct string) string {
	if idx := strings.IndexByte(ct, ';'); idx >= 0 {
		ct = ct[:idx]
	}
	ct = strings.TrimSpace(ct)

	switch ct {
	case "application/json":
		return ".json"
	case "text/html":
		return ".html"
	case "application/xml", "text/xml":
		return ".xml"
	case "text/css":
		return ".css"
	case "application/javascript", "text/javascript":
		return ".js"
	default:
		return ".txt"
	}
}

// openInEditor writes body to a temp file and returns a tea.Cmd that
// launches the user's editor via tea.ExecProcess. Returns nil if body is empty.
func openInEditor(body []byte, contentType string) tea.Cmd {
	if len(body) == 0 {
		return nil
	}

	ext := contentTypeToExt(contentType)
	f, err := os.CreateTemp("", "httpmon-body-*"+ext)
	if err != nil {
		return func() tea.Msg {
			return editorFinishedMsg{err: fmt.Errorf("create temp file: %w", err)}
		}
	}

	if _, err := f.Write(body); err != nil {
		f.Close()
		return func() tea.Msg {
			return editorFinishedMsg{err: fmt.Errorf("write temp file: %w", err)}
		}
	}
	f.Close()

	editor := resolveEditor()
	c := exec.Command(editor, f.Name()) //nolint:gosec
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}
