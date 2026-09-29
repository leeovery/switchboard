package childenv_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/childenv"
)

func TestMinimal(t *testing.T) {
	env := map[string]string{
		"PATH":                    "/usr/bin:/bin",
		"HOME":                    "/home/tester",
		"TMPDIR":                  "/tmp/tester",
		"LANG":                    "en_GB.UTF-8",
		"CLAUDE_TOKEN_WORK":       "test-token-work",
		"CLAUDE_CODE_OAUTH_TOKEN": "test-token-oauth",
		"ANTHROPIC_API_KEY":       "test-key",
		"TERM":                    "xterm-256color",
	}

	got := childenv.Minimal(func(key string) string { return env[key] })
	if want := []string{"PATH=/usr/bin:/bin", "HOME=/home/tester", "TMPDIR=/tmp/tester", "LANG=en_GB.UTF-8"}; !slices.Equal(got, want) {
		t.Errorf("Minimal() = %q, want %q", got, want)
	}
}

func TestMinimalOfNothingSetIsAnEmptyEnvironment(t *testing.T) {
	if got := childenv.Minimal(func(string) string { return "" }); got == nil || len(got) > 0 {
		t.Errorf("Minimal() = %#v, want an empty environment, not nil, which exec.Cmd takes for the whole of this one", got)
	}
}
