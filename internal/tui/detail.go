package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kostyay/httpmon/internal/bodydecoder"
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
	b.WriteString(renderTab("Request", a.detailTab == 0))
	b.WriteString("  ")
	b.WriteString(renderTab("Response", a.detailTab == 1))
	b.WriteString("\n\n")

	// Viewport content
	b.WriteString(a.detailVP.View())
	b.WriteString("\n")

	// Status
	scrollPct := fmt.Sprintf("%d%%", int(a.detailVP.ScrollPercent()*100))
	mode := "pretty"
	if a.detailRaw {
		mode = "raw"
	}
	imageHint := ""
	if a.detailBodyIsImage() {
		if a.detailImagePreview {
			imageHint = "  i:text"
		} else {
			imageHint = "  i:image"
		}
	}
	var bar string
	if a.detailSearch {
		matchInfo := fmt.Sprintf("%d/%d matches", a.searchMatchIdx+1, a.searchMatchCount)
		if a.searchMatchCount == 0 {
			matchInfo = "0 matches"
		}
		if a.searchInput.Focused() {
			bar = fmt.Sprintf("/%s  %s  Enter:commit  Esc:cancel", a.searchInput.Value(), matchInfo)
		} else {
			bar = fmt.Sprintf("search: %s  %s  n/N:next/prev  Esc:clear", a.searchQuery, matchInfo)
		}
	} else if a.detailDecodeErr != "" {
		bar = fmt.Sprintf("proto: %s  %s", a.detailDecodeErr, scrollPct)
	} else {
		bar = fmt.Sprintf("1/2 tabs  p:%s  e:edit%s  g/h/b:sections  Space:actions  Esc back  %s", mode, imageHint, scrollPct)
	}
	b.WriteString(styleStatusBar.Width(a.width).Render(truncate(bar, a.width)))

	return b.String()
}

// detailBodyIsImage returns true if the currently viewed tab's body is a renderable image.
func (a *App) detailBodyIsImage() bool {
	_, data, err := a.store.Get(a.selectedID)
	if err != nil || data == nil {
		return false
	}
	var ct string
	if a.detailTab == 0 {
		ct = data.RequestHeaders.Get("Content-Type")
	} else {
		ct = data.ResponseHeaders.Get("Content-Type")
	}
	return isRenderableImage(ct)
}

func renderTab(label string, active bool) string {
	if active {
		return styleActiveTab.Render("[" + label + "]")
	}
	return styleInactiveTab.Render(label)
}

func sectionIcon(collapsed bool) string {
	if collapsed {
		return "▸"
	}
	return "▾"
}

// renderOpts bundles display options threaded through the detail render pipeline.
type renderOpts struct {
	DarkBg     bool
	PrettyJSON bool
	Collapsed  map[string]bool
	Decoder    BodyDecoderRegistry
}

func renderDetailBody(meta *store.FlowMeta, data *store.FlowData, tab int, opts renderOpts) (string, string) {
	if meta == nil {
		return "Flow no longer available.", ""
	}

	var b strings.Builder
	var decErr string

	if tab == 0 {
		decErr = renderRequestDetail(&b, meta, data, opts)
	} else {
		decErr = renderResponseDetail(&b, meta, data, opts)
	}

	return b.String(), decErr
}

func renderRequestDetail(b *strings.Builder, meta *store.FlowMeta, data *store.FlowData, opts renderOpts) string {
	b.WriteString(styleSection.Render(sectionIcon(opts.Collapsed["general"]) + " General"))
	b.WriteString("\n")
	if !opts.Collapsed["general"] {
		fmt.Fprintf(b, "  Method: %s\n", meta.Method)
		fmt.Fprintf(b, "  URL: %s://%s%s\n", meta.Scheme, meta.Host, meta.Path)
		fmt.Fprintf(b, "  Scheme: %s\n", meta.Scheme)
	}
	b.WriteString("\n")

	if data != nil && data.ProcessPID != 0 {
		renderProcessSection(b, meta, data, opts.Collapsed["process"])
	}

	if data == nil {
		return ""
	}

	if data.RequestHeaders != nil {
		renderHeaders(b, "Request Headers", data.RequestHeaders, opts.Collapsed["headers"])
	}

	if len(data.RequestBody) > 0 {
		b.WriteString(styleSection.Render(sectionIcon(opts.Collapsed["body"]) + " Body"))
		b.WriteString("\n")
		if !opts.Collapsed["body"] {
			dm := bodydecoder.DecoderMetadata{RequestPath: meta.Path, IsRequest: true}
			return renderBody(b, data.RequestBody, data.RequestHeaders.Get("Content-Type"), opts, dm)
		}
		b.WriteString("\n")
	}
	return ""
}

