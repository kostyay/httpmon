package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/kostyay/httpmon/internal/store"
)

// Column widths
const (
	colMethod = 7
	colStatus = 6
	colDur    = 8
	colSize   = 8
)

func (a *App) viewList() string {
	var b strings.Builder

	// Filter bar
	if a.filterInput.Focused() {
		b.WriteString(a.filterInput.View())
	} else if a.filterText != "" {
		b.WriteString(styleMuted.Render(fmt.Sprintf("Filter: %s", a.filterText)))
	} else {
		b.WriteString(styleMuted.Render("/ to filter..."))
	}
	b.WriteString("\n")

	// Column headers
	hostW, pathW := a.columnWidths()
	hdr := fmt.Sprintf("%-*s %-*s %-*s %-*s %*s %*s",
		colMethod, "METHOD",
		colStatus, "STATUS",
		hostW, "HOST",
		pathW, "PATH",
		colDur, "DUR",
		colSize, "SIZE",
	)
	b.WriteString(styleHeader.Render(truncate(hdr, a.width)))
	b.WriteString("\n")

	// Separator
	b.WriteString(strings.Repeat("─", a.width))
	b.WriteString("\n")

	if len(a.flows) == 0 {
		empty := fmt.Sprintf("Waiting for traffic... proxy at %s", a.proxyAddr())
		b.WriteString(styleMuted.Render(empty))
		b.WriteString("\n")
	}

	// Flow rows
	maxRows := a.height - 5 // filter + header + sep + status + margin
	if maxRows < 0 {
		maxRows = 0
	}
	for i, f := range a.flows {
		if i >= maxRows {
			break
		}
		row := a.renderFlowRow(f, hostW, pathW)
		if i == a.selectedIdx {
			row = styleSelected.Width(a.width).Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	// Pad remaining space
	used := 4 + min(len(a.flows), maxRows)
	for used < a.height-1 {
		b.WriteString("\n")
		used++
	}

	// Status bar
	b.WriteString(a.statusText())

	return b.String()
}

func (a *App) columnWidths() (hostW, pathW int) {
	fixed := colMethod + colStatus + colDur + colSize + 5 // 5 spaces between columns
	remaining := a.width - fixed
	if remaining < 20 {
		remaining = 20
	}
	hostW = remaining * 40 / 100
	pathW = remaining - hostW
	return
}

func (a *App) renderFlowRow(f store.FlowMeta, hostW, pathW int) string {
	method := styleMethod.Render(fmt.Sprintf("%-*s", colMethod, f.Method))

	var status string
	if f.StatusCode > 0 {
		status = statusStyle(f.StatusCode).Render(fmt.Sprintf("%-*d", colStatus, f.StatusCode))
	} else {
		status = styleMuted.Render(fmt.Sprintf("%-*s", colStatus, "..."))
	}

	host := truncPad(f.Host, hostW)
	path := truncPad(f.Path, pathW)

	var dur string
	if f.State == store.StateCompleted {
		dur = fmt.Sprintf("%*s", colDur, formatDuration(f.Duration))
	} else {
		dur = fmt.Sprintf("%*s", colDur, "...")
	}

	size := fmt.Sprintf("%*s", colSize, formatSize(f.SizeBytes))

	return fmt.Sprintf("%s %s %s %s %s %s", method, status, host, path, dur, size)
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
}

func formatSize(b int64) string {
	switch {
	case b == 0:
		return "0B"
	case b < 1024:
		return fmt.Sprintf("%dB", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(b)/1024)
	default:
		return fmt.Sprintf("%.1fM", float64(b)/(1024*1024))
	}
}

func truncPad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		if w > 1 {
			return string(r[:w-1]) + "…"
		}
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}

func truncate(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return s
}
