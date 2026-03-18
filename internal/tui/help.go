package tui

import (
	"fmt"
	"strings"
)

type helpBinding struct {
	key  string
	desc string
}

type helpGroup struct {
	title    string
	bindings []helpBinding
}

func helpGroups() []helpGroup {
	return []helpGroup{
		{
			title: "Navigation",
			bindings: []helpBinding{
				{"j/k  ↑/↓", "Move cursor up/down"},
				{"g/G", "Jump to first/last"},
				{"Ctrl+D/U", "Page down/up"},
				{"Enter", "Open detail / toggle host"},
			},
		},
		{
			title: "List View",
			bindings: []helpBinding{
				{"t", "Cycle flat/host/process tree"},
				{"f", "Focus group (tree mode)"},
				{"l/h  →/←", "Expand/collapse group"},
			},
		},
		{
			title: "Detail View",
			bindings: []helpBinding{
				{"1/2  ←/→", "Request/Response tab"},
				{"j/k", "Scroll up/down"},
				{"d/u", "Half-page down/up"},
				{"n/N", "Next/prev flow"},
				{"p", "Toggle pretty/raw"},
				{"e", "Open in editor"},
				{"i", "Toggle image preview"},
				{"g/h/b", "Collapse general/headers/body"},
			},
		},
		{
			title: "Filter",
			bindings: []helpBinding{
				{"/", "Focus filter input"},
				{"Enter", "Apply filter"},
				{"Esc", "Dismiss filter"},
			},
		},
		{
			title: "Global",
			bindings: []helpBinding{
				{"?", "Toggle this help"},
				{"P", "Settings"},
				{"S", "Scripts manager"},
				{"T", "Throttle settings"},
				{"q / Ctrl+C", "Quit"},
				{"Esc", "Back / dismiss"},
			},
		},
	}
}

func (a *App) viewHelp() string {
	var b strings.Builder

	b.WriteString(styleHeader.Render("Keybindings"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", a.width))
	b.WriteString("\n\n")

	groups := helpGroups()
	used := 3 // title + sep + blank
	for _, g := range groups {
		b.WriteString(styleSection.Render(g.title))
		b.WriteString("\n")
		for _, kb := range g.bindings {
			fmt.Fprintf(&b, "  %-16s %s\n", kb.key, kb.desc)
		}
		b.WriteString("\n")
		used += 2 + len(g.bindings)
	}

	for used < a.height-1 {
		b.WriteString("\n")
		used++
	}

	b.WriteString(styleStatusBar.Width(a.width).Render("? or Esc to dismiss"))

	return b.String()
}