func renderResponseDetail(b *strings.Builder, meta *store.FlowMeta, data *store.FlowData, opts renderOpts) string {
	if meta.State == store.StateInProgress {
		b.WriteString(styleMuted.Render("Awaiting response..."))
		b.WriteString("\n")
		return ""
	}

	b.WriteString(styleSection.Render(sectionIcon(opts.Collapsed["general"]) + " General"))
	b.WriteString("\n")
	if !opts.Collapsed["general"] {
		fmt.Fprintf(b, "  Status: %d\n", meta.StatusCode)
		fmt.Fprintf(b, "  Content-Type: %s\n", meta.ContentType)
		fmt.Fprintf(b, "  Duration: %s\n", formatDuration(meta.Duration))
		fmt.Fprintf(b, "  Size: %s\n", formatSize(meta.SizeBytes))
	}
	b.WriteString("\n")

	if data == nil {
		return ""
	}

	if data.ResponseHeaders != nil {
		renderHeaders(b, "Response Headers", data.ResponseHeaders, opts.Collapsed["headers"])
	}

	if len(data.ResponseBody) > 0 {
		b.WriteString(styleSection.Render(sectionIcon(opts.Collapsed["body"]) + " Body"))
		b.WriteString("\n")
		if !opts.Collapsed["body"] {
			dm := bodydecoder.DecoderMetadata{RequestPath: meta.Path, IsRequest: false}
			return renderBody(b, data.ResponseBody, meta.ContentType, opts, dm)
		}
		b.WriteString("\n")
	}
	return ""
}

func renderHeaders(b *strings.Builder, title string, h map[string][]string, collapsed bool) {
	b.WriteString(styleSection.Render(fmt.Sprintf("%s %s (%d)", sectionIcon(collapsed), title, len(h))))
	b.WriteString("\n")
	if !collapsed {
		keys := make([]string, 0, len(h))
		for k := range h {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			for _, v := range h[k] {
				fmt.Fprintf(b, "  %s: %s\n", k, v)
			}
		}
	}
	b.WriteString("\n")
}

const maxCmdlineLen = 100

func renderProcessSection(b *strings.Builder, meta *store.FlowMeta, data *store.FlowData, collapsed bool) {
	b.WriteString(styleSection.Render(sectionIcon(collapsed) + " Process"))
	b.WriteString("\n")
	if !collapsed {
		fmt.Fprintf(b, "  Name: %s\n", meta.Process)
		fmt.Fprintf(b, "  PID:  %d\n", data.ProcessPID)
		if data.ProcessCmdline != "" {
			cmd := data.ProcessCmdline
			if len(cmd) > maxCmdlineLen {
				cmd = cmd[:maxCmdlineLen] + "..."
			}
			fmt.Fprintf(b, "  Cmd:  %s\n", cmd)
		}
	}
	b.WriteString("\n")
}

func renderBody(b *strings.Builder, body []byte, contentType string, opts renderOpts, meta bodydecoder.DecoderMetadata) string {
	displayBody := body
	displayCT := contentType
	var decErr string

	if opts.Decoder != nil {
		decoded, resultCT, err := opts.Decoder.Decode(body, contentType, meta)
		if err == nil {
			displayBody = []byte(decoded)
			displayCT = resultCT
		} else if !errors.Is(err, bodydecoder.ErrNoDecoder) {
			decErr = err.Error()
		}
	}

	highlighted := highlight.Highlight(displayBody, displayCT, opts.DarkBg, opts.PrettyJSON)
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
	return decErr
}
