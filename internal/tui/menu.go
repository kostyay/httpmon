package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type menuItem struct {
	label string // display text, e.g. "Export HAR"
	key   string // shortcut key, also dispatched on Enter
}

// toKeyMsg converts the shortcut key into the equivalent tea.KeyMsg.
func (m menuItem) toKeyMsg() tea.KeyMsg {
	if m.key == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(m.key)}
}

// displayKey returns a human-readable label for the shortcut (e.g. "Enter" instead of "enter").
func (m menuItem) displayKey() string {
	if m.key == "enter" {
		return "Enter"
	}
	return m.key
}

func (a *App) initMenu() {
	a.showMenu = true
	a.menuCursor = 0

	if a.showDetail {
		a.menuItems = a.detailMenuItems()
	} else {
		a.menuItems = a.listMenuItems()
	}
}

func (a *App) listMenuItems() []menuItem {
	items := []menuItem{
		{label: "Open Detail", key: "enter"},
		{label: "Export HAR", key: "x"},
		{label: "Compose Request", key: "C"},
		{label: "Mark for Diff", key: "d"},
		{label: "Toggle Tree/Flat", key: "t"},
	}
	if a.listMode == modeTree {
		items = append(items, menuItem{label: "Focus Host", key: "f"})
	}
	return items
}

func (a *App) detailMenuItems() []menuItem {
	items := []menuItem{
		{label: "Export HAR", key: "x"},
		{label: "Copy cURL", key: "c"},
		{label: "Repeat Request", key: "r"},
		{label: "Open in Editor", key: "e"},
		{label: "Toggle Pretty/Raw", key: "p"},
	}
	if a.detailBodyIsImage() {
		items = append(items, menuItem{label: "Toggle Image Preview", key: "i"})
	}
	return items
}

func (a *App) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", " ":
		a.showMenu = false
		return a, nil
	case "j", "down":
		if a.menuCursor < len(a.menuItems)-1 {
			a.menuCursor++
		}
		return a, nil
	case "k", "up":
		if a.menuCursor > 0 {
			a.menuCursor--
		}
		return a, nil
	case "enter":
		return a.dispatchMenuAction()
	}
	return a, nil
}

func (a *App) dispatchMenuAction() (tea.Model, tea.Cmd) {
	if a.menuCursor >= len(a.menuItems) {
		a.showMenu = false
		return a, nil
	}
	item := a.menuItems[a.menuCursor]
	inDetail := a.showDetail
	a.showMenu = false

	if inDetail {
		return a.updateDetail(item.toKeyMsg())
	}
	return a.updateList(item.toKeyMsg())
}

func (a *App) viewMenu() string {
	var b strings.Builder

	b.WriteString(styleMenuTitle.Render(" Actions "))
	b.WriteString("\n")

	for i, item := range a.menuItems {
		line := fmt.Sprintf(" %-22s %s ", item.label, styleMuted.Render(item.displayKey()))
		if i == a.menuCursor {
			line = styleMenuSelected.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString(styleMuted.Render(" j/k navigate  Enter select  Esc close "))

	popup := styleMenuBorder.Render(b.String())

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		popup,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.NoColor{}),
	)
}
