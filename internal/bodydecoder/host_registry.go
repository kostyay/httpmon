package bodydecoder

import "strings"

// ProtoRegResolver returns the ProtoRegistry for a given host.
type ProtoRegResolver func(host string) *ProtoRegistry

// HostAwareRegistry maps host patterns to per-host ProtoRegistry instances
// and falls back to a default registry when no pattern matches.
type HostAwareRegistry struct {
	fallback *ProtoRegistry
	hosts    map[string]*ProtoRegistry // host pattern → registry
}

// NewHostAwareRegistry creates a registry that resolves by host pattern.
// The fallback is used when no pattern matches.
func NewHostAwareRegistry(fallback *ProtoRegistry, hosts map[string]*ProtoRegistry) *HostAwareRegistry {
	return &HostAwareRegistry{fallback: fallback, hosts: hosts}
}

// Resolve returns the ProtoRegistry for the given host.
// Exact match is tried first, then wildcard patterns (*.example.com).
// Falls back to the default registry if nothing matches.
func (r *HostAwareRegistry) Resolve(host string) *ProtoRegistry {
	if r == nil {
		return nil
	}

	// Strip port if present.
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	host = strings.ToLower(host)

	// Exact match.
	if reg, ok := r.hosts[host]; ok {
		return reg
	}

	// Wildcard match: *.example.com matches foo.example.com.
	// Non-wildcard entries were already handled by the exact lookup above.
	for pattern, reg := range r.hosts {
		if strings.HasPrefix(pattern, "*.") && matchHostPattern(pattern, host) {
			return reg
		}
	}

	return r.fallback
}

// Resolver returns a ProtoRegResolver func backed by this HostAwareRegistry.
func (r *HostAwareRegistry) Resolver() ProtoRegResolver {
	return r.Resolve
}

// StaticResolver returns a ProtoRegResolver that always returns the given registry.
func StaticResolver(reg *ProtoRegistry) ProtoRegResolver {
	return func(_ string) *ProtoRegistry { return reg }
}

// matchHostPattern checks if host matches a wildcard pattern like *.example.com.
func matchHostPattern(pattern, host string) bool {
	if !strings.HasPrefix(pattern, "*.") {
		return pattern == host
	}
	suffix := pattern[1:] // ".example.com"
	return strings.HasSuffix(host, suffix) && len(host) > len(suffix)
}
