package tui

import (
	"time"

	"github.com/kostyay/httpmon/internal/bodydecoder"
	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/kostyay/httpmon/internal/store"
)

// FlowReader is the read-only port the TUI uses to access captured flows.
type FlowReader interface {
	List(filter store.Filter, offset, limit int) ([]store.FlowMeta, int)
	Get(id store.FlowID) (*store.FlowMeta, *store.FlowData, error)
}

// ProxyInfo exposes proxy status to the TUI (listen address, etc.).
type ProxyInfo interface {
	Addr() string
}

// ScriptManager exposes script operations to the TUI.
type ScriptManager interface {
	Scripts() []scripting.ScriptInfo
	Toggle(filePath string) error
	Delete(filePath string) error
	CreateNew() (string, error)
	QuickAddMapLocal(pattern, localPath string) (string, error)
	ScriptDir() string
	Reload()
}

// ThrottleController manages bandwidth throttling at runtime.
type ThrottleController interface {
	SetThrottle(bps int64, latency time.Duration)
	GetThrottleBPS() int64
	GetThrottleLatency() time.Duration
}

// MCPServer exposes MCP server status to the TUI.
type MCPServer interface {
	Running() bool
	Port() int
}

// BodyDecoderRegistry decodes wire-format bodies (e.g. protobuf) into
// human-readable text for display in the detail view.
type BodyDecoderRegistry interface {
	Decode(body []byte, contentType string, meta bodydecoder.DecoderMetadata) (decoded string, resultContentType string, err error)
}
