package procinfo

import (
	"errors"
	"fmt"
	"net"
	"os"
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
	if meta.ProcessPID != 123 {
		t.Errorf("meta.ProcessPID = %d, want 123", meta.ProcessPID)
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

func TestRecordFailureThreshold(t *testing.T) {
	ms := newMockStore()
	for i := range failThreshold + 1 {
		id := store.FlowID(fmt.Sprintf("f%d", i))
		ms.metas[id] = store.FlowMeta{ID: id}
	}

	r := newTestResolver(ms)
	r.findPID = func(port uint16) (int32, error) {
		return 0, errors.New("permission denied")
	}

	for i := range failThreshold + 1 {
		id := store.FlowID(fmt.Sprintf("f%d", i))
		r.Resolve(id, uint16(4000+i))
	}
	r.Wait()

	if got := r.failCount.Load(); got < int32(failThreshold) {
		t.Errorf("failCount = %d, want >= %d", got, failThreshold)
	}
}

func TestFailCountResetsOnSuccess(t *testing.T) {
	ms := newMockStore()
	for i := range 4 {
		id := store.FlowID(fmt.Sprintf("r%d", i))
		ms.metas[id] = store.FlowMeta{ID: id}
	}

	var callNum atomic.Int32
	r := newTestResolver(ms)
	r.findPID = func(port uint16) (int32, error) {
		n := callNum.Add(1)
		if n <= 3 {
			return 0, errors.New("no connection")
		}
		return 42, nil
	}
	r.processName = func(pid int32) (string, error) {
		return "curl", nil
	}
	r.processCmdline = func(pid int32) (string, error) {
		return "curl http://example.com", nil
	}

	// Fail 3 times sequentially.
	for i := range 3 {
		id := store.FlowID(fmt.Sprintf("r%d", i))
		r.Resolve(id, uint16(5000+i))
		r.Wait()
	}
	if got := r.failCount.Load(); got != 3 {
		t.Fatalf("failCount after 3 failures = %d, want 3", got)
	}

	// Succeed once.
	r.Resolve("r3", 5003)
	r.Wait()

	meta := ms.getMeta("r3")
	if meta.Process != "curl" {
		t.Errorf("Process = %q, want curl", meta.Process)
	}
	if got := r.failCount.Load(); got != 0 {
		t.Errorf("failCount after success = %d, want 0", got)
	}
}

func TestResolveCmdlineErrorNameSucceeds(t *testing.T) {
	ms := newMockStore()
	ms.metas["c1"] = store.FlowMeta{ID: "c1"}

	r := newTestResolver(ms)
	r.findPID = func(port uint16) (int32, error) {
		return 77, nil
	}
	r.processName = func(pid int32) (string, error) {
		return "node", nil
	}
	r.processCmdline = func(pid int32) (string, error) {
		return "", errors.New("cmdline unavailable")
	}

	r.Resolve("c1", 6000)
	r.Wait()

	meta := ms.getMeta("c1")
	if meta.Process != "node" {
		t.Errorf("Process = %q, want node", meta.Process)
	}
	data := ms.getData("c1")
	if data == nil {
		t.Fatal("data should not be nil")
	}
	if data.ProcessCmdline != "" {
		t.Errorf("ProcessCmdline = %q, want empty", data.ProcessCmdline)
	}
}

func TestNewSetsDefaults(t *testing.T) {
	r := New(nil)
	if r.findPID == nil {
		t.Error("findPID should not be nil")
	}
	if r.processName == nil {
		t.Error("processName should not be nil")
	}
	if r.processCmdline == nil {
		t.Error("processCmdline should not be nil")
	}
	if cap(r.sem) != maxConcurrent {
		t.Errorf("sem capacity = %d, want %d", cap(r.sem), maxConcurrent)
	}
}

func TestDefaultProcessName(t *testing.T) {
	r := New(nil)
	pid := int32(os.Getpid())
	name, err := r.defaultProcessName(pid)
	if err != nil {
		t.Fatalf("defaultProcessName(%d): %v", pid, err)
	}
	if name == "" {
		t.Error("expected non-empty process name")
	}
}

func TestDefaultProcessCmdline(t *testing.T) {
	r := New(nil)
	pid := int32(os.Getpid())
	cmdline, err := r.defaultProcessCmdline(pid)
	if err != nil {
		t.Fatalf("defaultProcessCmdline(%d): %v", pid, err)
	}
	if cmdline == "" {
		t.Error("expected non-empty cmdline")
	}
}

func TestDefaultFindPID(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	r := New(nil)
	pid, err := r.defaultFindPID(port)
	if err != nil {
		t.Fatalf("defaultFindPID(%d): %v", port, err)
	}
	if pid != int32(os.Getpid()) {
		t.Errorf("pid = %d, want %d", pid, os.Getpid())
	}
}

func TestDefaultFindPIDNoMatch(t *testing.T) {
	r := New(nil)
	_, err := r.defaultFindPID(1) // port 1 is unlikely to be in use
	if err == nil {
		t.Error("expected error for unmatched port")
	}
}

func TestCacheDoesNotStoreErrors(t *testing.T) {
	ms := newMockStore()
	ms.metas["e1"] = store.FlowMeta{ID: "e1"}
	ms.metas["e2"] = store.FlowMeta{ID: "e2"}

	nameCallCount := 0
	r := newTestResolver(ms)
	r.findPID = func(port uint16) (int32, error) { return 50, nil }
	r.processCmdline = func(pid int32) (string, error) { return "", nil }
	r.processName = func(pid int32) (string, error) {
		nameCallCount++
		if nameCallCount == 1 {
			return "", errors.New("temporary failure")
		}
		return "recovered", nil
	}

	r.Resolve("e1", 7000)
	r.Wait()
	meta1 := ms.getMeta("e1")
	if meta1.Process != fallbackName {
		t.Errorf("first resolve: Process = %q, want %q", meta1.Process, fallbackName)
	}

	r.Resolve("e2", 7001)
	r.Wait()
	meta2 := ms.getMeta("e2")
	if meta2.Process != "recovered" {
		t.Errorf("second resolve: Process = %q, want recovered", meta2.Process)
	}
	if nameCallCount != 2 {
		t.Errorf("processName called %d times, want 2", nameCallCount)
	}
}

func TestCacheSeparateEntriesPerPID(t *testing.T) {
	ms := newMockStore()
	ms.metas["p1"] = store.FlowMeta{ID: "p1"}
	ms.metas["p2"] = store.FlowMeta{ID: "p2"}

	nameCallCount := 0
	r := newTestResolver(ms)
	r.findPID = func(port uint16) (int32, error) {
		return int32(port), nil
	}
	r.processName = func(pid int32) (string, error) {
		nameCallCount++
		return fmt.Sprintf("proc-%d", pid), nil
	}
	r.processCmdline = func(pid int32) (string, error) { return "", nil }

	r.Resolve("p1", 100)
	r.Wait()
	r.Resolve("p2", 200)
	r.Wait()

	if nameCallCount != 2 {
		t.Errorf("processName called %d times, want 2 (different PIDs)", nameCallCount)
	}
	if m := ms.getMeta("p1"); m.Process != "proc-100" {
		t.Errorf("p1 Process = %q, want proc-100", m.Process)
	}
	if m := ms.getMeta("p2"); m.Process != "proc-200" {
		t.Errorf("p2 Process = %q, want proc-200", m.Process)
	}
}

func TestResolveNonExistentFlowID(t *testing.T) {
	ms := newMockStore()
	r := newTestResolver(ms)
	r.findPID = func(port uint16) (int32, error) { return 1, nil }
	r.processName = func(pid int32) (string, error) { return "x", nil }
	r.processCmdline = func(pid int32) (string, error) { return "", nil }

	r.Resolve("missing", 8000)
	r.Wait()

	meta := ms.getMeta("missing")
	if meta.Process != "" {
		t.Errorf("Process = %q, want empty for missing flow", meta.Process)
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
