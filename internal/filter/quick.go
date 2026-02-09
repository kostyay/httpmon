package filter

import (
	"strings"

	"github.com/kostyay/httpmon/internal/store"
)

// QuickFilter matches flows by case-insensitive substring on host+path.
type QuickFilter struct {
	query string
}

// CompileQuick creates a QuickFilter from user input.
// An empty input returns nil (matches all).
func CompileQuick(input string) *QuickFilter {
	q := strings.TrimSpace(input)
	if q == "" {
		return nil
	}
	return &QuickFilter{query: strings.ToLower(q)}
}

func (f *QuickFilter) Match(flow *store.FlowMeta) bool {
	target := strings.ToLower(flow.Host + flow.Path)
	return strings.Contains(target, f.query)
}
