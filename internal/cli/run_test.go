package cli_test

import (
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/cli"
)

func TestRun(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"ANTHROPIC_CUSTOM_HEADERS": "X-Trace: on"})
	srv.start(t)
	tests := []struct {
		name        string
		args        []string
		wantArgv    []string
		wantToken   string
		wantHeaders string
	}{
		{
			name:        "pinned, its conversation alone",
			args:        []string{"run", "--account", "side", "--", "--print", "a prompt"},
			wantArgv:    []string{"claude", "--print", "a prompt"},
			wantToken:   "test-token-work",
			wantHeaders: "X-Trace: on\nX-Switchboard-Account: side",
		},
		{
			name:        "on the primary's token",
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
			handed := recordHandOffs(t, &deps)

			if got := run(t, deps, tt.args...); got != (result{}) {
				t.Errorf("switchboard %s = %+v, want exit status 0 and no output", strings.Join(tt.args, " "), got)
			}
			got := handed.only(t)
			if got.path != handed.claude || !slices.Equal(got.argv, tt.wantArgv) {
				t.Errorf("handed over to %s as %q, want %s as %q", got.path, got.argv, handed.claude, tt.wantArgv)
			}
			want := map[string]string{
				"ANTHROPIC_BASE_URL":       "http://" + srv.listen,
				"CLAUDE_CODE_OAUTH_TOKEN":  tt.wantToken,
				"ANTHROPIC_CUSTOM_HEADERS": tt.wantHeaders,
			}
			if env := got.only("ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_CUSTOM_HEADERS"); !maps.Equal(env, want) {
				t.Errorf("handed over with\n%q\nwant\n%q", env, want)
			}
			if want := strconv.Itoa(testPID) + ":" + strconv.FormatInt(testNow.Unix(), 10) + ":" + handed.claude; got.mark != want {
				t.Errorf("handed over marked %q, want %q: this process's id, when, and where the claude is", got.mark, want)
			}
		})
	}
}

func TestRunGoesWhereTheRouterListensWhateverTheConfigSays(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)
	routerListens := srv.listen
	// The config's listen edited since the router started, which the router
	// hasn't taken up: nothing listens there.
	srv.listen = freeAddress(t)
	srv.writeConfig(t)
	handed := recordHandOffs(t, &srv.deps)

	if got := run(t, srv.deps, "run"); got != (result{}) {
		t.Errorf("switchboard run = %+v, want exit status 0 and no output", got)
	}
	if got, want := handed.only(t).env["ANTHROPIC_BASE_URL"], "http://"+routerListens; got != want {
		t.Errorf("handed over with ANTHROPIC_BASE_URL %q, want %q, where the router listens", got, want)
	}
}

func TestRunWithoutTheRouter(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"ANTHROPIC_BASE_URL": "http://127.0.0.1:4747"})
	handed := recordHandOffs(t, &srv.deps)

	got := run(t, srv.deps, "run", "--", "--resume")
	if want := (result{stderr: "switchboard: the router isn't running — connecting directly on work · Work\n"}); got != want {
		t.Errorf("switchboard run = %+v, want %+v", got, want)
	}
	env := handed.only(t).only("ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN")
	if want := map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "test-token-work"}; !maps.Equal(env, want) {
		t.Errorf("handed over with\n%q\nwant\n%q", env, want)
	}
}

func TestRunWithoutThePrimarysToken(t *testing.T) {
	tests := []struct {
		name string
		// empty leaves the primary's token file empty, as a writer does for a
		// moment before it writes the token, where it's otherwise missing;
		// and written has the file hold the token again once run pauses.
		empty, written bool
		wantAccount    string
		wantToken      string
		wantPaused     []time.Duration
	}{
		{name: "missing, looked at once", wantAccount: "side · Side", wantToken: "test-token-side"},
		{
			name:        "caught empty, looked at again once it's written",
			empty:       true,
			written:     true,
			wantAccount: "work · Work",
			wantToken:   "test-token-work",
			wantPaused:  []time.Duration{200 * time.Millisecond},
		},
		{
			name:        "empty still at a second look",
			empty:       true,
			wantAccount: "side · Side",
			wantToken:   "test-token-side",
			wantPaused:  []time.Duration{200 * time.Millisecond},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), nil)
			if tt.empty {
				writeToken(t, srv.deps, "work", "")
			} else {
				removeToken(t, srv.deps, "work")
			}
			var paused []time.Duration
			srv.deps.Pause = func(d time.Duration) {
				paused = append(paused, d)
				if tt.written {
					writeToken(t, srv.deps, "work", "test-token-work")
				}
			}
			handed := recordHandOffs(t, &srv.deps)

			got := run(t, srv.deps, "run")
			if want := (result{stderr: "switchboard: the router isn't running — connecting directly on " + tt.wantAccount + "\n"}); got != want {
				t.Errorf("switchboard run = %+v, want %+v", got, want)
			}
			if token := handed.only(t).env["CLAUDE_CODE_OAUTH_TOKEN"]; token != tt.wantToken {
				t.Errorf("handed over on %q, want %q", token, tt.wantToken)
			}
			if !slices.Equal(paused, tt.wantPaused) {
				t.Errorf("paused %v, want %v", paused, tt.wantPaused)
			}
		})
	}
}

