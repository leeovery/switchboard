package cli

import (
	"testing"

	"github.com/leeovery/switchboard/internal/views"
)

// history says a version's share of use as Accounts' BY MODEL does, a half
// percent among the shares that round to none, and a dash where it has none
// or isn't listed.
func TestHistorySaysAVersionsShareAsByModelDoes(t *testing.T) {
	shares := []views.ModelShare{{Model: "claude-opus-5-5", Share: new(0.55)}, {Model: "claude-haiku-4-5", Share: new(0.005)}, {Model: "claude-fable-5"}}
	tests := []struct {
		model, want string
	}{
		{model: "claude-opus-5-5", want: "55%"},
		{model: "claude-haiku-4-5", want: "<1%"},
		{model: "claude-fable-5", want: dash},
		{model: "claude-sonnet-5-5", want: dash},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := shareText(shares, tt.model); got != tt.want {
				t.Errorf("shareText(%s) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}
