package breakpoint

import (
	"github.com/kostyay/httpmon/internal/store"
)

// Phase indicates whether a breakpoint was hit during request or response.
type Phase int

const (
	PhaseRequest Phase = iota
	PhaseResponse
)

// BreakpointHit describes a paused flow awaiting user action.
type BreakpointHit struct {
	FlowID  store.FlowID
	Phase   Phase
	Headers map[string]string
	Body    []byte
	Meta    store.FlowMeta
}

// BreakpointResume carries the user's edits (or skip) back to the proxy goroutine.
type BreakpointResume struct {
	Headers map[string]string
	Body    []byte
	Skipped bool
}

// Controller manages breakpoint pause/resume lifecycle.
type Controller interface {
	Pause(hit BreakpointHit) BreakpointResume
	Subscribe() <-chan BreakpointHit
	Resume(flowID store.FlowID, resp BreakpointResume)
	Pending() []BreakpointHit
	ResumeAll()
}
