package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kostyay/httpmon/internal/store"
)

// stripView returns the view content with ANSI escape codes removed.
// lipgloss v2 wraps individual characters in ANSI codes, making raw
// strings.Contains unreliable for styled text.
func stripView(app *App) string {
	return ansi.Strip(app.viewContent())
}

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

func fmtID(i int) string   { return "flow-" + strconv.Itoa(i) }
func fmtPath(i int) string { return "/v1/test/" + strconv.Itoa(i) }

func newTestApp(n int) *App {
	s := seedStore(n)
	app := NewApp(AppConfig{Store: s, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))
	return app
}

func newMockApp(n int) *App {
	m := seedMock(n)
	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))
	return app
}

func sendKey(app *App, key string) {
	app.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
}

// --- existing tests (using real RingBuffer) ---

func TestNavigateDown(t *testing.T) {
	app := newTestApp(5)
	if app.selectedIdx != 0 {
		t.Errorf("initial selectedIdx = %d, want 0", app.selectedIdx)
	}

	app.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if app.selectedIdx != 1 {
		t.Errorf("after j: selectedIdx = %d, want 1", app.selectedIdx)
	}
}

func TestNavigateUp(t *testing.T) {
	app := newTestApp(5)
	app.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	app.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	app.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if app.selectedIdx != 1 {
		t.Errorf("after j,j,k: selectedIdx = %d, want 1", app.selectedIdx)
	}
}

func TestEnterDetail(t *testing.T) {
	app := newTestApp(5)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !app.showDetail {
		t.Error("expected showDetail=true after Enter")
	}
}

func TestEscFromDetail(t *testing.T) {
	app := newTestApp(5)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.showDetail {
		t.Error("expected showDetail=false after Esc")
	}
}

func TestListViewContainsFlows(t *testing.T) {
	app := newTestApp(3)
	view := stripView(app)
	if !strings.Contains(view, "flows") {
		t.Error("list view should contain 'flows'")
	}
	if !strings.Contains(view, "api.example.com") {
		t.Error("list view should contain host")
	}
}

func TestDetailViewContainsHeaders(t *testing.T) {
	app := newTestApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	view := stripView(app)
	if !strings.Contains(view, "Method:") {
		t.Error("detail view should contain 'Method:'")
	}
	if !strings.Contains(view, "GET") {
		t.Error("detail view should contain method GET")
	}
}

func TestFilterFocusBlur(t *testing.T) {
	app := newTestApp(5)

	app.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !app.filterInput.Focused() {
		t.Error("filter should be focused after /")
	}

	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.filterInput.Focused() {
		t.Error("filter should be blurred after Esc")
	}
}

func TestQDoesNotQuitWhenFilterFocused(t *testing.T) {
	app := newTestApp(5)
	app.Update(tea.KeyPressMsg{Code: '/', Text: "/"})

	_, cmd := app.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Log("cmd returned from q while filter focused (may or may not be quit)")
	}
	if app.showDetail {
		t.Error("q while filter focused should not open detail")
	}
}

func TestNavigateBeyondBounds(t *testing.T) {
	app := newTestApp(3)
	app.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if app.selectedIdx != 0 {
		t.Errorf("selectedIdx = %d, want 0 (can't go above top)", app.selectedIdx)
	}

	app.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if app.selectedIdx != 2 {
		t.Errorf("selectedIdx = %d, want 2 after G", app.selectedIdx)
	}

	app.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if app.selectedIdx != 2 {
		t.Errorf("selectedIdx = %d, want 2 (can't go below bottom)", app.selectedIdx)
	}
}

// --- new tests using mocks ---

