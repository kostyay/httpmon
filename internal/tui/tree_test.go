package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kostyay/httpmon/internal/store"
)

// --- buildGroups tests ---

func TestBuildGroupsGroupsByHost(t *testing.T) {
	flows := []store.FlowMeta{
		{ID: "1", Host: "api.example.com", StartedAt: time.Unix(100, 0)},
		{ID: "2", Host: "cdn.example.com", StartedAt: time.Unix(200, 0)},
		{ID: "3", Host: "api.example.com", StartedAt: time.Unix(300, 0)},
	}
	groups := buildGroups(flows, hostKey)

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
}

func TestBuildGroupsSortByNewest(t *testing.T) {
	flows := []store.FlowMeta{
		{ID: "1", Host: "old.com", StartedAt: time.Unix(100, 0)},
		{ID: "2", Host: "new.com", StartedAt: time.Unix(300, 0)},
		{ID: "3", Host: "mid.com", StartedAt: time.Unix(200, 0)},
	}
	groups := buildGroups(flows, hostKey)

	if groups[0].Key != "new.com" {
		t.Errorf("first group = %s, want new.com", groups[0].Key)
	}
	if groups[1].Key != "mid.com" {
		t.Errorf("second group = %s, want mid.com", groups[1].Key)
	}
	if groups[2].Key != "old.com" {
		t.Errorf("third group = %s, want old.com", groups[2].Key)
	}
}

func TestBuildGroupsEmpty(t *testing.T) {
	groups := buildGroups(nil, hostKey)
	if len(groups) != 0 {
		t.Errorf("got %d groups for nil input, want 0", len(groups))
	}
}

func TestBuildGroupsFlowOrder(t *testing.T) {
	flows := []store.FlowMeta{
		{ID: "1", Host: "a.com", Path: "/first", StartedAt: time.Unix(100, 0)},
		{ID: "2", Host: "a.com", Path: "/second", StartedAt: time.Unix(200, 0)},
	}
	groups := buildGroups(flows, hostKey)

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

// --- buildGroups with processKey ---

func TestBuildGroupsByProcess(t *testing.T) {
	flows := []store.FlowMeta{
		{ID: "1", Host: "api.com", Process: "curl", StartedAt: time.Unix(100, 0)},
		{ID: "2", Host: "cdn.com", Process: "firefox", StartedAt: time.Unix(200, 0)},
		{ID: "3", Host: "api.com", Process: "curl", StartedAt: time.Unix(300, 0)},
		{ID: "4", Host: "other.com", Process: "\u2014", StartedAt: time.Unix(50, 0)},
	}
	groups := buildGroups(flows, processKey)

	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(groups))
	}
	// curl is newest (300), firefox (200), — (50)
	if groups[0].Key != "curl" {
		t.Errorf("first group = %q, want curl", groups[0].Key)
	}
	if groups[1].Key != "firefox" {
		t.Errorf("second group = %q, want firefox", groups[1].Key)
	}
	if len(groups[0].Flows) != 2 {
		t.Errorf("curl group has %d flows, want 2", len(groups[0].Flows))
	}
}

// --- flattenTree tests ---

func TestFlattenTreeCollapsed(t *testing.T) {
	groups := []flowGroup{
		{Key: "a.com", Flows: []store.FlowMeta{{ID: "1"}, {ID: "2"}}},
		{Key: "b.com", Flows: []store.FlowMeta{{ID: "3"}}},
	}
	rows := flattenTree(groups, map[string]bool{})

	// Only header nodes when all collapsed
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if !rows[0].IsHeader || rows[0].GroupKey != "a.com" {
		t.Error("first row should be header a.com")
	}
	if !rows[1].IsHeader || rows[1].GroupKey != "b.com" {
		t.Error("second row should be header b.com")
	}
}

