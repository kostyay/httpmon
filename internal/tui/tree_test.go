package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kostyay/httpmon/internal/store"
)

// --- buildHostGroups tests ---

func TestBuildHostGroupsGroupsByHost(t *testing.T) {
	flows := []store.FlowMeta{
		{ID: "1", Host: "api.example.com", StartedAt: time.Unix(100, 0)},
		{ID: "2", Host: "cdn.example.com", StartedAt: time.Unix(200, 0)},
		{ID: "3", Host: "api.example.com", StartedAt: time.Unix(300, 0)},
	}
	groups := buildHostGroups(flows)

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
}

func TestBuildHostGroupsSortByNewest(t *testing.T) {
	flows := []store.FlowMeta{
		{ID: "1", Host: "old.com", StartedAt: time.Unix(100, 0)},
		{ID: "2", Host: "new.com", StartedAt: time.Unix(300, 0)},
		{ID: "3", Host: "mid.com", StartedAt: time.Unix(200, 0)},
	}
	groups := buildHostGroups(flows)

	if groups[0].Host != "new.com" {
		t.Errorf("first group = %s, want new.com", groups[0].Host)
	}
	if groups[1].Host != "mid.com" {
		t.Errorf("second group = %s, want mid.com", groups[1].Host)
	}
	if groups[2].Host != "old.com" {
		t.Errorf("third group = %s, want old.com", groups[2].Host)
	}
}

func TestBuildHostGroupsEmpty(t *testing.T) {
	groups := buildHostGroups(nil)
	if len(groups) != 0 {
		t.Errorf("got %d groups for nil input, want 0", len(groups))
	}
}

func TestBuildHostGroupsFlowOrder(t *testing.T) {
	flows := []store.FlowMeta{
		{ID: "1", Host: "a.com", Path: "/first", StartedAt: time.Unix(100, 0)},
		{ID: "2", Host: "a.com", Path: "/second", StartedAt: time.Unix(200, 0)},
	}
	groups := buildHostGroups(flows)

	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	if len(groups[0].Flows) != 2 {
		t.Fatalf("got %d flows, want 2", len(groups[0].Flows))
	}
	// Flows should be in insertion order (store already sorts newest-first)
	if groups[0].Flows[0].Path != "/first" {
		t.Errorf("first flow path = %s, want /first", groups[0].Flows[0].Path)
	}
}

// --- flattenTree tests ---

func TestFlattenTreeCollapsed(t *testing.T) {
	groups := []hostGroup{
		{Host: "a.com", Flows: []store.FlowMeta{{ID: "1"}, {ID: "2"}}},
		{Host: "b.com", Flows: []store.FlowMeta{{ID: "3"}}},
	}
	rows := flattenTree(groups, map[string]bool{})

	// Only host nodes when all collapsed
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if !rows[0].IsHost || rows[0].Host != "a.com" {
		t.Error("first row should be host a.com")
	}
	if !rows[1].IsHost || rows[1].Host != "b.com" {
		t.Error("second row should be host b.com")
	}
}

func TestFlattenTreeExpanded(t *testing.T) {
	groups := []hostGroup{
		{Host: "a.com", Flows: []store.FlowMeta{{ID: "1"}, {ID: "2"}}},
		{Host: "b.com", Flows: []store.FlowMeta{{ID: "3"}}},
	}
	expanded := map[string]bool{"a.com": true}
	rows := flattenTree(groups, expanded)

	// a.com host + 2 flows + b.com host = 4
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	if !rows[0].IsHost {
		t.Error("row 0 should be host")
	}
	if rows[1].IsHost || rows[1].Flow.ID != "1" {
		t.Error("row 1 should be flow 1")
	}
	if rows[2].IsHost || rows[2].Flow.ID != "2" {
		t.Error("row 2 should be flow 2")
	}
	if !rows[3].IsHost || rows[3].Host != "b.com" {
		t.Error("row 3 should be host b.com")
	}
}

// --- flattenFocus tests ---

func TestFlattenFocusOnlyTargetHost(t *testing.T) {
	groups := []hostGroup{
		{Host: "a.com", Flows: []store.FlowMeta{{ID: "1"}, {ID: "2"}}},
		{Host: "b.com", Flows: []store.FlowMeta{{ID: "3"}}},
	}
	rows := flattenFocus(groups, "a.com")

	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for _, r := range rows {
		if r.IsHost {
			t.Error("focus mode should not include host nodes")
		}
	}
}

func TestFlattenFocusMissingHost(t *testing.T) {
	groups := []hostGroup{
		{Host: "a.com", Flows: []store.FlowMeta{{ID: "1"}}},
	}
	rows := flattenFocus(groups, "gone.com")
	if len(rows) != 0 {
		t.Errorf("got %d rows for missing host, want 0", len(rows))
	}
}

// --- Mode transition tests ---

