package claude_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
)

func TestProviderThinkingBound(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "claude-sonnet-5-5", want: true},
		{model: "claude-sonnet-5-5-20260915", want: true},
		{model: "claude-sonnet-5", want: false},
		{model: "claude-sonnet-4-5-20250929", want: false},
		{model: "claude-opus-5-5", want: false},
		{model: "claude-fable-5-1", want: false},
		{model: "claude-haiku-4-5-20251001", want: false},
		{model: "sonnet", want: false},
		{model: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := (claude.Provider{}).ThinkingBound(tt.model); got != tt.want {
				t.Errorf("ThinkingBound(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}
