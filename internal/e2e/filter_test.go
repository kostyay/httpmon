//go:build e2e

package e2e

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestE2E_QuickFilter(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/echo")
	h.waitForCondition(t, "2 flows", func() bool {
		return h.store.Len() >= 2
	})
	h.tick()

	// Activate filter and type "json".
	h.sendKey("/")
	h.typeText("json")
	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	view := h.view()
	if !strings.Contains(view, "/json") {
		t.Errorf("filter should show /json flow, got:\n%s", view)
	}
	if strings.Contains(view, "/echo") {
		t.Error("filter should hide /echo flow")
	}
}

func TestE2E_AdvancedFilter_Status(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/status/404")
	h.waitForCondition(t, "2 flows", func() bool {
		return h.store.Len() >= 2
	})
	h.tick()

	// Filter by status 404.
	h.sendKey("/")
	h.typeText("s:404")
	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	view := h.view()
	if !strings.Contains(view, "404") {
		t.Errorf("filter should show 404 flow, got:\n%s", view)
	}
	if strings.Contains(view, "/json") {
		t.Error("filter should hide 200 flow")
	}
}

func TestE2E_AdvancedFilter_Method(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doPost(t, "/echo", `{"x":1}`)
	h.waitForCondition(t, "2 flows", func() bool {
		return h.store.Len() >= 2
	})
	h.tick()

	// Filter by method POST.
	h.sendKey("/")
	h.typeText("m:POST")
	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	view := h.view()
	if !strings.Contains(view, "POST") {
		t.Errorf("filter should show POST flow, got:\n%s", view)
	}
	// The GET /json flow should be hidden.
	if strings.Contains(view, "/json") {
		t.Error("filter should hide GET /json flow")
	}
}

func TestE2E_AdvancedFilter_Negate(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/echo")
	h.waitForCondition(t, "2 flows", func() bool {
		return h.store.Len() >= 2
	})
	h.tick()

	// Filter excluding "json".
	h.sendKey("/")
	h.typeText("!json")
	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	view := h.view()
	if strings.Contains(view, "/json") {
		t.Error("negated filter should hide /json flow")
	}
	if !strings.Contains(view, "/echo") {
		t.Errorf("negated filter should show /echo flow, got:\n%s", view)
	}
}

func TestE2E_AdvancedFilter_Regex(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/binary")
	h.waitForCondition(t, "2 flows", func() bool {
		return h.store.Len() >= 2
	})
	h.tick()

	// Filter by regex matching "bin".
	h.sendKey("/")
	h.typeText("re:/bin/")
	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	view := h.view()
	if !strings.Contains(view, "/binary") {
		t.Errorf("regex filter should show /binary flow, got:\n%s", view)
	}
	if strings.Contains(view, "/json") {
		t.Error("regex filter should hide /json flow")
	}
}

func TestE2E_Filter_Clear(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	h.doGet(t, "/json")
	h.doGet(t, "/echo")
	h.waitForCondition(t, "2 flows", func() bool {
		return h.store.Len() >= 2
	})
	h.tick()

	// Apply filter.
	h.sendKey("/")
	h.typeText("json")
	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	// Verify filter is active.
	view := h.view()
	if strings.Contains(view, "/echo") {
		t.Fatal("filter should hide /echo before clear")
	}

	// Clear: open filter, ctrl+u to clear, enter.
	h.sendKey("/")
	h.app.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	view = h.view()
	if !strings.Contains(view, "/echo") {
		t.Errorf("after clear, /echo should be visible, got:\n%s", view)
	}
	if !strings.Contains(view, "/json") {
		t.Errorf("after clear, /json should be visible, got:\n%s", view)
	}
}
