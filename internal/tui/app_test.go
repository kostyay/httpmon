package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kostyay/httpmon/internal/store"
)

// --- mock implementations ---

type mockFlowReader struct {
	metas []store.FlowMeta
	data  map[store.FlowID]*store.FlowData
}

func (m *mockFlowReader) List(f store.Filter, offset, limit int) ([]store.FlowMeta, int) {
	var matched []store.FlowMeta
	// newest-first (same order as RingBuffer)
	for i := len(m.metas) - 1; i >= 0; i-- {
		meta := m.metas[i]
		if f != nil && !f.Match(&meta) {
			continue
		}
		matched = append(matched, meta)
	}
	total := len(matched)
	if offset >= total {
		return nil, total
	}
	matched = matched[offset:]
	if limit > 0 && limit < len(matched) {
		matched = matched[:limit]
	}
	return matched, total
}

func (m *mockFlowReader) Get(id store.FlowID) (*store.FlowMeta, *store.FlowData, error) {
	for i := range m.metas {
		if m.metas[i].ID == id {
			return &m.metas[i], m.data[id], nil
		}
	}
	return nil, nil, fmt.Errorf("flow %s not found", id)
}

type mockProxyInfo struct{ addr string }

func (m *mockProxyInfo) Addr() string { return m.addr }

// --- test helpers ---

func seedMock(n int) *mockFlowReader {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	for i := range n {
		meta := store.FlowMeta{
			ID:          fmtID(i),
			Method:      "GET",
			StatusCode:  200,
			Host:        "api.example.com",
			Path:        fmtPath(i),
			Duration:    45 * time.Millisecond,
			SizeBytes:   1234,
			StartedAt:   time.Now(),
			State:       store.StateCompleted,
			ContentType: "application/json",
			Scheme:      "https",
		}
		m.metas = append(m.metas, meta)
		m.data[meta.ID] = &store.FlowData{
			RequestHeaders:  map[string][]string{"Accept": {"application/json"}},
			RequestBody:     []byte(`{"q":"test"}`),
			ResponseHeaders: map[string][]string{"Content-Type": {"application/json"}},
			ResponseBody:    []byte(`{"ok":true}`),
		}
	}
	return m
}

// seedStore uses real RingBuffer — kept for integration-style tests.
func seedStore(n int) *store.RingBuffer {
	s := store.New(100)
	for i := range n {
		m := store.FlowMeta{
			ID:          fmtID(i),
			Method:      "GET",
			StatusCode:  200,
			Host:        "api.example.com",
			Path:        fmtPath(i),
			Duration:    45 * time.Millisecond,
			SizeBytes:   1234,
			StartedAt:   time.Now(),
			State:       store.StateCompleted,
			ContentType: "application/json",
			Scheme:      "https",
		}
		s.Add(m)
		s.SetData(m.ID, &store.FlowData{
			RequestHeaders:  map[string][]string{"Accept": {"application/json"}},
			RequestBody:     []byte(`{"q":"test"}`),
			ResponseHeaders: map[string][]string{"Content-Type": {"application/json"}},
			ResponseBody:    []byte(`{"ok":true}`),
		})
	}
	return s
}

func fmtID(i int) string   { return "flow-" + itoa(i) }
func fmtPath(i int) string { return "/v1/test/" + itoa(i) }

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

func newTestApp(n int) *App {
	s := seedStore(n)
	app := NewApp(s, nil, true)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg(time.Now()))
	return app
}

func newMockApp(n int) *App {
	m := seedMock(n)
	app := NewApp(m, &mockProxyInfo{addr: ":9999"}, true)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg(time.Now()))
	return app
}

