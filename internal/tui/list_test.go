package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kostyay/httpmon/internal/store"
)

func TestProcessLabelEmpty(t *testing.T) {
	got := processLabel("", 0)
	if got != "\u2014" {
		t.Errorf("processLabel(\"\", 0) = %q, want em dash", got)
	}
}

func TestProcessLabelNonEmpty(t *testing.T) {
	got := processLabel("curl", 0)
	if got != "curl" {
		t.Errorf("processLabel(\"curl\", 0) = %q, want \"curl\"", got)
	}
}

func TestProcessLabelWithPID(t *testing.T) {
	got := processLabel("curl", 1234)
	if got != "curl(1234)" {
		t.Errorf("processLabel(\"curl\", 1234) = %q, want \"curl(1234)\"", got)
	}
}

func TestRenderFlowColumnsStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		state      store.FlowState
		want       string
	}{
		{
			name:       "completed with 200",
			statusCode: 200,
			state:      store.StateCompleted,
			want:       "200",
		},
		{
			name:       "in-progress no status",
			statusCode: 0,
			state:      store.StateInProgress,
			want:       "...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := store.FlowMeta{
				Method:     "GET",
				StatusCode: tt.statusCode,
				Path:       "/test",
				State:      tt.state,
			}
			out := ansi.Strip(renderFlowColumns(f, 30, "", ""))
			if !strings.Contains(out, tt.want) {
				t.Errorf("expected %q in output, got: %s", tt.want, out)
			}
		})
	}
}

func TestRenderBodyTruncation(t *testing.T) {
	t.Run("long body truncated", func(t *testing.T) {
		var lines []string
		for i := range maxBodyLines + 10 {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
		body := []byte(strings.Join(lines, "\n"))

		var b strings.Builder
		renderBody(&b, body, "text/plain", true, false)
		out := ansi.Strip(b.String())
		if !strings.Contains(out, "truncated") {
			t.Errorf("expected 'truncated' in output, got: %s", out)
		}
	})

	t.Run("short body not truncated", func(t *testing.T) {
		body := []byte("short\nbody\n")

		var b strings.Builder
		renderBody(&b, body, "text/plain", true, false)
		out := ansi.Strip(b.String())
		if strings.Contains(out, "truncated") {
			t.Errorf("short body should not be truncated, got: %s", out)
		}
	})
}

func TestRenderFlowColumnsDuration(t *testing.T) {
	f := store.FlowMeta{
		Method:     "GET",
		StatusCode: 200,
		Path:       "/test",
		State:      store.StateCompleted,
		Duration:   150 * time.Millisecond,
	}
	out := ansi.Strip(renderFlowColumns(f, 30, "", ""))
	if !strings.Contains(out, "150ms") {
		t.Errorf("expected '150ms' in output, got: %s", out)
	}
}