func TestEmptyStore(t *testing.T) {
	app := newMockApp(0)
	view := stripView(app)
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

	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))

	// Focus filter, type "other", apply
	sendKey(app, "/")
	for _, ch := range "other" {
		app.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app.Update(TickMsg(time.Now()))

	view := stripView(app)
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

	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))

	// Apply filter
	sendKey(app, "/")
	for _, ch := range "other" {
		app.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app.Update(TickMsg(time.Now()))

	// Clear filter: open filter, clear text, enter
	sendKey(app, "/")
	// Select all + delete to clear
	app.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}) // clear line
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app.Update(TickMsg(time.Now()))

	view := stripView(app)
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
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !app.showDetail {
		t.Fatal("expected detail view")
	}

	// Default is request tab (0)
	if app.detailTab != 0 {
		t.Errorf("detailTab = %d, want 0", app.detailTab)
	}
	view := stripView(app)
	if !strings.Contains(view, "[Request]") {
		t.Errorf("request tab should be active, got:\n%s", view)
	}

	// Switch to response tab
	sendKey(app, "2")
	if app.detailTab != 1 {
		t.Errorf("after '2': detailTab = %d, want 1", app.detailTab)
	}
	view = stripView(app)
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
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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
	app := NewApp(AppConfig{Store: seedMock(1), CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))

	got := app.proxyAddr()
	if got != ":8080" {
		t.Errorf("proxyAddr() = %q, want %q", got, ":8080")
	}
}

func TestProxyAddrFromMock(t *testing.T) {
	app := NewApp(AppConfig{Store: seedMock(1), Proxy: &mockProxyInfo{addr: ":3128"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))

	got := app.proxyAddr()
	if got != ":3128" {
		t.Errorf("proxyAddr() = %q, want %q", got, ":3128")
	}
}

// TestRingBufferSatisfiesFlowReader confirms the real store works through the interface.
func TestRingBufferSatisfiesFlowReader(t *testing.T) {
	s := seedStore(3)
	app := NewApp(AppConfig{Store: s, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))

	if len(app.flows) != 3 {
		t.Errorf("flows = %d, want 3", len(app.flows))
	}

	// Enter detail and verify content
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	view := stripView(app)
	if !strings.Contains(view, "api.example.com") {
		t.Error("detail should show host from real RingBuffer")
	}
}

func TestPToggleRaw(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	view := stripView(app)
	if !strings.Contains(view, "p:pretty") {
		t.Error("status bar should show p:pretty when detailRaw=false")
	}

	sendKey(app, "p")
	view = stripView(app)
	if !strings.Contains(view, "p:raw") {
		t.Error("status bar should show p:raw when detailRaw=true")
	}
}

func TestStatusBarShowsCAWarning(t *testing.T) {
	m := seedMock(1)
	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: false})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))

	view := stripView(app)
	if !strings.Contains(view, "CA NOT TRUSTED") {
		t.Error("status bar should show CA NOT TRUSTED when caTrusted=false")
	}
}

func TestStatusBarNoWarningWhenTrusted(t *testing.T) {
	app := newMockApp(1) // caTrusted=true
	view := stripView(app)
	if strings.Contains(view, "CA NOT TRUSTED") {
		t.Error("status bar should NOT show CA NOT TRUSTED when caTrusted=true")
	}
}

func TestFlatListScrollsWithCursor(t *testing.T) {
	// Height=12 → maxRows = 12-5 = 7. With 20 flows, scrolling past row 6
	// should shift the viewport so the selected row is visible.
	m := seedMock(20)
	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	app.Update(TickMsg(time.Now()))

	// Move cursor to row 10 (well past the 7-row viewport).
	for range 10 {
		sendKey(app, "j")
	}
	if app.selectedIdx != 10 {
		t.Fatalf("selectedIdx = %d, want 10", app.selectedIdx)
	}

	view := stripView(app)
	// The selected flow's path must appear in the rendered output.
	// Flows are newest-first, so flow-19 is index 0, flow-9 is index 10.
	selectedPath := app.flows[10].Path
	if !strings.Contains(view, selectedPath) {
		t.Errorf("view should contain selected flow path %q when cursor is at row 10\nview:\n%s", selectedPath, view)
	}
}

