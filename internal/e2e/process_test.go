//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestE2E_ProcessResolution(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	h.doGet(t, "/api/process")
	h.waitForText(t, "/api/process")

	if !h.tryWaitForProcess(2 * time.Second) {
		t.Skip("process resolution not available (OS-dependent)")
	}

	flows, _ := h.store.List(nil, 0, 0)
	if flows[0].Process == "" {
		t.Error("process name should be resolved")
	}
}

func TestE2E_ProcessColumnInFlatView(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	h.doGet(t, "/api/proc-flat")
	h.waitForText(t, "/api/proc-flat")

	view := h.view()
	if !strings.Contains(view, "PROCESS") {
		t.Errorf("flat view should show PROCESS column, got:\n%s", view)
	}
}

func TestE2E_ProcessInDetailCard(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	h.doGet(t, "/api/proc-detail")
	h.waitForText(t, "/api/proc-detail")

	if !h.tryWaitForProcess(2 * time.Second) {
		t.Skip("process resolution not available (OS-dependent)")
	}

	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	view := h.view()
	if !strings.Contains(view, "Process") {
		t.Errorf("detail should show Process section, got:\n%s", view)
	}
	if !strings.Contains(view, "PID:") {
		t.Errorf("detail should show PID field, got:\n%s", view)
	}
}

func TestE2E_ProcessTreeMode(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	h.doGet(t, "/api/tree1")
	h.doGet(t, "/api/tree2")
	h.waitForCondition(t, "2 flows", func() bool {
		return h.store.Len() >= 2
	})
	h.tick()

	// flat -> host-tree -> process-tree
	h.sendKey("t")
	h.tick()
	h.sendKey("t")
	h.tick()

	view := h.view()
	if !strings.Contains(view, "▸") && !strings.Contains(view, "▾") {
		t.Errorf("process tree should show group markers, got:\n%s", view)
	}
	if !strings.Contains(view, "t:flat") {
		t.Errorf("process tree status should show t:flat, got:\n%s", view)
	}
}

func TestE2E_ProcessTreeFocus(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	h.doGet(t, "/api/focus1")
	h.waitForText(t, "/api/focus1")
	h.tick()

	// flat -> host-tree -> process-tree
	h.sendKey("t")
	h.tick()
	h.sendKey("t")
	h.tick()

	h.sendKey("f")
	h.tick()

	view := h.view()
	if !strings.Contains(view, "Esc to unfocus") {
		t.Errorf("focus mode should show unfocus hint, got:\n%s", view)
	}
}
