package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/kostyay/httpmon/internal/store"
)

// Column widths
const (
	colMethod  = 7
	colStatus  = 6
	colProcess = 15
	colType    = 12
	colDur     = 8
	colSize    = 8
)

func (a *App) viewList() string {
	switch a.listMode {
	case modeTree:
		return a.viewTreeList()
	case modeFocus:
		return a.viewFocusList()
	default:
		return a.viewFlatList()
	}
}

func (a *App) viewFlatList() string {
	var b strings.Builder

	a.writeFilterBar(&b)

	hostW, pathW := a.columnWidths()
	hdr := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %-*s %*s %*s",
		colMethod, "METHOD",
		colStatus, "STATUS",
		hostW, "HOST",
		colProcess, "PROCESS",
		pathW, "PATH",
		colType, "TYPE",
		colDur, "DUR",
		colSize, "SIZE",
	)
	a.writeColumnHeader(&b, hdr)

	if len(a.flows) == 0 {
		a.writeEmptyMessage(&b, fmt.Sprintf("Waiting for traffic... proxy at %s", a.proxyAddr()))
	}

	a.writeRows(&b, len(a.flows), func(i int) string {
		return a.renderFlowRow(a.flows[i], hostW, pathW)
	})

	return b.String()
}

func (a *App) viewTreeList() string {
	var b strings.Builder

	a.writeFilterBar(&b)

	pathW := a.treePathWidth()
	hdr := fmt.Sprintf("    %-*s %-*s %-*s %-*s %-*s %*s %*s",
		colMethod, "METHOD",
		colStatus, "STATUS",
		colProcess, "PROCESS",
		pathW, "PATH",
		colType, "TYPE",
		colDur, "DUR",
		colSize, "SIZE",
	)
	a.writeColumnHeader(&b, hdr)

	if len(a.treeRows) == 0 {
		a.writeEmptyMessage(&b, fmt.Sprintf("Waiting for traffic... proxy at %s", a.proxyAddr()))
	}

	a.writeRows(&b, len(a.treeRows), func(i int) string {
		return a.renderTreeRow(a.treeRows[i], pathW)
	})

	return b.String()
}

func (a *App) viewFocusList() string {
	var b strings.Builder

	// Focus header replaces filter bar.
	b.WriteString(styleHeader.Render(fmt.Sprintf("[%s]", a.focusKey)))
	b.WriteString("  ")
	b.WriteString(styleMuted.Render("Esc to unfocus"))
	b.WriteString("\n")

	pathW := a.focusPathWidth()
	hdr := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %*s %*s",
		colMethod, "METHOD",
		colStatus, "STATUS",
		colProcess, "PROCESS",
		pathW, "PATH",
		colType, "TYPE",
		colDur, "DUR",
		colSize, "SIZE",
	)
	a.writeColumnHeader(&b, hdr)

	if len(a.treeRows) == 0 {
		a.writeEmptyMessage(&b, "No flows for this host")
	}

	a.writeRows(&b, len(a.treeRows), func(i int) string {
		f := a.treeRows[i].Flow
		return renderFlowColumns(f, pathW, "", truncPad(processLabel(f.Process), colProcess))
	})

	return b.String()
}

// writeColumnHeader writes a styled header line and separator.
func (a *App) writeColumnHeader(b *strings.Builder, hdr string) {
	b.WriteString(styleHeader.Render(truncate(hdr, a.width)))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", a.width))
	b.WriteString("\n")
}

// writeEmptyMessage writes a muted placeholder line.
func (a *App) writeEmptyMessage(b *strings.Builder, msg string) {
	b.WriteString(styleMuted.Render(msg))
	b.WriteString("\n")
}

