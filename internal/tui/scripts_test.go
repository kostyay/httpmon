package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kostyay/httpmon/internal/scripting"
)

type mockScriptManager struct {
	scripts    []scripting.ScriptInfo
	toggled    []string
	deleted    []string
	reloadCnt  int
	createPath string
	createErr  error
	dir        string
}

func (m *mockScriptManager) Scripts() []scripting.ScriptInfo { return m.scripts }
func (m *mockScriptManager) Toggle(fp string) error         { m.toggled = append(m.toggled, fp); return nil }
func (m *mockScriptManager) Delete(fp string) error         { m.deleted = append(m.deleted, fp); return nil }
func (m *mockScriptManager) CreateNew() (string, error)     { return m.createPath, m.createErr }
func (m *mockScriptManager) ScriptDir() string              { return m.dir }
func (m *mockScriptManager) Reload() {
	m.reloadCnt++
}

func newAppWithScripts(n int, sm ScriptManager) *App {
	m := seedMock(n)
	app := NewApp(m, &mockProxyInfo{addr: ":9999"}, true, sm)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg(time.Now()))
	return app
}

func testScriptManager() *mockScriptManager {
	return &mockScriptManager{
		scripts: []scripting.ScriptInfo{
			{Name: "block-ads", FilePath: "/scripts/block-ads.js", Enabled: true, Matches: []string{"*://ads.*"}},
			{Name: "add-header", FilePath: "/scripts/add-header.js", Enabled: false, Matches: []string{"*://api.*"}},
			{Name: "log-all", FilePath: "/scripts/log-all.js", Enabled: true},
		},
		dir: "/scripts",
	}
}

func TestInitScripts(t *testing.T) {
	sm := testScriptManager()
	app := newAppWithScripts(3, sm)

	sendKey(app, "S")
	if !app.showScripts {
		t.Fatal("S should open scripts modal")
	}
	if app.scriptsCursor != 0 {
		t.Errorf("cursor = %d, want 0", app.scriptsCursor)
	}
	if sm.reloadCnt != 1 {
		t.Errorf("Reload called %d times, want 1", sm.reloadCnt)
	}
}

func TestScriptsNavigation(t *testing.T) {
	sm := testScriptManager()
	app := newAppWithScripts(3, sm)
	sendKey(app, "S")

	app.updateScripts(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if app.scriptsCursor != 1 {
		t.Errorf("after j: cursor = %d, want 1", app.scriptsCursor)
	}

	app.updateScripts(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if app.scriptsCursor != 2 {
		t.Errorf("after 2j: cursor = %d, want 2", app.scriptsCursor)
	}

	// Clamp at end.
	app.updateScripts(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if app.scriptsCursor != 2 {
		t.Errorf("after 3j: cursor = %d, want 2 (clamped)", app.scriptsCursor)
	}

	app.updateScripts(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if app.scriptsCursor != 1 {
		t.Errorf("after k: cursor = %d, want 1", app.scriptsCursor)
	}
}

func TestScriptsToggle(t *testing.T) {
	sm := testScriptManager()
	app := newAppWithScripts(3, sm)
	sendKey(app, "S")

	app.updateScripts(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if len(sm.toggled) != 1 {
		t.Fatalf("toggled count = %d, want 1", len(sm.toggled))
	}
	if sm.toggled[0] != "/scripts/block-ads.js" {
		t.Errorf("toggled = %q, want block-ads.js path", sm.toggled[0])
	}
}

func TestScriptsDelete(t *testing.T) {
	sm := testScriptManager()
	app := newAppWithScripts(3, sm)
	sendKey(app, "S")

	// Press d → confirm prompt.
	app.updateScripts(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !app.scriptsConfirmDelete {
		t.Fatal("d should set confirmDelete")
	}

	// Press y → delete.
	app.updateScripts(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if app.scriptsConfirmDelete {
		t.Error("confirmDelete should be cleared after y")
	}
	if len(sm.deleted) != 1 {
		t.Fatalf("deleted count = %d, want 1", len(sm.deleted))
	}
	if sm.deleted[0] != "/scripts/block-ads.js" {
		t.Errorf("deleted = %q, want block-ads.js path", sm.deleted[0])
	}
}

func TestScriptsDeleteCancel(t *testing.T) {
	sm := testScriptManager()
	app := newAppWithScripts(3, sm)
	sendKey(app, "S")

	app.updateScripts(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	app.updateScripts(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if app.scriptsConfirmDelete {
		t.Error("n should cancel confirmDelete")
	}
	if len(sm.deleted) != 0 {
		t.Error("nothing should be deleted after cancel")
	}
}

func TestScriptsEscCloses(t *testing.T) {
	sm := testScriptManager()
	app := newAppWithScripts(3, sm)
	sendKey(app, "S")

	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if app.showScripts {
		t.Error("Esc should close scripts modal")
	}
}

func TestScriptsSKeyCloses(t *testing.T) {
	sm := testScriptManager()
	app := newAppWithScripts(3, sm)
	sendKey(app, "S")
	if !app.showScripts {
		t.Fatal("should be open")
	}

	app.updateScripts(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	if app.showScripts {
		t.Error("S should close scripts modal")
	}
}

func TestViewScriptsEmpty(t *testing.T) {
	sm := &mockScriptManager{dir: "/scripts"}
	app := newAppWithScripts(3, sm)
	sendKey(app, "S")

	view := app.viewScripts()
	if !strings.Contains(view, "No scripts found") {
		t.Error("empty scripts should show 'No scripts found'")
	}
	if !strings.Contains(view, "/scripts") {
		t.Error("empty scripts should show script dir")
	}
}

func TestViewScriptsPopulated(t *testing.T) {
	sm := testScriptManager()
	app := newAppWithScripts(3, sm)
	sendKey(app, "S")

	view := app.viewScripts()
	if !strings.Contains(view, "block-ads") {
		t.Error("scripts view should contain script name")
	}
	if !strings.Contains(view, "add-header") {
		t.Error("scripts view should contain second script name")
	}
}
