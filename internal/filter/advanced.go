package filter

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/kostyay/httpmon/internal/store"
)

// AdvancedFilter ANDs multiple terms together.
type AdvancedFilter struct {
	terms []term
}

type term struct {
	negate  bool
	matcher func(*store.FlowMeta) bool
}

// Compile parses a filter expression into an AdvancedFilter.
// Returns nil for empty input (matches all).
func Compile(input string) *AdvancedFilter {
	terms := parseTerms(input)
	if len(terms) == 0 {
		return nil
	}
	return &AdvancedFilter{terms: terms}
}

func (f *AdvancedFilter) Match(flow *store.FlowMeta) bool {
	for _, t := range f.terms {
		m := t.matcher(flow)
		if t.negate {
			m = !m
		}
		if !m {
			return false
		}
	}
	return true
}

func parseTerms(input string) []term {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	fields := strings.Fields(input)
	terms := make([]term, 0, len(fields))

	for _, field := range fields {
		t := parseSingleTerm(field)
		if t.matcher != nil {
			terms = append(terms, t)
		}
	}
	return terms
}

func parseSingleTerm(s string) term {
	var negate bool
	if strings.HasPrefix(s, "!") {
		negate = true
		s = s[1:]
	}

	var matcher func(*store.FlowMeta) bool

	switch {
	case strings.HasPrefix(s, "s:"):
		matcher = compileStatus(s[2:])
	case strings.HasPrefix(s, "m:"):
		matcher = compileMethod(s[2:])
	case strings.HasPrefix(s, "ct:"):
		matcher = compileContentType(s[3:])
	case strings.HasPrefix(s, "re:"):
		matcher = compileRegex(s[3:])
	default:
		// Backward-compatible substring on host+path.
		q := strings.ToLower(s)
		matcher = func(flow *store.FlowMeta) bool {
			return strings.Contains(strings.ToLower(flow.Host+flow.Path), q)
		}
	}

	return term{negate: negate, matcher: matcher}
}

func compileStatus(val string) func(*store.FlowMeta) bool {
	// Range: 2xx, 3xx, 4xx, 5xx
	if len(val) == 3 && val[1] == 'x' && val[2] == 'x' {
		base := int(val[0]-'0') * 100
		return func(flow *store.FlowMeta) bool {
			return flow.StatusCode >= base && flow.StatusCode < base+100
		}
	}
	// Exact status code.
	code, err := strconv.Atoi(val)
	if err != nil {
		return func(*store.FlowMeta) bool { return false }
	}
	return func(flow *store.FlowMeta) bool {
		return flow.StatusCode == code
	}
}

func compileMethod(val string) func(*store.FlowMeta) bool {
	method := strings.ToUpper(val)
	return func(flow *store.FlowMeta) bool {
		return strings.EqualFold(flow.Method, method)
	}
}

func compileContentType(val string) func(*store.FlowMeta) bool {
	q := strings.ToLower(val)
	return func(flow *store.FlowMeta) bool {
		return strings.Contains(strings.ToLower(flow.ContentType), q)
	}
}

func compileRegex(val string) func(*store.FlowMeta) bool {
	// Strip surrounding slashes if present.
	val = strings.TrimPrefix(val, "/")
	val = strings.TrimSuffix(val, "/")
	re, err := regexp.Compile(val)
	if err != nil {
		return func(*store.FlowMeta) bool { return false }
	}
	return func(flow *store.FlowMeta) bool {
		return re.MatchString(flow.Host + flow.Path)
	}
}
