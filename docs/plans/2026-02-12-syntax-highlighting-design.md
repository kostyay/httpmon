# Syntax Highlighting Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add syntax highlighting to request/response bodies in TUI detail view using chroma, with JSON pretty-print toggle.

**Architecture:** New `internal/highlight` package wraps chroma. `renderBody()` in `detail.go` calls `highlight.Highlight()` instead of writing raw text. Content-type auto-detected via `lexers.MatchMimeType()`. `p` key toggles JSON pretty-print.

**Tech Stack:** `github.com/alecthomas/chroma/v2`, existing Bubble Tea / lipgloss stack.

---

### Task 1: Add chroma dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add dependency**

Run: `cd /Users/kostya/personal/httpmon && go get github.com/alecthomas/chroma/v2`

**Step 2: Verify**

Run: `go build ./...`
Expected: compiles clean

**Step 3: Commit**

```
git add go.mod go.sum
git commit -m "build: add chroma/v2 dependency for syntax highlighting"
```

---

### Task 2: Create `internal/highlight` package — tests first

**Files:**
- Create: `internal/highlight/highlight.go` (stub)
- Create: `internal/highlight/highlight_test.go`

**Step 1: Write stub**

```go
// internal/highlight/highlight.go
package highlight

// Highlight applies syntax highlighting to body based on contentType.
// Returns ANSI-colored string. Falls back to plain text for unknown types.
func Highlight(body []byte, contentType string, darkBg bool, prettyJSON bool) string {
	return string(body)
}
```

**Step 2: Write failing tests**

```go
// internal/highlight/highlight_test.go
package highlight

import (
	"strings"
	"testing"
)

func TestJSONHighlighted(t *testing.T) {
	out := Highlight([]byte(`{"key":"value"}`), "application/json", true, false)
	// Chroma ANSI output contains escape codes
	if !strings.Contains(out, "\x1b[") {
		t.Error("JSON output should contain ANSI escape codes")
	}
}

func TestJSONPrettyPrint(t *testing.T) {
	out := Highlight([]byte(`{"a":1,"b":2}`), "application/json", true, true)
	if !strings.Contains(out, "\n") {
		t.Error("pretty-printed JSON should contain newlines")
	}
}

func TestJSONPrettyPrintDisabled(t *testing.T) {
	out := Highlight([]byte(`{"a":1}`), "application/json", true, false)
	// Should not add newlines beyond what chroma adds
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > 1 {
		t.Errorf("raw JSON should stay single-line, got %d lines", len(lines))
	}
}

func TestHTMLHighlighted(t *testing.T) {
	out := Highlight([]byte(`<html><body>Hello</body></html>`), "text/html", true, false)
	if !strings.Contains(out, "\x1b[") {
		t.Error("HTML output should contain ANSI escape codes")
	}
}

func TestUnknownContentTypePlaintext(t *testing.T) {
	out := Highlight([]byte("hello world"), "application/octet-stream", true, false)
	if out != "hello world" {
		t.Errorf("unknown content-type should return plain text, got %q", out)
	}
}

func TestEmptyBody(t *testing.T) {
	out := Highlight([]byte{}, "application/json", true, false)
	if out != "" {
		t.Errorf("empty body should return empty string, got %q", out)
	}
}

func TestContentTypeWithCharset(t *testing.T) {
	out := Highlight([]byte(`{"k":1}`), "application/json; charset=utf-8", true, false)
	if !strings.Contains(out, "\x1b[") {
		t.Error("should handle content-type with charset param")
	}
}

func TestDarkVsLightTheme(t *testing.T) {
	dark := Highlight([]byte(`{"k":1}`), "application/json", true, false)
	light := Highlight([]byte(`{"k":1}`), "application/json", false, false)
	// Both should have ANSI, but different codes (different themes)
	if !strings.Contains(dark, "\x1b[") {
		t.Error("dark theme should produce ANSI")
	}
	if !strings.Contains(light, "\x1b[") {
		t.Error("light theme should produce ANSI")
	}
}

func TestInvalidJSONPrettyPrintFallback(t *testing.T) {
	// Invalid JSON with pretty=true should still highlight, just not indent
	out := Highlight([]byte(`{not json}`), "application/json", true, true)
	if !strings.Contains(out, "\x1b[") {
		t.Error("invalid JSON should still get highlighted")
	}
}
```

**Step 3: Run tests to verify they fail**

Run: `go test -v ./internal/highlight/`
Expected: FAIL — stub returns raw text, no ANSI codes

**Step 4: Commit**

```
git add internal/highlight/
git commit -m "test: add highlight package tests (red)"
```

---

### Task 3: Implement `highlight.Highlight()`

**Files:**
- Modify: `internal/highlight/highlight.go`

**Step 1: Implement**

```go
package highlight

import (
	"bytes"
	"encoding/json"
	"mime"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Highlight applies syntax highlighting to body based on contentType.
// Returns ANSI-colored string. Falls back to plain text for unknown types.
func Highlight(body []byte, contentType string, darkBg bool, prettyJSON bool) string {
	if len(body) == 0 {
		return ""
	}

	mediaType, _, _ := mime.ParseMediaType(contentType)

	lexer := lexers.MatchMimeType(mediaType)
	if lexer == nil {
		return string(body)
	}

	src := body
	if prettyJSON && isJSON(mediaType) {
		if pretty, err := prettyPrintJSON(body); err == nil {
			src = pretty
		}
	}

	style := styles.Get("monokai")
	if !darkBg {
		style = styles.Get("github")
	}

	formatter := formatters.Get("terminal256")

	iter, err := lexer.Tokenise(nil, string(src))
	if err != nil {
		return string(src)
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iter); err != nil {
		return string(src)
	}

	return strings.TrimRight(buf.String(), "\n")
}

func isJSON(mediaType string) bool {
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func prettyPrintJSON(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
```

