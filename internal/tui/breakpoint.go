package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/kostyay/httpmon/internal/breakpoint"
)

const (
	bpPaneHeaders = 0
	bpPaneBody    = 1
)

func (a *App) drainBreakpointSub() {
	if a.breakpoints == nil {
		return
	}
	if a.breakpointSub == nil {
		a.breakpointSub = a.breakpoints.Subscribe()
	}
	for {
		select {
		case <-a.breakpointSub:
			a.breakpointHitCount++
		default:
			return
		}
	}
}

func (a *App) initBreakpointQueue() {
	a.showBreakpointQueue = true
	a.breakpointCursor = 0
	a.editingBreakpoint = nil
	a.refreshBreakpointPending()
}

func (a *App) refreshBreakpointPending() {
	if a.breakpoints == nil {
		return
	}
	a.breakpointPending = a.breakpoints.Pending()
	sort.Slice(a.breakpointPending, func(i, j int) bool {
		return a.breakpointPending[i].FlowID < a.breakpointPending[j].FlowID
	})
}

func (a *App) initBreakpointEditor(hit breakpoint.BreakpointHit) {
	a.editingBreakpoint = &hit

	headerLines := formatHeadersForEditor(hit.Headers)
	a.bpHeadersTA = textarea.New()
	a.bpHeadersTA.SetValue(headerLines)
	a.bpHeadersTA.Focus()

	a.bpBodyTA = textarea.New()
	a.bpBodyTA.SetValue(string(hit.Body))

	a.bpFocusedPane = bpPaneHeaders
}

func formatHeadersForEditor(h map[string]string) string {
	if len(h) == 0 {
		return ""
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(": ")
		sb.WriteString(h[k])
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func parseHeadersFromEditor(s string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result
}

func (a *App) updateBreakpoint(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.editingBreakpoint != nil {
		return a.updateBreakpointEditor(msg)
	}
	return a.updateBreakpointQueue(msg)
}

func (a *App) updateBreakpointQueue(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	a.refreshBreakpointPending()
	n := len(a.breakpointPending)

	if n == 0 {
		a.showBreakpointQueue = false
		return a, nil
	}

	switch msg.String() {
	case "esc", "B":
		a.showBreakpointQueue = false
		return a, nil
	case "j", "down":
		if a.breakpointCursor < n-1 {
			a.breakpointCursor++
		}
	case "k", "up":
		if a.breakpointCursor > 0 {
			a.breakpointCursor--
		}
	case "enter":
		if a.breakpointCursor < n {
			a.initBreakpointEditor(a.breakpointPending[a.breakpointCursor])
		}
	case "A":
		if a.breakpoints != nil {
			a.breakpoints.ResumeAll()
			a.refreshBreakpointPending()
			if len(a.breakpointPending) == 0 {
				a.showBreakpointQueue = false
			}
		}
	}

	if a.breakpointCursor >= n {
		a.breakpointCursor = max(n-1, 0)
	}
	return a, nil
}

func (a *App) updateBreakpointEditor(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	hit := a.editingBreakpoint

	switch msg.String() {
	case "esc":
		a.breakpoints.Resume(hit.FlowID, breakpoint.BreakpointResume{Skipped: true})
		a.editingBreakpoint = nil
		a.refreshBreakpointPending()
		if len(a.breakpointPending) == 0 {
			a.showBreakpointQueue = false
		}
		return a, nil

	case "ctrl+s":
		resp := breakpoint.BreakpointResume{
			Headers: parseHeadersFromEditor(a.bpHeadersTA.Value()),
			Body:    []byte(a.bpBodyTA.Value()),
		}
		a.breakpoints.Resume(hit.FlowID, resp)
		a.editingBreakpoint = nil
		a.refreshBreakpointPending()
		if len(a.breakpointPending) == 0 {
			a.showBreakpointQueue = false
		}
		return a, nil

	case "tab":
		if a.bpFocusedPane == bpPaneHeaders {
			a.bpFocusedPane = bpPaneBody
			a.bpHeadersTA.Blur()
			a.bpBodyTA.Focus()
		} else {
			a.bpFocusedPane = bpPaneHeaders
			a.bpBodyTA.Blur()
			a.bpHeadersTA.Focus()
		}
		return a, nil

	case "E":
		ct := hit.Meta.ContentType
		return a, openInEditor([]byte(a.bpBodyTA.Value()), ct)
	}

	var cmd tea.Cmd
	if a.bpFocusedPane == bpPaneHeaders {
		a.bpHeadersTA, cmd = a.bpHeadersTA.Update(msg)
	} else {
		a.bpBodyTA, cmd = a.bpBodyTA.Update(msg)
	}
	return a, cmd
}

func (a *App) viewBreakpoint() string {
	if a.editingBreakpoint != nil {
		return a.viewBreakpointEditor()
	}
	return a.viewBreakpointQueue()
}

func (a *App) viewBreakpointQueue() string {
	var b strings.Builder

	b.WriteString(styleHeader.Render("Breakpoint Queue"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", a.width))
	b.WriteString("\n\n")

	if len(a.breakpointPending) == 0 {
		b.WriteString("  No paused flows\n")
	} else {
		for i, hit := range a.breakpointPending {
			phase := "REQ"
			if hit.Phase == breakpoint.PhaseResponse {
				phase = "RSP"
			}
			line := fmt.Sprintf("  %s  %s %s%s",
				phase, hit.Meta.Method, hit.Meta.Host, hit.Meta.Path)
			if i == a.breakpointCursor {
				line = styleSelected.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	used := 4 + len(a.breakpointPending)
	for used < a.height-1 {
		b.WriteString("\n")
		used++
	}

	bar := "Enter:edit  A:resume-all  Esc:back"
	b.WriteString(styleStatusBar.Width(a.width).Render(bar))
	return b.String()
}

func (a *App) viewBreakpointEditor() string {
	var b strings.Builder
	hit := a.editingBreakpoint

	phase := "Request"
	if hit.Phase == breakpoint.PhaseResponse {
		phase = "Response"
	}
	title := fmt.Sprintf("Breakpoint: %s %s%s (%s)",
		hit.Meta.Method, hit.Meta.Host, hit.Meta.Path, phase)
	b.WriteString(styleHeader.Render(title))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", a.width))
	b.WriteString("\n")

	hdrLabel := "Headers"
	if a.bpFocusedPane == bpPaneHeaders {
		hdrLabel = "▸ Headers"
	}
	b.WriteString(styleSection.Render(hdrLabel))
	b.WriteString("\n")
	b.WriteString(a.bpHeadersTA.View())
	b.WriteString("\n\n")

	bodyLabel := "Body"
	if a.bpFocusedPane == bpPaneBody {
		bodyLabel = "▸ Body"
	}
	b.WriteString(styleSection.Render(bodyLabel))
	b.WriteString("\n")
	b.WriteString(a.bpBodyTA.View())
	b.WriteString("\n")

	bar := "Ctrl+S:resume  Tab:switch-pane  E:editor  Esc:skip"
	b.WriteString(styleStatusBar.Width(a.width).Render(bar))
	return b.String()
}
