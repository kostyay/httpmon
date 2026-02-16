package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kostyay/httpmon/internal/config"
)

type settingsField struct {
	label    string
	kind     string // "int", "bool", "enum", "string"
	options  []string
	restart  bool
	get      func(*config.Config) string
	set      func(*config.Config, string)
}

var settingsFields = []settingsField{
	{
		label: "Proxy Port", kind: "int", restart: true,
		get: func(c *config.Config) string { return strconv.Itoa(c.ProxyPort) },
		set: func(c *config.Config, v string) {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 65535 {
				c.ProxyPort = n
			}
		},
	},
	{
		label: "MCP Enabled", kind: "bool", restart: true,
		get: func(c *config.Config) string { return strconv.FormatBool(c.MCPEnabled) },
		set: func(c *config.Config, v string) { c.MCPEnabled = v == "true" },
	},
	{
		label: "MCP Address", kind: "string", restart: true,
		get: func(c *config.Config) string { return c.MCPAddr },
		set: func(c *config.Config, v string) { c.MCPAddr = v },
	},
	{
		label: "Buffer Size", kind: "int", restart: true,
		get: func(c *config.Config) string { return strconv.Itoa(c.BufferSize) },
		set: func(c *config.Config, v string) {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				c.BufferSize = n
			}
		},
	},
	{
		label: "Throttle Preset", kind: "enum", options: []string{"", "3g", "4g", "wifi"},
		get: func(c *config.Config) string { return c.ThrottlePreset },
		set: func(c *config.Config, v string) { c.ThrottlePreset = v },
	},
	{
		label: "List Mode", kind: "enum", options: []string{"", "flat", "tree"},
		get: func(c *config.Config) string { return c.ListMode },
		set: func(c *config.Config, v string) { c.ListMode = v },
	},
	{
		label: "Tree Group By", kind: "enum", options: []string{"", "host", "process"},
		get: func(c *config.Config) string { return c.TreeGroupBy },
		set: func(c *config.Config, v string) { c.TreeGroupBy = v },
	},
}

func (a *App) initSettings() {
	a.showSettings = true
	a.settingsCursor = 0
	a.settingsEditing = false

	// Reload config from disk.
	cfg, err := config.Load(a.dataDir)
	if err != nil {
		return
	}
	a.settingsConfig = cfg
}

func (a *App) updateSettings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.settingsEditing {
		return a.updateSettingsInput(msg)
	}

	switch msg.String() {
	case "esc", "P":
		// Save and close.
		if a.settingsConfig != nil {
			_ = a.settingsConfig.Save(a.dataDir)
		}
		a.showSettings = false
		return a, nil
	case "j", "down":
		if a.settingsCursor < len(settingsFields)-1 {
			a.settingsCursor++
		}
	case "k", "up":
		if a.settingsCursor > 0 {
			a.settingsCursor--
		}
	case "enter":
		if a.settingsConfig == nil {
			return a, nil
		}
		f := settingsFields[a.settingsCursor]
		switch f.kind {
		case "bool":
			f.set(a.settingsConfig, strconv.FormatBool(f.get(a.settingsConfig) != "true"))
		case "enum":
			cur := f.get(a.settingsConfig)
			idx := 0
			for i, opt := range f.options {
				if opt == cur {
					idx = i
					break
				}
			}
			next := (idx + 1) % len(f.options)
			f.set(a.settingsConfig, f.options[next])
		case "int", "string":
			a.settingsEditing = true
			a.settingsInput = textinput.New()
			a.settingsInput.SetValue(f.get(a.settingsConfig))
			a.settingsInput.CharLimit = 64
			cmd := a.settingsInput.Focus()
			return a, cmd
		}
	}
	return a, nil
}

func (a *App) updateSettingsInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if a.settingsConfig != nil {
			f := settingsFields[a.settingsCursor]
			f.set(a.settingsConfig, a.settingsInput.Value())
		}
		a.settingsEditing = false
		return a, nil
	case "esc":
		a.settingsEditing = false
		return a, nil
	}

	var cmd tea.Cmd
	a.settingsInput, cmd = a.settingsInput.Update(msg)
	return a, cmd
}

// settingsFieldDisplay returns the rendered display string for a single settings field.
func (a *App) settingsFieldDisplay(f settingsField, val string, idx int) string {
	if a.settingsEditing && idx == a.settingsCursor {
		return a.settingsInput.View()
	}
	switch f.kind {
	case "bool":
		if val == "true" {
			return styleEnabled.Render("on")
		}
		return styleMuted.Render("off")
	default:
		if val == "" {
			return styleMuted.Render("(default)")
		}
		return val
	}
}

func (a *App) viewSettings() string {
	var b strings.Builder

	b.WriteString(styleMenuTitle.Render(" Settings "))
	b.WriteString("\n\n")

	for i, f := range settingsFields {
		val := ""
		if a.settingsConfig != nil {
			val = f.get(a.settingsConfig)
		}

		display := a.settingsFieldDisplay(f, val, i)

		hint := ""
		if f.restart {
			hint = styleMuted.Render(" (restart)")
		}

		line := fmt.Sprintf(" %-18s %s%s", f.label, display, hint)
		if i == a.settingsCursor && !a.settingsEditing {
			line = styleMenuSelected.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	help := " j/k navigate  Enter edit/toggle  Esc save & close "
	b.WriteString(styleMuted.Render(help))

	popup := styleMenuBorder.Render(b.String())

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		popup,
	)
}