func newMultiHostApp() *App {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	now := time.Now()
	m.metas = []store.FlowMeta{
		{ID: "a1", Method: "GET", Host: "api.example.com", Path: "/v1/users", StatusCode: 200, State: store.StateCompleted, StartedAt: now.Add(-2 * time.Second), Scheme: "https"},
		{ID: "a2", Method: "POST", Host: "api.example.com", Path: "/v1/users", StatusCode: 201, State: store.StateCompleted, StartedAt: now.Add(-1 * time.Second), Scheme: "https"},
		{ID: "c1", Method: "GET", Host: "cdn.example.com", Path: "/assets/main.js", StatusCode: 200, State: store.StateCompleted, StartedAt: now, Scheme: "https"},
	}
	for _, meta := range m.metas {
		m.data[meta.ID] = &store.FlowData{}
	}

	app := NewApp(m, &mockProxyInfo{addr: ":9999"}, true, nil)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg(time.Now()))
	return app
}

func TestToggleFlatToTree(t *testing.T) {
	app := newMultiHostApp()

	if app.listMode != modeFlat {
		t.Fatalf("initial mode = %d, want flat", app.listMode)
	}

	sendKey(app, "t")
	if app.listMode != modeTree {
		t.Errorf("mode after t = %d, want tree", app.listMode)
	}
	if len(app.treeRows) == 0 {
		t.Error("treeRows should be populated")
	}
}

func TestToggleTreeToFlat(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t") // → tree
	sendKey(app, "t") // → flat
	if app.listMode != modeFlat {
		t.Errorf("mode after t,t = %d, want flat", app.listMode)
	}
}

func TestTreeExpandCollapse(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t") // → tree

	// All collapsed initially — only host rows
	hostCount := 0
	for _, r := range app.treeRows {
		if r.IsHost {
			hostCount++
		}
	}
	if hostCount != 2 {
		t.Fatalf("expected 2 host rows, got %d", hostCount)
	}

	// Expand first host (cursor is on row 0)
	sendKey(app, "l")
	app.Update(tickMsg(time.Now())) // refresh
	if !app.hostExpanded[app.treeRows[0].Host] {
		t.Error("host should be expanded after l")
	}
	if len(app.treeRows) <= 2 {
		t.Error("expanding should add flow rows")
	}
}

func TestTreeCollapseFromChild(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t") // → tree

	// Expand first host
	sendKey(app, "l")
	app.Update(tickMsg(time.Now()))

	// Move to first child flow
	sendKey(app, "j")
	if app.treeRows[app.selectedIdx].IsHost {
		t.Fatal("cursor should be on a flow row after j")
	}

	// Press h — should jump to parent host
	sendKey(app, "h")
	if !app.treeRows[app.selectedIdx].IsHost {
		t.Error("h from flow should jump to parent host")
	}
}

func TestTreeFocusMode(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t") // → tree
	sendKey(app, "f") // focus on first host

	if app.listMode != modeFocus {
		t.Errorf("mode = %d, want focus", app.listMode)
	}
	if app.focusHost == "" {
		t.Error("focusHost should be set")
	}
	// All rows should be flows (no host nodes)
	for _, r := range app.treeRows {
		if r.IsHost {
			t.Error("focus mode should not have host node rows")
		}
	}
}

func TestFocusEscBackToTree(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	sendKey(app, "f")

	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if app.listMode != modeTree {
		t.Errorf("mode after Esc = %d, want tree", app.listMode)
	}
}

func TestFocusTBackToFlat(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	sendKey(app, "f")
	sendKey(app, "t")

	if app.listMode != modeFlat {
		t.Errorf("mode after t in focus = %d, want flat", app.listMode)
	}
}

func TestTreeEnterOnHost(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")

	// Enter on collapsed host should expand
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app.Update(tickMsg(time.Now()))

	host := app.treeRows[0].Host
	if !app.hostExpanded[host] {
		t.Error("Enter on collapsed host should expand it")
	}

	// Enter again should collapse
	app.selectedIdx = 0
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app.Update(tickMsg(time.Now()))
	if app.hostExpanded[host] {
		t.Error("Enter on expanded host should collapse it")
	}
}

func TestTreeEnterOnFlow(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")

	// Expand and navigate to flow
	sendKey(app, "l") // expand
	app.Update(tickMsg(time.Now()))
	sendKey(app, "j") // move to first flow

	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !app.showDetail {
		t.Error("Enter on flow should open detail")
	}
}

func TestFocusEnterOnFlow(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	sendKey(app, "f")

	if len(app.treeRows) == 0 {
		t.Fatal("focus should have rows")
	}

	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !app.showDetail {
		t.Error("Enter in focus mode should open detail")
	}
}

// --- View rendering tests ---

func TestTreeViewContainsHostNodes(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	app.Update(tickMsg(time.Now()))

	view := app.View()
	if !strings.Contains(view, "api.example.com") {
		t.Error("tree view should contain api.example.com")
	}
	if !strings.Contains(view, "cdn.example.com") {
		t.Error("tree view should contain cdn.example.com")
	}
	if !strings.Contains(view, "▸") {
		t.Error("collapsed hosts should show ▸")
	}
}

