package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kostyay/httpmon/internal/diff"
	"github.com/kostyay/httpmon/internal/store"
)

var (
	styleDiffAdd = lipgloss.NewStyle().Foreground(colorGreen)
	styleDiffDel = lipgloss.NewStyle().Foreground(colorRed)
)

func (a *App) handleDiffMark() (tea.Model, tea.Cmd) {
	// Get current flow ID from list.
	var currentID store.FlowID
	if a.listMode == modeFlat {
		if a.selectedIdx < len(a.flows) {
			currentID = a.flows[a.selectedIdx].ID
		}
	} else {
		if a.selectedIdx < len(a.treeRows) && !a.treeRows[a.selectedIdx].IsHost {
			currentID = a.treeRows[a.selectedIdx].Flow.ID
		}
	}

	if currentID == "" {
		return a, nil
	}

	if a.diffMarkID == "" {
		// First mark.
		a.diffMarkID = currentID
		return a, nil
	}

	// Second mark → open diff.
	meta1, data1, err1 := a.store.Get(a.diffMarkID)
	meta2, data2, err2 := a.store.Get(currentID)
	if err1 != nil || err2 != nil || meta1 == nil || meta2 == nil {
		a.diffMarkID = ""
		return a, nil
	}

	result := diff.Compare(meta1, data1, meta2, data2)
	a.diffContent = a.renderDiff(result)
	a.showDiff = true
	a.diffMarkID = ""
	return a, nil
}

func (a *App) renderDiff(result *diff.Result) string {
	var b strings.Builder
	for _, l := range result.Lines {
		switch l.Type {
		case "add":
			b.WriteString(styleDiffAdd.Render("+ " + l.Content))
		case "del":
			b.WriteString(styleDiffDel.Render("- " + l.Content))
		default:
			b.WriteString("  " + l.Content)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (a *App) viewDiff() string {
	var b strings.Builder

	b.WriteString(styleHeader.Render("Flow Diff"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", a.width))
	b.WriteString("\n\n")

	b.WriteString(a.diffContent)

	// Count used lines.
	lines := strings.Count(a.diffContent, "\n")
	used := 3 + lines
	for used < a.height-1 {
		b.WriteString("\n")
		used++
	}

	bar := "Esc:back  + added  - removed"
	b.WriteString(styleStatusBar.Width(a.width).Render(bar))

	return b.String()
}
