package scripting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

const validScript = `// ---
// name: Test Script
// match:
//   - "*"
// enabled: true
// ---

function onRequest(ctx) {}
`

const disabledScript = `// ---
// name: Disabled Script
// match:
//   - "*"
// enabled: false
// ---

function onRequest(ctx) {}
`

const badScript = `function onRequest(ctx) {}`

func TestLoadDir_Valid(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "test.js", validScript)

	scripts, errs := LoadDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(scripts) != 1 {
		t.Fatalf("got %d scripts, want 1", len(scripts))
	}

	s := scripts[0]
	if s.Meta.Name != "Test Script" {
		t.Errorf("Name = %q, want %q", s.Meta.Name, "Test Script")
	}
	if s.FilePath != filepath.Join(dir, "test.js") {
		t.Errorf("FilePath = %q, want %q", s.FilePath, filepath.Join(dir, "test.js"))
	}
	if !strings.Contains(s.Source, "function onRequest") {
		t.Error("Source should contain script body")
	}
	if s.Error != "" {
		t.Errorf("unexpected Error: %s", s.Error)
	}
}

func TestLoadDir_BadYAML(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "bad.js", badScript)

	scripts, errs := LoadDir(dir)
	if len(scripts) != 0 {
		t.Fatalf("got %d scripts, want 0", len(scripts))
	}
	if len(errs) != 1 {
		t.Fatalf("got %d errs, want 1", len(errs))
	}

	e := errs[0]
	if e.Error == "" {
		t.Error("Error should be set for bad script")
	}
	if e.Meta == nil {
		t.Fatal("Meta should not be nil for bad script")
	}
	if e.Meta.Name != "bad" {
		t.Errorf("Meta.Name = %q, want %q (filename fallback)", e.Meta.Name, "bad")
	}
}

func TestLoadDir_Empty(t *testing.T) {
	dir := t.TempDir()

	scripts, errs := LoadDir(dir)
	if len(scripts) != 0 {
		t.Errorf("got %d scripts, want 0", len(scripts))
	}
	if len(errs) != 0 {
		t.Errorf("got %d errs, want 0", len(errs))
	}
}

func TestLoadDir_MissingDir(t *testing.T) {
	scripts, errs := LoadDir("/tmp/nonexistent-loader-test-dir-12345")

	if len(scripts) != 0 {
		t.Errorf("got %d scripts, want 0", len(scripts))
	}
	if len(errs) != 0 {
		t.Errorf("got %d errs, want 0", len(errs))
	}
}

func TestLoadDir_MultipleScripts(t *testing.T) {
	dir := t.TempDir()
	writeTestScript(t, dir, "enabled.js", validScript)
	writeTestScript(t, dir, "disabled.js", disabledScript)

	scripts, errs := LoadDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(scripts) != 2 {
		t.Fatalf("got %d scripts, want 2", len(scripts))
	}

	var foundEnabled, foundDisabled bool
	for _, s := range scripts {
		switch s.Meta.Name {
		case "Test Script":
			foundEnabled = true
			if !s.Meta.IsEnabled() {
				t.Error("Test Script should be enabled")
			}
		case "Disabled Script":
			foundDisabled = true
			if s.Meta.IsEnabled() {
				t.Error("Disabled Script should be disabled")
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

func TestToggleEnabled(t *testing.T) {
	dir := t.TempDir()
	path := writeTestScript(t, dir, "toggle.js", validScript)

	// Toggle true -> false.
	if err := ToggleEnabled(path); err != nil {
		t.Fatalf("toggle true->false: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "enabled: false") {
		t.Errorf("expected enabled: false after toggle, got:\n%s", data)
	}

	// Toggle false -> true.
	if err := ToggleEnabled(path); err != nil {
		t.Fatalf("toggle false->true: %v", err)
	}

	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "enabled: true") {
		t.Errorf("expected enabled: true after toggle, got:\n%s", data)
	}
}

func TestToggleEnabled_InsertLine(t *testing.T) {
	// Script without an enabled line.
	noEnabledScript := `// ---
// name: No Enabled
// match:
//   - "*"
// ---

function onRequest(ctx) {}
`
	dir := t.TempDir()
	path := writeTestScript(t, dir, "noenabled.js", noEnabledScript)

	// Default is true, so toggling should insert enabled: false.
	if err := ToggleEnabled(path); err != nil {
		t.Fatalf("toggle insert: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "enabled: false") {
		t.Errorf("expected inserted enabled: false, got:\n%s", data)
	}

	// Verify it still parses.
	meta, _, parseErr := ParseHeader(string(data))
	if parseErr != nil {
		t.Fatalf("reparse: %v", parseErr)
	}
	if meta.IsEnabled() {
		t.Error("should be disabled after toggle insert")
	}
}

func TestCreateNewScript(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scripts")

	path, err := CreateNewScript(dir)
	if err != nil {
		t.Fatalf("CreateNewScript: %v", err)
	}

	if !strings.HasPrefix(path, dir) {
		t.Errorf("path %q not in dir %q", path, dir)
	}
	if !strings.HasSuffix(path, ".js") {
		t.Errorf("path %q should end with .js", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Should be a valid script.
	meta, _, parseErr := ParseHeader(string(data))
	if parseErr != nil {
		t.Fatalf("template parse: %v", parseErr)
	}
	if meta.Name == "" {
		t.Error("template should have a name")
	}
}

func TestDeleteScript(t *testing.T) {
	dir := t.TempDir()
	path := writeTestScript(t, dir, "del.js", validScript)

	if err := DeleteScript(path); err != nil {
		t.Fatalf("DeleteScript: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestCreateMapLocalScript(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scripts")

	path, err := CreateMapLocalScript(dir, "*://api.example.com/*", "./mock.json")
	if err != nil {
		t.Fatalf("CreateMapLocalScript: %v", err)
	}

	if !strings.HasPrefix(path, dir) {
		t.Errorf("path %q not in dir %q", path, dir)
	}
	if !strings.HasSuffix(path, ".js") {
		t.Errorf("path %q should end with .js", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	source := string(data)

	meta, body, parseErr := ParseHeader(source)
	if parseErr != nil {
		t.Fatalf("template parse: %v", parseErr)
	}
	if meta.Name == "" {
		t.Error("should have a name")
	}
	if len(meta.Match) != 1 || meta.Match[0] != "*://api.example.com/*" {
		t.Errorf("match = %v, want [*://api.example.com/*]", meta.Match)
	}
	if !meta.IsEnabled() {
		t.Error("should be enabled")
	}
	if !strings.Contains(body, `ctx.respondWith`) {
		t.Error("body should contain ctx.respondWith call")
	}
	if !strings.Contains(body, `"./mock.json"`) {
		t.Error("body should contain the file path")
	}
}

func TestCreateMapLocalScript_Slug(t *testing.T) {
	dir := t.TempDir()

	path, err := CreateMapLocalScript(dir, "*://api.example.com/v1/users*", "./data.json")
	if err != nil {
		t.Fatalf("CreateMapLocalScript: %v", err)
	}

	base := filepath.Base(path)
	if !strings.HasPrefix(base, "mock-") {
		t.Errorf("filename %q should start with 'mock-'", base)
	}
}

func TestFilenameToName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"test.js", "test"},
		{"my-script.js", "my-script"},
		{"noext", "noext"},
	}
	for _, tt := range tests {
		got := filenameToName(tt.in)
		if got != tt.want {
			t.Errorf("filenameToName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