func TestScrollUpKeepsViewportStable(t *testing.T) {
	// Height=12 → maxRows=7. 20 flows.
	// Scroll down to row 10 (offset=4, showing rows 4-10).
	// Then scroll up one — cursor at 9. Viewport should NOT shift:
	// bottom row should still be row 10 (not row 9).
	m := seedMock(20)
	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	app.Update(TickMsg(time.Now()))

	// Move cursor to row 10, calling View() each step (like real Bubble Tea).
	for range 10 {
		sendKey(app, "j")
		stripView(app)
	}

	// Before scrolling up: row 10 is selected, viewport shows rows 4-10.
	// Row at bottom of viewport = row 10 → flow path for index 10.
	bottomPath := app.flows[10].Path

	// Scroll up one.
	sendKey(app, "k")
	if app.selectedIdx != 9 {
		t.Fatalf("selectedIdx = %d, want 9", app.selectedIdx)
	}

	view := stripView(app)
	// Row 10 should still be visible (viewport didn't retract).
	if !strings.Contains(view, bottomPath) {
		t.Errorf("after scrolling up, row 10 (%q) should still be visible (viewport stable)\nview:\n%s", bottomPath, view)
	}

	// Selected row must also be visible.
	selPath := app.flows[9].Path
	if !strings.Contains(view, selPath) {
		t.Errorf("selected row %q must be visible\nview:\n%s", selPath, view)
	}
}

func TestDetailCollapseGeneral(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	view := stripView(app)
	if !strings.Contains(view, "Method:") {
		t.Fatal("General section should be visible by default")
	}

	// Collapse general
	sendKey(app, "g")
	view = stripView(app)
	if strings.Contains(view, "Method:") {
		t.Error("General section should be hidden after pressing g")
	}
	if !strings.Contains(view, "▸ General") {
		t.Error("collapsed General should show ▸ icon")
	}

	// Expand again
	sendKey(app, "g")
	view = stripView(app)
	if !strings.Contains(view, "Method:") {
		t.Error("General section should be visible after second g")
	}
	if !strings.Contains(view, "▾ General") {
		t.Error("expanded General should show ▾ icon")
	}
}

func TestDetailCollapseHeaders(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	view := stripView(app)
	if !strings.Contains(view, "Accept:") {
		t.Fatal("Headers should be visible by default")
	}

	sendKey(app, "h")
	view = stripView(app)
	if strings.Contains(view, "Accept:") {
		t.Error("Headers should be hidden after pressing h")
	}

	sendKey(app, "h")
	view = stripView(app)
	if !strings.Contains(view, "Accept:") {
		t.Error("Headers should be visible after second h")
	}
}

func TestDetailCollapseBody(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	view := stripView(app)
	if !strings.Contains(view, `"q"`) {
		t.Fatal("Body content should be visible by default")
	}

	sendKey(app, "b")
	view = stripView(app)
	if strings.Contains(view, `"q"`) {
		t.Error("Body content should be hidden after pressing b")
	}

	sendKey(app, "b")
	view = stripView(app)
	if !strings.Contains(view, `"q"`) {
		t.Error("Body content should be visible after second b")
	}
}

// --- Help overlay tests ---

func TestHelpToggle(t *testing.T) {
	app := newMockApp(3)

	// Initially help is hidden.
	if app.showHelp {
		t.Fatal("showHelp should be false initially")
	}

	// Press ? to show help.
	sendKey(app, "?")
	if !app.showHelp {
		t.Error("? should toggle showHelp to true")
	}

	// Press ? again to dismiss.
	sendKey(app, "?")
	if app.showHelp {
		t.Error("second ? should toggle showHelp back to false")
	}
}

func TestHelpEscDismisses(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, "?")
	if !app.showHelp {
		t.Fatal("showHelp should be true after ?")
	}

	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.showHelp {
		t.Error("Esc should dismiss help overlay")
	}
}

func TestHelpContent(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, "?")

	view := stripView(app)
	// Should contain key groups.
	for _, expect := range []string{"Navigation", "Filter", "Detail View", "?", "Esc"} {
		if !strings.Contains(view, expect) {
			t.Errorf("help overlay should contain %q, got:\n%s", expect, view)
		}
	}
}

