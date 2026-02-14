package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kostyay/httpmon/internal/maplocal"
)

func (a *App) initMapLocal() {
	a.showMapLocal = true
	a.mapLocalCursor = 0
	a.mapLocalConfirmDelete = false
	a.mapLocalAdding = false
	if a.mapLocal != nil {
		a.mapLocalRules = a.mapLocal.Rules()
	}
}

func (a *App) updateMapLocal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.mapLocalConfirmDelete {
		switch msg.String() {
		case "y", "Y":
			if a.mapLocal != nil && a.mapLocalCursor < len(a.mapLocalRules) {
				a.mapLocal.RemoveRule(a.mapLocalCursor)
				a.mapLocalRules = a.mapLocal.Rules()
				if a.mapLocalCursor >= len(a.mapLocalRules) {
					a.mapLocalCursor = max(len(a.mapLocalRules)-1, 0)
				}
				a.autoSaveMapLocal()
			}
			a.mapLocalConfirmDelete = false
		default:
			a.mapLocalConfirmDelete = false
		}
		return a, nil
	}

	if a.mapLocalAdding {
		return a.updateMapLocalAdd(msg)
	}

	switch msg.String() {
	case "esc", "M":
		a.showMapLocal = false
		return a, nil
	case "j", "down":
		if a.mapLocalCursor < len(a.mapLocalRules)-1 {
			a.mapLocalCursor++
		}
	case "k", "up":
		if a.mapLocalCursor > 0 {
			a.mapLocalCursor--
		}
	case "n":
		a.mapLocalAdding = true
		a.mapLocalAddFocus = 0
		a.mapLocalPatternInput = textinput.New()
		a.mapLocalPatternInput.Placeholder = "host/path pattern"
		a.mapLocalPatternInput.CharLimit = 256
		a.mapLocalPathInput = textinput.New()
		a.mapLocalPathInput.Placeholder = "/path/to/local/file"
		a.mapLocalPathInput.CharLimit = 512
		return a, a.mapLocalPatternInput.Focus()
	case "d":
		if len(a.mapLocalRules) > 0 {
			a.mapLocalConfirmDelete = true
		}
	}
	return a, nil
}

func (a *App) updateMapLocalAdd(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.mapLocalAdding = false
		return a, nil
	case "tab":
		if a.mapLocalAddFocus == 0 {
			a.mapLocalAddFocus = 1
			a.mapLocalPatternInput.Blur()
			return a, a.mapLocalPathInput.Focus()
		}
		a.mapLocalAddFocus = 0
		a.mapLocalPathInput.Blur()
		return a, a.mapLocalPatternInput.Focus()
	case "enter":
		pattern := a.mapLocalPatternInput.Value()
		localPath := a.mapLocalPathInput.Value()
		if pattern != "" && localPath != "" && a.mapLocal != nil {
			a.mapLocal.AddRule(maplocal.Rule{
				Pattern:   pattern,
				LocalPath: localPath,
			})
			a.mapLocalRules = a.mapLocal.Rules()
			a.autoSaveMapLocal()
		}
		a.mapLocalAdding = false
		return a, nil
	}

	var cmd tea.Cmd
	if a.mapLocalAddFocus == 0 {
		a.mapLocalPatternInput, cmd = a.mapLocalPatternInput.Update(msg)
	} else {
		a.mapLocalPathInput, cmd = a.mapLocalPathInput.Update(msg)
	}
	return a, cmd
}

func (a *App) autoSaveMapLocal() {
	if a.mapLocalFile != "" && a.mapLocal != nil {
		_ = a.mapLocal.SaveToFile(a.mapLocalFile)
	}
}

func (a *App) viewMapLocal() string {
	var b strings.Builder

	b.WriteString(styleMenuTitle.Render(" Map Local "))
	b.WriteString("\n")

	if a.mapLocalAdding {
		b.WriteString("  Pattern: ")
		b.WriteString(a.mapLocalPatternInput.View())
		b.WriteString("\n")
		b.WriteString("  Path:    ")
		b.WriteString(a.mapLocalPathInput.View())
		b.WriteString("\n\n")
		b.WriteString(styleMuted.Render(" Tab:switch  Enter:save  Esc:cancel "))
	} else if len(a.mapLocalRules) == 0 {
		b.WriteString(styleMuted.Render("  No rules configured"))
		b.WriteString("\n\n")
	} else {
		for i, r := range a.mapLocalRules {
			pattern := r.Pattern
			if len(pattern) > 30 {
				pattern = pattern[:27] + "..."
			}
			localPath := r.LocalPath
			if len(localPath) > 30 {
				localPath = localPath[:27] + "..."
			}
			line := fmt.Sprintf(" %-30s → %-30s ", pattern, localPath)
			if i == a.mapLocalCursor {
				line = styleMenuSelected.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	if a.mapLocalConfirmDelete {
		b.WriteString("\n")
		b.WriteString(styleWarning.Render(" Delete this rule? (y/N) "))
	} else if !a.mapLocalAdding {
		b.WriteString("\n")
		b.WriteString(styleMuted.Render(" n:new  d:delete  Esc:close "))
	}

	popup := styleMenuBorder.Render(b.String())

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		popup,
	)
}
