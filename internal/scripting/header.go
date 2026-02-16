package scripting

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v3"
)

// ScriptMeta holds parsed YAML frontmatter from a script file.
type ScriptMeta struct {
	ID      string   `yaml:"id,omitempty"`
	Name    string   `yaml:"name"`
	Match   []string `yaml:"match"`
	Enabled *bool    `yaml:"enabled,omitempty"`
}

// IsEnabled returns the enabled state, defaulting to true when unset.
func (m *ScriptMeta) IsEnabled() bool {
	if m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

const headerDelimiter = "// ---"

// ParseHeader extracts YAML frontmatter from a script source string.
// The frontmatter is delimited by "// ---" lines, with each YAML line
// prefixed by "// ". Returns the parsed metadata, the remaining script
// body, and any error.
func ParseHeader(source string) (*ScriptMeta, string, error) {
	lines := strings.Split(source, "\n")

	// Find opening delimiter.
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == headerDelimiter {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, "", errors.New("missing header: no opening // --- delimiter")
	}

	// Find closing delimiter.
	end := -1
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == headerDelimiter {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, "", errors.New("missing header: no closing // --- delimiter")
	}

	// Extract YAML lines, stripping the "// " prefix.
	var yamlLines []string
	for _, l := range lines[start+1 : end] {
		stripped := strings.TrimPrefix(l, "// ")
		// Handle lines that are just "//" with no trailing space.
		stripped = strings.TrimPrefix(stripped, "//")
		yamlLines = append(yamlLines, stripped)
	}
	yamlStr := strings.Join(yamlLines, "\n")

	var meta ScriptMeta
	if err := yaml.Unmarshal([]byte(yamlStr), &meta); err != nil {
		return nil, "", errors.New("invalid header YAML: " + err.Error())
	}

	if meta.Name == "" {
		return nil, "", errors.New("header missing required field: name")
	}
	if len(meta.Match) == 0 {
		return nil, "", errors.New("header missing required field: match")
	}

	// Body is everything after the closing delimiter.
	body := strings.Join(lines[end+1:], "\n")

	return &meta, body, nil
}
