package maplocal

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Rule maps a URL pattern to a local file.
type Rule struct {
	Pattern    string            `json:"pattern"`
	LocalPath  string            `json:"local_path"`
	StatusCode int               `json:"status_code,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
}

// MapLocal holds rules for serving local files instead of upstream responses.
type MapLocal struct {
	mu    sync.RWMutex
	rules []Rule
}

// New creates an empty MapLocal.
func New() *MapLocal {
	return &MapLocal{}
}

// AddRule adds a mapping rule.
func (ml *MapLocal) AddRule(r Rule) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	if r.StatusCode == 0 {
		r.StatusCode = 200
	}
	ml.rules = append(ml.rules, r)
}

// Rules returns a copy of all rules.
func (ml *MapLocal) Rules() []Rule {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	out := make([]Rule, len(ml.rules))
	copy(out, ml.rules)
	return out
}

// RemoveRule removes the rule at the given index.
func (ml *MapLocal) RemoveRule(index int) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	if index < 0 || index >= len(ml.rules) {
		return
	}
	ml.rules = append(ml.rules[:index], ml.rules[index+1:]...)
}

// RuleCount returns the number of active rules.
func (ml *MapLocal) RuleCount() int {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	return len(ml.rules)
}

// Match checks if a host+path matches any rule. Returns body, status, and whether matched.
// Falls through (returns false) if the local file doesn't exist.
func (ml *MapLocal) Match(host, path string) ([]byte, int, bool) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	target := host + path

	for _, r := range ml.rules {
		if matchPattern(r.Pattern, target) {
			body, err := os.ReadFile(r.LocalPath) // #nosec G304 -- user-configured local path
			if err != nil {
				continue
			}
			return body, r.StatusCode, true
		}
	}
	return nil, 0, false
}

// LoadFromFile loads rules from a JSON file (array of Rule objects).
func (ml *MapLocal) LoadFromFile(path string) error {
	data, err := os.ReadFile(path) // #nosec G304 -- user-configured path
	if err != nil {
		return fmt.Errorf("read maplocal rules: %w", err)
	}

	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return fmt.Errorf("parse maplocal rules: %w", err)
	}

	ml.mu.Lock()
	defer ml.mu.Unlock()
	for i := range rules {
		if rules[i].StatusCode == 0 {
			rules[i].StatusCode = 200
		}
	}
	ml.rules = rules
	return nil
}

// SaveToFile persists current rules to a JSON file.
func (ml *MapLocal) SaveToFile(path string) error {
	ml.mu.RLock()
	data, err := json.MarshalIndent(ml.rules, "", "  ")
	ml.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal maplocal rules: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write maplocal rules: %w", err)
	}
	return nil
}

// matchPattern matches a URL string against a pattern with wildcards.
// Supports * as a glob: *.example.com/api/* matches sub.example.com/api/anything.
func matchPattern(pattern, target string) bool {
	pattern = strings.ToLower(pattern)
	target = strings.ToLower(target)

	if !strings.Contains(pattern, "*") {
		return pattern == target
	}

	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(target[pos:], part)
		if idx < 0 {
			return false
		}
		if i == 0 && idx != 0 {
			return false
		}
		pos += idx + len(part)
	}
	if !strings.HasSuffix(pattern, "*") && pos != len(target) {
		return false
	}
	return true
}
