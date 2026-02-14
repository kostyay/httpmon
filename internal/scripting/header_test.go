package scripting

import (
	"strings"
	"testing"
)

func TestParseHeader_Valid(t *testing.T) {
	source := `// ---
// name: Strip Auth Headers
// match:
//   - "*://api.example.com/*"
//   - "*://staging.example.com/v2/*"
// enabled: true
// ---

function onRequest(ctx) {
	ctx.headers["Authorization"] = "";
}
`
	meta, body, err := ParseHeader(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Name != "Strip Auth Headers" {
		t.Errorf("Name = %q, want %q", meta.Name, "Strip Auth Headers")
	}

	if len(meta.Match) != 2 {
		t.Fatalf("Match len = %d, want 2", len(meta.Match))
	}
	if meta.Match[0] != "*://api.example.com/*" {
		t.Errorf("Match[0] = %q, want %q", meta.Match[0], "*://api.example.com/*")
	}
	if meta.Match[1] != "*://staging.example.com/v2/*" {
		t.Errorf("Match[1] = %q, want %q", meta.Match[1], "*://staging.example.com/v2/*")
	}

	if !meta.IsEnabled() {
		t.Error("IsEnabled() = false, want true")
	}

	if !strings.Contains(body, "function onRequest") {
		t.Errorf("body should contain script code, got %q", body)
	}

	// Body should not contain the header delimiters.
	if strings.Contains(body, "// ---") {
		t.Error("body should not contain header delimiters")
	}
}

func TestParseHeader_DefaultEnabled(t *testing.T) {
	source := `// ---
// name: Log All
// match:
//   - "*"
// ---

function onRequest(ctx) {}
`
	meta, _, err := ParseHeader(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Enabled != nil {
		t.Errorf("Enabled should be nil when omitted, got %v", *meta.Enabled)
	}

	if !meta.IsEnabled() {
		t.Error("IsEnabled() should default to true when Enabled is nil")
	}
}

func TestParseHeader_NoHeader(t *testing.T) {
	source := `function onRequest(ctx) {
	ctx.blocked = true;
}
`
	_, _, err := ParseHeader(source)
	if err == nil {
		t.Fatal("expected error for missing header, got nil")
	}
}

func TestParseHeader_MissingName(t *testing.T) {
	source := `// ---
// match:
//   - "*"
// ---

function onRequest(ctx) {}
`
	_, _, err := ParseHeader(source)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

func TestParseHeader_MissingMatch(t *testing.T) {
	source := `// ---
// name: No Match
// ---

function onRequest(ctx) {}
`
	_, _, err := ParseHeader(source)
	if err == nil {
		t.Fatal("expected error for missing match, got nil")
	}
}
