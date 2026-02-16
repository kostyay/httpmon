package tui

import "testing"

func TestProcessLabelEmpty(t *testing.T) {
	got := processLabel("")
	if got != "\u2014" {
		t.Errorf("processLabel(\"\") = %q, want em dash", got)
	}
}

func TestProcessLabelNonEmpty(t *testing.T) {
	got := processLabel("curl")
	if got != "curl" {
		t.Errorf("processLabel(\"curl\") = %q, want \"curl\"", got)
	}
}
