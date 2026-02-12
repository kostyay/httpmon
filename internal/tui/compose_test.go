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
