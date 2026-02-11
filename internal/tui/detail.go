package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kostyay/httpmon/internal/highlight"
	"github.com/kostyay/httpmon/internal/store"
)

const maxBodyLines = 50

func (a *App) viewDetail() string {
	var b strings.Builder

	meta, _, _ := a.store.Get(a.selectedID)
	if meta == nil {
		return "Flow no longer available. Press any key to return."
	}

	// Header line
	url := fmt.Sprintf("%s://%s%s", meta.Scheme, meta.Host, meta.Path)
	hdr := fmt.Sprintf("< Esc    %s %d %s    %s  %s",
		meta.Method,
		meta.StatusCode,
		url,
		formatDuration(meta.Duration),
		formatSize(meta.SizeBytes),
	)
	b.WriteString(styleDetailHeader.Render(truncate(hdr, a.width)))
	b.WriteString("\n")

	// Separator
	b.WriteString(strings.Repeat("─", a.width))
	b.WriteString("\n")

	// Tabs
	reqTab := "[Request]"
	respTab := "Response"
	if a.detailTab == 1 {
		reqTab = "Request"
		respTab = "[Response]"
	}
	if a.detailTab == 0 {
		b.WriteString(styleActiveTab.Render(reqTab))
	} else {
		b.WriteString(styleInactiveTab.Render(reqTab))
	}
	b.WriteString("  ")
	if a.detailTab == 1 {
		b.WriteString(styleActiveTab.Render(respTab))
	} else {
		b.WriteString(styleInactiveTab.Render(respTab))
	}
	b.WriteString("\n\n")

	// Viewport content
	b.WriteString(a.detailVP.View())
	b.WriteString("\n")

	// Status
	scrollPct := fmt.Sprintf("%d%%", int(a.detailVP.ScrollPercent()*100))
	bar := fmt.Sprintf("n/N prev/next flow  j/k scroll  1/2 tabs  Esc back  %s", scrollPct)
	b.WriteString(styleStatusBar.Width(a.width).Render(truncate(bar, a.width)))

	return b.String()
}

func renderDetailBody(meta *store.FlowMeta, data *store.FlowData, tab int, width int, darkBg bool, prettyJSON bool) string {
	if meta == nil {
		return "Flow no longer available."
	}

	var b strings.Builder

	if tab == 0 {
		renderRequestDetail(&b, meta, data, darkBg, prettyJSON)
	} else {
		renderResponseDetail(&b, meta, data, darkBg, prettyJSON)
	}

	return b.String()
}

func renderRequestDetail(b *strings.Builder, meta *store.FlowMeta, data *store.FlowData, darkBg bool, prettyJSON bool) {
	b.WriteString(styleSection.Render("▸ General"))
	b.WriteString("\n")
	fmt.Fprintf(b, "  Method: %s\n", meta.Method)
	fmt.Fprintf(b, "  URL: %s://%s%s\n", meta.Scheme, meta.Host, meta.Path)
	fmt.Fprintf(b, "  Scheme: %s\n", meta.Scheme)
	b.WriteString("\n")

	if data != nil && data.RequestHeaders != nil {
		renderHeaders(b, "Request Headers", data.RequestHeaders)
	}

	if data != nil && len(data.RequestBody) > 0 {
		reqCT := ""
		if data.RequestHeaders != nil {
			reqCT = data.RequestHeaders.Get("Content-Type")
		}
		b.WriteString(styleSection.Render("▸ Body"))
		b.WriteString("\n")
		renderBody(b, data.RequestBody, reqCT, darkBg, prettyJSON)
	}
}

func renderResponseDetail(b *strings.Builder, meta *store.FlowMeta, data *store.FlowData, darkBg bool, prettyJSON bool) {
	if meta.State == store.StateInProgress {
		b.WriteString(styleMuted.Render("Awaiting response..."))
		b.WriteString("\n")
		return
	}

	b.WriteString(styleSection.Render("▸ General"))
	b.WriteString("\n")
	fmt.Fprintf(b, "  Status: %d\n", meta.StatusCode)
	fmt.Fprintf(b, "  Content-Type: %s\n", meta.ContentType)
	fmt.Fprintf(b, "  Duration: %s\n", formatDuration(meta.Duration))
	fmt.Fprintf(b, "  Size: %s\n", formatSize(meta.SizeBytes))
	b.WriteString("\n")

	if data != nil && data.ResponseHeaders != nil {
		renderHeaders(b, "Response Headers", data.ResponseHeaders)
	}

	if data != nil && len(data.ResponseBody) > 0 {
		b.WriteString(styleSection.Render("▸ Body"))
		b.WriteString("\n")
		renderBody(b, data.ResponseBody, meta.ContentType, darkBg, prettyJSON)
	}
}

func renderHeaders(b *strings.Builder, title string, h map[string][]string) {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	b.WriteString(styleSection.Render(fmt.Sprintf("▸ %s (%d)", title, len(h))))
	b.WriteString("\n")
	for _, k := range keys {
		for _, v := range h[k] {
			fmt.Fprintf(b, "  %s: %s\n", k, v)
		}
	}
	b.WriteString("\n")
}

func renderBody(b *strings.Builder, body []byte, contentType string, darkBg bool, prettyJSON bool) {
	highlighted := highlight.Highlight(body, contentType, darkBg, prettyJSON)
	lines := strings.Split(highlighted, "\n")
	if len(lines) > maxBodyLines {
		totalLines := len(lines)
		lines = lines[:maxBodyLines]
		lines = append(lines, styleMuted.Render(fmt.Sprintf("... truncated (%d lines total)", totalLines)))
	}
	for _, line := range lines {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("\n")
}
