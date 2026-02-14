package diff

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kostyay/httpmon/internal/store"
)

func TestDiffIdenticalFlows(t *testing.T) {
	meta := &store.FlowMeta{Method: "GET", Host: "api.example.com", Path: "/v1/users", StatusCode: 200, Scheme: "https"}
	data := &store.FlowData{
		RequestHeaders:  http.Header{"Accept": {"application/json"}},
		RequestBody:     []byte(`{"q":"test"}`),
		ResponseHeaders: http.Header{"Content-Type": {"application/json"}},
		ResponseBody:    []byte(`{"ok":true}`),
	}

	result := Compare(meta, data, meta, data)
	if result.HasChanges() {
		t.Error("identical flows should have no changes")
	}
}

func TestDiffHeaders(t *testing.T) {
	meta1 := &store.FlowMeta{Method: "GET", Host: "x.com", Path: "/", StatusCode: 200, Scheme: "https"}
	data1 := &store.FlowData{
		RequestHeaders: http.Header{"Accept": {"application/json"}, "X-Old": {"val"}},
	}

	meta2 := &store.FlowMeta{Method: "GET", Host: "x.com", Path: "/", StatusCode: 200, Scheme: "https"}
	data2 := &store.FlowData{
		RequestHeaders: http.Header{"Accept": {"text/html"}, "X-New": {"val"}},
	}

	result := Compare(meta1, data1, meta2, data2)
	if !result.HasChanges() {
		t.Fatal("should have changes")
	}

	rendered := result.Render(false)
	if !strings.Contains(rendered, "X-Old") {
		t.Error("should show removed header X-Old")
	}
	if !strings.Contains(rendered, "X-New") {
		t.Error("should show added header X-New")
	}
}

func TestDiffBody(t *testing.T) {
	meta := &store.FlowMeta{Method: "POST", Host: "x.com", Path: "/", StatusCode: 200, Scheme: "https"}
	data1 := &store.FlowData{
		RequestBody:  []byte(`{"name":"old"}`),
		ResponseBody: []byte(`{"ok":true}`),
	}
	data2 := &store.FlowData{
		RequestBody:  []byte(`{"name":"new"}`),
		ResponseBody: []byte(`{"ok":true}`),
	}

	result := Compare(meta, data1, meta, data2)
	if !result.HasChanges() {
		t.Fatal("should have body changes")
	}
	rendered := result.Render(false)
	if !strings.Contains(rendered, "old") {
		t.Error("should show removed body content")
	}
	if !strings.Contains(rendered, "new") {
		t.Error("should show added body content")
	}
}

func TestDiffMethodAndStatus(t *testing.T) {
	meta1 := &store.FlowMeta{Method: "GET", Host: "x.com", Path: "/", StatusCode: 200, Scheme: "https"}
	meta2 := &store.FlowMeta{Method: "POST", Host: "x.com", Path: "/api", StatusCode: 404, Scheme: "https"}

	result := Compare(meta1, nil, meta2, nil)
	if !result.HasChanges() {
		t.Fatal("different method+status should have changes")
	}
	rendered := result.Render(false)
	if !strings.Contains(rendered, "GET") || !strings.Contains(rendered, "POST") {
		t.Error("should show method change")
	}
}
