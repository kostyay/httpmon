package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestComposeScreen(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, "C")
	if !app.showCompose {
		t.Fatal("C should open compose screen")
	}

	view := app.View()
	if !strings.Contains(view, "Compose") {
		t.Error("compose view should show 'Compose'")
	}
	if !strings.Contains(view, "URL") {
		t.Error("compose view should show URL field")
	}
}

func TestComposeEscCancels(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, "C")
	if !app.showCompose {
		t.Fatal("should be in compose")
	}

	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if app.showCompose {
		t.Error("Esc should cancel compose")
	}
}

func TestComposeMethodCycle(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, "C")

	// Default method should be GET.
	if app.composeMethod != "GET" {
		t.Errorf("default method = %q, want GET", app.composeMethod)
	}

	// Tab to cycle method.
	app.Update(tea.KeyMsg{Type: tea.KeyTab})
	if app.composeMethod == "GET" {
		t.Error("Tab should cycle method away from GET")
	}
}

func TestComposeMethodFullCycle(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, "C")

	expected := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "GET"}
	for i, want := range expected {
		if app.composeMethod != want {
			t.Errorf("step %d: method = %q, want %q", i, app.composeMethod, want)
		}
		app.updateCompose(tea.KeyMsg{Type: tea.KeyTab})
	}
}

func TestComposeFocusCycle(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, "C")

	if app.composeFocus != 0 {
		t.Fatalf("initial focus = %d, want 0", app.composeFocus)
	}

	app.updateCompose(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if app.composeFocus != 1 {
		t.Errorf("after Ctrl+J: focus = %d, want 1", app.composeFocus)
	}

	app.updateCompose(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if app.composeFocus != 2 {
		t.Errorf("after 2x Ctrl+J: focus = %d, want 2", app.composeFocus)
	}

	app.updateCompose(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if app.composeFocus != 0 {
		t.Errorf("after 3x Ctrl+J: focus = %d, want 0 (wrap)", app.composeFocus)
	}

	// Ctrl+K goes backward.
	app.updateCompose(tea.KeyMsg{Type: tea.KeyCtrlK})
	if app.composeFocus != 2 {
		t.Errorf("after Ctrl+K from 0: focus = %d, want 2", app.composeFocus)
	}
}

func TestComposeFocusFieldCorrect(t *testing.T) {
	app := newMockApp(3)
	sendKey(app, "C")

	// Focus 0: URL focused.
	if !app.composeURL.Focused() {
		t.Error("focus 0: URL should be focused")
	}
	if app.composeHeaders.Focused() {
		t.Error("focus 0: headers should NOT be focused")
	}

	app.updateCompose(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if app.composeURL.Focused() {
		t.Error("focus 1: URL should NOT be focused")
	}
	if !app.composeHeaders.Focused() {
		t.Error("focus 1: headers should be focused")
	}

	app.updateCompose(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if !app.composeBody.Focused() {
		t.Error("focus 2: body should be focused")
	}
}
