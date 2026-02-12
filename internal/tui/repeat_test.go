package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRepeatKeyInDetail(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyMsg{Type: tea.KeyEnter}) // enter detail

	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	// cmd should be non-nil (fire-and-forget goroutine).
	if cmd == nil {
		t.Error("r in detail should return a non-nil cmd")
	}
	if !app.showDetail {
		t.Error("r should not exit detail view")
	}
}

func TestRepeatPreservesMethod(t *testing.T) {
	m := seedMock(1)
	m.metas[0].Method = "POST"
	app := NewApp(m, &mockProxyInfo{addr: ":9999"}, true, nil)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg{})
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	req := app.buildRepeatRequest()
	if req == nil {
		t.Fatal("buildRepeatRequest returned nil")
	}
	if req.Method != "POST" {
		t.Errorf("method = %q, want POST", req.Method)
	}
}

func TestRepeatPreservesURL(t *testing.T) {
	app := newMockApp(1)
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	req := app.buildRepeatRequest()
	if req == nil {
		t.Fatal("buildRepeatRequest returned nil")
	}
	if req.URL.Host != "api.example.com" {
		t.Errorf("host = %q, want api.example.com", req.URL.Host)
	}
}

func TestRepeatPreservesHeaders(t *testing.T) {
	app := newMockApp(1)
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	req := app.buildRepeatRequest()
	if req == nil {
		t.Fatal("buildRepeatRequest returned nil")
	}
	if req.Header.Get("Accept") != "application/json" {
		t.Errorf("Accept header = %q", req.Header.Get("Accept"))
	}
}