func TestFlattenTreeExpanded(t *testing.T) {
	groups := []flowGroup{
		{Key: "a.com", Flows: []store.FlowMeta{{ID: "1"}, {ID: "2"}}},
		{Key: "b.com", Flows: []store.FlowMeta{{ID: "3"}}},
	}
	expanded := map[string]bool{"a.com": true}
	rows := flattenTree(groups, expanded)

	// a.com header + 2 flows + b.com header = 4
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	if !rows[0].IsHeader {
		t.Error("row 0 should be header")
	}
	if rows[1].IsHeader || rows[1].Flow.ID != "1" {
		t.Error("row 1 should be flow 1")
	}
	if rows[2].IsHeader || rows[2].Flow.ID != "2" {
		t.Error("row 2 should be flow 2")
	}
	if !rows[3].IsHeader || rows[3].GroupKey != "b.com" {
		t.Error("row 3 should be header b.com")
	}
}

// --- flattenFocus tests ---

func TestFlattenFocusOnlyTargetGroup(t *testing.T) {
	groups := []flowGroup{
		{Key: "a.com", Flows: []store.FlowMeta{{ID: "1"}, {ID: "2"}}},
		{Key: "b.com", Flows: []store.FlowMeta{{ID: "3"}}},
	}
	rows := flattenFocus(groups, "a.com")

	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for _, r := range rows {
		if r.IsHeader {
			t.Error("focus mode should not include header nodes")
		}
	}
}

func TestFlattenFocusMissingKey(t *testing.T) {
	groups := []flowGroup{
		{Key: "a.com", Flows: []store.FlowMeta{{ID: "1"}}},
	}
	rows := flattenFocus(groups, "gone.com")
	if len(rows) != 0 {
		t.Errorf("got %d rows for missing key, want 0", len(rows))
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

	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))
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
	sendKey(app, "t") // flat → host-tree
	if app.listMode != modeTree || app.treeGroupBy != groupByHost {
		t.Errorf("expected host-tree, got mode=%d groupBy=%d", app.listMode, app.treeGroupBy)
	}
	sendKey(app, "t") // host-tree → process-tree
	if app.listMode != modeTree || app.treeGroupBy != groupByProcess {
		t.Errorf("expected process-tree, got mode=%d groupBy=%d", app.listMode, app.treeGroupBy)
	}
	sendKey(app, "t") // process-tree → flat
	if app.listMode != modeFlat {
		t.Errorf("mode after t,t,t = %d, want flat", app.listMode)
	}
}

func TestTreeExpandCollapse(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t") // → tree

	// All collapsed initially — only header rows
	headerCount := 0
	for _, r := range app.treeRows {
		if r.IsHeader {
			headerCount++
		}
	}
	if headerCount != 2 {
		t.Fatalf("expected 2 header rows, got %d", headerCount)
	}

	// Expand first group (cursor is on row 0)
	sendKey(app, "l")
	app.Update(TickMsg(time.Now())) // refresh
	if !app.groupExpanded[app.treeRows[0].GroupKey] {
		t.Error("group should be expanded after l")
	}
	if len(app.treeRows) <= 2 {
		t.Error("expanding should add flow rows")
	}
}

func TestTreeCollapseFromChild(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t") // → tree

	// Expand first group
	sendKey(app, "l")
	app.Update(TickMsg(time.Now()))

	// Move to first child flow
	sendKey(app, "j")
	if app.treeRows[app.selectedIdx].IsHeader {
		t.Fatal("cursor should be on a flow row after j")
	}

	// Press h — should jump to parent header
	sendKey(app, "h")
	if !app.treeRows[app.selectedIdx].IsHeader {
		t.Error("h from flow should jump to parent header")
	}
}

func TestTreeFocusMode(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t") // → tree
	sendKey(app, "f") // focus on first group

	if app.listMode != modeFocus {
		t.Errorf("mode = %d, want focus", app.listMode)
	}
	if app.focusKey == "" {
		t.Error("focusKey should be set")
	}
	// All rows should be flows (no header nodes)
	for _, r := range app.treeRows {
		if r.IsHeader {
			t.Error("focus mode should not have header node rows")
		}
	}
}

