package bodydecoder

import "testing"

func TestHostAwareRegistry_ExactMatch(t *testing.T) {
	fallback := emptyRegistry()
	api := emptyRegistry()
	api.methods["test/Method"] = nil // marker

	har := NewHostAwareRegistry(fallback, map[string]*ProtoRegistry{
		"api.example.com": api,
	})

	got := har.Resolve("api.example.com")
	if got != api {
		t.Error("expected exact match for api.example.com")
	}
}

func TestHostAwareRegistry_ExactMatchWithPort(t *testing.T) {
	api := emptyRegistry()
	har := NewHostAwareRegistry(nil, map[string]*ProtoRegistry{
		"api.example.com": api,
	})

	got := har.Resolve("api.example.com:443")
	if got != api {
		t.Error("expected match after stripping port")
	}
}

func TestHostAwareRegistry_WildcardMatch(t *testing.T) {
	fallback := emptyRegistry()
	wildcard := emptyRegistry()

	har := NewHostAwareRegistry(fallback, map[string]*ProtoRegistry{
		"*.example.com": wildcard,
	})

	got := har.Resolve("api.example.com")
	if got != wildcard {
		t.Error("expected wildcard match for api.example.com")
	}

	got = har.Resolve("grpc.example.com")
	if got != wildcard {
		t.Error("expected wildcard match for grpc.example.com")
	}
}

func TestHostAwareRegistry_WildcardNoMatchBareHost(t *testing.T) {
	fallback := emptyRegistry()
	wildcard := emptyRegistry()

	har := NewHostAwareRegistry(fallback, map[string]*ProtoRegistry{
		"*.example.com": wildcard,
	})

	// "example.com" itself should NOT match "*.example.com"
	got := har.Resolve("example.com")
	if got != fallback {
		t.Error("bare domain should not match wildcard, expected fallback")
	}
}

func TestHostAwareRegistry_Fallback(t *testing.T) {
	fallback := emptyRegistry()
	har := NewHostAwareRegistry(fallback, map[string]*ProtoRegistry{
		"api.example.com": emptyRegistry(),
	})

	got := har.Resolve("other.example.com")
	if got != fallback {
		t.Error("expected fallback for non-matching host")
	}
}

func TestHostAwareRegistry_NilFallback(t *testing.T) {
	har := NewHostAwareRegistry(nil, map[string]*ProtoRegistry{
		"api.example.com": emptyRegistry(),
	})

	got := har.Resolve("unknown.host")
	if got != nil {
		t.Error("expected nil for non-matching host with nil fallback")
	}
}

func TestHostAwareRegistry_CaseInsensitive(t *testing.T) {
	api := emptyRegistry()
	har := NewHostAwareRegistry(nil, map[string]*ProtoRegistry{
		"api.example.com": api,
	})

	got := har.Resolve("API.Example.COM")
	if got != api {
		t.Error("expected case-insensitive match")
	}
}

func TestHostAwareRegistry_NilReceiver(t *testing.T) {
	var har *HostAwareRegistry
	got := har.Resolve("anything")
	if got != nil {
		t.Error("nil receiver should return nil")
	}
}

func TestStaticResolver(t *testing.T) {
	reg := emptyRegistry()
	resolver := StaticResolver(reg)
	if resolver("any-host") != reg {
		t.Error("StaticResolver should always return the same registry")
	}
}

func TestMatchHostPattern(t *testing.T) {
	tests := []struct {
		pattern, host string
		want          bool
	}{
		{"api.example.com", "api.example.com", true},
		{"api.example.com", "other.example.com", false},
		{"*.example.com", "api.example.com", true},
		{"*.example.com", "grpc.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.example.com", "sub.api.example.com", true},
		{"exact", "exact", true},
		{"exact", "notexact", false},
	}
	for _, tt := range tests {
		got := matchHostPattern(tt.pattern, tt.host)
		if got != tt.want {
			t.Errorf("matchHostPattern(%q, %q) = %v, want %v", tt.pattern, tt.host, got, tt.want)
		}
	}
}
