package tui

import (
	"strings"
	"testing"
)

func TestViewDiffContent(t *testing.T) {
	app := newMockApp(3)
	app.showDiff = true
	app.diffContent = "line1\nline2\nline3"
	app.width = 80
	app.height = 30

	view := app.viewDiff()
	if !strings.Contains(view, "Flow Diff") {
		t.Error("diff view should contain 'Flow Diff' header")
	}
	if !strings.Contains(view, "line1") {
		t.Error("diff view should contain diff content")
	}
	if !strings.Contains(view, "Esc:back") {
		t.Error("diff view should show Esc hint")
	}
}
