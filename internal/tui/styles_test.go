package tui

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestStatusStyle(t *testing.T) {
	tests := []struct {
		code int
		want lipgloss.Style
	}{
		{200, styleStatus2xx},
		{204, styleStatus2xx},
		{301, styleStatus3xx},
		{404, styleStatus4xx},
		{500, styleStatus5xx},
		{100, styleMuted},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("code_%d", tt.code), func(t *testing.T) {
			got := statusStyle(tt.code)
			if got.GetForeground() != tt.want.GetForeground() {
				t.Errorf("statusStyle(%d) foreground = %v, want %v", tt.code, got.GetForeground(), tt.want.GetForeground())
			}
		})
	}
}