func TestRunWithoutAConfigItCanRead(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "config.toml")
	invalid := writeConfig(t, invalidConfig)
	tests := []struct {
		name     string
		config   string
		wantSaid string
	}{
		{name: "missing", config: missing, wantSaid: "switchboard: couldn't read the config (no config file at " + missing + ") — starting claude without it\n"},
		{name: "invalid", config: invalid, wantSaid: "switchboard: couldn't read the config (invalid config " + invalid + ") — starting claude without it\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{
				"SWITCHBOARD_CONFIG":       tt.config,
				"CLAUDE_CODE_OAUTH_TOKEN":  "test-token-work",
				"ANTHROPIC_CUSTOM_HEADERS": "X-Switchboard-Account: side",
			}
			deps := testDeps(env, t.TempDir())
			handed := recordHandOffs(t, &deps)

			if got, want := run(t, deps, "run", "--account", "work", "--", "--resume"), (result{stderr: tt.wantSaid}); got != want {
				t.Errorf("switchboard run = %+v, want %+v", got, want)
			}
			got := handed.only(t)
			if !slices.Equal(got.argv, []string{"claude", "--resume"}) || !maps.Equal(got.env, env) {
				t.Errorf("handed over as %q with %q, want claude --resume, its environment untouched", got.argv, got.env)
			}
		})
	}
}

func TestRunStepsAsideForAKeyClaudeCodeMayUse(t *testing.T) {
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
		t.Run(key, func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{key: "test-key", "ANTHROPIC_CUSTOM_HEADERS": "X-Trace: on"})
			srv.start(t)
			inherited := environMap(srv.deps.Environ())
			handed := recordHandOffs(t, &srv.deps)

			got := run(t, srv.deps, "run", "--account", "side", "--", "--resume")
			if want := (result{stderr: "switchboard: " + key + " is set, so Claude Code uses it — starting claude without switchboard\n"}); got != want {
				t.Errorf("switchboard run = %+v, want %+v", got, want)
			}
			if h := handed.only(t); !slices.Equal(h.argv, []string{"claude", "--resume"}) || !maps.Equal(h.env, inherited) {
				t.Errorf("handed over as %q with %q, want claude --resume, its environment untouched", h.argv, h.env)
			}
		})
	}
}

func TestRunDirectAndLocalSubcommandsGoAsTheyDoWithAKey(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": "test-key", "CLAUDE_CODE_OAUTH_TOKEN": "test-token-work", "ANTHROPIC_BASE_URL": "http://127.0.0.1:4747"}
	tests := []struct {
		name    string
		args    []string
		wantEnv map[string]string
	}{
		{name: "on Claude Code's own login", args: []string{"run", "--direct", "--", "--resume"}, wantEnv: map[string]string{"ANTHROPIC_API_KEY": "test-key"}},
		{name: "a local subcommand", args: []string{"run", "--", "doctor"}, wantEnv: env},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(env, t.TempDir())
			handed := recordHandOffs(t, &deps)

			if got := run(t, deps, tt.args...); got != (result{}) {
				t.Errorf("switchboard %s = %+v, want exit status 0 and no output", strings.Join(tt.args, " "), got)
			}
			if got := handed.only(t).env; !maps.Equal(got, tt.wantEnv) {
				t.Errorf("handed over with %q, want %q", got, tt.wantEnv)
			}
		})
	}
}

