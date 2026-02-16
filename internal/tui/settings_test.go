package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kostyay/httpmon/internal/config"
)

func newAppWithSettings(dir string) *App {
	m := seedMock(3)
	app := NewApp(AppConfig{
		Store:     m,
		Proxy:     &mockProxyInfo{addr: ":9999"},
		CATrusted: true,
		DataDir:   dir,
	})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))
	return app
}

// --- Open / Close / Navigate ---

func TestSettingsOpenClose(t *testing.T) {
	app := newAppWithSettings(t.TempDir())

	sendKey(app, "P")
	if !app.showSettings {
		t.Fatal("P should open settings")
	}

	app.updateSettings(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.showSettings {
		t.Error("Esc should close settings")
	}
}

func TestSettingsPKeyCloses(t *testing.T) {
	app := newAppWithSettings(t.TempDir())

	sendKey(app, "P")
	if !app.showSettings {
		t.Fatal("P should open settings")
	}

	app.updateSettings(tea.KeyPressMsg{Code: 'P', Text: "P"})
	if app.showSettings {
		t.Error("second P should close settings")
	}
}

func TestSettingsNavigate(t *testing.T) {
	app := newAppWithSettings(t.TempDir())
	sendKey(app, "P")

	app.updateSettings(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if app.settingsCursor != 1 {
		t.Errorf("after j: cursor = %d, want 1", app.settingsCursor)
	}

	app.updateSettings(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if app.settingsCursor != 0 {
		t.Errorf("after k: cursor = %d, want 0", app.settingsCursor)
	}

	// Clamp at 0.
	app.updateSettings(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if app.settingsCursor != 0 {
		t.Errorf("clamp at 0: cursor = %d", app.settingsCursor)
	}

	// Clamp at max.
	for range len(settingsFields) + 2 {
		app.updateSettings(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	if app.settingsCursor != len(settingsFields)-1 {
		t.Errorf("clamp at max: cursor = %d, want %d", app.settingsCursor, len(settingsFields)-1)
	}
}

// --- Field interactions ---

func TestSettingsBoolToggle(t *testing.T) {
	app := newAppWithSettings(t.TempDir())
	sendKey(app, "P")
	app.settingsConfig = &config.Config{ProxyPort: 8080, MCPAddr: "127.0.0.1:9551", BufferSize: 10000}

	// MCPEnabled is field index 1.
	app.settingsCursor = 1
	if app.settingsConfig.MCPEnabled {
		t.Fatal("MCPEnabled should start false")
	}

	app.updateSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !app.settingsConfig.MCPEnabled {
		t.Error("Enter should toggle MCPEnabled to true")
	}

	app.updateSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.settingsConfig.MCPEnabled {
		t.Error("second Enter should toggle MCPEnabled back to false")
	}
}

func TestSettingsEnumCycle(t *testing.T) {
	app := newAppWithSettings(t.TempDir())
	sendKey(app, "P")
	app.settingsConfig = &config.Config{ProxyPort: 8080, MCPAddr: "127.0.0.1:9551", BufferSize: 10000}

	// ThrottlePreset is field index 4; options: "", "3g", "4g", "wifi".
	app.settingsCursor = 4

	want := []string{"3g", "4g", "wifi", ""}
	for _, exp := range want {
		app.updateSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
		if app.settingsConfig.ThrottlePreset != exp {
			t.Errorf("ThrottlePreset = %q, want %q", app.settingsConfig.ThrottlePreset, exp)
		}
	}
}

func TestSettingsIntEdit(t *testing.T) {
	app := newAppWithSettings(t.TempDir())
	sendKey(app, "P")
	app.settingsConfig = &config.Config{ProxyPort: 8080, MCPAddr: "127.0.0.1:9551", BufferSize: 10000}

	// ProxyPort is field index 0.
	app.settingsCursor = 0
	app.updateSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !app.settingsEditing {
		t.Fatal("Enter on int field should open text input")
	}

	// Clear existing value and type new one.
	app.settingsInput.SetValue("9090")
	app.updateSettingsInput(tea.KeyPressMsg{Code: tea.KeyEnter})

	if app.settingsEditing {
		t.Error("Enter should close text input")
	}
	if app.settingsConfig.ProxyPort != 9090 {
		t.Errorf("ProxyPort = %d, want 9090", app.settingsConfig.ProxyPort)
	}
}

func TestSettingsEditCancel(t *testing.T) {
	app := newAppWithSettings(t.TempDir())
	sendKey(app, "P")
	app.settingsConfig = &config.Config{ProxyPort: 8080, MCPAddr: "127.0.0.1:9551", BufferSize: 10000}

	app.settingsCursor = 0
	app.updateSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !app.settingsEditing {
		t.Fatal("should be editing")
	}

	// Modify the input but cancel.
	app.settingsInput.SetValue("1234")
	app.updateSettingsInput(tea.KeyPressMsg{Code: tea.KeyEscape})

	if app.settingsEditing {
		t.Error("Esc should close text input")
	}
	if app.settingsConfig.ProxyPort != 8080 {
		t.Errorf("ProxyPort = %d, want 8080 (original value)", app.settingsConfig.ProxyPort)
	}
}

// --- Persistence + View + Menu ---

func TestSettingsPersistence(t *testing.T) {
	dir := t.TempDir()
	app := newAppWithSettings(dir)

	sendKey(app, "P")

	// Toggle MCPEnabled (index 1).
	app.settingsCursor = 1
	app.updateSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !app.settingsConfig.MCPEnabled {
		t.Fatal("MCPEnabled should be true after toggle")
	}

	// Close settings (saves to disk).
	app.updateSettings(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.showSettings {
		t.Fatal("settings should be closed")
	}

	// Reload from disk and verify.
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !cfg.MCPEnabled {
		t.Error("MCPEnabled should be persisted as true")
	}
}

func TestSettingsViewContent(t *testing.T) {
	app := newAppWithSettings(t.TempDir())
	sendKey(app, "P")

	view := ansi.Strip(app.viewSettings())
	if view == "" {
		t.Fatal("viewSettings should return non-empty")
	}

	// All 7 field labels.
	for _, label := range []string{
		"Proxy Port", "MCP Enabled", "MCP Address", "Buffer Size",
		"Throttle Preset", "List Mode", "Tree Group By",
	} {
		if !strings.Contains(view, label) {
			t.Errorf("view should contain label %q", label)
		}
	}

	// Restart hints.
	if !strings.Contains(view, "(restart)") {
		t.Error("view should contain (restart) hint")
	}

	// Help text.
	if !strings.Contains(view, "Esc save & close") {
		t.Error("view should contain help text")
	}
}

func TestSettingsMenuEntry(t *testing.T) {
	app := newAppWithSettings(t.TempDir())

	// List menu.
	listItems := app.listMenuItems()
	found := false
	for _, item := range listItems {
		if item.label == "Settings" {
			found = true
			break
		}
	}
	if !found {
		t.Error("listMenuItems should contain Settings")
	}

	// Detail menu.
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // enter detail
	detailItems := app.detailMenuItems()
	found = false
	for _, item := range detailItems {
		if item.label == "Settings" {
			found = true
			break
		}
	}
	if !found {
		t.Error("detailMenuItems should contain Settings")
	}
}
