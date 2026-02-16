package scripting

import (
	"fmt"
)

// Manager wraps Engine and provides TUI-friendly script management.
type Manager struct {
	engine *Engine
	dir    string
}

// NewManager creates a Manager for the given engine and scripts directory.
func NewManager(engine *Engine, dir string) *Manager {
	return &Manager{engine: engine, dir: dir}
}

// Scripts returns info about all scripts (enabled, disabled, and errored).
// Unlike Engine.ScriptInfos(), this scans the directory directly to include
// disabled and errored scripts that the engine doesn't load.
func (m *Manager) Scripts() []ScriptInfo {
	valid, errored := LoadDir(m.dir)

	infos := make([]ScriptInfo, 0, len(valid)+len(errored))
	for _, sf := range valid {
		infos = append(infos, ScriptInfo{
			ID:         sf.Meta.ID,
			Name:       sf.Meta.Name,
			Matches:    sf.Meta.Match,
			FilePath:   sf.FilePath,
			Enabled:    sf.Meta.IsEnabled(),
			Categories: DetectCategories(sf.Source),
		})
	}
	for _, sf := range errored {
		infos = append(infos, ScriptInfo{
			Name:     sf.Meta.Name,
			FilePath: sf.FilePath,
			Error:    sf.Error,
		})
	}

	return infos
}

// ScriptByID finds a script by its unique ID.
func (m *Manager) ScriptByID(id string) (ScriptInfo, bool) {
	for _, info := range m.Scripts() {
		if info.ID == id {
			return info, true
		}
	}
	return ScriptInfo{}, false
}

// Toggle flips the enabled state of a script file and reloads the engine.
func (m *Manager) Toggle(filePath string) error {
	if err := ToggleEnabled(filePath); err != nil {
		return err
	}
	m.Reload()
	return nil
}

// Delete removes a script file and reloads the engine.
func (m *Manager) Delete(filePath string) error {
	if err := DeleteScript(filePath); err != nil {
		return err
	}
	m.Reload()
	return nil
}

// CreateNew creates a new script from the default template and returns its path.
func (m *Manager) CreateNew() (string, error) {
	return CreateNewScript(m.dir)
}

// ScriptDir returns the scripts directory path.
func (m *Manager) ScriptDir() string {
	return m.dir
}

// Reload re-scans the scripts directory and reloads the engine.
func (m *Manager) Reload() {
	m.engine.Reload(m.dir)
}

// QuickAddMapLocal creates a map-local script with respondWith({file}).
func (m *Manager) QuickAddMapLocal(
	pattern, localPath string,
) (string, error) {
	path, err := CreateMapLocalScript(m.dir, pattern, localPath)
	if err != nil {
		return "", fmt.Errorf("quick add map-local: %w", err)
	}
	m.Reload()
	return path, nil
}
