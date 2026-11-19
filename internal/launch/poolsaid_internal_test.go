package launch

import (
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

// A pool's label is redacted as it's shown, cleaned: a token split by a
// character drawn as nothing, joined again as that's dropped, is hidden.
func TestAPoolIsSaidRedactedAsItShows(t *testing.T) {
	tests := []struct {
		name, label, want string
	}{
		{name: "a token whole", label: "Acme sk-ant-oat01-fake_token-shaped", want: "Acme [redacted]"},
		{name: "a token split by a zero-width space", label: "Acme sk\u200b-ant-oat01-fake_token-shaped", want: "Acme [redacted]"},
		{name: "a label showing nothing, by its id", label: "\u200e", want: "q7mt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := poolSaid(config.Pool{ID: "q7mt", Label: tt.label}); got != tt.want {
				t.Errorf("poolSaid() = %q, want %q", got, tt.want)
			}
		})
	}
}
