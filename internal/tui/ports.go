package tui

import "github.com/kostyay/httpmon/internal/store"

// FlowReader is the read-only port the TUI uses to access captured flows.
type FlowReader interface {
	List(filter store.Filter, offset, limit int) ([]store.FlowMeta, int)
	Get(id store.FlowID) (*store.FlowMeta, *store.FlowData, error)
}

// ProxyInfo exposes proxy status to the TUI (listen address, etc.).
type ProxyInfo interface {
	Addr() string
}
