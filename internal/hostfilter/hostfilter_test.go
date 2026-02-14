package hostfilter

import (
	"testing"
)

func TestBlockRule(t *testing.T) {
	hf := New([]string{"*.ads.example.com", "tracker.io"}, nil)
	if hf.ShouldIntercept("sub.ads.example.com") {
		t.Error("sub.ads.example.com should be blocked")
	}
	if !hf.ShouldIntercept("ads.example.com") {
		t.Error("ads.example.com should NOT be blocked (wildcard requires subdomain)")
	}
	if hf.ShouldIntercept("tracker.io") {
		t.Error("tracker.io should be blocked (exact)")
	}
	if !hf.ShouldIntercept("api.example.com") {
		t.Error("api.example.com should NOT be blocked")
	}
}

func TestAllowRule(t *testing.T) {
	hf := New(nil, []string{"api.example.com", "*.stripe.com"})
	if !hf.ShouldIntercept("api.example.com") {
		t.Error("api.example.com should be allowed (exact)")
	}
	if !hf.ShouldIntercept("dashboard.stripe.com") {
		t.Error("dashboard.stripe.com should be allowed (wildcard)")
	}
	if hf.ShouldIntercept("other.io") {
		t.Error("other.io should NOT be allowed")
	}
}

func TestWildcardMatch(t *testing.T) {
	tests := []struct {
		pattern string
		host    string
		want    bool
	}{
		{"*.example.com", "sub.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.example.com", "deep.sub.example.com", true},
		{"example.com", "example.com", true},
		{"example.com", "sub.example.com", false},
	}
	for _, tt := range tests {
		got := matchHost(tt.pattern, tt.host)
		if got != tt.want {
			t.Errorf("matchHost(%q, %q) = %v, want %v", tt.pattern, tt.host, got, tt.want)
		}
	}
}

func TestBlockAllowMutuallyExclusive(t *testing.T) {
	// When both block and allow are set, allow takes priority
	hf := New([]string{"tracker.io"}, []string{"api.example.com"})
	// Only allowed hosts should be intercepted.
	if !hf.ShouldIntercept("api.example.com") {
		t.Error("api.example.com should be intercepted (in allow list)")
	}
	if hf.ShouldIntercept("tracker.io") {
		t.Error("tracker.io should NOT be intercepted (not in allow list)")
	}
	if hf.ShouldIntercept("random.com") {
		t.Error("random.com should NOT be intercepted (not in allow list)")
	}
}

func TestRuleCount(t *testing.T) {
	hf := New([]string{"a.com", "b.com"}, nil)
	if hf.RuleCount() != 2 {
		t.Errorf("RuleCount = %d, want 2", hf.RuleCount())
	}

	hf2 := New(nil, []string{"c.com"})
	if hf2.RuleCount() != 1 {
		t.Errorf("RuleCount = %d, want 1", hf2.RuleCount())
	}
}

func TestNoRules(t *testing.T) {
	hf := New(nil, nil)
	if !hf.ShouldIntercept("anything.com") {
		t.Error("no rules means intercept everything")
	}
}