// writeRows renders visible rows with selection highlight, pads remaining space,
// and appends the status bar. Adjusts listOffset only when cursor escapes the
// visible window, keeping the viewport stable otherwise.
func (a *App) writeRows(b *strings.Builder, count int, renderFn func(int) string) {
	maxRows := max(a.height-5, 0)

	// Clamp listOffset: scroll down if cursor below viewport, up if above.
	if maxRows > 0 {
		if a.selectedIdx < a.listOffset {
			a.listOffset = a.selectedIdx
		}
		if a.selectedIdx >= a.listOffset+maxRows {
			a.listOffset = a.selectedIdx - maxRows + 1
		}
	}
	// Ensure offset doesn't exceed valid range.
	if a.listOffset > count-maxRows {
		a.listOffset = max(count-maxRows, 0)
	}

	visible := 0
	for i := a.listOffset; i < count && visible < maxRows; i++ {
		line := renderFn(i)
		if i == a.selectedIdx {
			line = styleSelected.Width(a.width).Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
		visible++
	}

	used := 4 + visible
	for used < a.height-1 {
		b.WriteString("\n")
		used++
	}

	b.WriteString(a.statusText())
}

func (a *App) writeFilterBar(b *strings.Builder) {
	if a.filterInput.Focused() {
		b.WriteString(a.filterInput.View())
	} else if a.filterText != "" {
		b.WriteString(styleMuted.Render(fmt.Sprintf("Filter: %s", a.filterText)))
	} else {
		b.WriteString(styleMuted.Render("/ to filter..."))
	}
	b.WriteString("\n")
}

func (a *App) columnWidths() (hostW, pathW int) {
	fixed := colMethod + colStatus + colProcess + colType + colDur + colSize + 7 // 7 spaces between columns
	remaining := max(a.width-fixed, 20)
	hostW = remaining * 40 / 100
	pathW = remaining - hostW
	return
}

// treePathWidth returns path column width for tree mode (no HOST column, 4-char indent).
func (a *App) treePathWidth() int {
	fixed := 4 + colMethod + colStatus + colProcess + colType + colDur + colSize + 6 // indent + gaps
	return max(a.width-fixed, 10)
}

// focusPathWidth returns path column width for focus mode (no HOST, no indent).
func (a *App) focusPathWidth() int {
	fixed := colMethod + colStatus + colProcess + colType + colDur + colSize + 6 // gaps
	return max(a.width-fixed, 10)
}

// processLabel returns the process name or an em dash if empty.
func processLabel(proc string) string {
	if proc == "" {
		return "\u2014"
	}
	return proc
}

func (a *App) renderTreeRow(row treeRow, pathW int) string {
	if row.IsHeader {
		return a.renderGroupNode(row.GroupKey)
	}
	proc := processLabel(row.Flow.Process)
	return "    " + renderFlowColumns(row.Flow, pathW, "", truncPad(proc, colProcess))
}

func (a *App) renderGroupNode(key string) string {
	icon := "▸"
	if a.groupExpanded[key] {
		icon = "▾"
	}
	keyFn := a.treeKeyFn()
	count := 0
	for _, f := range a.flows {
		if keyFn(f) == key {
			count++
		}
	}
	return fmt.Sprintf("%s %s (%d)", icon, key, count)
}

// renderFlowColumns formats method/status/path/dur/size columns.
// hostCol and processCol are inserted between status and path when non-empty.
func renderFlowColumns(f store.FlowMeta, pathW int, hostCol string, processCol string) string {
	method := styleMethod.Render(fmt.Sprintf("%-*s", colMethod, f.Method))

	var status string
	if f.StatusCode > 0 {
		status = statusStyle(f.StatusCode).Render(fmt.Sprintf("%-*d", colStatus, f.StatusCode))
	} else {
		status = styleMuted.Render(fmt.Sprintf("%-*s", colStatus, "..."))
	}

	path := truncPad(f.Path, pathW)
	ctype := styleMuted.Render(truncPad(shortContentType(f.ContentType), colType))

	var dur string
	if f.State == store.StateCompleted {
		dur = fmt.Sprintf("%*s", colDur, formatDuration(f.Duration))
	} else {
		dur = fmt.Sprintf("%*s", colDur, "...")
	}

	size := fmt.Sprintf("%*s", colSize, formatSize(f.SizeBytes))

	var cols []string
	cols = append(cols, method, status)
	if hostCol != "" {
		cols = append(cols, hostCol)
	}
	if processCol != "" {
		cols = append(cols, processCol)
	}
	cols = append(cols, path, ctype, dur, size)
	return strings.Join(cols, " ")
}

func (a *App) renderFlowRow(f store.FlowMeta, hostW, pathW int) string {
	return renderFlowColumns(f, pathW, truncPad(f.Host, hostW), truncPad(processLabel(f.Process), colProcess))
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

// shortContentType returns a compact label for common MIME types.
func shortContentType(ct string) string {
	if ct == "" {
		return ""
	}
	// Strip parameters (e.g. "; charset=utf-8").
	if idx := strings.IndexByte(ct, ';'); idx >= 0 {
		ct = ct[:idx]
	}
	ct = strings.TrimSpace(ct)

	// Well-known short labels.
	switch ct {
	case "application/json":
		return "json"
	case "application/xml", "text/xml":
		return "xml"
	case "text/html":
		return "html"
	case "text/plain":
		return "text"
	case "text/css":
		return "css"
	case "application/javascript", "text/javascript":
		return "js"
	case "application/octet-stream":
		return "binary"
	case "multipart/form-data":
		return "multipart"
	case "application/x-www-form-urlencoded":
		return "form"
	case "application/grpc":
		return "grpc"
	case "application/pdf":
		return "pdf"
	case "application/protobuf", "application/x-protobuf":
		return "protobuf"
	}

	// image/png → png, font/woff2 → woff2, etc.
	if strings.HasPrefix(ct, "image/") {
		return ct[6:]
	}
	if strings.HasPrefix(ct, "font/") {
		return ct[5:]
	}

	return ct
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
