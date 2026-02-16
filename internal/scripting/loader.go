package scripting

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// generateID creates a random 16-hex-char identifier.
func generateID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// ensureScriptID backfills a missing id into the YAML header on disk.
// It inserts an "// id: <hex>" line after the opening "// ---" delimiter.
func ensureScriptID(path string, meta *ScriptMeta, source string) (string, error) {
	if meta.ID != "" {
		return meta.ID, nil
	}
	id := generateID()
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == headerDelimiter {
			// Insert id line after opening delimiter.
			idLine := fmt.Sprintf("// id: %s", id)
			newLines := make([]string, 0, len(lines)+1)
			newLines = append(newLines, lines[:i+1]...)
			newLines = append(newLines, idLine)
			newLines = append(newLines, lines[i+1:]...)
			if err := os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0o600); err != nil {
				return "", fmt.Errorf("backfill id in %s: %w", path, err)
			}
			meta.ID = id
			return id, nil
		}
	}
	return "", fmt.Errorf("no opening delimiter in %s", path)
}

// ScriptFile represents a loaded script file with metadata.
type ScriptFile struct {
	Meta     *ScriptMeta
	Source   string // JS body (after header)
	FilePath string
	Error    string // non-empty if script failed to load
}

// LoadDir reads all .js files from dir and parses their headers.
// Missing dir returns empty results (not error). Bad files go into errs
// with Error set and Meta.Name as filename fallback.
func LoadDir(dir string) (scripts []ScriptFile, errs []ScriptFile) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		data, readErr := os.ReadFile(path) // #nosec G304 -- path built from os.ReadDir entries within known dir
		if readErr != nil {
			errs = append(errs, ScriptFile{
				Meta:     &ScriptMeta{Name: filenameToName(e.Name())},
				FilePath: path,
				Error:    readErr.Error(),
			})
			continue
		}

		source := string(data)
		meta, body, parseErr := ParseHeader(source)
		if parseErr != nil {
			errs = append(errs, ScriptFile{
				Meta:     &ScriptMeta{Name: filenameToName(e.Name())},
				FilePath: path,
				Error:    parseErr.Error(),
			})
			continue
		}

		// Backfill missing ID on disk.
		if _, idErr := ensureScriptID(path, meta, source); idErr != nil {
			errs = append(errs, ScriptFile{
				Meta:     meta,
				FilePath: path,
				Error:    idErr.Error(),
			})
			continue
		}

		scripts = append(scripts, ScriptFile{
			Meta:     meta,
			Source:   body,
			FilePath: path,
		})
	}

	return scripts, errs
}

// ToggleEnabled reads a script file, flips its enabled state, and rewrites
// the file. If no "// enabled:" line exists, one is inserted before the
// closing "// ---" delimiter.
func ToggleEnabled(path string) error {
	data, err := os.ReadFile(path) // #nosec G304 -- user-selected script path from known scripts dir
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	source := string(data)
	meta, _, parseErr := ParseHeader(source)
	if parseErr != nil {
		return fmt.Errorf("parse %s: %w", path, parseErr)
	}

	newEnabled := !meta.IsEnabled()
	lines := strings.Split(source, "\n")

	// Try to find and replace existing enabled line.
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "// enabled:") {
			lines[i] = fmt.Sprintf("// enabled: %v", newEnabled)
			replaced = true
			break
		}
	}

	// If no enabled line found, insert before closing delimiter.
	if !replaced {
		// Find closing delimiter (second occurrence of "// ---").
		delimCount := 0
		for i, line := range lines {
			if strings.TrimSpace(line) == headerDelimiter {
				delimCount++
				if delimCount == 2 {
					insert := fmt.Sprintf("// enabled: %v", newEnabled)
					// Insert before closing delimiter.
					newLines := make([]string, 0, len(lines)+1)
					newLines = append(newLines, lines[:i]...)
					newLines = append(newLines, insert)
					newLines = append(newLines, lines[i:]...)
					lines = newLines
					break
				}
			}
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}

// NewScriptTemplate returns the default script template with a fresh ID.
func NewScriptTemplate() string {
	return fmt.Sprintf(`// ---
// id: %s
// name: New Script
// match:
//   - "*"
// enabled: true
// ---

function onRequest(ctx) {
  // Modify request before it is sent.
  // ctx.method, ctx.url, ctx.headers, ctx.body
}

function onResponse(ctx) {
  // Modify response before it reaches the browser.
  // ctx.status, ctx.headers, ctx.body
}
`, generateID())
}

// CreateNewScript creates dir if needed and writes the default template
// to a new file with the pattern script-*.js.
func CreateNewScript(dir string) (string, error) {
	if err := EnsureScriptDir(dir); err != nil {
		return "", err
	}

	f, err := os.CreateTemp(dir, "script-*.js")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	defer f.Close()

	path := f.Name()
	if _, err := f.WriteString(NewScriptTemplate()); err != nil {
		_ = os.Remove(path) //#nosec G703 -- path from os.CreateTemp
		return "", fmt.Errorf("write template: %w", err)
	}

	return path, nil
}

// CreateMapLocalScript creates a map-local script in dir with the given
// URL pattern and local file path. Returns the path to the created file.
func CreateMapLocalScript(dir, pattern, localPath string) (string, error) {
	if err := EnsureScriptDir(dir); err != nil {
		return "", err
	}

	slug := patternSlug(pattern)

	f, err := os.CreateTemp(dir, "mock-*.js")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	defer f.Close()

	content := fmt.Sprintf(`// ---
// id: %s
// name: mock-%s
// match:
//   - "%s"
// enabled: true
// ---

function onRequest(ctx) {
  ctx.respondWith({file: "%s"});
}
`, generateID(), slug, pattern, localPath)

	path := f.Name()
	if _, err := f.WriteString(content); err != nil {
		_ = os.Remove(path) //#nosec G703 -- path from os.CreateTemp
		return "", fmt.Errorf("write template: %w", err)
	}

	return path, nil
}

// patternSlug creates a short slug from a URL pattern for naming.
func patternSlug(pattern string) string {
	s := strings.NewReplacer(
		"*://", "", "http://", "", "https://", "",
		"*", "", "/", "-", ".", "-",
	).Replace(pattern)
	s = strings.Trim(s, "-")
	if len(s) > 30 {
		s = s[:30]
	}
	if s == "" {
		s = "local"
	}
	return s
}

// DeleteScript removes the file at path.
func DeleteScript(path string) error {
	return os.Remove(path)
}

// EnsureScriptDir creates the directory (and parents) if it doesn't exist.
func EnsureScriptDir(dir string) error {
	return os.MkdirAll(dir, 0o750)
}

// filenameToName strips the .js extension from a filename.
func filenameToName(name string) string {
	return strings.TrimSuffix(name, ".js")
}
