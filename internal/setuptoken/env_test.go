package setuptoken

import (
	"slices"
	"testing"
)

func TestForSetupTokenIsTheUsersEnvironmentWithTheShimAsItsBrowser(t *testing.T) {
	env := environ{
		"PATH=/usr/bin:/bin",
		"HOME=/home/work",
		"ANTHROPIC_BASE_URL=http://127.0.0.1:4747",
		"CLAUDE_CODE_OAUTH_TOKEN=test-token-work",
		"ANTHROPIC_API_KEY=test-key-work",
		"ANTHROPIC_AUTH_TOKEN=test-token-personal",
		"BROWSER=/usr/bin/false",
		"EDITOR=vi",
	}

	got := env.forSetupToken("/opt/claude/bin/claude", "/tmp/shim/browser")

	want := []string{"PATH=/opt/claude/bin:/usr/bin:/bin", "HOME=/home/work", "EDITOR=vi", "BROWSER=/tmp/shim/browser"}
	if !slices.Equal(got, want) {
		t.Errorf("forSetupToken() = %q, want %q", got, want)
	}
	if env[2] != "ANTHROPIC_BASE_URL=http://127.0.0.1:4747" {
		t.Errorf("forSetupToken() changed the environment it was given: %q", env)
	}
}

func TestEnvironGetsAKeysValue(t *testing.T) {
	env := ownEnviron([]string{"EDITOR=vi", "PATH=/bin", "PATH=/usr/bin", "EMPTY="})
	tests := []struct {
		key, want string
	}{
		{key: "PATH", want: "/usr/bin"},
		{key: "EMPTY"},
		{key: "UNSET"},
		{key: "EDIT"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := env.get(tt.key); got != tt.want {
				t.Errorf("get(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}
