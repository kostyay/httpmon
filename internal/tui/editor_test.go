package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kostyay/httpmon/internal/store"
)

func TestContentTypeToExt(t *testing.T) {
	tests := []struct {
		ct   string
		want string
	}{
		{"application/json", ".json"},
		{"application/json; charset=utf-8", ".json"},
		{"text/html", ".html"},
		{"text/xml", ".xml"},
		{"application/xml", ".xml"},
		{"text/plain", ".txt"},
		{"text/css", ".css"},
		{"application/javascript", ".js"},
		{"", ".txt"},
		{"application/octet-stream", ".txt"},
	}
	for _, tt := range tests {
		t.Run(tt.ct, func(t *testing.T) {
			got := contentTypeToExt(tt.ct)
			if got != tt.want {
				t.Errorf("contentTypeToExt(%q) = %q, want %q", tt.ct, got, tt.want)
			}
		})
	}
}

func TestResolveEditor(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	if got := resolveEditor(); got != "vi" {
		t.Errorf("resolveEditor() = %q, want %q", got, "vi")
	}

	t.Setenv("EDITOR", "nano")
	if got := resolveEditor(); got != "nano" {
		t.Errorf("resolveEditor() = %q, want %q (EDITOR set)", got, "nano")
	}

	t.Setenv("VISUAL", "code")
	if got := resolveEditor(); got != "code" {
		t.Errorf("resolveEditor() = %q, want %q (VISUAL set)", got, "code")
	}
}

func TestOpenInEditorEmptyBody(t *testing.T) {
	if cmd := openInEditor(nil, "application/json"); cmd != nil {
		t.Error("nil body should return nil cmd")
	}
	if cmd := openInEditor([]byte{}, "text/plain"); cmd != nil {
		t.Error("empty body should return nil cmd")
	}
}

func TestOpenInEditorNonEmpty(t *testing.T) {
	cmd := openInEditor([]byte(`{"hello":"world"}`), "application/json")
	if cmd == nil {
		t.Error("non-empty body should return non-nil cmd")
	}
}

func TestEditKeyInDetail(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // enter detail
	app.detailTab = 1                          // response tab

	if !app.showDetail {
		t.Fatal("expected showDetail=true")
	}

	_, cmd := app.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd == nil {
		t.Error("pressing e on response tab with body should return non-nil cmd")
	}
}

func TestEditKeyEmptyBody(t *testing.T) {
	m := &mockFlowReader{
		data: map[store.FlowID]*store.FlowData{
			"1": {},
		},
		metas: []store.FlowMeta{
			{ID: "1", Method: "GET", Host: "example.com", Path: "/", StatusCode: 200, State: store.StateCompleted, Scheme: "https"},
		},
	}

	app := NewApp(AppConfig{Store: m, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app.Update(TickMsg(time.Now()))
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	_, cmd := app.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd != nil {
		t.Error("pressing e with empty body should return nil cmd")
	}
}