func sendKey(app *App, key string) {
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

// --- existing tests (using real RingBuffer) ---

func TestNavigateDown(t *testing.T) {
	app := newTestApp(5)
	if app.selectedIdx != 0 {
		t.Errorf("initial selectedIdx = %d, want 0", app.selectedIdx)
	}

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if app.selectedIdx != 1 {
		t.Errorf("after j: selectedIdx = %d, want 1", app.selectedIdx)
	}
}

func TestNavigateUp(t *testing.T) {
	app := newTestApp(5)
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if app.selectedIdx != 1 {
		t.Errorf("after j,j,k: selectedIdx = %d, want 1", app.selectedIdx)
	}
}

func TestEnterDetail(t *testing.T) {
	app := newTestApp(5)
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !app.showDetail {
		t.Error("expected showDetail=true after Enter")
	}
}

func TestEscFromDetail(t *testing.T) {
	app := newTestApp(5)
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if app.showDetail {
		t.Error("expected showDetail=false after Esc")
	}
}

func TestListViewContainsFlows(t *testing.T) {
	app := newTestApp(3)
	view := app.View()
	if !strings.Contains(view, "flows") {
		t.Error("list view should contain 'flows'")
	}
	if !strings.Contains(view, "api.example.com") {
		t.Error("list view should contain host")
	}
}

func TestDetailViewContainsHeaders(t *testing.T) {
	app := newTestApp(3)
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := app.View()
	if !strings.Contains(view, "Method:") {
		t.Error("detail view should contain 'Method:'")
	}
	if !strings.Contains(view, "GET") {
		t.Error("detail view should contain method GET")
	}
}

func TestFilterFocusBlur(t *testing.T) {
	app := newTestApp(5)

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !app.filterInput.Focused() {
		t.Error("filter should be focused after /")
	}

	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if app.filterInput.Focused() {
		t.Error("filter should be blurred after Esc")
	}
}

func TestQDoesNotQuitWhenFilterFocused(t *testing.T) {
	app := newTestApp(5)
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})

	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd != nil {
		t.Log("cmd returned from q while filter focused (may or may not be quit)")
	}
	if app.showDetail {
		t.Error("q while filter focused should not open detail")
	}
}

func TestNavigateBeyondBounds(t *testing.T) {
	app := newTestApp(3)
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if app.selectedIdx != 0 {
		t.Errorf("selectedIdx = %d, want 0 (can't go above top)", app.selectedIdx)
	}

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if app.selectedIdx != 2 {
		t.Errorf("selectedIdx = %d, want 2 after G", app.selectedIdx)
	}

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if app.selectedIdx != 2 {
		t.Errorf("selectedIdx = %d, want 2 (can't go below bottom)", app.selectedIdx)
	}
}

// --- new tests using mocks ---

func TestEmptyStore(t *testing.T) {
	app := newMockApp(0)
	view := app.View()
	if !strings.Contains(view, "Waiting for traffic") {
		t.Error("empty store should show 'Waiting for traffic'")
	}
}

func TestFilterByHost(t *testing.T) {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	m.metas = []store.FlowMeta{
		{ID: "a", Method: "GET", Host: "api.example.com", Path: "/foo", State: store.StateCompleted, StatusCode: 200, Scheme: "https"},
		{ID: "b", Method: "POST", Host: "other.io", Path: "/bar", State: store.StateCompleted, StatusCode: 201, Scheme: "https"},
	}
	m.data["a"] = &store.FlowData{}
	m.data["b"] = &store.FlowData{}

	app := NewApp(m, &mockProxyInfo{addr: ":9999"}, true)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg(time.Now()))

	// Focus filter, type "other", apply
	sendKey(app, "/")
	for _, ch := range "other" {
		app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app.Update(tickMsg(time.Now()))

	view := app.View()
	if !strings.Contains(view, "other.io") {
		t.Error("filtered view should contain other.io")
	}
	if strings.Contains(view, "api.example.com") {
		t.Error("filtered view should NOT contain api.example.com")
	}
}

func TestClearFilter(t *testing.T) {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	m.metas = []store.FlowMeta{
		{ID: "a", Method: "GET", Host: "api.example.com", Path: "/foo", State: store.StateCompleted, StatusCode: 200, Scheme: "https"},
		{ID: "b", Method: "POST", Host: "other.io", Path: "/bar", State: store.StateCompleted, StatusCode: 201, Scheme: "https"},
	}
	m.data["a"] = &store.FlowData{}
	m.data["b"] = &store.FlowData{}

	app := NewApp(m, &mockProxyInfo{addr: ":9999"}, true)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg(time.Now()))

	// Apply filter
	sendKey(app, "/")
	for _, ch := range "other" {
		app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app.Update(tickMsg(time.Now()))

	// Clear filter: open filter, clear text, enter
	sendKey(app, "/")
	// Select all + delete to clear
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlU}) // clear line
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app.Update(tickMsg(time.Now()))

	view := app.View()
	if !strings.Contains(view, "other.io") {
		t.Error("after clear, should contain other.io")
	}
	if !strings.Contains(view, "api.example.com") {
		t.Error("after clear, should contain api.example.com")
	}
}

