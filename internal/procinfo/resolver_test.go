package procinfo

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kostyay/httpmon/internal/store"
)

type mockStore struct {
	mu    sync.Mutex
	metas map[store.FlowID]store.FlowMeta
	data  map[store.FlowID]*store.FlowData
}

func newMockStore() *mockStore {
	return &mockStore{
		metas: make(map[store.FlowID]store.FlowMeta),
		data:  make(map[store.FlowID]*store.FlowData),
	}
}

func (m *mockStore) Update(id store.FlowID, fn func(*store.FlowMeta)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, ok := m.metas[id]
	if !ok {
		return
	}
	fn(&meta)
	m.metas[id] = meta
}

func (m *mockStore) UpdateData(id store.FlowID, fn func(*store.FlowData)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.metas[id]; !ok {
		return
	}
	d, ok := m.data[id]
	if !ok {
		d = &store.FlowData{}
		m.data[id] = d
	}
	fn(d)
}

func (m *mockStore) getMeta(id store.FlowID) store.FlowMeta {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.metas[id]
}

func (m *mockStore) getData(id store.FlowID) *store.FlowData {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[id]
}

func TestResolveSetsMeta(t *testing.T) {
	ms := newMockStore()
	ms.metas["f1"] = store.FlowMeta{ID: "f1"}

	r := newTestResolver(ms)
	r.findPID = func(port uint16) (int32, error) {
		return 123, nil
	}
	r.processName = func(pid int32) (string, error) {
		return "curl", nil
	}
	r.processCmdline = func(pid int32) (string, error) {
		return "curl https://example.com", nil
	}

	r.Resolve("f1", 54321)
	r.Wait()

	meta := ms.getMeta("f1")
	if meta.Process != "curl" {
		t.Errorf("Process = %q, want curl", meta.Process)
	}

	data := ms.getData("f1")
	if data == nil {
		t.Fatal("data should not be nil")
	}
	if data.ProcessPID != 123 {
		t.Errorf("ProcessPID = %d, want 123", data.ProcessPID)
	}
	if data.ProcessCmdline != "curl https://example.com" {
		t.Errorf("ProcessCmdline = %q, want curl...", data.ProcessCmdline)
	}
}

func TestResolveCacheHit(t *testing.T) {
	ms := newMockStore()
	ms.metas["f1"] = store.FlowMeta{ID: "f1"}
	ms.metas["f2"] = store.FlowMeta{ID: "f2"}

	var nameCallCount atomic.Int32
	r := newTestResolver(ms)
	r.findPID = func(port uint16) (int32, error) {
		return 42, nil
	}
	r.processName = func(pid int32) (string, error) {
		nameCallCount.Add(1)
		return "firefox", nil
	}
	r.processCmdline = func(pid int32) (string, error) {
		return "/usr/bin/firefox", nil
	}

	r.Resolve("f1", 1000)
	r.Wait()
	r.Resolve("f2", 2000)
	r.Wait()

	if nameCallCount.Load() != 1 {
		t.Errorf("processName called %d times, want 1 (cache hit)", nameCallCount.Load())
	}

	meta1 := ms.getMeta("f1")
	meta2 := ms.getMeta("f2")
	if meta1.Process != "firefox" || meta2.Process != "firefox" {
		t.Errorf("both flows should have Process=firefox, got %q %q", meta1.Process, meta2.Process)
	}
}

func TestResolveUnknownProcess(t *testing.T) {
	ms := newMockStore()
	ms.metas["f1"] = store.FlowMeta{ID: "f1"}

	r := newTestResolver(ms)
	r.findPID = func(port uint16) (int32, error) {
		return 99, nil
	}
	r.processName = func(pid int32) (string, error) {
		return "", errors.New("no such process")
	}
	r.processCmdline = func(pid int32) (string, error) {
		return "", errors.New("no such process")
	}

	r.Resolve("f1", 5000)
	r.Wait()

	meta := ms.getMeta("f1")
	if meta.Process != fallbackName {
		t.Errorf("Process = %q, want %q", meta.Process, fallbackName)
	}
}

func TestResolvePortNotFound(t *testing.T) {
	ms := newMockStore()
	ms.metas["f1"] = store.FlowMeta{ID: "f1"}

	r := newTestResolver(ms)
	r.findPID = func(port uint16) (int32, error) {
		return 0, errors.New("no connection found")
	}

	r.Resolve("f1", 9999)
	r.Wait()

	meta := ms.getMeta("f1")
	if meta.Process != fallbackName {
		t.Errorf("Process = %q, want %q", meta.Process, fallbackName)
	}

	data := ms.getData("f1")
	if data != nil && data.ProcessPID != 0 {
		t.Errorf("ProcessPID = %d, want 0 for not-found", data.ProcessPID)
	}
}

func TestResolveSemaphoreLimitsConcurrency(t *testing.T) {
	ms := newMockStore()
	for i := range 20 {
		id := store.FlowID("f" + string(rune('a'+i)))
		ms.metas[id] = store.FlowMeta{ID: id}
	}

	var maxConcur atomic.Int32
	var current atomic.Int32

	r := newTestResolver(ms)
	r.findPID = func(port uint16) (int32, error) {
		cur := current.Add(1)
		for {
			old := maxConcur.Load()
			if cur <= old || maxConcur.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		current.Add(-1)
		return 1, nil
	}
	r.processName = func(pid int32) (string, error) {
		return "test", nil
	}
	r.processCmdline = func(pid int32) (string, error) {
		return "test", nil
	}

	for i := range 20 {
		id := store.FlowID("f" + string(rune('a'+i)))
		r.Resolve(id, uint16(3000+i))
	}
	r.Wait()

	if maxConcur.Load() > maxConcurrent {
		t.Errorf("max concurrent = %d, want <= %d", maxConcur.Load(), maxConcurrent)
	}
}

// newTestResolver creates a resolver with mock store and no-op default funcs.
func newTestResolver(s flowStore) *Resolver {
	return &Resolver{
		store: s,
		sem:   make(chan struct{}, maxConcurrent),
		findPID: func(port uint16) (int32, error) {
			return 0, errors.New("not configured")
		},
		processName: func(pid int32) (string, error) {
			return "", errors.New("not configured")
		},
		processCmdline: func(pid int32) (string, error) {
			return "", errors.New("not configured")
		},
	}
}
