package scripting

import (
	"path"
	"slices"
	"strings"
)

// GlobMatch checks if a URL matches a glob pattern.
// Pattern format: scheme://host/path where * is a wildcard.
// Matching is case-insensitive. In the host part, * matches a single
// domain label (via path.Match). In the path part, * matches any
// substring including slashes.
func GlobMatch(pattern, url string) bool {
	pattern = strings.ToLower(pattern)
	url = strings.ToLower(url)

	pScheme, pRest := splitScheme(pattern)
	uScheme, uRest := splitScheme(url)

	// Match scheme: "*" matches any scheme.
	if pScheme != "*" && pScheme != uScheme {
		return false
	}

	pHost, pPath := splitHostPath(pRest)
	uHost, uPath := splitHostPath(uRest)

	// Match host via path.Match (supports * wildcards for domain labels).
	ok, err := path.Match(pHost, uHost)
	if err != nil || !ok {
		return false
	}

	// Match path: * matches any substring (including /).
	return wildcardMatch(pPath, uPath)
}

// GlobMatchAny returns true if the URL matches any of the patterns.
func GlobMatchAny(patterns []string, url string) bool {
	return slices.ContainsFunc(patterns, func(p string) bool { return GlobMatch(p, url) })
}

// wildcardMatch matches s against pattern where * matches any substring.
func wildcardMatch(pattern, s string) bool {
	// Split pattern by * and check that all literal parts appear in order.
	parts := strings.Split(pattern, "*")

	if len(parts) == 1 {
		// No wildcards — exact match.
		return pattern == s
	}

	// First part must be a prefix.
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]

	// Middle parts must appear in order.
	for _, part := range parts[1 : len(parts)-1] {
		idx := strings.Index(s, part)
		if idx < 0 {
			return false
		}
		s = s[idx+len(part):]
	}

	// Last part must be a suffix.
	return strings.HasSuffix(s, parts[len(parts)-1])
}

// splitScheme splits "scheme://rest" into (scheme, rest).
// If no "://" is found, returns ("", s).
func splitScheme(s string) (scheme, rest string) {
	if before, after, ok := strings.Cut(s, "://"); ok {
		return before, after
	}
	return "", s
}

// splitHostPath splits "host/path" into (host, path).
// If no "/" is found, path is empty.
func splitHostPath(s string) (host, pathPart string) {
	if before, after, ok := strings.Cut(s, "/"); ok {
		return before, "/" + after
	}
	return s, ""
}