func TestRunDirect(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "a session", args: []string{"--resume"}},
		{name: "a local subcommand, on the login asked for", args: []string{"auth", "status"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(map[string]string{
				"CLAUDE_CODE_OAUTH_TOKEN": "test-token-work",
				"ANTHROPIC_BASE_URL":      "http://127.0.0.1:4747",
				"TERM":                    "xterm-256color",
			}, t.TempDir())
			handed := recordHandOffs(t, &deps)

			if got := run(t, deps, append([]string{"run", "--direct", "--"}, tt.args...)...); got != (result{}) {
				t.Errorf("switchboard run --direct = %+v, want exit status 0 and no output, with no config needed", got)
			}
			got := handed.only(t)
			if want := append([]string{"claude"}, tt.args...); !slices.Equal(got.argv, want) || !maps.Equal(got.env, map[string]string{"TERM": "xterm-256color"}) {
				t.Errorf("handed over as %q with %q, want %q on its own login", got.argv, got.env, want)
			}
		})
	}
}

func TestRunStartsClaudeCodesLocalSubcommandsAsIfSwitchboardWerentThere(t *testing.T) {
	// What a session started through the router hands the programs it runs.
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{
		"ANTHROPIC_BASE_URL":       "http://127.0.0.1:4747",
		"CLAUDE_CODE_OAUTH_TOKEN":  "test-token-side",
		"ANTHROPIC_CUSTOM_HEADERS": "X-Switchboard-Account: side",
	})
	srv.start(t)
	inherited := environMap(srv.deps.Environ())
	tests := []struct {
		name string
		args []string
		// local says whether claude starts as if switchboard weren't there,
		// rather than through the router.
		local bool
	}{
		{name: "setup-token", args: []string{"setup-token"}, local: true},
		{name: "update", args: []string{"update"}, local: true},
		{name: "upgrade", args: []string{"upgrade"}, local: true},
		{name: "install", args: []string{"install", "stable"}, local: true},
		{name: "doctor", args: []string{"doctor"}, local: true},
		{name: "mcp", args: []string{"mcp", "add", "--transport", "http", "docs", "https://docs.example.com/mcp"}, local: true},
		{name: "plugin", args: []string{"plugin", "install", "formatter"}, local: true},
		{name: "plugins", args: []string{"plugins"}, local: true},
		{name: "auth", args: []string{"auth", "status"}, local: true},
		{name: "import", args: []string{"import"}, local: true},
		{name: "project", args: []string{"project"}, local: true},
		{name: "auto-mode", args: []string{"auto-mode"}, local: true},
		{name: "gateway", args: []string{"gateway"}, local: true},
		{name: "a prompt that's a subcommand's name", args: []string{"-p", "doctor"}},
		{name: "a subcommand after an option", args: []string{"--debug", "mcp", "list"}},
		{name: "a background session", args: []string{"--bg", "fix the tests"}},
		{name: "agents", args: []string{"agents"}},
		{name: "attach", args: []string{"attach", "0b5c6f2e"}},
		{name: "respawn", args: []string{"respawn"}},
		{name: "ultrareview", args: []string{"ultrareview"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := srv.deps
			handed := recordHandOffs(t, &deps)

			if got := run(t, deps, append([]string{"run", "--"}, tt.args...)...); got != (result{}) {
				t.Errorf("switchboard run -- %s = %+v, want exit status 0 and no output", strings.Join(tt.args, " "), got)
			}
			got := handed.only(t)
			if want := append([]string{"claude"}, tt.args...); got.path != handed.claude || !slices.Equal(got.argv, want) {
				t.Errorf("handed over to %s as %q, want %s as %q", got.path, got.argv, handed.claude, want)
			}
			want := maps.Clone(inherited)
			if !tt.local {
				want["ANTHROPIC_BASE_URL"] = "http://" + srv.listen
				want["CLAUDE_CODE_OAUTH_TOKEN"] = "test-token-work"
				delete(want, "ANTHROPIC_CUSTOM_HEADERS")
			}
			if !maps.Equal(got.env, want) {
				t.Errorf("handed over with\n%q\nwant\n%q", got.env, want)
			}
		})
	}
}

