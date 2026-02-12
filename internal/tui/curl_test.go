package tui

import (
	"net/http"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFormatCurlGET(t *testing.T) {
	cmd := formatCurl("GET", "https://api.example.com/v1/users", http.Header{
		"Accept": {"application/json"},
	}, nil)

	if !strings.Contains(cmd, "curl") {
		t.Error("should start with curl")
	}
	if !strings.Contains(cmd, "'https://api.example.com/v1/users'") {
		t.Error("should contain quoted URL")
	}
	if strings.Contains(cmd, "-X GET") {
		t.Error("GET is default; should not include -X GET")
	}
	if !strings.Contains(cmd, "-H 'Accept: application/json'") {
		t.Error("should include Accept header")
	}
}

func TestFormatCurlPOST(t *testing.T) {
	body := []byte(`{"name":"test"}`)
	cmd := formatCurl("POST", "https://api.example.com/v1/users", http.Header{
		"Content-Type": {"application/json"},
	}, body)

	if !strings.Contains(cmd, "-X POST") {
		t.Error("should include -X POST")
	}
	if !strings.Contains(cmd, `-d '{"name":"test"}'`) {
		t.Error("should include body with -d")
	}
}

func TestFormatCurlMultipleHeaders(t *testing.T) {
	cmd := formatCurl("GET", "https://x.com/", http.Header{
		"Accept":        {"application/json"},
		"Authorization": {"Bearer token123"},
	}, nil)

	if !strings.Contains(cmd, "-H 'Accept: application/json'") {
		t.Error("should include Accept header")
	}
	if !strings.Contains(cmd, "-H 'Authorization: Bearer token123'") {
		t.Error("should include Authorization header")
	}
}

func TestFormatCurlEmptyBody(t *testing.T) {
	cmd := formatCurl("DELETE", "https://api.example.com/v1/users/1", nil, nil)
	if !strings.Contains(cmd, "-X DELETE") {
		t.Error("should include -X DELETE")
	}
	if strings.Contains(cmd, "-d") {
		t.Error("no body means no -d flag")
	}
}

func TestOSC52Write(t *testing.T) {
	seq := osc52Sequence("hello world")
	if !strings.HasPrefix(seq, "\033]52;c;") {
		t.Error("OSC 52 should start with \\033]52;c;")
	}
	if !strings.HasSuffix(seq, "\a") {
		t.Error("OSC 52 should end with BEL")
	}
	// Base64 of "hello world" = "aGVsbG8gd29ybGQ="
	if !strings.Contains(seq, "aGVsbG8gd29ybGQ=") {
		t.Error("should contain base64 encoded content")
	}
}

func TestCurlKeyInDetail(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyMsg{Type: tea.KeyEnter}) // enter detail
	if !app.showDetail {
		t.Fatal("should be in detail view")
	}

	sendKey(app, "c")
	// After pressing c, we expect a status flash message.
	// The copy is fire-and-forget; we verify state didn't break.
	if !app.showDetail {
		t.Error("c should not exit detail view")
	}
}
