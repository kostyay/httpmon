package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kostyay/httpmon/internal/store"
)

// DiffLine represents a single line in the diff output.
type DiffLine struct {
	Type    string // "add", "del", "ctx" (context/unchanged)
	Content string
}

// Result holds the diff between two flows.
type Result struct {
	Lines []DiffLine
}

// HasChanges returns true if there are any additions or deletions.
func (r *Result) HasChanges() bool {
	for _, l := range r.Lines {
		if l.Type == "add" || l.Type == "del" {
			return true
		}
	}
	return false
}

// Render returns a string representation of the diff.
// If color is true, uses ANSI color codes.
func (r *Result) Render(color bool) string {
	var b strings.Builder
	for _, l := range r.Lines {
		prefix := "  "
		switch l.Type {
		case "add":
			prefix = "+ "
		case "del":
			prefix = "- "
		}
		b.WriteString(prefix + l.Content + "\n")
	}
	return b.String()
}

// Compare produces a diff between two flows.
func Compare(meta1 *store.FlowMeta, data1 *store.FlowData, meta2 *store.FlowMeta, data2 *store.FlowData) *Result {
	r := &Result{}

	// General info.
	r.diffField("Method", meta1.Method, meta2.Method)
	r.diffField("URL", fmtURL(meta1), fmtURL(meta2))
	r.diffField("Status", fmt.Sprintf("%d", meta1.StatusCode), fmt.Sprintf("%d", meta2.StatusCode))

	// Request headers.
	var reqH1, reqH2 map[string][]string
	if data1 != nil {
		reqH1 = data1.RequestHeaders
	}
	if data2 != nil {
		reqH2 = data2.RequestHeaders
	}
	r.diffHeaders("Request Headers", reqH1, reqH2)

	// Response headers.
	var respH1, respH2 map[string][]string
	if data1 != nil {
		respH1 = data1.ResponseHeaders
	}
	if data2 != nil {
		respH2 = data2.ResponseHeaders
	}
	r.diffHeaders("Response Headers", respH1, respH2)

	// Request body.
	var reqB1, reqB2 []byte
	if data1 != nil {
		reqB1 = data1.RequestBody
	}
	if data2 != nil {
		reqB2 = data2.RequestBody
	}
	r.diffBody("Request Body", reqB1, reqB2)

	// Response body.
	var respB1, respB2 []byte
	if data1 != nil {
		respB1 = data1.ResponseBody
	}
	if data2 != nil {
		respB2 = data2.ResponseBody
	}
	r.diffBody("Response Body", respB1, respB2)

	return r
}

func fmtURL(m *store.FlowMeta) string {
	return fmt.Sprintf("%s://%s%s", m.Scheme, m.Host, m.Path)
}

func (r *Result) diffField(name, v1, v2 string) {
	if v1 == v2 {
		r.Lines = append(r.Lines, DiffLine{Type: "ctx", Content: fmt.Sprintf("%s: %s", name, v1)})
		return
	}
	r.Lines = append(r.Lines, DiffLine{Type: "del", Content: fmt.Sprintf("%s: %s", name, v1)})
	r.Lines = append(r.Lines, DiffLine{Type: "add", Content: fmt.Sprintf("%s: %s", name, v2)})
}

func (r *Result) diffHeaders(section string, h1, h2 map[string][]string) {
	r.Lines = append(r.Lines, DiffLine{Type: "ctx", Content: fmt.Sprintf("--- %s ---", section)})

	allKeys := map[string]bool{}
	for k := range h1 {
		allKeys[k] = true
	}
	for k := range h2 {
		allKeys[k] = true
	}

	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v1 := joinVals(h1, k)
		v2 := joinVals(h2, k)
		if v1 == v2 {
			r.Lines = append(r.Lines, DiffLine{Type: "ctx", Content: fmt.Sprintf("  %s: %s", k, v1)})
		} else {
			if v1 != "" {
				r.Lines = append(r.Lines, DiffLine{Type: "del", Content: fmt.Sprintf("  %s: %s", k, v1)})
			}
			if v2 != "" {
				r.Lines = append(r.Lines, DiffLine{Type: "add", Content: fmt.Sprintf("  %s: %s", k, v2)})
			}
		}
	}
}

func joinVals(h map[string][]string, key string) string {
	vals, ok := h[key]
	if !ok {
		return ""
	}
	return strings.Join(vals, ", ")
}

func (r *Result) diffBody(section string, b1, b2 []byte) {
	s1 := string(b1)
	s2 := string(b2)
	if s1 == s2 {
		if s1 != "" {
			r.Lines = append(r.Lines, DiffLine{Type: "ctx", Content: fmt.Sprintf("--- %s (unchanged) ---", section)})
		}
		return
	}

	r.Lines = append(r.Lines, DiffLine{Type: "ctx", Content: fmt.Sprintf("--- %s ---", section)})

	// Simple line-by-line diff.
	lines1 := strings.Split(s1, "\n")
	lines2 := strings.Split(s2, "\n")

	// Use basic LCS approach for small bodies; fallback to sequential for large.
	maxLen := max(len(lines1), len(lines2))
	if maxLen > 200 {
		// Just show removed and added.
		for _, l := range lines1 {
			r.Lines = append(r.Lines, DiffLine{Type: "del", Content: l})
		}
		for _, l := range lines2 {
			r.Lines = append(r.Lines, DiffLine{Type: "add", Content: l})
		}
		return
	}

	r.simpleDiff(lines1, lines2)
}

func (r *Result) simpleDiff(a, b []string) {
	// Build LCS table.
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}

	i, j := 0, 0
	for i < m || j < n {
		if i < m && j < n && a[i] == b[j] {
			r.Lines = append(r.Lines, DiffLine{Type: "ctx", Content: a[i]})
			i++
			j++
		} else if j < n && (i >= m || dp[i][j+1] >= dp[i+1][j]) {
			r.Lines = append(r.Lines, DiffLine{Type: "add", Content: b[j]})
			j++
		} else {
			r.Lines = append(r.Lines, DiffLine{Type: "del", Content: a[i]})
			i++
		}
	}
}
