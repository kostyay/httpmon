package maplocal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
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

func TestRemoveRule(t *testing.T) {
	ml := New()
	ml.AddRule(Rule{Pattern: "a.com/*", LocalPath: "/a"})
	ml.AddRule(Rule{Pattern: "b.com/*", LocalPath: "/b"})
	ml.AddRule(Rule{Pattern: "c.com/*", LocalPath: "/c"})

	ml.RemoveRule(1)

	rules := ml.Rules()
	if len(rules) != 2 {
		t.Fatalf("len = %d, want 2", len(rules))
	}
	if rules[0].Pattern != "a.com/*" {
		t.Errorf("rules[0].Pattern = %q, want a.com/*", rules[0].Pattern)
	}
	if rules[1].Pattern != "c.com/*" {
		t.Errorf("rules[1].Pattern = %q, want c.com/*", rules[1].Pattern)
	}
}

func TestRemoveRuleOutOfBounds(t *testing.T) {
	ml := New()
	ml.AddRule(Rule{Pattern: "a.com/*", LocalPath: "/a"})

	ml.RemoveRule(-1)
	ml.RemoveRule(5)

	if ml.RuleCount() != 1 {
		t.Errorf("count = %d, want 1 (no change)", ml.RuleCount())
	}
}

func TestRuleCount(t *testing.T) {
	ml := New()
	if ml.RuleCount() != 0 {
		t.Errorf("initial count = %d", ml.RuleCount())
	}

	ml.AddRule(Rule{Pattern: "a.com/*", LocalPath: "/a"})
	ml.AddRule(Rule{Pattern: "b.com/*", LocalPath: "/b"})

	if ml.RuleCount() != 2 {
		t.Errorf("count = %d, want 2", ml.RuleCount())
	}
}

func TestAddRuleDefaultsStatusCode(t *testing.T) {
	ml := New()
	ml.AddRule(Rule{Pattern: "a.com/*", LocalPath: "/a"})

	rules := ml.Rules()
	if rules[0].StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200 (default)", rules[0].StatusCode)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	rulesFile := filepath.Join(dir, "rules.json")

	rules := []Rule{
		{Pattern: "api.example.com/v1/*", LocalPath: "/tmp/mock.json", StatusCode: 200},
		{Pattern: "*.cdn.com/*", LocalPath: "/tmp/img.png"},
	}
	data, _ := json.Marshal(rules)
	os.WriteFile(rulesFile, data, 0o644)

	ml := New()
	if err := ml.LoadFromFile(rulesFile); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	loaded := ml.Rules()
	if len(loaded) != 2 {
		t.Fatalf("len = %d, want 2", len(loaded))
	}
	if loaded[0].Pattern != "api.example.com/v1/*" {
		t.Errorf("rules[0].Pattern = %q", loaded[0].Pattern)
	}
	// StatusCode 0 should default to 200.
	if loaded[1].StatusCode != 200 {
		t.Errorf("rules[1].StatusCode = %d, want 200", loaded[1].StatusCode)
	}
}

func TestLoadFromFileMissing(t *testing.T) {
	ml := New()
	if err := ml.LoadFromFile("/nonexistent/rules.json"); err == nil {
		t.Error("should error on missing file")
	}
}

func TestLoadFromFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	rulesFile := filepath.Join(dir, "bad.json")
	os.WriteFile(rulesFile, []byte("not json"), 0o644)

	ml := New()
	if err := ml.LoadFromFile(rulesFile); err == nil {
		t.Error("should error on invalid JSON")
	}
}

func TestSaveToFile(t *testing.T) {
	ml := New()
	ml.AddRule(Rule{Pattern: "a.com/*", LocalPath: "/a", StatusCode: 200})
	ml.AddRule(Rule{Pattern: "b.com/*", LocalPath: "/b", StatusCode: 404})

	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.json")

	if err := ml.SaveToFile(outFile); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var loaded []Rule
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("len = %d, want 2", len(loaded))
	}
	if loaded[0].Pattern != "a.com/*" {
		t.Errorf("rules[0].Pattern = %q", loaded[0].Pattern)
	}
	if loaded[1].StatusCode != 404 {
		t.Errorf("rules[1].StatusCode = %d, want 404", loaded[1].StatusCode)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	ml := New()
	ml.AddRule(Rule{Pattern: "test.com/*", LocalPath: "/test", StatusCode: 201})

	dir := t.TempDir()
	f := filepath.Join(dir, "rt.json")

	if err := ml.SaveToFile(f); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	ml2 := New()
	if err := ml2.LoadFromFile(f); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	rules := ml2.Rules()
	if len(rules) != 1 || rules[0].Pattern != "test.com/*" {
		t.Errorf("round-trip failed: %+v", rules)
	}
}

func TestConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "data.json")
	os.WriteFile(file, []byte("ok"), 0o644)

	ml := New()
	ml.AddRule(Rule{Pattern: "*", LocalPath: file, StatusCode: 200})

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			ml.Match("test.com", "/path")
		}()
		go func() {
			defer wg.Done()
			ml.Rules()
		}()
		go func() {
			defer wg.Done()
			ml.RuleCount()
		}()
	}
	wg.Wait()
}
