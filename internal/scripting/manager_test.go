package scripting

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager_Scripts(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "enabled.js", validScript)
	writeTestScript(t, dir, "disabled.js", disabledScript)

	eng := New()
	mgr := NewManager(eng, dir)
	mgr.Reload()

	infos := mgr.Scripts()
	if len(infos) != 2 {
		t.Fatalf("got %d scripts, want 2", len(infos))
	}

	var foundEnabled, foundDisabled bool
	for _, info := range infos {
		switch info.Name {
		case "Test Script":
			foundEnabled = true
			if !info.Enabled {
				t.Error("Test Script should be enabled")
			}
			if info.FilePath != filepath.Join(dir, "enabled.js") {
				t.Errorf("FilePath = %q, want %q", info.FilePath, filepath.Join(dir, "enabled.js"))
			}
			if len(info.Matches) == 0 || info.Matches[0] != "*" {
				t.Errorf("Matches = %v, want [*]", info.Matches)
			}
			if info.Error != "" {
				t.Errorf("unexpected Error: %s", info.Error)
			}
		case "Disabled Script":
			foundDisabled = true
			if info.Enabled {
				t.Error("Disabled Script should be disabled")
			}
			if info.FilePath != filepath.Join(dir, "disabled.js") {
				t.Errorf("FilePath = %q, want %q", info.FilePath, filepath.Join(dir, "disabled.js"))
			}
		}
	}
	if !foundEnabled {
		t.Error("missing enabled script")
	}
	if !foundDisabled {
		t.Error("missing disabled script")
	}
}

func TestManager_Scripts_IncludesErrored(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "good.js", validScript)
	writeTestScript(t, dir, "bad.js", badScript)

	eng := New()
	mgr := NewManager(eng, dir)
	mgr.Reload()

	infos := mgr.Scripts()
	if len(infos) != 2 {
		t.Fatalf("got %d scripts, want 2", len(infos))
	}

	var foundGood, foundBad bool
	for _, info := range infos {
		switch {
		case info.Name == "Test Script":
			foundGood = true
			if info.Error != "" {
				t.Errorf("good script has unexpected error: %s", info.Error)
			}
		case info.Error != "":
			foundBad = true
			if info.FilePath != filepath.Join(dir, "bad.js") {
				t.Errorf("bad script FilePath = %q", info.FilePath)
			}
		}
	}
	if !foundGood {
		t.Error("missing good script")
	}
	if !foundBad {
		t.Error("missing errored script")
	}
}

func TestManager_Toggle(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "toggle.js", validScript)

	eng := New()
	mgr := NewManager(eng, dir)
	mgr.Reload()

	path := filepath.Join(dir, "toggle.js")

	// Initially enabled; toggle to disabled.
	if err := mgr.Toggle(path); err != nil {
		t.Fatalf("Toggle: %v", err)
	}

	infos := mgr.Scripts()
	if len(infos) != 1 {
		t.Fatalf("got %d scripts, want 1", len(infos))
	}
	if infos[0].Enabled {
		t.Error("should be disabled after toggle")
	}

	// Toggle back to enabled.
	if err := mgr.Toggle(path); err != nil {
		t.Fatalf("Toggle back: %v", err)
	}

	infos = mgr.Scripts()
	if len(infos) != 1 {
		t.Fatalf("got %d scripts, want 1", len(infos))
	}
	if !infos[0].Enabled {
		t.Error("should be enabled after second toggle")
	}
}

func TestManager_Delete(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "del.js", validScript)

	eng := New()
	mgr := NewManager(eng, dir)
	mgr.Reload()

	path := filepath.Join(dir, "del.js")
	if err := mgr.Delete(path); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}

	infos := mgr.Scripts()
	if len(infos) != 0 {
		t.Errorf("got %d scripts, want 0 after delete", len(infos))
	}
}

func TestManager_CreateNew(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scripts")

	eng := New()
	mgr := NewManager(eng, dir)

	path, err := mgr.CreateNew()
	if err != nil {
		t.Fatalf("CreateNew: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("created file should exist: %v", err)
	}

	// Verify it parses.
	data, _ := os.ReadFile(path)
	meta, _, parseErr := ParseHeader(string(data))
	if parseErr != nil {
		t.Fatalf("template parse: %v", parseErr)
	}
	if meta.Name == "" {
		t.Error("template should have a name")
	}
}

func TestManager_Reload(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "initial.js", validScript)

	eng := New()
	mgr := NewManager(eng, dir)
	mgr.Reload()

	infos := mgr.Scripts()
	if len(infos) != 1 {
		t.Fatalf("got %d scripts, want 1 initially", len(infos))
	}

	// Add another script file.
	writeTestScript(t, dir, "added.js", disabledScript)
	mgr.Reload()

	infos = mgr.Scripts()
	if len(infos) != 2 {
		t.Fatalf("got %d scripts, want 2 after reload", len(infos))
	}
}

func TestManager_ScriptDir(t *testing.T) {
	eng := New()
	mgr := NewManager(eng, "/some/dir")

	if mgr.ScriptDir() != "/some/dir" {
		t.Errorf("ScriptDir() = %q, want %q", mgr.ScriptDir(), "/some/dir")
	}
}
