package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/kostyay/httpmon/internal/har"
	"github.com/kostyay/httpmon/internal/store"
)

func (a *App) initExport(single bool) tea.Cmd {
	a.showExport = true
	a.exportSingle = single

	a.exportInput = textinput.New()
	a.exportInput.Placeholder = "filename.har"
	a.exportInput.CharLimit = 512
	a.exportInput.SetValue(fmt.Sprintf("httpmon-%s.har", time.Now().Format("20060102-150405")))
	a.exportInput.Focus()

	return a.exportInput.Focus()
}

func (a *App) updateExport(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.showExport = false
		return a, nil
	case "enter":
		return a.doExportHAR()
	}

	var cmd tea.Cmd
	a.exportInput, cmd = a.exportInput.Update(msg)
	return a, cmd
}

func (a *App) doExportHAR() (tea.Model, tea.Cmd) {
	defer func() { a.showExport = false }()

	filename := a.exportInput.Value()
	if filename == "" {
		return a, tea.Printf("filename is required")
	}

	var flows []store.FlowMeta
	if a.exportSingle {
		meta, _, err := a.store.Get(a.selectedID)
		if err != nil || meta == nil {
			return a, nil
		}
		flows = []store.FlowMeta{*meta}
	} else {
		flows = a.flows
	}

	fetcher := func(id store.FlowID) *store.FlowData {
		_, d, _ := a.store.Get(id)
		return d
	}

	data, err := har.Export(flows, fetcher)
	if err != nil {
		return a, tea.Printf("HAR export failed: %v", err)
	}

	if writeErr := os.WriteFile(filename, data, 0o644); writeErr != nil {
		return a, tea.Printf("HAR write failed: %v", writeErr)
	}

	return a, tea.Printf("Exported to %s", filename)
}

func (a *App) viewExport() string {
	var b strings.Builder

	b.WriteString(styleHeader.Render("Export HAR"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", a.width))
	b.WriteString("\n\n")

	b.WriteString(styleSection.Render("▸ Filename"))
	b.WriteString("\n  ")
	b.WriteString(a.exportInput.View())
	b.WriteString("\n\n")

	if a.exportSingle {
		b.WriteString(styleMuted.Render("  Exporting selected flow"))
	} else {
		b.WriteString(styleMuted.Render(fmt.Sprintf("  Exporting %d flows", len(a.flows))))
	}
	b.WriteString("\n")

	used := 7
	for used < a.height-1 {
		b.WriteString("\n")
		used++
	}

	bar := "Enter:export  Esc:cancel"
	b.WriteString(styleStatusBar.Width(a.width).Render(bar))

	return b.String()
}
