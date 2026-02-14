package scripting

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

const scriptTemplate = `// ---
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
`

// NewScriptTemplate returns the default script template string.
func NewScriptTemplate() string {
	return scriptTemplate
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

	if _, err := f.WriteString(NewScriptTemplate()); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("write template: %w", err)
	}

	return f.Name(), nil
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
