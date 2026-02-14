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
				{"t", "Toggle flat/tree mode"},
				{"f", "Focus host (tree mode)"},
				{"l/h  →/←", "Expand/collapse host"},
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
				{"S", "Scripts manager"},
				{"q / Ctrl+C", "Quit"},
				{"Esc", "Back / dismiss"},
			},
		},
	}
}

func (a *App) viewHelp() string {
	var b strings.Builder

	title := "Keybindings"
	b.WriteString(styleHeader.Render(title))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", a.width))
	b.WriteString("\n\n")

	for _, g := range helpGroups() {
		b.WriteString(styleSection.Render(g.title))
		b.WriteString("\n")
		for _, kb := range g.bindings {
			b.WriteString(fmt.Sprintf("  %-16s %s\n", kb.key, kb.desc))
		}
		b.WriteString("\n")
	}

	used := 3 // title + sep + blank
	for _, g := range helpGroups() {
		used += 2 + len(g.bindings) // title + blank + bindings
	}
	for used < a.height-1 {
		b.WriteString("\n")
		used++
	}

	bar := "? or Esc to dismiss"
	b.WriteString(styleStatusBar.Width(a.width).Render(bar))

	return b.String()
}
