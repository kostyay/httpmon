package filter

import (
	"testing"

	"github.com/kostyay/httpmon/internal/store"
)

func flow(method string, status int, host, path, ct string) *store.FlowMeta {
	return &store.FlowMeta{
		Method:      method,
		StatusCode:  status,
		Host:        host,
		Path:        path,
		ContentType: ct,
	}
}

func TestParseFilterTerms(t *testing.T) {
	terms := parseTerms("m:POST s:2xx api.example")
	if len(terms) != 3 {
		t.Fatalf("parseTerms count = %d, want 3", len(terms))
	}
}

func TestStatusFilterExact(t *testing.T) {
	f := Compile("s:200")
	if f == nil {
		t.Fatal("compile returned nil")
	}
	if !f.Match(flow("GET", 200, "x.com", "/", "")) {
		t.Error("s:200 should match status 200")
	}
	if f.Match(flow("GET", 201, "x.com", "/", "")) {
		t.Error("s:200 should NOT match status 201")
	}
}

func TestStatusFilterRange(t *testing.T) {
	f := Compile("s:2xx")
	if !f.Match(flow("GET", 200, "x.com", "/", "")) {
		t.Error("s:2xx should match 200")
	}
	if !f.Match(flow("GET", 299, "x.com", "/", "")) {
		t.Error("s:2xx should match 299")
	}
	if f.Match(flow("GET", 404, "x.com", "/", "")) {
		t.Error("s:2xx should NOT match 404")
	}
}

func TestStatusFilter4xx(t *testing.T) {
	f := Compile("s:4xx")
	if !f.Match(flow("GET", 404, "x.com", "/", "")) {
		t.Error("s:4xx should match 404")
	}
	if f.Match(flow("GET", 200, "x.com", "/", "")) {
		t.Error("s:4xx should NOT match 200")
	}
}

func TestMethodFilter(t *testing.T) {
	f := Compile("m:post")
	if !f.Match(flow("POST", 200, "x.com", "/", "")) {
		t.Error("m:post should match POST (case insensitive)")
	}
	if f.Match(flow("GET", 200, "x.com", "/", "")) {
		t.Error("m:post should NOT match GET")
	}
}

func TestContentTypeFilter(t *testing.T) {
	f := Compile("ct:json")
	if !f.Match(flow("GET", 200, "x.com", "/", "application/json")) {
		t.Error("ct:json should match application/json")
	}
	if f.Match(flow("GET", 200, "x.com", "/", "text/html")) {
		t.Error("ct:json should NOT match text/html")
	}
}

func TestRegexFilter(t *testing.T) {
	f := Compile("re:/v[12]/users")
	if !f.Match(flow("GET", 200, "x.com", "/v1/users", "")) {
		t.Error("re:/v[12]/users should match /v1/users")
	}
	if !f.Match(flow("GET", 200, "x.com", "/v2/users", "")) {
		t.Error("re:/v[12]/users should match /v2/users")
	}
	if f.Match(flow("GET", 200, "x.com", "/v3/users", "")) {
		t.Error("re:/v[12]/users should NOT match /v3/users")
	}
}

func TestNegation(t *testing.T) {
	tests := []struct {
		name    string
		filter  string
		flow    *store.FlowMeta
		wantMatch bool
	}{
		{"!m:GET blocks GET", "!m:GET", flow("GET", 200, "x.com", "/", ""), false},
		{"!m:GET allows POST", "!m:GET", flow("POST", 200, "x.com", "/", ""), true},
		{"!s:4xx blocks 404", "!s:4xx", flow("GET", 404, "x.com", "/", ""), false},
		{"!s:4xx allows 200", "!s:4xx", flow("GET", 200, "x.com", "/", ""), true},
		{"!ct:json blocks json", "!ct:json", flow("GET", 200, "x.com", "/", "application/json"), false},
		{"!ct:json allows html", "!ct:json", flow("GET", 200, "x.com", "/", "text/html"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Compile(tt.filter)
			got := f.Match(tt.flow)
			if got != tt.wantMatch {
				t.Errorf("filter %q match = %v, want %v", tt.filter, got, tt.wantMatch)
			}
		})
	}
}

func TestCombinedFilter(t *testing.T) {
	f := Compile("m:POST s:2xx api.example")
	if !f.Match(flow("POST", 201, "api.example.com", "/create", "")) {
		t.Error("combined filter should match POST+201+api.example")
	}
	if f.Match(flow("GET", 201, "api.example.com", "/create", "")) {
		t.Error("combined: GET should NOT match m:POST")
	}
	if f.Match(flow("POST", 404, "api.example.com", "/create", "")) {
		t.Error("combined: 404 should NOT match s:2xx")
	}
	if f.Match(flow("POST", 201, "other.com", "/create", "")) {
		t.Error("combined: other.com should NOT match api.example")
	}
}

func TestBackwardCompat(t *testing.T) {
	f := Compile("stripe")
	if !f.Match(flow("GET", 200, "api.stripe.com", "/v1/charges", "")) {
		t.Error("plain substring should match host")
	}
	if f.Match(flow("GET", 200, "other.com", "/foo", "")) {
		t.Error("plain substring should NOT match unrelated host")
	}
}

func TestCompileEmpty(t *testing.T) {
	f := Compile("")
	if f != nil {
		t.Error("empty input should return nil")
	}
	f = Compile("   ")
	if f != nil {
		t.Error("whitespace should return nil")
	}
}
