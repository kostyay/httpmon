package mcpserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Mocks ---

type mockStore struct {
	metas []store.FlowMeta
	data  map[store.FlowID]*store.FlowData
}

func (m *mockStore) List(f store.Filter, offset, limit int) ([]store.FlowMeta, int) {
	var out []store.FlowMeta
	for _, meta := range m.metas {
		if f == nil || f.Match(&meta) {
			out = append(out, meta)
		}
	}
	total := len(out)
	if offset > len(out) {
		return nil, total
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, total
}

func (m *mockStore) Get(id store.FlowID) (*store.FlowMeta, *store.FlowData, error) {
	for i := range m.metas {
		if m.metas[i].ID == id {
			return &m.metas[i], m.data[id], nil
		}
	}
	return nil, nil, fmt.Errorf("not found: %s", id)
}

type mockProxy struct{ addr string }

func (m *mockProxy) Addr() string { return m.addr }

type mockThrottle struct {
	bps     int64
	latency time.Duration
}

func (m *mockThrottle) SetThrottle(bps int64, latency time.Duration) {
	m.bps = bps
	m.latency = latency
}
func (m *mockThrottle) GetThrottleBPS() int64            { return m.bps }
func (m *mockThrottle) GetThrottleLatency() time.Duration { return m.latency }

type mockScripts struct {
	scripts   []scripting.ScriptInfo
	dir       string
	reloaded  int
	toggled   string
	deleted   string
	toggleErr error
	deleteErr error
}

func (m *mockScripts) Scripts() []scripting.ScriptInfo { return m.scripts }
func (m *mockScripts) ScriptByID(id string) (scripting.ScriptInfo, bool) {
	for _, s := range m.scripts {
		if s.ID == id {
			return s, true
		}
	}
	return scripting.ScriptInfo{}, false
}
func (m *mockScripts) Toggle(path string) error {
	m.toggled = path
	return m.toggleErr
}
func (m *mockScripts) Delete(path string) error {
	m.deleted = path
	return m.deleteErr
}
func (m *mockScripts) CreateNew() (string, error)                   { return "", nil }
func (m *mockScripts) QuickAddMapLocal(_, _ string) (string, error) { return "", nil }
func (m *mockScripts) ScriptDir() string                            { return m.dir }
func (m *mockScripts) Reload()                                      { m.reloaded++ }

// --- clampLimit ---

func TestClampLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want int
	}{
		{0, 50},
		{-1, 50},
		{1, 1},
		{50, 50},
		{200, 200},
		{201, 200},
		{9999, 200},
	}
	for _, tt := range tests {
		if got := clampLimit(tt.in); got != tt.want {
			t.Errorf("clampLimit(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// --- encodeBody ---

func TestEncodeBody(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		if got := encodeBody(nil, 100); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("utf8", func(t *testing.T) {
		if got := encodeBody([]byte("hello"), 100); got != "hello" {
			t.Errorf("got %q, want hello", got)
		}
	})

	t.Run("binary_base64", func(t *testing.T) {
		bin := []byte{0xff, 0xfe, 0xfd}
		got := encodeBody(bin, 100)
		want := "base64:" + base64.StdEncoding.EncodeToString(bin)
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("truncation", func(t *testing.T) {
		got := encodeBody([]byte("abcdefghij"), 5)
		if got != "abcde" {
			t.Errorf("got %q, want %q", got, "abcde")
		}
	})
}

// --- headerMap ---

func TestHeaderMap(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		if got := headerMap(nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("single", func(t *testing.T) {
		m := headerMap(http.Header{"Content-Type": {"text/html"}})
		if m["Content-Type"] != "text/html" {
			t.Errorf("got %v", m)
		}
	})

	t.Run("multi_value_first_wins", func(t *testing.T) {
		m := headerMap(http.Header{"X-Foo": {"a", "b"}})
		if m["X-Foo"] != "a" {
			t.Errorf("got %q, want a", m["X-Foo"])
		}
	})
}

// --- harHeaders ---

func TestHarHeaders(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		if got := harHeaders(nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("converts", func(t *testing.T) {
		got := harHeaders(http.Header{"X-Key": {"val"}})
		if len(got) != 1 || got[0]["name"] != "X-Key" || got[0]["value"] != "val" {
			t.Errorf("got %v", got)
		}
	})
}

// --- contentTypeFromHeaders ---

func TestContentTypeFromHeaders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		h    http.Header
		want string
	}{
		{"nil", nil, ""},
		{"missing", http.Header{}, "application/octet-stream"},
		{"plain", http.Header{"Content-Type": {"text/html"}}, "text/html"},
		{"strip_params", http.Header{"Content-Type": {"text/html; charset=utf-8"}}, "text/html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentTypeFromHeaders(tt.h); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// --- metaToSummary / metasToSummaries ---

func TestMetaToSummary(t *testing.T) {
	t.Parallel()
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		state     store.FlowState
		wantState string
	}{
		{store.StateInProgress, "in_progress"},
		{store.StateCompleted, "completed"},
		{store.StateFailed, "failed"},
		{store.StateBreakpoint, "breakpoint"},
	}
	for _, tt := range tests {
		t.Run(tt.wantState, func(t *testing.T) {
			m := store.FlowMeta{
				ID: "f1", Method: "GET", Host: "h", Path: "/p",
				Duration: 150 * time.Millisecond, StartedAt: now,
				State: tt.state, Scheme: "https",
			}
			s := metaToSummary(m)
			if s.State != tt.wantState {
				t.Errorf("state = %q, want %q", s.State, tt.wantState)
			}
			if s.DurationMS != 150 {
				t.Errorf("duration = %d, want 150", s.DurationMS)
			}
			if s.StartedAt != now.Format(time.RFC3339) {
				t.Errorf("started_at = %q", s.StartedAt)
			}
		})
	}
}

func TestMetasToSummaries(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		if got := metasToSummaries(nil); len(got) != 0 {
			t.Errorf("got %d items", len(got))
		}
	})

	t.Run("multiple", func(t *testing.T) {
		got := metasToSummaries([]store.FlowMeta{{ID: "a"}, {ID: "b"}})
		if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
			t.Errorf("got %+v", got)
		}
	})
}

// --- slugify ---

func TestSlugify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"https://example.com/api/v1", "example-com-api-v1"},
		{"*://*/foo", "foo"},
		{"", "rule"},
		{strings.Repeat("a", 50), strings.Repeat("a", 30)},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := slugify(tt.in); got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// --- infoToSummary ---

func TestInfoToSummary(t *testing.T) {
	t.Parallel()

	t.Run("with_categories", func(t *testing.T) {
		s := infoToSummary(scripting.ScriptInfo{
			ID: "s1", Name: "test", Matches: []string{"*"},
			Enabled: true, Categories: []scripting.ScriptCategory{scripting.CategoryMapLocal},
		})
		if s.Category != "map local" {
			t.Errorf("primary = %q, want 'map local'", s.Category)
		}
		if s.ScriptID != "s1" {
			t.Errorf("id = %q", s.ScriptID)
		}
	})

	t.Run("empty_categories", func(t *testing.T) {
		s := infoToSummary(scripting.ScriptInfo{ID: "s2", Name: "x"})
		if s.Category != "script" {
			t.Errorf("primary = %q, want 'script'", s.Category)
		}
	})
}

// --- jsonResult / textResult / errorResult ---

func TestResultHelpers(t *testing.T) {
	t.Parallel()

	t.Run("jsonResult", func(t *testing.T) {
		r := jsonResult(map[string]int{"a": 1})
		text := r.Content[0].(*mcp.TextContent).Text
		var m map[string]int
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			t.Fatal(err)
		}
		if m["a"] != 1 {
			t.Errorf("got %v", m)
		}
		if r.IsError {
			t.Error("IsError should be false")
		}
	})

	t.Run("textResult", func(t *testing.T) {
		r := textResult("hi")
		text := r.Content[0].(*mcp.TextContent).Text
		if text != "hi" {
			t.Errorf("got %q", text)
		}
		if r.IsError {
			t.Error("IsError should be false")
		}
	})

	t.Run("errorResult", func(t *testing.T) {
		r := errorResult("boom")
		text := r.Content[0].(*mcp.TextContent).Text
		if text != "boom" {
			t.Errorf("got %q", text)
		}
		if !r.IsError {
			t.Error("IsError should be true")
		}
	})
}
