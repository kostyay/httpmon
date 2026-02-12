package hostfilter

import "strings"

// HostFilter controls which hosts are intercepted (MITM) vs tunneled.
type HostFilter struct {
	block []string
	allow []string
}

// New creates a HostFilter. If allow is non-empty, only allowed hosts are
// intercepted; block is ignored. If only block is set, all hosts except
// blocked ones are intercepted.
func New(block, allow []string) *HostFilter {
	return &HostFilter{block: block, allow: allow}
}

// ShouldIntercept returns true if the host should be MITM intercepted.
func (hf *HostFilter) ShouldIntercept(host string) bool {
	// Strip port if present.
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	host = strings.ToLower(host)

	// Allow list takes priority: only intercept if in allow list.
	if len(hf.allow) > 0 {
		for _, pattern := range hf.allow {
			if matchHost(pattern, host) {
				return true
			}
		}
		return false
	}

	// Block list: intercept everything except blocked.
	if len(hf.block) > 0 {
		for _, pattern := range hf.block {
			if matchHost(pattern, host) {
				return false
			}
		}
	}

	return true
}

// RuleCount returns the total number of active rules.
func (hf *HostFilter) RuleCount() int {
	return len(hf.block) + len(hf.allow)
}

// matchHost matches a host against a pattern. Supports leading wildcard (*.example.com).
func matchHost(pattern, host string) bool {
	pattern = strings.ToLower(pattern)
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		return strings.HasSuffix(host, suffix)
	}
	return pattern == host
}