func TestFocusEscBackToTree(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	sendKey(app, "f")

	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
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

func TestTreeEnterOnHeader(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")

	// Enter on collapsed header should expand
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app.Update(TickMsg(time.Now()))

	key := app.treeRows[0].GroupKey
	if !app.groupExpanded[key] {
		t.Error("Enter on collapsed header should expand it")
	}

	// Enter again should collapse
	app.selectedIdx = 0
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app.Update(TickMsg(time.Now()))
	if app.groupExpanded[key] {
		t.Error("Enter on expanded header should collapse it")
	}
}

func TestTreeEnterOnFlow(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")

	// Expand and navigate to flow
	sendKey(app, "l") // expand
	app.Update(TickMsg(time.Now()))
	sendKey(app, "j") // move to first flow

	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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

	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !app.showDetail {
		t.Error("Enter in focus mode should open detail")
	}
}

// --- View rendering tests ---

func TestTreeViewContainsHeaderNodes(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	app.Update(TickMsg(time.Now()))

	view := stripView(app)
	if !strings.Contains(view, "api.example.com") {
		t.Error("tree view should contain api.example.com")
	}
	if !strings.Contains(view, "cdn.example.com") {
		t.Error("tree view should contain cdn.example.com")
	}
	if !strings.Contains(view, "▸") {
		t.Error("collapsed groups should show ▸")
	}
}

func TestTreeViewExpandedShowsFlows(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	sendKey(app, "l") // expand
	app.Update(TickMsg(time.Now()))

	view := stripView(app)
	if !strings.Contains(view, "▾") {
		t.Error("expanded group should show ▾")
	}
}

func TestFocusViewShowsHeader(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	sendKey(app, "f")
	app.Update(TickMsg(time.Now()))

	view := stripView(app)
	if !strings.Contains(view, "Esc to unfocus") {
		t.Error("focus view should show unfocus hint")
	}
}

func TestStatusBarTreeMode(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t") // → host-tree

	view := stripView(app)
	if !strings.Contains(view, "t:proc") {
		t.Error("host-tree mode status should show t:proc")
	}
	if !strings.Contains(view, "f:focus") {
		t.Error("tree mode status should show f:focus")
	}
}

func TestStatusBarFocusMode(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t")
	sendKey(app, "f")

	view := stripView(app)
	if !strings.Contains(view, "Esc:unfocus") {
		t.Error("focus mode status should show Esc:unfocus")
	}
}

func TestStatusBarFlatMode(t *testing.T) {
	app := newMultiHostApp()
	view := stripView(app)
	if !strings.Contains(view, "t:host") {
		t.Error("flat mode status should show t:host")
	}
}

func TestStatusBarProcessTreeMode(t *testing.T) {
	app := newMultiHostApp()
	sendKey(app, "t") // → host-tree
	sendKey(app, "t") // → process-tree

	view := stripView(app)
	if !strings.Contains(view, "t:flat") {
		t.Error("process-tree mode status should show t:flat")
	}
	if !strings.Contains(view, "f:focus") {
		t.Error("process-tree mode status should show f:focus")
	}
}

func TestProcessTreeGroupsByProcess(t *testing.T) {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	m.metas = []store.FlowMeta{
		{ID: "1", Host: "a.com", Path: "/1", Process: "curl", StartedAt: time.Now()},
		{ID: "2", Host: "b.com", Path: "/2", Process: "curl", StartedAt: time.Now()},
		{ID: "3", Host: "a.com", Path: "/3", Process: "wget", StartedAt: time.Now()},
	}

	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))

	sendKey(app, "t") // → host-tree
	sendKey(app, "t") // → process-tree

	if app.treeGroupBy != groupByProcess {
		t.Fatal("expected groupByProcess")
	}

	headers := 0
	for _, r := range app.treeRows {
		if r.IsHeader {
			headers++
		}
	}
	if headers != 2 {
		t.Errorf("expected 2 process groups (curl, wget), got %d", headers)
	}
}

// --- Edge cases ---

