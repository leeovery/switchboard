package claude_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
)

func TestIsLocal(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "setup-token", args: []string{"setup-token"}, want: true},
		{name: "update", args: []string{"update"}, want: true},
		{name: "upgrade", args: []string{"upgrade"}, want: true},
		{name: "install", args: []string{"install", "stable"}, want: true},
		{name: "doctor", args: []string{"doctor"}, want: true},
		{name: "mcp", args: []string{"mcp", "list"}, want: true},
		{name: "plugin", args: []string{"plugin", "install", "formatter"}, want: true},
		{name: "plugins", args: []string{"plugins"}, want: true},
		{name: "auth", args: []string{"auth", "status"}, want: true},
		{name: "import", args: []string{"import"}, want: true},
		{name: "project", args: []string{"project"}, want: true},
		{name: "auto-mode", args: []string{"auto-mode"}, want: true},
		{name: "gateway", args: []string{"gateway"}, want: true},
		{name: "no arguments"},
		{name: "a prompt that's a subcommand's name", args: []string{"-p", "doctor"}},
		{name: "a subcommand after an option", args: []string{"--debug", "mcp", "list"}},
		{name: "a subcommand's name in another case", args: []string{"Doctor"}},
		{name: "a prompt", args: []string{"update the docs"}},
		{name: "a background session", args: []string{"--bg", "fix the tests"}},
		{name: "agents", args: []string{"agents"}},
		{name: "attach", args: []string{"attach", "0b5c6f2e"}},
		{name: "respawn", args: []string{"respawn"}},
		{name: "ultrareview", args: []string{"ultrareview"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claude.IsLocal(tt.args); got != tt.want {
				t.Errorf("IsLocal(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
