package breakpoint

import (
	"sync"

	"github.com/kostyay/httpmon/internal/store"
)

type pendingHit struct {
	hit    BreakpointHit
	respCh chan BreakpointResume
}

type controller struct {
	mu          sync.Mutex
	pending     map[store.FlowID]*pendingHit
	subscribers []chan BreakpointHit
}

var _ Controller = (*controller)(nil)

// NewController returns a thread-safe Controller implementation.
func NewController() *controller {
	return &controller{
		pending: make(map[store.FlowID]*pendingHit),
	}
}

func (c *controller) Pause(hit BreakpointHit) BreakpointResume {
	ph := &pendingHit{
		hit:    hit,
		respCh: make(chan BreakpointResume, 1),
	}

	c.mu.Lock()
	c.pending[hit.FlowID] = ph
	subs := make([]chan BreakpointHit, len(c.subscribers))
	copy(subs, c.subscribers)
	c.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub <- hit:
		default:
		}
	}

	return <-ph.respCh
}

func (c *controller) Subscribe() <-chan BreakpointHit {
	ch := make(chan BreakpointHit, 16)
	c.mu.Lock()
	c.subscribers = append(c.subscribers, ch)
	c.mu.Unlock()
	return ch
}

func (c *controller) Resume(
	flowID store.FlowID, resp BreakpointResume,
) {
	c.mu.Lock()
	ph, ok := c.pending[flowID]
	if ok {
		delete(c.pending, flowID)
	}
	c.mu.Unlock()

	if ok {
		ph.respCh <- resp
	}
}

func (c *controller) Pending() []BreakpointHit {
	c.mu.Lock()
	defer c.mu.Unlock()

	hits := make([]BreakpointHit, 0, len(c.pending))
	for _, ph := range c.pending {
		hits = append(hits, ph.hit)
	}
	return hits
}

func (c *controller) ResumeAll() {
	c.mu.Lock()
	all := make([]*pendingHit, 0, len(c.pending))
	for _, ph := range c.pending {
		all = append(all, ph)
	}
	c.pending = make(map[store.FlowID]*pendingHit)
	c.mu.Unlock()

	for _, ph := range all {
		ph.respCh <- BreakpointResume{Skipped: true}
	}
}
