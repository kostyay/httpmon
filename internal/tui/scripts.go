package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
func (a *App) updateScripts(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	case " ":
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
	c := exec.Command(editor, path) //nolint:gosec
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
				icon = lipgloss.NewStyle().Foreground(colorGreen).Render("+")
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

			name := info.Name
			if len(name) > 24 {
				name = name[:21] + "..."
			}

			line := fmt.Sprintf(" %s  %-24s %s ", icon, name, detail)
			if i == a.scriptsCursor {
				line = styleMenuSelected.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	if a.scriptsConfirmDelete {
		b.WriteString("\n")
		b.WriteString(styleWarning.Render(" Delete this script? (y/N) "))
	} else {
		b.WriteString("\n")
		b.WriteString(styleMuted.Render(" n:new  e:edit  space:toggle  d:delete  esc:close "))
	}

	popup := styleMenuBorder.Render(b.String())

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		popup,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.NoColor{}),
	)
}
