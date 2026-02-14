package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kostyay/httpmon/internal/scripting"
)

// initScripts prepares the scripts modal.
func (a *App) initScripts() {
	a.showScripts = true
	a.scriptsCursor = 0
	a.scriptsConfirmDelete = false
	if a.scripts != nil {
		a.scripts.Reload()
		a.scriptsList = a.scripts.Scripts()
	}
}

// updateScripts handles key events in the scripts modal.
func (a *App) updateScripts(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.scriptsAddMapLocal {
		return a.updateScriptsMapLocalAdd(msg)
	}
	if a.scriptsConfirmDelete {
		switch msg.String() {
		case "y", "Y":
			if a.scriptsCursor < len(a.scriptsList) {
				_ = a.scripts.Delete(a.scriptsList[a.scriptsCursor].FilePath)
				a.scriptsList = a.scripts.Scripts()
				if a.scriptsCursor >= len(a.scriptsList) {
					a.scriptsCursor = max(len(a.scriptsList)-1, 0)
				}
			}
			a.scriptsConfirmDelete = false
		default:
			a.scriptsConfirmDelete = false
		}
		return a, nil
	}

	switch msg.String() {
	case "esc", "S":
		a.showScripts = false
		return a, nil
	case "j", "down":
		if a.scriptsCursor < len(a.scriptsList)-1 {
			a.scriptsCursor++
		}
	case "k", "up":
		if a.scriptsCursor > 0 {
			a.scriptsCursor--
		}
	case "space":
		if a.scriptsCursor < len(a.scriptsList) {
			_ = a.scripts.Toggle(a.scriptsList[a.scriptsCursor].FilePath)
			a.scriptsList = a.scripts.Scripts()
		}
	case "e":
		if a.scriptsCursor < len(a.scriptsList) {
			path := a.scriptsList[a.scriptsCursor].FilePath
			return a, a.openScriptInEditor(path)
		}
	case "n":
		path, err := a.scripts.CreateNew()
		if err == nil {
			return a, a.openScriptInEditor(path)
		}
	case "m":
		a.scriptsAddMapLocal = true
		a.scriptsMLFocus = 0
		a.scriptsMLPattern = textinput.New()
		a.scriptsMLPattern.Placeholder = "*://host/path*"
		a.scriptsMLPattern.CharLimit = 256
		a.scriptsMLPath = textinput.New()
		a.scriptsMLPath.Placeholder = "/path/to/local/file"
		a.scriptsMLPath.CharLimit = 512
		return a, a.scriptsMLPattern.Focus()
	case "d":
		if a.scriptsCursor < len(a.scriptsList) {
			a.scriptsConfirmDelete = true
		}
	}
	return a, nil
}

// openScriptInEditor opens a script file in the user's editor.
func (a *App) openScriptInEditor(path string) tea.Cmd {
	editor := resolveEditor()
	c := exec.Command(editor, path) // #nosec G204 -- editor from $VISUAL/$EDITOR env
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return scriptEditorFinishedMsg{path: path}
	})
}

type scriptEditorFinishedMsg struct {
	path string
}

// viewScripts renders the scripts modal as a centered popup.
func (a *App) viewScripts() string {
	var b strings.Builder

	b.WriteString(styleMenuTitle.Render(" Scripts "))
	b.WriteString("\n")

	if len(a.scriptsList) == 0 {
		b.WriteString(styleMuted.Render("  No scripts found"))
		b.WriteString("\n")
		b.WriteString(styleMuted.Render(fmt.Sprintf("  Dir: %s", a.scripts.ScriptDir())))
		b.WriteString("\n\n")
	} else {
		for i, info := range a.scriptsList {
			var icon string
			if info.Error != "" {
				icon = styleWarning.Render("!")
			} else if info.Enabled {
				icon = styleEnabled.Render("+")
			} else {
				icon = styleMuted.Render("-")
			}

			var detail string
			if info.Error != "" {
				detail = styleWarning.Render("error")
			} else if len(info.Matches) > 0 {
				detail = info.Matches[0]
				if len(info.Matches) > 1 {
					detail += styleMuted.Render(fmt.Sprintf(" +%d", len(info.Matches)-1))
				}
			}

			badges := categoryBadges(info.Categories)

			name := info.Name
			if len(name) > 24 {
				name = name[:21] + "..."
			}

			line := fmt.Sprintf(" %s %s %-24s %s ",
				icon, badges, name, detail)
			if i == a.scriptsCursor {
				line = styleMenuSelected.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	if a.scriptsAddMapLocal {
		b.WriteString("\n")
		b.WriteString("  Pattern: ")
		b.WriteString(a.scriptsMLPattern.View())
		b.WriteString("\n")
		b.WriteString("  File:    ")
		b.WriteString(a.scriptsMLPath.View())
		b.WriteString("\n\n")
		b.WriteString(styleMuted.Render(" Tab:switch  Enter:save  Esc:cancel "))
	} else if a.scriptsConfirmDelete {
		b.WriteString("\n")
		b.WriteString(styleWarning.Render(" Delete this script? (y/N) "))
	} else {
		b.WriteString("\n")
		b.WriteString(styleMuted.Render(
			" n:new  m:map-local  e:edit  space:toggle  d:delete  esc:close "))
	}

	popup := styleMenuBorder.Render(b.String())

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		popup,
	)
}

func (a *App) updateScriptsMapLocalAdd(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.scriptsAddMapLocal = false
		return a, nil
	case "tab":
		if a.scriptsMLFocus == 0 {
			a.scriptsMLFocus = 1
			a.scriptsMLPattern.Blur()
			return a, a.scriptsMLPath.Focus()
		}
		a.scriptsMLFocus = 0
		a.scriptsMLPath.Blur()
		return a, a.scriptsMLPattern.Focus()
	case "enter":
		pattern := a.scriptsMLPattern.Value()
		localPath := a.scriptsMLPath.Value()
		if pattern != "" && localPath != "" && a.scripts != nil {
			_, _ = a.scripts.QuickAddMapLocal(pattern, localPath)
			a.scriptsList = a.scripts.Scripts()
		}
		a.scriptsAddMapLocal = false
		return a, nil
	}

	var cmd tea.Cmd
	if a.scriptsMLFocus == 0 {
		a.scriptsMLPattern, cmd = a.scriptsMLPattern.Update(msg)
	} else {
		a.scriptsMLPath, cmd = a.scriptsMLPath.Update(msg)
	}
	return a, cmd
}

func categoryBadges(cats []scripting.ScriptCategory) string {
	if len(cats) == 0 {
		return ""
	}
	var b strings.Builder
	for _, c := range cats {
		b.WriteString(styleMuted.Render("[" + string(c) + "]"))
	}
	return b.String()
}