func TestTreeViewExpandedShowsFlows(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	sendKey(app, "l") // expand
	app.Update(tickMsg(time.Now()))

	view := app.View()
	if !strings.Contains(view, "▾") {
		t.Error("expanded host should show ▾")
	}
}

func TestFocusViewShowsHostHeader(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	sendKey(app, "f")
	app.Update(tickMsg(time.Now()))

	view := app.View()
	if !strings.Contains(view, "Esc to unfocus") {
		t.Error("focus view should show unfocus hint")
	}
}

func TestStatusBarTreeMode(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")

	view := app.View()
	if !strings.Contains(view, "t:flat") {
		t.Error("tree mode status should show t:flat")
	}
	if !strings.Contains(view, "f:focus") {
		t.Error("tree mode status should show f:focus")
	}
}

func TestStatusBarFocusMode(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	sendKey(app, "f")

	view := app.View()
	if !strings.Contains(view, "Esc:unfocus") {
		t.Error("focus mode status should show Esc:unfocus")
	}
}

func TestStatusBarFlatMode(t *testing.T) {
	app := newMultiHostApp()
	view := app.View()
	if !strings.Contains(view, "t:tree") {
		t.Error("flat mode status should show t:tree")
	}
}

// --- Edge cases ---

func TestTreeEmptyStore(t *testing.T) {
	app := newMockApp(0)
	sendKey(app, "t")
	app.Update(tickMsg(time.Now()))

	view := app.View()
	if !strings.Contains(view, "Waiting for traffic") {
		t.Error("empty tree should show waiting message")
	}
}

func TestTreeSingleHost(t *testing.T) {
	app := newMockApp(3) // all same host
	sendKey(app, "t")
	app.Update(tickMsg(time.Now()))

	hostCount := 0
	for _, r := range app.treeRows {
		if r.IsHost {
			hostCount++
		}
	}
	if hostCount != 1 {
		t.Errorf("single-host tree should have 1 host node, got %d", hostCount)
	}
}

func TestTreeNavigationBounds(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")

	// Try going up at top
	sendKey(app, "k")
	if app.selectedIdx != 0 {
		t.Errorf("k at top: idx = %d, want 0", app.selectedIdx)
	}

	// Go to end
	sendKey(app, "G")
	last := len(app.treeRows) - 1
	if app.selectedIdx != last {
		t.Errorf("G: idx = %d, want %d", app.selectedIdx, last)
	}

	// Try going down at bottom
	sendKey(app, "j")
	if app.selectedIdx != last {
		t.Errorf("j at bottom: idx = %d, want %d", app.selectedIdx, last)
	}

	// Go to top
	sendKey(app, "g")
	if app.selectedIdx != 0 {
		t.Errorf("g: idx = %d, want 0", app.selectedIdx)
	}
}

func TestTreeFilterIntegration(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")

	// Filter to only api.example.com
	sendKey(app, "/")
	for _, ch := range "api" {
		app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app.Update(tickMsg(time.Now()))

	// Should only have api.example.com host
	for _, r := range app.treeRows {
		if r.IsHost && r.Host != "api.example.com" {
			t.Errorf("filtered tree should only have api.example.com, got %s", r.Host)
		}
	}
}

func TestFocusHostEvicted(t *testing.T) {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	now := time.Now()
	m.metas = []store.FlowMeta{
		{ID: "a1", Host: "api.com", Path: "/a", StartedAt: now.Add(-1 * time.Second), Scheme: "https", State: store.StateCompleted, StatusCode: 200},
		{ID: "b1", Host: "cdn.com", Path: "/b", StartedAt: now, Scheme: "https", State: store.StateCompleted, StatusCode: 200},
	}
	m.data["a1"] = &store.FlowData{}
	m.data["b1"] = &store.FlowData{}

	app := NewApp(m, &mockProxyInfo{addr: ":9999"}, true, nil)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg(time.Now()))

	// Enter tree mode
	sendKey(app, "t")

	// cdn.com is newest so it's row 0; api.com is row 1. Focus cdn.com directly.
	if len(app.treeRows) < 1 || app.treeRows[0].Host != "cdn.com" {
		t.Fatalf("expected cdn.com at row 0, got %v", app.treeRows)
	}
	sendKey(app, "f")

	if app.listMode != modeFocus || app.focusHost != "cdn.com" {
		t.Fatalf("should be in focus mode on cdn.com, got mode=%d host=%s", app.listMode, app.focusHost)
	}

	// Remove cdn.com from mock
	m.metas = m.metas[:1] // only api.com remains

	app.Update(tickMsg(time.Now()))

	// Should fall back to tree mode
	if app.listMode != modeTree {
		t.Errorf("mode = %d, want tree (focus host evicted)", app.listMode)
	}
}
