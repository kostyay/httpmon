package maplocal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMapLocalMatch(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "response.json")
	os.WriteFile(file, []byte(`{"mock":true}`), 0o644)

	ml := New()
	ml.AddRule(Rule{
		Pattern:    "api.example.com/v1/users",
		LocalPath:  file,
		StatusCode: 200,
	})

	body, status, found := ml.Match("api.example.com", "/v1/users")
	if !found {
		t.Fatal("should match")
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	if string(body) != `{"mock":true}` {
		t.Errorf("body = %q", string(body))
	}
}

func TestMapLocalNoMatch(t *testing.T) {
	ml := New()
	ml.AddRule(Rule{
		Pattern:   "api.example.com/v1/users",
		LocalPath: "/nonexistent",
	})

	_, _, found := ml.Match("other.com", "/foo")
	if found {
		t.Error("should not match")
	}
}

func TestMapLocalMissingFile(t *testing.T) {
	ml := New()
	ml.AddRule(Rule{
		Pattern:   "api.example.com/v1/users",
		LocalPath: "/nonexistent/path.json",
	})

	_, _, found := ml.Match("api.example.com", "/v1/users")
	if found {
		t.Error("should fall through when file is missing")
	}
}

func TestMapLocalWildcard(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "mock.json")
	os.WriteFile(file, []byte(`{"wild":true}`), 0o644)

	ml := New()
	ml.AddRule(Rule{
		Pattern:    "*.example.com/api/*",
		LocalPath:  file,
		StatusCode: 201,
	})

	body, status, found := ml.Match("sub.example.com", "/api/test")
	if !found {
		t.Fatal("wildcard should match")
	}
	if status != 201 {
		t.Errorf("status = %d, want 201", status)
	}
	if string(body) != `{"wild":true}` {
		t.Errorf("body = %q", string(body))
	}
}

func TestMapLocalCustomStatus(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "error.json")
	os.WriteFile(file, []byte(`{"error":"not found"}`), 0o644)

	ml := New()
	ml.AddRule(Rule{
		Pattern:    "api.example.com/v1/missing",
		LocalPath:  file,
		StatusCode: 404,
	})

	_, status, found := ml.Match("api.example.com", "/v1/missing")
	if !found {
		t.Fatal("should match")
	}
	if status != 404 {
		t.Errorf("status = %d, want 404", status)
	}
}
