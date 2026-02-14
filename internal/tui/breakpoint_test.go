package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kostyay/httpmon/internal/breakpoint"
	"github.com/kostyay/httpmon/internal/store"
)

func newBPApp() (*App, *breakpoint.BreakpointHit) {
	ctrl := breakpoint.NewController()
	m := seedMock(3)
	app := NewApp(AppConfig{
		Store:       m,
		Proxy:       &mockProxyInfo{addr: ":9999"},
		CATrusted:   true,
		Breakpoints: ctrl,
	})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))

	hit := breakpoint.BreakpointHit{
		FlowID:  "bp-flow-1",
		Phase:   breakpoint.PhaseRequest,
		Headers: map[string]string{"Content-Type": "application/json", "Accept": "text/html"},
		Body:    []byte(`{"test":"data"}`),
		Meta:    store.FlowMeta{ID: "bp-flow-1", Method: "POST", Host: "api.example.com", Path: "/v1/submit"},
	}

	go ctrl.Pause(hit) //nolint:errcheck
	time.Sleep(50 * time.Millisecond)
	app.Update(TickMsg(time.Now()))

	return app, &hit
}

func TestBreakpointQueueOpensWithB(t *testing.T) {
	app, _ := newBPApp()

	if app.showBreakpointQueue {
		t.Fatal("queue should not be open initially")
	}

	sendKey(app, "B")

	if !app.showBreakpointQueue {
		t.Fatal("B should open breakpoint queue")
	}

	view := ansi.Strip(app.viewContent())
	if !strings.Contains(view, "Breakpoint Queue") {
		t.Error("expected 'Breakpoint Queue' in view")
	}
	if !strings.Contains(view, "api.example.com") {
		t.Error("expected host in queue view")
	}
}

func TestBreakpointQueueEscCloses(t *testing.T) {
	app, _ := newBPApp()
	sendKey(app, "B")

	if !app.showBreakpointQueue {
		t.Fatal("queue should be open")
	}

	sendKey(app, "esc")

	if app.showBreakpointQueue {
		t.Fatal("Esc should close breakpoint queue")
	}
}

func TestBreakpointEditorOpensOnEnter(t *testing.T) {
	app, _ := newBPApp()
	sendKey(app, "B")
	sendKey(app, "enter")

	if app.editingBreakpoint == nil {
		t.Fatal("Enter should open editor")
	}

	view := ansi.Strip(app.viewContent())
	if !strings.Contains(view, "Headers") {
		t.Error("expected 'Headers' label in editor")
	}
	if !strings.Contains(view, "Body") {
		t.Error("expected 'Body' label in editor")
	}
}

func TestBreakpointEditorTabSwitchesPanes(t *testing.T) {
	app, _ := newBPApp()
	sendKey(app, "B")
	sendKey(app, "enter")

	if app.bpFocusedPane != bpPaneHeaders {
		t.Errorf("initial pane = %d, want headers", app.bpFocusedPane)
	}

	sendKey(app, "tab")
	if app.bpFocusedPane != bpPaneBody {
		t.Errorf("after Tab: pane = %d, want body", app.bpFocusedPane)
	}

	sendKey(app, "tab")
	if app.bpFocusedPane != bpPaneHeaders {
		t.Errorf("after second Tab: pane = %d, want headers", app.bpFocusedPane)
	}
}

func TestBreakpointEditorEscSkips(t *testing.T) {
	app, _ := newBPApp()
	sendKey(app, "B")
	sendKey(app, "enter")

	sendKey(app, "esc")

	if app.editingBreakpoint != nil {
		t.Fatal("Esc should clear editor")
	}
	if app.showBreakpointQueue {
		t.Fatal("queue should close when no pending hits")
	}
}

func TestBreakpointEditorCtrlSResumes(t *testing.T) {
	app, _ := newBPApp()
	sendKey(app, "B")
	sendKey(app, "enter")

	app.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	if app.editingBreakpoint != nil {
		t.Fatal("Ctrl+S should clear editor")
	}
}

func TestBreakpointEditorInitData(t *testing.T) {
	app, hit := newBPApp()
	sendKey(app, "B")
	sendKey(app, "enter")

	headerVal := app.bpHeadersTA.Value()
	if !strings.Contains(headerVal, "Content-Type") {
		t.Error("editor should contain Content-Type header")
	}
	if !strings.Contains(headerVal, "application/json") {
		t.Error("editor should contain header value")
	}

	bodyVal := app.bpBodyTA.Value()
	if bodyVal != string(hit.Body) {
		t.Errorf("body = %q, want %q", bodyVal, string(hit.Body))
	}
}

func TestBreakpointHitCountUpdates(t *testing.T) {
	app, _ := newBPApp()

	if app.breakpointHitCount != 1 {
		t.Errorf("hit count = %d, want 1", app.breakpointHitCount)
	}
}

func TestBreakpointHeaderParsing(t *testing.T) {
	input := "Content-Type: application/json\nAccept: text/html\nX-Custom: value"
	headers := parseHeadersFromEditor(input)

	if headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q", headers["Content-Type"])
	}
	if headers["Accept"] != "text/html" {
		t.Errorf("Accept = %q", headers["Accept"])
	}
	if headers["X-Custom"] != "value" {
		t.Errorf("X-Custom = %q", headers["X-Custom"])
	}
}

func TestBreakpointStatusBarShowsPending(t *testing.T) {
	app, _ := newBPApp()

	status := ansi.Strip(app.statusText())
	if !strings.Contains(status, "⏸ 1") {
		t.Errorf("status bar should show pending count, got: %s", status)
	}
}

func TestBreakpointMenuItemPresent(t *testing.T) {
	app, _ := newBPApp()
	items := app.listMenuItems()

	found := false
	for _, item := range items {
		if item.key == "B" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Breakpoints (B) in menu items")
	}
}

func TestBreakpointHeaderFormatting(t *testing.T) {
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "text/html",
	}
	formatted := formatHeadersForEditor(headers)

	if !strings.Contains(formatted, "Accept: text/html") {
		t.Error("expected Accept header in formatted output")
	}
	if !strings.Contains(formatted, "Content-Type: application/json") {
		t.Error("expected Content-Type header in formatted output")
	}
}
