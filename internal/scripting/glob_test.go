package scripting

import "testing"

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		url     string
		want    bool
	}{
		// Exact match.
		{"http://example.com/api", "http://example.com/api", true},

		// Wildcard scheme.
		{"*://example.com/api", "https://example.com/api", true},
		{"*://example.com/api", "http://example.com/api", true},

		// Wildcard path.
		{"*://example.com/*", "https://example.com/foo/bar", true},
		{"*://example.com/*", "https://example.com/", true},

		// Wildcard host subdomain.
		{"*://*.example.com/*", "https://api.example.com/v1", true},
		{"*://*.example.com/*", "https://example.com/v1", false},

		// Host mismatch.
		{"*://api.example.com/*", "https://other.com/api", false},

		// Case insensitive.
		{"*://API.Example.COM/*", "https://api.example.com/foo", true},

		// Multi-segment path with wildcards.
		{"*://*.example.com/*/users/*", "https://api.example.com/v1/users/123", true},
		{"*://*.example.com/*/users/*", "https://api.example.com/v1/posts/123", false},

		// No path in pattern — should NOT match URLs with path.
		{"*://example.com", "https://example.com", true},
		{"*://example.com", "https://example.com/path", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.url, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.url)
			if got != tt.want {
				t.Errorf("GlobMatch(%q, %q) = %v, want %v", tt.pattern, tt.url, got, tt.want)
			}
		})
	}
}

func TestGlobMatchAny(t *testing.T) {
	patterns := []string{
		"*://api.example.com/*",
		"*://cdn.example.com/*",
	}

	tests := []struct {
		url  string
		want bool
	}{
		{"https://api.example.com/v1", true},
		{"https://cdn.example.com/img.png", true},
		{"https://other.com/foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := GlobMatchAny(patterns, tt.url)
			if got != tt.want {
				t.Errorf("GlobMatchAny(%v, %q) = %v, want %v", patterns, tt.url, got, tt.want)
			}
		})
	}

	// Empty patterns matches nothing.
	if GlobMatchAny(nil, "https://example.com") {
		t.Error("GlobMatchAny(nil, ...) should return false")
	}
}
