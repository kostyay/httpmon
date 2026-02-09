package store

import (
	"fmt"
	"sync"
)

// RingBuffer is a thread-safe circular buffer for captured flows.
type RingBuffer struct {
	mu       sync.RWMutex
	metas    []FlowMeta
	data     map[FlowID]*FlowData
	index    map[FlowID]int // FlowID → slot position
	head     int
	count    int
	capacity int
}

// New creates a RingBuffer with the given capacity.
func New(capacity int) *RingBuffer {
	return &RingBuffer{
		metas:    make([]FlowMeta, capacity),
		data:     make(map[FlowID]*FlowData),
		index:    make(map[FlowID]int),
		capacity: capacity,
	}
}

// Add inserts a flow into the buffer, evicting the oldest if full.
func (rb *RingBuffer) Add(meta FlowMeta) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	slot := (rb.head + rb.count) % rb.capacity
	if rb.count == rb.capacity {
		// evict oldest
		evicted := rb.metas[rb.head]
		delete(rb.data, evicted.ID)
		delete(rb.index, evicted.ID)
		rb.head = (rb.head + 1) % rb.capacity
	} else {
		rb.count++
	}

	rb.metas[slot] = meta
	rb.index[meta.ID] = slot
}

// Update applies fn to the FlowMeta with the given ID.
func (rb *RingBuffer) Update(id FlowID, fn func(*FlowMeta)) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	slot, ok := rb.index[id]
	if !ok {
		return
	}
	fn(&rb.metas[slot])
}

// SetData replaces the FlowData for the given ID.
func (rb *RingBuffer) SetData(id FlowID, d *FlowData) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if _, ok := rb.index[id]; !ok {
		return
	}
	rb.data[id] = d
}

// Get returns the FlowMeta and FlowData for the given ID.
func (rb *RingBuffer) Get(id FlowID) (*FlowMeta, *FlowData, error) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	slot, ok := rb.index[id]
	if !ok {
		return nil, nil, fmt.Errorf("flow %s not found", id)
	}
	meta := rb.metas[slot]
	return &meta, rb.data[id], nil
}

// List returns flows newest-first, filtered, with offset/limit pagination.
// The second return value is the total count matching the filter.
func (rb *RingBuffer) List(filter Filter, offset, limit int) ([]FlowMeta, int) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	var matched []FlowMeta
	// iterate newest-first
	for i := rb.count - 1; i >= 0; i-- {
		slot := (rb.head + i) % rb.capacity
		m := rb.metas[slot]
		if filter != nil && !filter.Match(&m) {
			continue
		}
		matched = append(matched, m)
	}

	total := len(matched)

	if offset >= total {
		return nil, total
	}
	matched = matched[offset:]
	if limit > 0 && limit < len(matched) {
		matched = matched[:limit]
	}
	return matched, total
}

// Len returns the number of flows in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}
