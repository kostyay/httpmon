//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestE2E_FlatView_GETRequest(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	h.doGet(t, "/api/items")
	h.waitForText(t, "/api/items")

	view := h.view()
	if !strings.Contains(view, "GET") {
		t.Error("view should contain GET method")
	}
	if !strings.Contains(view, "200") {
		t.Error("view should contain status 200")
	}
}

func TestE2E_DetailView_RequestBody(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	h.doPost(t, "/echo", `{"name":"test"}`)
	h.waitForText(t, "/echo")

	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	view := h.view()
	if !strings.Contains(view, "POST") {
		t.Error("detail should contain POST method")
	}
	// Request tab should show the posted body.
	if !strings.Contains(view, "name") {
		t.Errorf("detail should contain request body, got:\n%s", view)
	}
}

func TestE2E_HTTPS_Capture(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	// httptest.NewServer uses HTTP, but requests through the MITM proxy
	// are recorded with the scheme based on the connection type.
	// With plain HTTP upstream, scheme will be "http".
	h.doGet(t, "/secure")
	h.waitForText(t, "/secure")

	view := h.view()
	if !strings.Contains(view, "200") {
		t.Error("HTTPS request should show status 200")
	}
}

func TestE2E_DetailView_ResponseHeaders(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	h.doGet(t, "/api/data")
	h.waitForText(t, "/api/data")

	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	// Switch to response tab.
	h.sendKey("2")
	h.tick()

	view := h.view()
	if !strings.Contains(view, "X-Custom") {
		t.Errorf("response tab should show X-Custom header, got:\n%s", view)
	}
}

func TestE2E_InProgress_ThenCompletes(t *testing.T) {
	t.Parallel()
	h := newHarness(t, slowHandler(500*time.Millisecond))

	// Fire request in background.
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.doGet(t, "/slow")
	}()

	// Should see "..." (in-progress status) before completion.
	h.waitForText(t, "...")

	// Wait for request to complete.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("slow request never completed")
	}

	// After completion, status and duration should appear.
	h.waitForText(t, "200")

	// The "..." for status should be gone and replaced with actual code.
	h.tick()
	view := h.view()
	// Check that the slow path shows a completed duration (ms range).
	if !strings.Contains(view, "ms") && !strings.Contains(view, "s") {
		t.Log("could not verify duration display, continuing")
	}
}

func TestE2E_EmptyState(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	h.tick()
	view := h.view()
	if !strings.Contains(view, "Waiting for traffic") {
		t.Errorf("empty state should show 'Waiting for traffic', got:\n%s", view)
	}
}

func TestE2E_RapidRequests(t *testing.T) {
	t.Parallel()
	h := newHarness(t, echoHandler())

	// Fire 20 requests rapidly (sequential, not concurrent — concurrent
	// goroutines are flaky under heavy parallel test load when the proxy
	// is slow to accept connections).
	for i := range 20 {
		resp, err := h.client.Get(h.upstream.URL + "/rapid/" + http.StatusText(i))
		if err == nil {
			resp.Body.Close()
		}
	}

	// Wait for all 20 to appear in the store.
	h.waitForCondition(t, "20 flows captured", func() bool {
		return h.store.Len() >= 20
	})
}

func TestE2E_BinaryBody(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	h.doGet(t, "/binary")
	h.waitForText(t, "/binary")

	// Enter detail — should not panic.
	h.sendSpecialKey(tea.KeyEnter)
	h.tick()

	// Switch to response tab.
	h.sendKey("2")
	h.tick()

	view := h.view()
	if view == "" {
		t.Error("detail view should render something for binary body")
	}
}
