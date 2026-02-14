package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestRepeatKeyInDetail(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // enter detail

	_, cmd := app.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
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
	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg{})
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	req := app.buildRepeatRequest()
	if req == nil {
		t.Fatal("buildRepeatRequest returned nil")
	}
	if req.Header.Get("Accept") != "application/json" {
		t.Errorf("Accept header = %q", req.Header.Get("Accept"))
	}
}
