package maplocal

import (
	"os"
	"strings"
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
	rules []Rule
}

// New creates an empty MapLocal.
func New() *MapLocal {
	return &MapLocal{}
}

// AddRule adds a mapping rule.
func (ml *MapLocal) AddRule(r Rule) {
	if r.StatusCode == 0 {
		r.StatusCode = 200
	}
	ml.rules = append(ml.rules, r)
}

// RuleCount returns the number of active rules.
func (ml *MapLocal) RuleCount() int {
	return len(ml.rules)
}

// Match checks if a host+path matches any rule. Returns body, status, and whether matched.
// Falls through (returns false) if the local file doesn't exist.
func (ml *MapLocal) Match(host, path string) ([]byte, int, bool) {
	target := host + path

	for _, r := range ml.rules {
		if matchPattern(r.Pattern, target) {
			body, err := os.ReadFile(r.LocalPath)
			if err != nil {
				// File missing → fall through.
				continue
			}
			return body, r.StatusCode, true
		}
	}
	return nil, 0, false
}

// matchPattern matches a URL string against a pattern with wildcards.
// Supports * as a glob: *.example.com/api/* matches sub.example.com/api/anything.
func matchPattern(pattern, target string) bool {
	pattern = strings.ToLower(pattern)
	target = strings.ToLower(target)

	// Simple case: no wildcards.
	if !strings.Contains(pattern, "*") {
		return pattern == target
	}

	// Split by * and match segments in order.
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
		// First segment must match at start if pattern doesn't start with *.
		if i == 0 && idx != 0 {
			return false
		}
		pos += idx + len(part)
	}
	// If pattern doesn't end with *, target must end at pos.
	if !strings.HasSuffix(pattern, "*") && pos != len(target) {
		return false
	}
	return true
}
