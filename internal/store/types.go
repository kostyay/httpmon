package store

import (
	"net/http"
	"time"
)

// FlowID is the unique identifier for a captured flow.
type FlowID = string

// FlowState represents the lifecycle state of a flow.
type FlowState int

const (
	StateInProgress FlowState = iota
	StateCompleted
	StateFailed
)

// FlowMeta holds the summary fields displayed in the list view.
type FlowMeta struct {
	ID          FlowID
	Method      string
	StatusCode  int
	Host        string
	Path        string
	Duration    time.Duration
	SizeBytes   int64
	StartedAt   time.Time
	State       FlowState
	ContentType string
	Scheme      string
}

// FlowData holds the full request/response headers and bodies.
type FlowData struct {
	RequestHeaders  http.Header
	RequestBody     []byte
	ResponseHeaders http.Header
	ResponseBody    []byte
}

// Filter determines whether a FlowMeta should be included in results.
type Filter interface {
	Match(flow *FlowMeta) bool
}
