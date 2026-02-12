package scripting

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
		info := ScriptInfo{
			Name:     sf.Meta.Name,
			FilePath: sf.FilePath,
			Enabled:  sf.Meta.IsEnabled(),
		}
		if sf.Meta != nil {
			info.Matches = sf.Meta.Match
		}
		infos = append(infos, info)
	}
	for _, sf := range errored {
		info := ScriptInfo{
			Name:     sf.Meta.Name,
			FilePath: sf.FilePath,
			Error:    sf.Error,
		}
		infos = append(infos, info)
	}

	return infos
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