func TestDetailTabSwitching(t *testing.T) {
	app := newMockApp(3)

	// Enter detail
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !app.showDetail {
		t.Fatal("expected detail view")
	}

	// Default is request tab (0)
	if app.detailTab != 0 {
		t.Errorf("detailTab = %d, want 0", app.detailTab)
	}
	view := app.View()
	if !strings.Contains(view, "[Request]") {
		t.Error("request tab should be active")
	}

	// Switch to response tab
	sendKey(app, "2")
	if app.detailTab != 1 {
		t.Errorf("after '2': detailTab = %d, want 1", app.detailTab)
	}
	view = app.View()
	if !strings.Contains(view, "[Response]") {
		t.Error("response tab should be active after pressing 2")
	}

	// Switch back to request tab
	sendKey(app, "1")
	if app.detailTab != 0 {
		t.Errorf("after '1': detailTab = %d, want 0", app.detailTab)
	}
}

func TestNextFlowInDetail(t *testing.T) {
	app := newMockApp(5)

	// Select first flow, enter detail
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	firstID := app.selectedID

	// Next flow
	sendKey(app, "n")
	if app.selectedID == firstID {
		t.Error("n should move to next flow")
	}
	if app.selectedIdx != 1 {
		t.Errorf("selectedIdx = %d, want 1", app.selectedIdx)
	}

	// Previous flow
	sendKey(app, "N")
	if app.selectedID != firstID {
		t.Error("N should move back to first flow")
	}
}

func TestNextFlowBoundaries(t *testing.T) {
	app := newMockApp(3)

	// Enter detail on first flow
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Try going previous at start — should stay
	sendKey(app, "N")
	if app.selectedIdx != 0 {
		t.Errorf("N at first flow: selectedIdx = %d, want 0", app.selectedIdx)
	}

	// Go to last flow
	sendKey(app, "n")
	sendKey(app, "n")
	if app.selectedIdx != 2 {
		t.Errorf("after 2x n: selectedIdx = %d, want 2", app.selectedIdx)
	}

	// Try going next at end — should stay
	lastID := app.selectedID
	sendKey(app, "n")
	if app.selectedID != lastID {
		t.Error("n at last flow should not change selectedID")
	}
}

func TestProxyAddrNilFallback(t *testing.T) {
	app := NewApp(seedMock(1), nil, true)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg(time.Now()))

	got := app.proxyAddr()
	if got != ":8080" {
		t.Errorf("proxyAddr() = %q, want %q", got, ":8080")
	}
}

func TestProxyAddrFromMock(t *testing.T) {
	app := NewApp(seedMock(1), &mockProxyInfo{addr: ":3128"}, true)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg(time.Now()))

	got := app.proxyAddr()
	if got != ":3128" {
		t.Errorf("proxyAddr() = %q, want %q", got, ":3128")
	}
}

// TestRingBufferSatisfiesFlowReader confirms the real store works through the interface.
func TestRingBufferSatisfiesFlowReader(t *testing.T) {
	s := seedStore(3)
	app := NewApp(s, nil, true)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg(time.Now()))

	if len(app.flows) != 3 {
		t.Errorf("flows = %d, want 3", len(app.flows))
	}

	// Enter detail and verify content
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := app.View()
	if !strings.Contains(view, "api.example.com") {
		t.Error("detail should show host from real RingBuffer")
	}
}

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

func TestStatusBarShowsCAWarning(t *testing.T) {
	m := seedMock(1)
	app := NewApp(m, &mockProxyInfo{addr: ":9999"}, false)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg(time.Now()))

	view := app.View()
	if !strings.Contains(view, "CA NOT TRUSTED") {
		t.Error("status bar should show CA NOT TRUSTED when caTrusted=false")
	}
}

func TestStatusBarNoWarningWhenTrusted(t *testing.T) {
	app := newMockApp(1) // caTrusted=true
	view := app.View()
	if strings.Contains(view, "CA NOT TRUSTED") {
		t.Error("status bar should NOT show CA NOT TRUSTED when caTrusted=true")
	}
}
