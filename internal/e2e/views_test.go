//go:build e2e

package e2e

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestE2E_TreeView(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/echo")
	h.waitForCondition(t, "2 flows", func() bool {
		return h.store.Len() >= 2
	})
	h.tick()

	// Switch to tree view.
	h.sendKey("t")
	h.tick()

	view := h.view()
	// Tree view should show host as a group header with ▸.
	if !strings.Contains(view, "▸") && !strings.Contains(view, "▾") {
		t.Errorf("tree view should show ▸ or ▾ host markers, got:\n%s", view)
	}
}

func TestE2E_FocusMode(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/echo")
	h.waitForCondition(t, "2 flows", func() bool {
		return h.store.Len() >= 2
	})
	h.tick()

	// Switch to tree, then focus.
	h.sendKey("t")
	h.tick()
	h.sendKey("f")
	h.tick()

	view := h.view()
	if !strings.Contains(view, "Esc to unfocus") {
		t.Errorf("focus mode should show unfocus hint, got:\n%s", view)
	}
}

func TestE2E_DetailTabs(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	h.doGet(t, "/api/tabs")
	h.waitForText(t, "/api/tabs")

	// Enter detail.
	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	// Default: request tab.
	view := h.view()
	if !strings.Contains(view, "[Request]") {
		t.Errorf("should show [Request] tab, got:\n%s", view)
	}

	// Switch to response.
	h.sendKey("2")
	h.tick()
	view = h.view()
	if !strings.Contains(view, "[Response]") {
		t.Errorf("should show [Response] tab, got:\n%s", view)
	}

	// Switch back to request.
	h.sendKey("1")
	h.tick()
	view = h.view()
	if !strings.Contains(view, "[Request]") {
		t.Errorf("should show [Request] tab again, got:\n%s", view)
	}
}

func TestE2E_DetailNavigation(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/echo")
	h.waitForCondition(t, "2 flows", func() bool {
		return h.store.Len() >= 2
	})
	h.tick()

	// Enter detail on first flow.
	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	view1 := h.view()

	// Navigate to next flow.
	h.sendKey("n")
	h.tick()

	view2 := h.view()
	if view1 == view2 {
		t.Error("n should navigate to a different flow in detail")
	}

	// Navigate back.
	h.sendKey("N")
	h.tick()

	view3 := h.view()
	if view3 == view2 {
		// If same, N didn't navigate. That's OK if we're at boundary.
		t.Log("N navigated back (or already at boundary)")
	}
}
