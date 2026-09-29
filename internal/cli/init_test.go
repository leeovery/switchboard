package cli_test

import (
	"os"
	"path/filepath"
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

func TestInitZshHandsTheConfigGivenOnToRun(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "it's here", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[[account]]\nid = \"side\"\ntoken_env = \"CLAUDE_TOKEN_SIDE\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	deps := testDeps(nil, t.TempDir())
	deps.Executable = func() (string, error) { return "/usr/local/bin/switchboard", nil }

	got := run(t, deps, "--config", filepath.Join("it's here", "config.toml"), "init", "zsh")
	want := result{stdout: `function claude { '/usr/local/bin/switchboard' --config '` + dir + `/it'\''s here/config.toml' run -- "$@"; }
function cxside { '/usr/local/bin/switchboard' --config '` + dir + `/it'\''s here/config.toml' run --account side -- "$@"; }
if [[ -n ${CLAUDE_TOKEN_SIDE-} ]]; then
  export CLAUDE_CODE_OAUTH_TOKEN="${CLAUDE_TOKEN_SIDE}"
fi
`}
	if got != want {
		t.Errorf("switchboard --config init zsh =\n%+v\nwant\n%+v", got, want)
	}
}

func TestInitZshExportsATokenThatsSet(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "the first account's, when it's set", env: map[string]string{"CLAUDE_TOKEN_SIDE": "test-token-side", "CLAUDE_TOKEN_WORK": "test-token-work"}, want: "CLAUDE_TOKEN_WORK"},
		{name: "the first account's whose token is set", env: map[string]string{"CLAUDE_TOKEN_SIDE": "test-token-side"}, want: "CLAUDE_TOKEN_SIDE"},
		{name: "the first account's, when none is", want: "CLAUDE_TOKEN_WORK"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), map[string]string{"CLAUDE_TOKEN_WORK": "", "CLAUDE_TOKEN_SIDE": ""})
			getenv := deps.Getenv
			deps.Getenv = func(key string) string {
				if value, ok := tt.env[key]; ok {
					return value
				}
				return getenv(key)
			}
			deps.Executable = func() (string, error) { return "/usr/local/bin/switchboard", nil }

			got := run(t, deps, "init", "zsh")
			want := "if [[ -n ${" + tt.want + "-} ]]; then\n  export CLAUDE_CODE_OAUTH_TOKEN=\"${" + tt.want + "}\"\nfi\n"
			if got.code != 0 || !strings.HasSuffix(got.stdout, want) {
				t.Errorf("switchboard init zsh = %+v, want it to end\n%s", got, want)
			}
		})
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

func TestInitZshWithoutWhatItNeeds(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "config.toml")
	tests := []struct {
		name string
		// binary is the switchboard binary's path, or "" when it can't be
		// found.
		binary string
		want   result
	}{
		{
			name:   "a config it can read",
			binary: "/usr/local/bin/switchboard",
			want: result{
				stdout: `function claude { '/usr/local/bin/switchboard' run -- "$@"; }` + "\n",
				stderr: "switchboard: couldn't read the config (no config file at " + missing + ") — defining claude alone, with no launchers\n",
			},
		},
		{
			name: "its own binary",
			want: result{stderr: "switchboard: couldn't find its own binary (no test knows its switchboard binary) — defining nothing\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": missing}, t.TempDir())
			if tt.binary != "" {
				deps.Executable = func() (string, error) { return tt.binary, nil }
			}

			if got := run(t, deps, "init", "zsh"); got != tt.want {
				t.Errorf("switchboard init zsh = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestInitRefusesAnotherShell(t *testing.T) {
	got := run(t, testDeps(nil, t.TempDir()), "init", "bash")
	if want := "Error: unsupported shell \"bash\": only zsh is supported\n"; got.code != 1 || !strings.HasPrefix(got.stderr, want) {
		t.Errorf("switchboard init bash = %+v, want exit status 1 and an error starting %q", got, want)
	}
}