func TestRunStartsLocalSubcommandsWithoutAConfig(t *testing.T) {
	env := map[string]string{"SWITCHBOARD_CONFIG": filepath.Join(t.TempDir(), "config.toml"), "CLAUDE_CODE_OAUTH_TOKEN": "test-token-work"}
	deps := testDeps(env, t.TempDir())
	handed := recordHandOffs(t, &deps)

	if got := run(t, deps, "run", "--account", "side", "--", "doctor"); got != (result{}) {
		t.Errorf("switchboard run -- doctor = %+v, want exit status 0 and no output, switchboard having no part in it", got)
	}
	if got := handed.only(t); !slices.Equal(got.argv, []string{"claude", "doctor"}) || !maps.Equal(got.env, env) {
		t.Errorf("handed over as %q with %q, want claude doctor, its environment untouched", got.argv, got.env)
	}
}

func TestArgs(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{name: "switchboard alone", argv: []string{"switchboard"}, want: []string{}},
		{name: "switchboard's commands", argv: []string{"/opt/homebrew/bin/switchboard", "status", "--json"}, want: []string{"status", "--json"}},
		{name: "claude alone", argv: []string{"claude"}, want: []string{"run", "--"}},
		{name: "claude, by its path", argv: []string{"/Users/tester/bin/claude", "--help"}, want: []string{"run", "--", "--help"}},
		{
			name: "claude, with switchboard's words, Claude Code's all the same",
			argv: []string{"claude", "run", "--config", "config.toml", "--", "-v"},
			want: []string{"run", "--", "run", "--config", "config.toml", "--", "-v"},
		},
		{name: "a name starting claude", argv: []string{"claude-switchboard", "--help"}, want: []string{"--help"}},
		{name: "a directory named claude", argv: []string{"/opt/claude/switchboard", "--version"}, want: []string{"--version"}},
		{name: "no name at all", want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A nil slice would have Cobra parse os.Args itself.
			if got := cli.Args(tt.argv); got == nil || !slices.Equal(got, tt.want) {
				t.Errorf("Args(%q) = %#v, want %q", tt.argv, got, tt.want)
			}
		})
	}
}

func TestRunByTheNameClaude(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)
	tests := []struct {
		name string
		// args are claude's, after its name.
		args []string
	}{
		{name: "alone"},
		{name: "Claude Code's help", args: []string{"--help"}},
		{name: "Claude Code's version", args: []string{"-v"}},
		{name: "switchboard's flags, Claude Code's own", args: []string{"--config", "other.toml", "--account", "side", "--direct", "--version", "-h"}},
		{name: "switchboard's commands, Claude Code's words", args: []string{"status", "--json"}},
		{name: "after --", args: []string{"--", "--help"}},
		{name: "a prompt", args: []string{"-p", "a prompt", "--output-format", "json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := srv.deps
			handed := recordHandOffs(t, &deps)

			if got := run(t, deps, cli.Args(append([]string{"/Users/tester/bin/claude"}, tt.args...))...); got != (result{}) {
				t.Errorf("claude %s = %+v, want exit status 0 and no output", strings.Join(tt.args, " "), got)
			}
			got := handed.only(t)
			if want := append([]string{"claude"}, tt.args...); got.path != handed.claude || !slices.Equal(got.argv, want) {
				t.Errorf("handed over to %s as %q, want %s as %q", got.path, got.argv, handed.claude, want)
			}
			want := map[string]string{"ANTHROPIC_BASE_URL": "http://" + srv.listen, "CLAUDE_CODE_OAUTH_TOKEN": "test-token-work"}
			if env := got.only("ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_CUSTOM_HEADERS"); !maps.Equal(env, want) {
				t.Errorf("handed over with\n%q\nwant\n%q, routed and pinned to no account", env, want)
			}
		})
	}
}