func TestHelpDoesNotDisruptState(t *testing.T) {
	app := newMockApp(5)

	// Select third flow.
	sendKey(app, "j")
	sendKey(app, "j")
	savedIdx := app.selectedIdx

	// Open and close help.
	sendKey(app, "?")
	sendKey(app, "?")

	if app.selectedIdx != savedIdx {
		t.Errorf("selectedIdx changed: got %d, want %d", app.selectedIdx, savedIdx)
	}
	if app.showDetail {
		t.Error("help toggle should not enter detail view")
	}
}

func TestHelpInDetailView(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // enter detail
	if !app.showDetail {
		t.Fatal("should be in detail view")
	}

	sendKey(app, "?")
	if !app.showHelp {
		t.Error("? in detail should open help")
	}

	// Esc should dismiss help but not exit detail.
	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.showHelp {
		t.Error("Esc should dismiss help")
	}
	if !app.showDetail {
		t.Error("Esc from help in detail should stay in detail view")
	}
}

// --- Detail search tests ---

func TestSearchFindsText(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // enter detail

	// Press / to open search
	sendKey(app, "/")
	if !app.detailSearch {
		t.Fatal("/ in detail should activate search mode")
	}

	// Type search term
	for _, ch := range "test" {
		app.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}

	if app.searchMatchCount == 0 {
		t.Error("search for 'test' should find matches in body/headers")
	}
}

func TestSearchNoMatch(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	sendKey(app, "/")
	for _, ch := range "zzznomatch" {
		app.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}

	if app.searchMatchCount != 0 {
		t.Errorf("searchMatchCount = %d, want 0 for no matches", app.searchMatchCount)
	}
}

func TestSearchNavigation(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	sendKey(app, "/")
	// The body has `{"q":"test"}` — search for common char.
	for _, ch := range ":" {
		app.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}

	if app.searchMatchCount < 2 {
		t.Skipf("need >= 2 matches for navigation test, got %d", app.searchMatchCount)
	}

	// Commit search (Enter blurs input, n/N now navigate).
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	initial := app.searchMatchIdx
	sendKey(app, "n")
	if app.searchMatchIdx == initial {
		t.Error("n should advance to next match")
	}

	sendKey(app, "N")
	if app.searchMatchIdx != initial {
		t.Error("N should go back to previous match")
	}
}

func TestSearchEscClears(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	sendKey(app, "/")
	for _, ch := range "test" {
		app.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}

	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.detailSearch {
		t.Error("Esc should exit search mode")
	}
	if app.searchQuery != "" {
		t.Error("Esc should clear search query")
	}
}

func TestSearchEscInDetailStaysInDetail(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	sendKey(app, "/")
	for _, ch := range "test" {
		app.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	if !app.showDetail {
		t.Error("Esc from search should stay in detail view")
	}
}

func TestSearchStatusBar(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	sendKey(app, "/")
	for _, ch := range "test" {
		app.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}

	view := stripView(app)
	if !strings.Contains(view, "matches") {
		t.Error("status bar should show match count during search")
	}
}

// --- HAR export modal tests ---

func TestExportModalOpens(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, "x")
	if !app.showExport {
		t.Error("x should open export modal")
	}
	if app.exportSingle {
		t.Error("x from list should set exportSingle=false")
	}
	val := app.exportInput.Value()
	if !strings.HasPrefix(val, "httpmon-") || !strings.HasSuffix(val, ".har") {
		t.Errorf("default filename = %q, want httpmon-*.har", val)
	}
}

func TestExportModalEscCancels(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, "x")
	if !app.showExport {
		t.Fatal("modal should be open")
	}

	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.showExport {
		t.Error("Esc should dismiss export modal")
	}
}

func TestExportModalEnterExports(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, "x")
	// Set a temp filename.
	tmp := t.TempDir() + "/test-export.har"
	app.exportInput.SetValue(tmp)

	_, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.showExport {
		t.Error("Enter should dismiss export modal")
	}
	if cmd == nil {
		t.Error("Enter should return a feedback cmd")
	}
	// Verify file was written.
	if _, err := os.Stat(tmp); err != nil {
		t.Errorf("exported file should exist: %v", err)
	}
}

