package cli_test

import (
	"maps"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/status"
)

// claudePath is where run finds claude in these tests.
const claudePath = "/opt/tools/bin/claude"

func TestRun(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"ANTHROPIC_CUSTOM_HEADERS": "X-Trace: on"})
	srv.start(t)
	srv.waitForStatus(t, func(doc status.Document) bool { return doc.Best == "work" })
	tests := []struct {
		name        string
		args        []string
		wantArgv    []string
		wantToken   string
		wantHeaders string
	}{
		{
			name:        "pinned",
			args:        []string{"run", "--account", "side", "--", "--print", "a prompt"},
			wantArgv:    []string{"claude", "--print", "a prompt"},
			wantToken:   "test-token-side",
			wantHeaders: "X-Trace: on\nX-Switchboard-Account: side",
		},
		{
			name:        "on the router's best",
			args:        []string{"run", "--", "--resume"},
			wantArgv:    []string{"claude", "--resume"},
			wantToken:   "test-token-work",
			wantHeaders: "X-Trace: on",
		},
		{
			name:        "without arguments",
			args:        []string{"run"},
			wantArgv:    []string{"claude"},
			wantToken:   "test-token-work",
			wantHeaders: "X-Trace: on",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := srv.deps
			handed := recordHandOffs(&deps)

			if got := run(t, deps, tt.args...); got != (result{}) {
				t.Errorf("switchboard %s = %+v, want exit status 0 and no output", strings.Join(tt.args, " "), got)
			}
			got := handed.only(t)
			if got.path != claudePath || !slices.Equal(got.argv, tt.wantArgv) {
				t.Errorf("handed over to %s as %q, want %s as %q", got.path, got.argv, claudePath, tt.wantArgv)
			}
			want := map[string]string{
				"ANTHROPIC_BASE_URL":       "http://" + srv.listen,
				"CLAUDE_CODE_OAUTH_TOKEN":  tt.wantToken,
				"ANTHROPIC_CUSTOM_HEADERS": tt.wantHeaders,
			}
			if env := got.only("ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_CUSTOM_HEADERS"); !maps.Equal(env, want) {
				t.Errorf("handed over with\n%q\nwant\n%q", env, want)
			}
		})
	}
}

func TestRunWithoutTheRouter(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"ANTHROPIC_BASE_URL": "http://127.0.0.1:4747"})
	handed := recordHandOffs(&srv.deps)

	got := run(t, srv.deps, "run", "--", "--resume")
	if want := (result{stderr: "switchboard: the router isn't running — connecting directly on work · Work\n"}); got != want {
		t.Errorf("switchboard run = %+v, want %+v", got, want)
	}
	env := handed.only(t).only("ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN")
	if want := map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "test-token-work"}; !maps.Equal(env, want) {
		t.Errorf("handed over with\n%q\nwant\n%q", env, want)
	}
}

func TestRunDirect(t *testing.T) {
	deps := testDeps(map[string]string{
		"CLAUDE_CODE_OAUTH_TOKEN": "test-token-work",
		"ANTHROPIC_BASE_URL":      "http://127.0.0.1:4747",
		"TERM":                    "xterm-256color",
	}, t.TempDir())
	handed := recordHandOffs(&deps)

	if got := run(t, deps, "run", "--direct", "--", "--resume"); got != (result{}) {
		t.Errorf("switchboard run --direct = %+v, want exit status 0 and no output, with no config needed", got)
	}
	got := handed.only(t)
	if !slices.Equal(got.argv, []string{"claude", "--resume"}) || !maps.Equal(got.env, map[string]string{"TERM": "xterm-256color"}) {
		t.Errorf("handed over as %q with %q, want claude --resume on its own login", got.argv, got.env)
	}
}

func TestRunNeverLogsClaudesArguments(t *testing.T) {
	const prompt = "a prompt the log must never hold"
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"SWITCHBOARD_LOG_LEVEL": "debug"})
	srv.start(t)
	recordHandOffs(&srv.deps)

	if got := run(t, srv.deps, "run", "--account", "side", "--", "--print", prompt); got.code != 0 {
		t.Fatalf("switchboard run = %+v, want exit status 0", got)
	}
	log := srv.cliLog(t)
	if !hasLine(log, "level=INFO", `msg="starting claude" component=launch`, "mode=routed", "account=side", "chosen=pinned", "claude="+claudePath) {
		t.Errorf("cli.log reads\n%s\nwant the launch logged", log)
	}
	for _, secret := range []string{prompt, "test-token-work", "test-token-side"} {
		if strings.Contains(log, secret) {
			t.Errorf("cli.log reads\n%s\nwant it without %q", log, secret)
		}
	}
}

// handOff is a hand-over run made: the program, and the arguments and
// environment it was to start with.
type handOff struct {
	path string
	argv []string
	env  map[string]string
}

// only returns the variables named that the program was to start with.
func (h handOff) only(names ...string) map[string]string {
	env := maps.Clone(h.env)
	maps.DeleteFunc(env, func(name, _ string) bool { return !slices.Contains(names, name) })
	return env
}

// handOffs are the hand-overs run makes.
type handOffs struct {
	made []handOff
}

// only returns the one hand-over made.
func (h *handOffs) only(t *testing.T) handOff {
	t.Helper()
	if len(h.made) != 1 {
		t.Fatalf("handed over %d times, want once", len(h.made))
	}
	return h.made[0]
}

// recordHandOffs has deps find claude at claudePath, and note each hand-over
// to it in place of making it.
func recordHandOffs(deps *cli.Deps) *handOffs {
	h := &handOffs{}
	deps.LookPath = func(file string) (string, error) {
		if file != "claude" {
			return "", exec.ErrNotFound
		}
		return claudePath, nil
	}
	deps.Exec = func(path string, argv, env []string) error {
		vars := make(map[string]string)
		for _, variable := range env {
			name, value, _ := strings.Cut(variable, "=")
			vars[name] = value
		}
		h.made = append(h.made, handOff{path: path, argv: argv, env: vars})
		return nil
	}
	return h
}