func TestTheClaudeLinkNeverStartsSwitchboardAgain(t *testing.T) {
	// switchboard, linked as Homebrew links it, and its claude link, which
	// leads there through that link, ahead of Claude Code on PATH.
	dir := t.TempDir()
	brewLink := claudetest.Link(t, claudetest.Program(t, filepath.Join(dir, "Cellar", "switchboard")), filepath.Join(dir, "bin", "switchboard"))
	claudeLink := claudetest.Link(t, brewLink, filepath.Join(dir, "links", "claude"))
	claudeCode := claudetest.Program(t, filepath.Join(dir, "claude-code", "claude"))
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"PATH": filepath.Dir(claudeLink) + string(filepath.ListSeparator) + filepath.Dir(claudeCode)})
	srv.start(t)
	tests := []struct {
		name string
		// argv is how switchboard is run, and executable where
		// os.Executable says it is, as it gives the path it was run by.
		argv       []string
		executable string
	}{
		{name: "run by the claude link", argv: []string{"claude", "--resume"}, executable: claudeLink},
		{name: "run by the claude link, for a local subcommand", argv: []string{"claude", "doctor"}, executable: claudeLink},
		{name: "run by Homebrew's link", argv: []string{brewLink, "run", "--", "--resume"}, executable: brewLink},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := srv.deps
			handed := recordHandOffs(t, &deps)
			deps.Executable = func() (string, error) { return tt.executable, nil }

			if got := run(t, deps, cli.Args(tt.argv)...); got.code != 0 {
				t.Errorf("%s = %+v, want exit status 0", strings.Join(tt.argv, " "), got)
			}
			if got := handed.only(t).path; got != claudeCode {
				t.Errorf("handed over to %s, want %s, Claude Code past switchboard's claude link", got, claudeCode)
			}
		})
	}
}

func TestRunNeverLogsClaudesArguments(t *testing.T) {
	const prompt = "a prompt the log must never hold"
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"SWITCHBOARD_LOG_LEVEL": "debug"})
	srv.start(t)
	handed := recordHandOffs(t, &srv.deps)

	if got := run(t, srv.deps, "run", "--account", "side", "--", "--print", prompt); got.code != 0 {
		t.Fatalf("switchboard run = %+v, want exit status 0", got)
	}
	log := srv.cliLog(t)
	if !hasLine(log, "level=INFO", `msg="starting claude" component=launch`, "mode=routed", "account=work", `chosen="the primary"`, "pin=side", "claude="+handed.claude) {
		t.Errorf("cli.log reads\n%s\nwant the launch logged", log)
	}
	for _, secret := range []string{prompt, "test-token-work", "test-token-side"} {
		if strings.Contains(log, secret) {
			t.Errorf("cli.log reads\n%s\nwant it without %q", log, secret)
		}
	}
}

// handOff is a hand-over run made: the program, and the arguments and
// environment it was to start with, but for switchboard's mark of the claude
// it starts, which is mark.
type handOff struct {
	path string
	argv []string
	env  map[string]string
	mark string
}

// only returns the variables named that the program was to start with.
func (h handOff) only(names ...string) map[string]string {
	env := maps.Clone(h.env)
	maps.DeleteFunc(env, func(name, _ string) bool { return !slices.Contains(names, name) })
	return env
}

// markEnv is the variable switchboard marks the claude it starts with.
const markEnv = "SWITCHBOARD_STARTED"

// handOffs are the hand-overs run makes, and claude, the stand-in it finds.
type handOffs struct {
	claude string
	made   []handOff
}

// only returns the one hand-over made.
func (h *handOffs) only(t *testing.T) handOff {
	t.Helper()
	if len(h.made) != 1 {
		t.Fatalf("handed over %d times, want once", len(h.made))
	}
	return h.made[0]
}

// recordHandOffs puts a stand-in for claude where its installer puts it, in
// deps' home directory, and gives deps a stand-in for this switchboard
// binary, and has deps note each hand-over to claude in place of making it.
func recordHandOffs(t *testing.T, deps *cli.Deps) *handOffs {
	t.Helper()
	home, err := deps.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	h := &handOffs{claude: claudetest.Program(t, filepath.Join(home, ".local", "bin", "claude"))}
	switchboard := claudetest.Program(t, filepath.Join(t.TempDir(), "switchboard"))
	deps.Executable = func() (string, error) { return switchboard, nil }
	deps.Exec = func(path string, argv, env []string) error {
		vars := environMap(env)
		mark := vars[markEnv]
		delete(vars, markEnv)
		h.made = append(h.made, handOff{path: path, argv: argv, env: vars, mark: mark})
		return nil
	}
	return h
}

// environMap is an environment, as os.Environ gives it, by name.
func environMap(environ []string) map[string]string {
	vars := make(map[string]string)
	for _, variable := range environ {
		name, value, _ := strings.Cut(variable, "=")
		vars[name] = value
	}
	return vars
}
