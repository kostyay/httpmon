package har

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/kostyay/httpmon/internal/store"
)

func meta(id string, method string, status int, host, path string) store.FlowMeta {
	return store.FlowMeta{
		ID:          store.FlowID(id),
		Method:      method,
		StatusCode:  status,
		Host:        host,
		Path:        path,
		Duration:    50 * time.Millisecond,
		SizeBytes:   256,
		StartedAt:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		State:       store.StateCompleted,
		ContentType: "application/json",
		Scheme:      "https",
	}
}

func data() *store.FlowData {
	return &store.FlowData{
		RequestHeaders:  http.Header{"Accept": {"application/json"}},
		RequestBody:     []byte(`{"q":"test"}`),
		ResponseHeaders: http.Header{"Content-Type": {"application/json"}},
		ResponseBody:    []byte(`{"ok":true}`),
	}
}

func TestHARSingleFlow(t *testing.T) {
	m := meta("f1", "GET", 200, "api.example.com", "/v1/users")
	d := data()

	out, err := Export([]store.FlowMeta{m}, func(id store.FlowID) *store.FlowData {
		return d
	})
	if err != nil {
		t.Fatal(err)
	}

	var har HARRoot
	if err := json.Unmarshal(out, &har); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if har.Log.Version != "1.2" {
		t.Errorf("version = %q, want 1.2", har.Log.Version)
	}
	if len(har.Log.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(har.Log.Entries))
	}

	entry := har.Log.Entries[0]
	if entry.Request.Method != "GET" {
		t.Errorf("method = %q, want GET", entry.Request.Method)
	}
	if entry.Request.URL != "https://api.example.com/v1/users" {
		t.Errorf("url = %q", entry.Request.URL)
	}
	if entry.Response.Status != 200 {
		t.Errorf("status = %d, want 200", entry.Response.Status)
	}
}

func TestHARMultipleFlows(t *testing.T) {
	flows := []store.FlowMeta{
		meta("f1", "GET", 200, "a.com", "/1"),
		meta("f2", "POST", 201, "b.com", "/2"),
		meta("f3", "DELETE", 204, "c.com", "/3"),
	}
	d := data()

	out, err := Export(flows, func(id store.FlowID) *store.FlowData { return d })
	if err != nil {
		t.Fatal(err)
	}

	var har HARRoot
	if err := json.Unmarshal(out, &har); err != nil {
		t.Fatal(err)
	}

	if len(har.Log.Entries) != 3 {
		t.Errorf("entries = %d, want 3", len(har.Log.Entries))
	}
}

func TestHARHeaders(t *testing.T) {
	m := meta("f1", "GET", 200, "x.com", "/")
	d := data()

	out, err := Export([]store.FlowMeta{m}, func(id store.FlowID) *store.FlowData { return d })
	if err != nil {
		t.Fatal(err)
	}

	var har HARRoot
	json.Unmarshal(out, &har)

	entry := har.Log.Entries[0]
	found := false
	for _, h := range entry.Request.Headers {
		if h.Name == "Accept" && h.Value == "application/json" {
			found = true
		}
	}
	if !found {
		t.Error("request headers should include Accept: application/json")
	}
}

func TestHAREmptyBody(t *testing.T) {
	m := meta("f1", "GET", 200, "x.com", "/")
	d := &store.FlowData{
		RequestHeaders:  http.Header{},
		ResponseHeaders: http.Header{"Content-Type": {"text/html"}},
	}

	out, err := Export([]store.FlowMeta{m}, func(id store.FlowID) *store.FlowData { return d })
	if err != nil {
		t.Fatal(err)
	}

	var har HARRoot
	json.Unmarshal(out, &har)

	entry := har.Log.Entries[0]
	if entry.Request.BodySize != 0 {
		t.Errorf("request bodySize = %d, want 0", entry.Request.BodySize)
	}
}
