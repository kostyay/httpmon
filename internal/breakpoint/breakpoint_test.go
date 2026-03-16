package breakpoint

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kostyay/httpmon/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newHit(flowID string, phase Phase) BreakpointHit {
	return BreakpointHit{
		FlowID:  flowID,
		Phase:   phase,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    []byte(`{"key":"value"}`),
		Meta:    store.FlowMeta{ID: flowID, Method: "GET", Host: "example.com"},
	}
}

func TestPauseBlocksUntilResume(t *testing.T) {
	ctrl := NewController()
	hit := newHit("flow-1", PhaseRequest)

	resumed := make(chan BreakpointResume, 1)
	go func() {
		resumed <- ctrl.Pause(hit)
	}()

	// Wait for hit to appear in pending
	require.Eventually(t, func() bool {
		return len(ctrl.Pending()) == 1
	}, time.Second, 10*time.Millisecond)

	resp := BreakpointResume{
		Headers: map[string]string{"X-Modified": "true"},
		Body:    []byte("modified"),
	}
	ctrl.Resume("flow-1", resp)

	select {
	case got := <-resumed:
		assert.Equal(t, resp.Headers, got.Headers)
		assert.Equal(t, resp.Body, got.Body)
		assert.False(t, got.Skipped)
	case <-time.After(time.Second):
		t.Fatal("Pause did not unblock after Resume")
	}
}

func TestResumeWithModifiedData(t *testing.T) {
	ctrl := NewController()
	hit := newHit("flow-2", PhaseResponse)

	resumed := make(chan BreakpointResume, 1)
	go func() {
		resumed <- ctrl.Pause(hit)
	}()

	require.Eventually(t, func() bool {
		return len(ctrl.Pending()) == 1
	}, time.Second, 10*time.Millisecond)

	resp := BreakpointResume{
		Headers: map[string]string{"X-New": "header"},
		Body:    []byte("new body"),
	}
	ctrl.Resume("flow-2", resp)

	got := <-resumed
	assert.Equal(t, "header", got.Headers["X-New"])
	assert.Equal(t, []byte("new body"), got.Body)
}

func TestResumeWithSkipped(t *testing.T) {
	ctrl := NewController()
	hit := newHit("flow-3", PhaseRequest)

	resumed := make(chan BreakpointResume, 1)
	go func() {
		resumed <- ctrl.Pause(hit)
	}()

	require.Eventually(t, func() bool {
		return len(ctrl.Pending()) == 1
	}, time.Second, 10*time.Millisecond)

	ctrl.Resume("flow-3", BreakpointResume{Skipped: true})

	got := <-resumed
	assert.True(t, got.Skipped)
}

func TestConcurrentPausesResumeIndependently(t *testing.T) {
	ctrl := NewController()

	var wg sync.WaitGroup
	results := make(map[string]BreakpointResume)
	var mu sync.Mutex

	for _, id := range []string{"c-1", "c-2", "c-3"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := ctrl.Pause(newHit(id, PhaseRequest))
			mu.Lock()
			results[id] = resp
			mu.Unlock()
		}()
	}

	require.Eventually(t, func() bool {
		return len(ctrl.Pending()) == 3
	}, time.Second, 10*time.Millisecond)

	ctrl.Resume("c-2", BreakpointResume{Body: []byte("two")})
	ctrl.Resume("c-1", BreakpointResume{Body: []byte("one")})
	ctrl.Resume("c-3", BreakpointResume{Body: []byte("three")})

	wg.Wait()

	assert.Equal(t, []byte("one"), results["c-1"].Body)
	assert.Equal(t, []byte("two"), results["c-2"].Body)
	assert.Equal(t, []byte("three"), results["c-3"].Body)
}

func TestPendingAccuracy(t *testing.T) {
	ctrl := NewController()

	go ctrl.Pause(newHit("p-1", PhaseRequest))
	go ctrl.Pause(newHit("p-2", PhaseResponse))

	require.Eventually(t, func() bool {
		return len(ctrl.Pending()) == 2
	}, time.Second, 10*time.Millisecond)

	pending := ctrl.Pending()
	ids := []string{pending[0].FlowID, pending[1].FlowID}
	slices.Sort(ids)
	assert.Equal(t, []string{"p-1", "p-2"}, ids)

	ctrl.Resume("p-1", BreakpointResume{Skipped: true})

	require.Eventually(t, func() bool {
		return len(ctrl.Pending()) == 1
	}, time.Second, 10*time.Millisecond)

	assert.Equal(t, "p-2", ctrl.Pending()[0].FlowID)
}

func TestResumeAllUnblocksAll(t *testing.T) {
	ctrl := NewController()

	var wg sync.WaitGroup
	results := make([]BreakpointResume, 3)

	for i, id := range []string{"ra-1", "ra-2", "ra-3"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = ctrl.Pause(newHit(id, PhaseRequest))
		}()
	}

	require.Eventually(t, func() bool {
		return len(ctrl.Pending()) == 3
	}, time.Second, 10*time.Millisecond)

	ctrl.ResumeAll()
	wg.Wait()

	for i, r := range results {
		assert.True(t, r.Skipped, "result %d should be skipped", i)
	}
	assert.Empty(t, ctrl.Pending())
}

func TestSubscribeDeliversHits(t *testing.T) {
	ctrl := NewController()
	sub := ctrl.Subscribe()

	go ctrl.Pause(newHit("sub-1", PhaseRequest)) //nolint:errcheck

	select {
	case hit := <-sub:
		assert.Equal(t, "sub-1", hit.FlowID)
		assert.Equal(t, PhaseRequest, hit.Phase)
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive hit")
	}

	ctrl.Resume("sub-1", BreakpointResume{Skipped: true})
}

func TestResumeUnknownFlowIDIsNoop(t *testing.T) {
	ctrl := NewController()

	// Should not panic
	ctrl.Resume("nonexistent", BreakpointResume{Body: []byte("x")})
}
