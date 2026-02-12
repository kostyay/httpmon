package tui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kostyay/httpmon/internal/store"
)

func TestMenuOpenFromList(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, " ")
	if !app.showMenu {
		t.Fatal("Space should open menu from list view")
	}
	labels := menuLabels(app.menuItems)
	for _, want := range []string{"Open Detail", "Export HAR", "Compose Request", "Mark for Diff", "Toggle Tree/Flat"} {
		if !slices.Contains(labels, want) {
			t.Errorf("list menu should contain %q, got %v", want, labels)
		}
	}
}

func TestMenuOpenFromDetail(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyMsg{Type: tea.KeyEnter}) // enter detail

	sendKey(app, " ")
	if !app.showMenu {
		t.Fatal("Space should open menu from detail view")
	}
	labels := menuLabels(app.menuItems)
	for _, want := range []string{"Export HAR", "Copy cURL", "Repeat Request", "Open in Editor", "Toggle Pretty/Raw"} {
		if !slices.Contains(labels, want) {
			t.Errorf("detail menu should contain %q, got %v", want, labels)
		}
	}
}

func TestMenuEscDismisses(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, " ")
	if !app.showMenu {
		t.Fatal("menu should be open")
	}
	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if app.showMenu {
		t.Error("Esc should dismiss menu")
	}
}

func TestMenuSpaceToggle(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, " ") // open
	if !app.showMenu {
		t.Fatal("first Space should open menu")
	}
	sendKey(app, " ") // close
	if app.showMenu {
		t.Error("second Space should dismiss menu")
	}
}

func TestMenuNavigate(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, " ")

	if app.menuCursor != 0 {
		t.Fatalf("initial menuCursor = %d, want 0", app.menuCursor)
	}
	sendKey(app, "j")
	if app.menuCursor != 1 {
		t.Errorf("j: menuCursor = %d, want 1", app.menuCursor)
	}
	sendKey(app, "k")
	if app.menuCursor != 0 {
		t.Errorf("k: menuCursor = %d, want 0", app.menuCursor)
	}
	// Bounds: can't go above 0
	sendKey(app, "k")
	if app.menuCursor != 0 {
		t.Errorf("k at 0: menuCursor = %d, want 0", app.menuCursor)
	}
	// Bounds: can't go below len-1
	for range len(app.menuItems) + 2 {
		sendKey(app, "j")
	}
	if app.menuCursor != len(app.menuItems)-1 {
		t.Errorf("j overflow: menuCursor = %d, want %d", app.menuCursor, len(app.menuItems)-1)
	}
}

func TestMenuEnterDispatches(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, " ")

	// Find "Export HAR" and navigate to it.
	idx := findMenuItem(app.menuItems, "Export HAR")
	if idx < 0 {
		t.Fatal("menu should contain Export HAR")
	}
	for range idx {
		sendKey(app, "j")
	}
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if app.showMenu {
		t.Error("Enter should close menu")
	}
	if !app.showExport {
		t.Error("selecting Export HAR should open export modal")
	}
}

func TestMenuOpenDetailDispatches(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, " ")

	idx := findMenuItem(app.menuItems, "Open Detail")
	if idx < 0 {
		t.Fatal("menu should contain Open Detail")
	}
	for range idx {
		sendKey(app, "j")
	}
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if app.showMenu {
		t.Error("Enter should close menu")
	}
	if !app.showDetail {
		t.Error("selecting Open Detail should enter detail view")
	}
}

func TestMenuTreeShowsFocusHost(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, "t") // switch to tree
	if app.listMode != modeTree {
		t.Fatal("should be in tree mode")
	}

	sendKey(app, " ")
	labels := menuLabels(app.menuItems)
	if !slices.Contains(labels, "Focus Host") {
		t.Errorf("tree menu should contain 'Focus Host', got %v", labels)
	}
}

func TestMenuFlatNoFocusHost(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, " ")
	labels := menuLabels(app.menuItems)
	if slices.Contains(labels, "Focus Host") {
		t.Error("flat menu should NOT contain 'Focus Host'")
	}
}

func TestMenuDetailImageItem(t *testing.T) {
	m := &mockFlowReader{data: make(map[store.FlowID]*store.FlowData)}
	m.metas = []store.FlowMeta{
		{ID: "img-1", Method: "GET", Host: "cdn.example.com", Path: "/photo.png", State: store.StateCompleted, StatusCode: 200, ContentType: "image/png", Scheme: "https"},
	}
	m.data["img-1"] = &store.FlowData{
		RequestHeaders:  map[string][]string{},
		ResponseHeaders: map[string][]string{"Content-Type": {"image/png"}},
		ResponseBody:    []byte{0x89, 0x50, 0x4E, 0x47}, // PNG magic bytes
	}
	app := NewApp(m, &mockProxyInfo{addr: ":9999"}, true, nil)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(tickMsg{})

	// Enter detail on response tab (where image body is).
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	sendKey(app, "2") // switch to response tab

	sendKey(app, " ")
	labels := menuLabels(app.menuItems)
	if !slices.Contains(labels, "Toggle Image Preview") {
		t.Errorf("detail menu with image body should contain 'Toggle Image Preview', got %v", labels)
	}
}

func TestMenuNoOpenWhenFilterFocused(t *testing.T) {
	app := newMockApp(3)

	sendKey(app, "/") // focus filter
	if !app.filterInput.Focused() {
		t.Fatal("filter should be focused")
	}
	sendKey(app, " ")
	if app.showMenu {
		t.Error("Space should not open menu when filter is focused")
	}
}

func TestMenuNoOpenDuringSearch(t *testing.T) {
	app := newMockApp(3)
	app.Update(tea.KeyMsg{Type: tea.KeyEnter}) // enter detail

	sendKey(app, "/") // start detail search
	if !app.detailSearch {
		t.Fatal("should be in detail search mode")
	}
	sendKey(app, " ")
	if app.showMenu {
		t.Error("Space should not open menu during detail search")
	}
}

func TestMenuViewContent(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, " ")

	view := app.View()
	if !strings.Contains(view, "Actions") {
		t.Error("menu view should contain 'Actions' header")
	}
	for _, item := range app.menuItems {
		if !strings.Contains(view, item.label) {
			t.Errorf("menu view should contain label %q", item.label)
		}
		if !strings.Contains(view, item.displayKey()) {
			t.Errorf("menu view should contain shortcut %q", item.displayKey())
		}
	}
}

// --- helpers ---

func menuLabels(items []menuItem) []string {
	labels := make([]string, len(items))
	for i, item := range items {
		labels[i] = item.label
	}
	return labels
}

func findMenuItem(items []menuItem, label string) int {
	for i, item := range items {
		if item.label == label {
			return i
		}
	}
	return -1
}
