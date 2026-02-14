package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kostyay/httpmon/internal/maplocal"
)

type mockMapLocalManager struct {
	rules      []maplocal.Rule
	removedIdx []int
	savedPath  string
}

func (m *mockMapLocalManager) Rules() []maplocal.Rule {
	out := make([]maplocal.Rule, len(m.rules))
	copy(out, m.rules)
	return out
}

func (m *mockMapLocalManager) AddRule(r maplocal.Rule) {
	m.rules = append(m.rules, r)
}

func (m *mockMapLocalManager) RemoveRule(index int) {
	if index >= 0 && index < len(m.rules) {
		m.removedIdx = append(m.removedIdx, index)
		m.rules = append(m.rules[:index], m.rules[index+1:]...)
	}
}

func (m *mockMapLocalManager) SaveToFile(path string) error {
	m.savedPath = path
	return nil
}

func testMapLocalManager() *mockMapLocalManager {
	return &mockMapLocalManager{
		rules: []maplocal.Rule{
			{Pattern: "api.example.com/v1/*", LocalPath: "/tmp/mock.json", StatusCode: 200},
			{Pattern: "*.cdn.com/img/*", LocalPath: "/tmp/placeholder.png", StatusCode: 200},
		},
	}
}

func newAppWithMapLocal(mlm MapLocalManager) *App {
	m := seedMock(3)
	app := NewApp(AppConfig{
		Store:        m,
		Proxy:        &mockProxyInfo{addr: ":9999"},
		CATrusted:    true,
		MapLocal:     mlm,
		MapLocalFile: "/tmp/rules.json",
	})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))
	return app
}

func TestMapLocalOpenClose(t *testing.T) {
	mlm := testMapLocalManager()
	app := newAppWithMapLocal(mlm)

	sendKey(app, "M")
	if !app.showMapLocal {
		t.Fatal("M should open maplocal modal")
	}

	app.updateMapLocal(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.showMapLocal {
		t.Error("Esc should close maplocal modal")
	}
}

func TestMapLocalMKeyCloses(t *testing.T) {
	mlm := testMapLocalManager()
	app := newAppWithMapLocal(mlm)

	sendKey(app, "M")
	app.updateMapLocal(tea.KeyPressMsg{Code: 'M', Text: "M"})
	if app.showMapLocal {
		t.Error("M should close maplocal modal")
	}
}

func TestMapLocalNavigate(t *testing.T) {
	mlm := testMapLocalManager()
	app := newAppWithMapLocal(mlm)

	sendKey(app, "M")

	app.updateMapLocal(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if app.mapLocalCursor != 1 {
		t.Errorf("after j: cursor = %d, want 1", app.mapLocalCursor)
	}

	// Clamp at end.
	app.updateMapLocal(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if app.mapLocalCursor != 1 {
		t.Errorf("clamp at end: cursor = %d, want 1", app.mapLocalCursor)
	}

	app.updateMapLocal(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if app.mapLocalCursor != 0 {
		t.Errorf("after k: cursor = %d, want 0", app.mapLocalCursor)
	}
}

func TestMapLocalDelete(t *testing.T) {
	mlm := testMapLocalManager()
	app := newAppWithMapLocal(mlm)

	sendKey(app, "M")

	app.updateMapLocal(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if !app.mapLocalConfirmDelete {
		t.Fatal("d should set confirmDelete")
	}

	app.updateMapLocal(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if app.mapLocalConfirmDelete {
		t.Error("y should clear confirmDelete")
	}
	if len(mlm.removedIdx) != 1 {
		t.Fatalf("removed count = %d, want 1", len(mlm.removedIdx))
	}
	if mlm.removedIdx[0] != 0 {
		t.Errorf("removed index = %d, want 0", mlm.removedIdx[0])
	}
}

func TestMapLocalDeleteCancel(t *testing.T) {
	mlm := testMapLocalManager()
	app := newAppWithMapLocal(mlm)

	sendKey(app, "M")

	app.updateMapLocal(tea.KeyPressMsg{Code: 'd', Text: "d"})
	app.updateMapLocal(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if app.mapLocalConfirmDelete {
		t.Error("n should cancel delete")
	}
	if len(mlm.removedIdx) != 0 {
		t.Error("nothing should be removed after cancel")
	}
}

func TestMapLocalAddRule(t *testing.T) {
	mlm := testMapLocalManager()
	app := newAppWithMapLocal(mlm)
	initial := len(mlm.rules)

	sendKey(app, "M")

	app.updateMapLocal(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !app.mapLocalAdding {
		t.Fatal("n should start add mode")
	}

	// Type pattern.
	for _, ch := range "test.com/*" {
		app.updateMapLocalAdd(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}

	// Tab to path input.
	app.updateMapLocalAdd(tea.KeyPressMsg{Code: tea.KeyTab})
	if app.mapLocalAddFocus != 1 {
		t.Errorf("Tab should switch focus to path, got %d", app.mapLocalAddFocus)
	}

	// Type path.
	for _, ch := range "/tmp/test.json" {
		app.updateMapLocalAdd(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}

	// Submit.
	app.updateMapLocalAdd(tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.mapLocalAdding {
		t.Error("Enter should end add mode")
	}
	if len(mlm.rules) != initial+1 {
		t.Errorf("rules count = %d, want %d", len(mlm.rules), initial+1)
	}
}

func TestMapLocalAddCancel(t *testing.T) {
	mlm := testMapLocalManager()
	app := newAppWithMapLocal(mlm)
	initial := len(mlm.rules)

	sendKey(app, "M")

	app.updateMapLocal(tea.KeyPressMsg{Code: 'n', Text: "n"})
	app.updateMapLocalAdd(tea.KeyPressMsg{Code: tea.KeyEscape})

	if app.mapLocalAdding {
		t.Error("Esc should cancel add")
	}
	if len(mlm.rules) != initial {
		t.Error("cancel should not add rule")
	}
}

func TestMapLocalEmptyView(t *testing.T) {
	mlm := &mockMapLocalManager{}
	app := newAppWithMapLocal(mlm)

	sendKey(app, "M")
	view := ansi.Strip(app.viewMapLocal())

	if !strings.Contains(view, "No rules configured") {
		t.Error("empty rules should show 'No rules configured'")
	}
}

func TestMapLocalPopulatedView(t *testing.T) {
	mlm := testMapLocalManager()
	app := newAppWithMapLocal(mlm)

	sendKey(app, "M")
	view := ansi.Strip(app.viewMapLocal())

	if !strings.Contains(view, "api.example.com") {
		t.Error("view should contain rule pattern")
	}
	if !strings.Contains(view, "mock.json") {
		t.Error("view should contain local path")
	}
}

func TestMapLocalAutoSave(t *testing.T) {
	mlm := testMapLocalManager()
	app := newAppWithMapLocal(mlm)

	sendKey(app, "M")

	// Delete a rule — should trigger auto-save.
	app.updateMapLocal(tea.KeyPressMsg{Code: 'd', Text: "d"})
	app.updateMapLocal(tea.KeyPressMsg{Code: 'y', Text: "y"})

	if mlm.savedPath != "/tmp/rules.json" {
		t.Errorf("savedPath = %q, want /tmp/rules.json", mlm.savedPath)
	}
}