**Step 2: Run tests**

Run: `go test -v ./internal/highlight/`
Expected: ALL PASS

**Step 3: Commit**

```
git add internal/highlight/highlight.go
git commit -m "feat: implement syntax highlighting with chroma"
```

---

### Task 4: Wire highlighting into TUI — update `renderBody()` signature

**Files:**
- Modify: `internal/tui/detail.go`

This task changes `renderBody()` and its callers to pass content-type through, and calls `highlight.Highlight()`.

**Step 1: Update `renderBody` and callers**

In `detail.go`:
- Change `renderDetailBody` signature to accept `prettyJSON bool`
- Change `renderBody` signature to `renderBody(b *strings.Builder, body []byte, contentType string, darkBg bool, prettyJSON bool)`
- `renderRequestDetail`: get content-type from `data.RequestHeaders.Get("Content-Type")`
- `renderResponseDetail`: use `meta.ContentType`
- Call `highlight.Highlight()` in `renderBody()` instead of raw text

Updated `renderBody`:
```go
func renderBody(b *strings.Builder, body []byte, contentType string, darkBg bool, prettyJSON bool) {
	highlighted := highlight.Highlight(body, contentType, darkBg, prettyJSON)
	lines := strings.Split(highlighted, "\n")
	if len(lines) > maxBodyLines {
		lines = lines[:maxBodyLines]
		lines = append(lines, styleMuted.Render(fmt.Sprintf("... truncated (%d lines total)", len(strings.Split(highlighted, "\n")))))
	}
	for _, line := range lines {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("\n")
}
```

Updated callers in `renderRequestDetail`:
```go
contentType := ""
if data != nil && data.RequestHeaders != nil {
    contentType = data.RequestHeaders.Get("Content-Type")
}
// ...
renderBody(b, data.RequestBody, contentType, darkBg, prettyJSON)
```

Updated callers in `renderResponseDetail`:
```go
renderBody(b, data.ResponseBody, meta.ContentType, darkBg, prettyJSON)
```

Update `renderDetailBody` signature:
```go
func renderDetailBody(meta *store.FlowMeta, data *store.FlowData, tab int, width int, darkBg bool, prettyJSON bool) string
```

**Step 2: Update `updateDetailContent()` in `app.go`**

```go
func (a *App) updateDetailContent() {
	meta, data, err := a.store.Get(a.selectedID)
	if err != nil {
		a.detailVP.SetContent("Flow no longer available. Press Esc to return.")
		return
	}
	darkBg := lipgloss.HasDarkBackground()
	a.detailVP.SetContent(renderDetailBody(meta, data, a.detailTab, a.width, darkBg, !a.detailRaw))
}
```

**Step 3: Run tests**

Run: `go test -v ./internal/tui/`
Expected: ALL PASS (existing tests don't check body content format strictly)

**Step 4: Commit**

```
git add internal/tui/detail.go internal/tui/app.go
git commit -m "feat: wire syntax highlighting into detail body rendering"
```

---

### Task 5: Add `p` toggle + status bar hint

**Files:**
- Modify: `internal/tui/app.go` (add `detailRaw` field, `p` key handler)
- Modify: `internal/tui/detail.go` (status bar hint)

**Step 1: Write failing test**

Add to `app_test.go`:
```go
func TestPToggleRaw(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if app.detailRaw {
		t.Error("detailRaw should default to false")
	}

	sendKey(app, "p")
	if !app.detailRaw {
		t.Error("p should toggle detailRaw to true")
	}

	sendKey(app, "p")
	if app.detailRaw {
		t.Error("second p should toggle detailRaw back to false")
	}
}

func TestDetailStatusBarShowsPrettyRaw(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := app.View()
	if !strings.Contains(view, "p:pretty") {
		t.Error("status bar should show p:pretty when detailRaw=false")
	}

	sendKey(app, "p")
	view = app.View()
	if !strings.Contains(view, "p:raw") {
		t.Error("status bar should show p:raw when detailRaw=true")
	}
}
```

**Step 2: Run to verify fail**

Run: `go test -v -run TestPToggle ./internal/tui/`
Expected: FAIL

**Step 3: Implement**

In `app.go` `App` struct — add field:
```go
detailRaw bool // false=pretty-print, true=raw
```

In `updateDetail()` — add case:
```go
case "p":
    a.detailRaw = !a.detailRaw
    a.updateDetailContent()
```

In `detail.go` `viewDetail()` — update status bar line:
```go
prettyLabel := "p:pretty"
if a.detailRaw {
    prettyLabel = "p:raw"
}
bar := fmt.Sprintf("n/N prev/next flow  j/k scroll  1/2 tabs  %s  Esc back  %s", prettyLabel, scrollPct)
```

**Step 4: Run tests**

Run: `go test -v ./internal/tui/`
Expected: ALL PASS

**Step 5: Commit**

```
git add internal/tui/app.go internal/tui/detail.go internal/tui/app_test.go
git commit -m "feat: add p key toggle for pretty-print/raw body display"
```

---

### Task 6: Full gate

**Step 1:** Run: `make all`
Expected: lint + test + build all pass

**Step 2:** Fix any issues

**Step 3: Final commit if needed, then done**

---

## Unresolved Questions

None.
