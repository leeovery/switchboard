package cli_test

import (
	"strings"
	"testing"
)

func TestInitZsh(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), nil)
	deps.Executable = func() (string, error) { return "/usr/local/bin/switchboard", nil }

	got := run(t, deps, "init", "zsh")
	want := result{stdout: `function claude { '/usr/local/bin/switchboard' run -- "$@"; }
function cxwork { '/usr/local/bin/switchboard' run --account work -- "$@"; }
function cxpersonal { '/usr/local/bin/switchboard' run --account personal -- "$@"; }
function cxside { '/usr/local/bin/switchboard' run --account side -- "$@"; }
if [[ -n ${CLAUDE_TOKEN_WORK-} ]]; then
  export CLAUDE_CODE_OAUTH_TOKEN="${CLAUDE_TOKEN_WORK}"
fi
`}
	if got != want {
		t.Errorf("switchboard init zsh =\n%+v\nwant\n%+v", got, want)
	}
	for _, token := range []string{"test-token-work", "test-token-side"} {
		if strings.Contains(got.stdout+got.stderr, token) {
			t.Errorf("switchboard init zsh printed a token set for an account:\n%s", got.stdout)
		}
	}
}

func TestInitZshWithAPrefix(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), nil)
	deps.Executable = func() (string, error) { return "/usr/local/bin/switchboard", nil }

	got := run(t, deps, "init", "zsh", "--prefix", "claude-")
	want := "function claude-side { '/usr/local/bin/switchboard' run --account side -- \"$@\"; }\n"
	if got.code != 0 || !strings.Contains(got.stdout, want) {
		t.Errorf("switchboard init zsh --prefix claude- = %+v, want a launcher\n%s", got, want)
	}
}

func TestInitRefusesAnotherShell(t *testing.T) {
	got := run(t, testDeps(nil, t.TempDir()), "init", "bash")
	if want := "Error: unsupported shell \"bash\": only zsh is supported\n"; got.code != 1 || !strings.HasPrefix(got.stderr, want) {
		t.Errorf("switchboard init bash = %+v, want exit status 1 and an error starting %q", got, want)
	}
}