func TestTreeEmptyStore(t *testing.T) {
	app := newMockApp(0)
	sendKey(app, "t")
	app.Update(TickMsg(time.Now()))

	view := stripView(app)
	if !strings.Contains(view, "Waiting for traffic") {
		t.Error("empty tree should show waiting message")
	}
}

func TestTreeSingleHost(t *testing.T) {
	app := newMockApp(3) // all same host
	sendKey(app, "t")
	app.Update(TickMsg(time.Now()))

	headerCount := 0
	for _, r := range app.treeRows {
		if r.IsHeader {
			headerCount++
		}
	}
	if headerCount != 1 {
		t.Errorf("single-host tree should have 1 header node, got %d", headerCount)
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
		app.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app.Update(TickMsg(time.Now()))

	// Should only have api.example.com header
	for _, r := range app.treeRows {
		if r.IsHeader && r.GroupKey != "api.example.com" {
			t.Errorf("filtered tree should only have api.example.com, got %s", r.GroupKey)
		}
	}
}

// --- Process-tree edge cases ---

func newProcessApp() *App {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	now := time.Now()
	m.metas = []store.FlowMeta{
		{ID: "p1", Method: "GET", Host: "a.com", Path: "/1", Process: "curl", StatusCode: 200, State: store.StateCompleted, StartedAt: now.Add(-3 * time.Second), Scheme: "https"},
		{ID: "p2", Method: "POST", Host: "b.com", Path: "/2", Process: "curl", StatusCode: 201, State: store.StateCompleted, StartedAt: now.Add(-2 * time.Second), Scheme: "https"},
		{ID: "p3", Method: "GET", Host: "c.com", Path: "/3", Process: "curl", StatusCode: 200, State: store.StateCompleted, StartedAt: now.Add(-1 * time.Second), Scheme: "https"},
	}
	for _, meta := range m.metas {
		m.data[meta.ID] = &store.FlowData{}
	}
	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))
	return app
}

func TestProcessTreeAllSameProcess(t *testing.T) {
	app := newProcessApp()
	sendKey(app, "t") // → host-tree
	sendKey(app, "t") // → process-tree

	headers := 0
	for _, r := range app.treeRows {
		if r.IsHeader {
			headers++
		}
	}
	if headers != 1 {
		t.Errorf("expected 1 group header for all-curl flows, got %d", headers)
	}
	if app.treeRows[0].GroupKey != "curl" {
		t.Errorf("group key = %q, want curl", app.treeRows[0].GroupKey)
	}
}

func TestProcessTreeEmptyProcess(t *testing.T) {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	now := time.Now()
	m.metas = []store.FlowMeta{
		{ID: "e1", Method: "GET", Host: "a.com", Path: "/1", Process: "", StatusCode: 200, State: store.StateCompleted, StartedAt: now, Scheme: "https"},
	}
	m.data["e1"] = &store.FlowData{}
	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))

	sendKey(app, "t") // → host-tree
	sendKey(app, "t") // → process-tree

	if len(app.treeRows) == 0 {
		t.Fatal("expected at least 1 tree row")
	}
	if app.treeRows[0].GroupKey != "" {
		t.Errorf("group key = %q, want empty string", app.treeRows[0].GroupKey)
	}

	// Expand group and check that flow row uses em dash for empty process.
	sendKey(app, "l")
	app.Update(TickMsg(time.Now()))
	view := stripView(app)
	if !strings.Contains(view, "\u2014") {
		t.Error("empty process flow row should render with em dash (—)")
	}
}

func TestFullTreeCycle(t *testing.T) {
	app := newMultiHostApp()

	// Start: flat
	if app.listMode != modeFlat {
		t.Fatalf("initial mode = %d, want flat", app.listMode)
	}

	// t → host-tree
	sendKey(app, "t")
	if app.listMode != modeTree || app.treeGroupBy != groupByHost {
		t.Errorf("step 1: mode=%d groupBy=%d, want tree/host", app.listMode, app.treeGroupBy)
	}

	// t → process-tree
	sendKey(app, "t")
	if app.listMode != modeTree || app.treeGroupBy != groupByProcess {
		t.Errorf("step 2: mode=%d groupBy=%d, want tree/process", app.listMode, app.treeGroupBy)
	}

	// t → flat
	sendKey(app, "t")
	if app.listMode != modeFlat {
		t.Errorf("step 3: mode=%d, want flat", app.listMode)
	}

	// t → host-tree (wraps back)
	sendKey(app, "t")
	if app.listMode != modeTree || app.treeGroupBy != groupByHost {
		t.Errorf("step 4: mode=%d groupBy=%d, want tree/host (cycle)", app.listMode, app.treeGroupBy)
	}
}

