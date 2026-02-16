package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kostyay/httpmon/internal/store"
)

func newDetailAppWithProcess() *App {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	m.metas = []store.FlowMeta{
		{
			ID: "p1", Method: "GET", Host: "api.example.com",
			Path: "/v1/users", StatusCode: 200,
			State: store.StateCompleted, StartedAt: time.Now(),
			Scheme: "https", Process: "curl",
		},
	}
	m.data["p1"] = &store.FlowData{
		RequestHeaders: map[string][]string{"Accept": {"application/json"}},
		RequestBody:    []byte(`{"q":"test"}`),
		ProcessPID:     12345,
		ProcessCmdline: "curl -X GET https://api.example.com/v1/users",
	}

	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))
	return app
}

func TestDetailShowsProcessSection(t *testing.T) {
	app := newDetailAppWithProcess()
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	view := stripView(app)
	if !strings.Contains(view, "Process") {
		t.Error("detail view should contain Process section")
	}
	if !strings.Contains(view, "curl") {
		t.Error("detail view should show process name 'curl'")
	}
	if !strings.Contains(view, "12345") {
		t.Error("detail view should show PID 12345")
	}
	if !strings.Contains(view, "curl -X GET") {
		t.Error("detail view should show cmdline")
	}
}

func TestDetailHidesProcessWhenNoPID(t *testing.T) {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	m.metas = []store.FlowMeta{
		{
			ID: "n1", Method: "GET", Host: "api.example.com",
			Path: "/v1/users", StatusCode: 200,
			State: store.StateCompleted, StartedAt: time.Now(),
			Scheme: "https",
		},
	}
	m.data["n1"] = &store.FlowData{
		RequestHeaders: map[string][]string{"Accept": {"application/json"}},
	}

	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	view := stripView(app)
	if strings.Contains(view, "PID:") {
		t.Error("detail view should NOT show Process section when PID is 0")
	}
}

func TestDetailProcessCmdlineTruncation(t *testing.T) {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	m.metas = []store.FlowMeta{
		{
			ID: "t1", Method: "GET", Host: "api.example.com",
			Path: "/long", StatusCode: 200,
			State: store.StateCompleted, StartedAt: time.Now(),
			Scheme: "https", Process: "java",
		},
	}
	longCmd := strings.Repeat("x", 300)
	m.data["t1"] = &store.FlowData{
		RequestHeaders: map[string][]string{},
		ProcessPID:     999,
		ProcessCmdline: longCmd,
	}

	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	view := stripView(app)
	if !strings.Contains(view, "...") {
		t.Error("long cmdline should be truncated with ...")
	}
	if strings.Contains(view, longCmd) {
		t.Error("full 300-char cmdline should NOT appear in view")
	}
}
