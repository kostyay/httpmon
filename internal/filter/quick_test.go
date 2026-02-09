package filter

import (
	"testing"

	"github.com/kostyay/httpmon/internal/store"
)

func TestCompileQuickEmpty(t *testing.T) {
	f := CompileQuick("")
	if f != nil {
		t.Error("empty input should return nil")
	}

	f = CompileQuick("   ")
	if f != nil {
		t.Error("whitespace-only input should return nil")
	}
}

func TestQuickFilterCaseInsensitive(t *testing.T) {
	f := CompileQuick("STRIPE")
	flow := &store.FlowMeta{Host: "api.stripe.com", Path: "/v1/charges"}
	if !f.Match(flow) {
		t.Error("should match case-insensitive")
	}
}

func TestQuickFilterMatchesHost(t *testing.T) {
	f := CompileQuick("stripe")
	flow := &store.FlowMeta{Host: "api.stripe.com", Path: "/v1/charges"}
	if !f.Match(flow) {
		t.Error("should match host")
	}
}

func TestQuickFilterMatchesPath(t *testing.T) {
	f := CompileQuick("charges")
	flow := &store.FlowMeta{Host: "api.stripe.com", Path: "/v1/charges"}
	if !f.Match(flow) {
		t.Error("should match path")
	}
}

func TestQuickFilterMatchesCombined(t *testing.T) {
	f := CompileQuick("com/v1")
	flow := &store.FlowMeta{Host: "api.stripe.com", Path: "/v1/charges"}
	if !f.Match(flow) {
		t.Error("should match across host+path boundary")
	}
}

func TestQuickFilterNoMatch(t *testing.T) {
	f := CompileQuick("github")
	flow := &store.FlowMeta{Host: "api.stripe.com", Path: "/v1/charges"}
	if f.Match(flow) {
		t.Error("should not match")
	}
}
