package store

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func makeMeta(id string) FlowMeta {
	return FlowMeta{
		ID:        id,
		Method:    "GET",
		Host:      "example.com",
		Path:      "/test",
		StartedAt: time.Now(),
		State:     StateInProgress,
	}
}

func TestAddGetRoundTrip(t *testing.T) {
	rb := New(10)
	meta := makeMeta("f1")
	rb.Add(meta)

	got, data, err := rb.Get("f1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.ID != "f1" {
		t.Errorf("got ID %q, want f1", got.ID)
	}
	if data != nil {
		t.Error("expected nil data before SetData")
	}

	rb.SetData("f1", &FlowData{RequestBody: []byte("hello")})
	_, data, err = rb.Get("f1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if string(data.RequestBody) != "hello" {
		t.Errorf("got body %q, want hello", data.RequestBody)
	}
}

func TestEvictionAtCapacity(t *testing.T) {
	rb := New(3)
	rb.Add(makeMeta("f1"))
	rb.SetData("f1", &FlowData{RequestBody: []byte("body1")})
	rb.Add(makeMeta("f2"))
	rb.Add(makeMeta("f3"))

	// buffer full; adding f4 should evict f1
	rb.Add(makeMeta("f4"))

	if rb.Len() != 3 {
		t.Errorf("Len() = %d, want 3", rb.Len())
	}

	// f1 evicted
	_, _, err := rb.Get("f1")
	if err == nil {
		t.Error("expected error for evicted flow f1")
	}

	// f2, f3, f4 still present
	for _, id := range []string{"f2", "f3", "f4"} {
		if _, _, err := rb.Get(id); err != nil {
			t.Errorf("Get(%q) unexpected error: %v", id, err)
		}
	}
}

func TestUpdateConcurrent(t *testing.T) {
	rb := New(100)
	for i := range 100 {
		rb.Add(makeMeta(fmt.Sprintf("f%d", i)))
	}

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			rb.Update(id, func(m *FlowMeta) {
				m.StatusCode = 200
				m.State = StateCompleted
			})
		}(fmt.Sprintf("f%d", i))
	}
	wg.Wait()

	for i := range 100 {
		meta, _, err := rb.Get(fmt.Sprintf("f%d", i))
		if err != nil {
			t.Fatalf("Get(f%d) error: %v", i, err)
		}
		if meta.StatusCode != 200 {
			t.Errorf("f%d StatusCode = %d, want 200", i, meta.StatusCode)
		}
	}
}

type hostFilter struct {
	host string
}

func (f *hostFilter) Match(m *FlowMeta) bool {
	return m.Host == f.host
}

func TestListNilFilter(t *testing.T) {
	rb := New(10)
	rb.Add(makeMeta("f1"))
	rb.Add(makeMeta("f2"))
	rb.Add(makeMeta("f3"))

	flows, total := rb.List(nil, 0, 0)
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(flows) != 3 {
		t.Errorf("len(flows) = %d, want 3", len(flows))
	}
	// newest first
	if flows[0].ID != "f3" {
		t.Errorf("first flow = %q, want f3", flows[0].ID)
	}
}

func TestListWithFilter(t *testing.T) {
	rb := New(10)
	m1 := makeMeta("f1")
	m1.Host = "api.stripe.com"
	m2 := makeMeta("f2")
	m2.Host = "other.com"
	m3 := makeMeta("f3")
	m3.Host = "api.stripe.com"
	rb.Add(m1)
	rb.Add(m2)
	rb.Add(m3)

	flows, total := rb.List(&hostFilter{host: "api.stripe.com"}, 0, 0)
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(flows) != 2 {
		t.Errorf("len = %d, want 2", len(flows))
	}
}

func TestListOffsetLimit(t *testing.T) {
	rb := New(10)
	for i := range 5 {
		rb.Add(makeMeta(fmt.Sprintf("f%d", i)))
	}

	flows, total := rb.List(nil, 1, 2)
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(flows) != 2 {
		t.Errorf("len = %d, want 2", len(flows))
	}
	// newest-first: f4,f3,f2,f1,f0 → offset 1 → f3,f2
	if flows[0].ID != "f3" {
		t.Errorf("flows[0] = %q, want f3", flows[0].ID)
	}
	if flows[1].ID != "f2" {
		t.Errorf("flows[1] = %q, want f2", flows[1].ID)
	}
}

func TestGetEvictedReturnsError(t *testing.T) {
	rb := New(2)
	rb.Add(makeMeta("f1"))
	rb.Add(makeMeta("f2"))
	rb.Add(makeMeta("f3")) // evicts f1

	_, _, err := rb.Get("f1")
	if err == nil {
		t.Error("expected error for evicted flow")
	}
}

func TestGetNonexistentReturnsError(t *testing.T) {
	rb := New(5)
	_, _, err := rb.Get("nope")
	if err == nil {
		t.Error("expected error for nonexistent flow")
	}
}

func TestSetDataIgnoresEvicted(t *testing.T) {
	rb := New(2)
	rb.Add(makeMeta("f1"))
	rb.Add(makeMeta("f2"))
	rb.Add(makeMeta("f3")) // evicts f1

	// should not panic or corrupt
	rb.SetData("f1", &FlowData{RequestBody: []byte("gone")})
	if rb.Len() != 2 {
		t.Errorf("Len = %d, want 2", rb.Len())
	}
}

func TestUpdateDataMergesIntoExisting(t *testing.T) {
	rb := New(10)
	rb.Add(makeMeta("f1"))
	rb.SetData("f1", &FlowData{RequestBody: []byte("body")})

	rb.UpdateData("f1", func(d *FlowData) {
		d.ProcessPID = 42
		d.ProcessCmdline = "curl https://example.com"
	})

	_, data, err := rb.Get("f1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if string(data.RequestBody) != "body" {
		t.Errorf("RequestBody = %q, want body", data.RequestBody)
	}
	if data.ProcessPID != 42 {
		t.Errorf("ProcessPID = %d, want 42", data.ProcessPID)
	}
	if data.ProcessCmdline != "curl https://example.com" {
		t.Errorf("ProcessCmdline = %q, want curl...", data.ProcessCmdline)
	}
}

func TestUpdateDataCreatesWhenMissing(t *testing.T) {
	rb := New(10)
	rb.Add(makeMeta("f1"))

	rb.UpdateData("f1", func(d *FlowData) {
		d.ProcessPID = 99
	})

	_, data, err := rb.Get("f1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if data == nil {
		t.Fatal("data should not be nil after UpdateData")
	}
	if data.ProcessPID != 99 {
		t.Errorf("ProcessPID = %d, want 99", data.ProcessPID)
	}
}

func TestUpdateDataIgnoresEvicted(t *testing.T) {
	rb := New(2)
	rb.Add(makeMeta("f1"))
	rb.Add(makeMeta("f2"))
	rb.Add(makeMeta("f3")) // evicts f1

	// should not panic
	rb.UpdateData("f1", func(d *FlowData) {
		d.ProcessPID = 42
	})
}

func TestUpdateIgnoresEvicted(t *testing.T) {
	rb := New(2)
	rb.Add(makeMeta("f1"))
	rb.Add(makeMeta("f2"))
	rb.Add(makeMeta("f3")) // evicts f1

	// should not panic
	rb.Update("f1", func(m *FlowMeta) {
		m.StatusCode = 999
	})
}