func TestProcessTreeExpandCollapse(t *testing.T) {
	app := newProcessApp()
	sendKey(app, "t") // → host-tree
	sendKey(app, "t") // → process-tree

	// Initially collapsed — only header rows
	if len(app.treeRows) != 1 {
		t.Fatalf("expected 1 header row, got %d", len(app.treeRows))
	}

	// Expand via 'l'
	sendKey(app, "l")
	app.Update(TickMsg(time.Now()))
	if !app.groupExpanded["curl"] {
		t.Error("curl group should be expanded after l")
	}
	// 1 header + 3 flows = 4
	if len(app.treeRows) != 4 {
		t.Errorf("expected 4 rows after expand, got %d", len(app.treeRows))
	}

	// Collapse via header selection + 'h'
	app.selectedIdx = 0
	sendKey(app, "h")
	app.Update(TickMsg(time.Now()))
	if app.groupExpanded["curl"] {
		t.Error("curl group should be collapsed after h on header")
	}
	if len(app.treeRows) != 1 {
		t.Errorf("expected 1 row after collapse, got %d", len(app.treeRows))
	}
}

func TestProcessTreeFocusEscReturnsToProcessTree(t *testing.T) {
	app := newProcessApp()
	sendKey(app, "t") // → host-tree
	sendKey(app, "t") // → process-tree

	sendKey(app, "f") // → focus on "curl"
	if app.listMode != modeFocus {
		t.Fatalf("expected focus mode, got %d", app.listMode)
	}
	if app.focusKey != "curl" {
		t.Errorf("focusKey = %q, want curl", app.focusKey)
	}

	// Esc should return to process-tree
	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.listMode != modeTree {
		t.Errorf("mode after Esc = %d, want tree", app.listMode)
	}
	if app.treeGroupBy != groupByProcess {
		t.Errorf("groupBy after Esc = %d, want process", app.treeGroupBy)
	}
}

func TestFocusKeyEvicted(t *testing.T) {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	now := time.Now()
	m.metas = []store.FlowMeta{
		{ID: "a1", Host: "api.com", Path: "/a", StartedAt: now.Add(-1 * time.Second), Scheme: "https", State: store.StateCompleted, StatusCode: 200},
		{ID: "b1", Host: "cdn.com", Path: "/b", StartedAt: now, Scheme: "https", State: store.StateCompleted, StatusCode: 200},
	}
	m.data["a1"] = &store.FlowData{}
	m.data["b1"] = &store.FlowData{}

	app := NewApp(AppConfig{Store: m, Proxy: &mockProxyInfo{addr: ":9999"}, CATrusted: true})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))

	// Enter tree mode
	sendKey(app, "t")

	// cdn.com is newest so it's row 0; api.com is row 1. Focus cdn.com directly.
	if len(app.treeRows) < 1 || app.treeRows[0].GroupKey != "cdn.com" {
		t.Fatalf("expected cdn.com at row 0, got %v", app.treeRows)
	}
	sendKey(app, "f")

	if app.listMode != modeFocus || app.focusKey != "cdn.com" {
		t.Fatalf("should be in focus mode on cdn.com, got mode=%d key=%s", app.listMode, app.focusKey)
	}

	// Remove cdn.com from mock
	m.metas = m.metas[:1] // only api.com remains

	app.Update(TickMsg(time.Now()))

	// Should fall back to tree mode
	if app.listMode != modeTree {
		t.Errorf("mode = %d, want tree (focus key evicted)", app.listMode)
	}
}