func TestExportModalSingleFlow(t *testing.T) {
	app := newMockApp(3)

	// Enter detail, press x.
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	sendKey(app, "x")
	if !app.showExport {
		t.Fatal("x in detail should open export modal")
	}
	if !app.exportSingle {
		t.Error("x from detail should set exportSingle=true")
	}
}

func TestExportModalViewContent(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, "x")
	view := stripView(app)
	if !strings.Contains(view, "Export HAR") {
		t.Error("export view should contain header")
	}
	if !strings.Contains(view, "Enter:export") {
		t.Error("export view should show Enter hint")
	}
	if !strings.Contains(view, "Esc:cancel") {
		t.Error("export view should show Esc hint")
	}
}

func TestExportHARTreeMode(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, "t")
	if app.listMode != modeTree {
		t.Fatal("should be in tree mode")
	}

	sendKey(app, "x")
	if !app.showExport {
		t.Error("x in tree mode should open export modal")
	}
}

func TestExportHARFocusMode(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, "t")
	sendKey(app, "f")
	if app.listMode != modeFocus {
		t.Fatal("should be in focus mode")
	}

	sendKey(app, "x")
	if !app.showExport {
		t.Error("x in focus mode should open export modal")
	}
}

// --- Diff tests ---

func TestDiffMarkAndCompare(t *testing.T) {
	app := newMockApp(5)

	// Mark first flow with 'd'.
	sendKey(app, "d")
	if app.diffMarkID == "" {
		t.Fatal("d should mark first flow for diff")
	}
	// Move to second flow and press d again to open diff.
	sendKey(app, "j")
	sendKey(app, "d")

	if !app.showDiff {
		t.Error("second d should open diff view")
	}
	if app.diffMarkID != "" {
		t.Error("diff mark should be cleared after opening diff")
	}
}

func TestDiffEscReturns(t *testing.T) {
	app := newMockApp(5)

	// Mark and diff.
	sendKey(app, "d")
	sendKey(app, "j")
	sendKey(app, "d")

	if !app.showDiff {
		t.Fatal("should be in diff view")
	}

	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.showDiff {
		t.Error("Esc should exit diff view")
	}
}

func TestUpdateDetailContentError(t *testing.T) {
	app := newMockApp(3)
	app.showDetail = true
	app.selectedID = "nonexistent"
	app.detailReady = true
	app.detailVP = viewport.New(viewport.WithWidth(120), viewport.WithHeight(37))
	app.updateDetailContent()

	content := app.detailVP.View()
	if !strings.Contains(content, "no longer available") {
		t.Errorf("expected 'no longer available', got: %s", content)
	}
}

// selectMenuItem navigates to the menu item with the given label and selects it.
// Fatals if the item is not found.
func selectMenuItem(t *testing.T, app *App, label string) {
	t.Helper()
	idx := -1
	for i, item := range app.menuItems {
		if item.label == label {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("menu item %q not found", label)
	}
	for range idx {
		app.updateMenu(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	app.updateMenu(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func TestDispatchMenuFromList(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, " ")
	if !app.showMenu {
		t.Fatal("Space should open menu")
	}

	selectMenuItem(t, app, "Export HAR")
	if !app.showExport {
		t.Error("selecting Export HAR should open export modal")
	}
	if app.showMenu {
		t.Error("menu should be closed after dispatch")
	}
}

func TestDispatchMenuFromDetail(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // enter detail

	sendKey(app, " ")
	if !app.showMenu {
		t.Fatal("Space should open menu in detail")
	}

	wasRaw := app.detailRaw
	selectMenuItem(t, app, "Toggle Pretty/Raw")
	if app.detailRaw == wasRaw {
		t.Error("selecting Toggle Pretty/Raw should flip detailRaw")
	}
}
