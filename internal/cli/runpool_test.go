package cli_test

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/router"
)

// pooledAccounts are threeAccounts in two pools: work, the primary, and
// personal in Default, unfenced; and side in Acme, fenced to ~/Code/acme.
const pooledAccounts = `
[[account]]
id    = "work"
label = "Work"
pool  = "b2n8"

[[account]]
id    = "personal"
label = "Personal"
pool  = "b2n8"

[[account]]
id    = "side"
label = "Side"
pool  = "q7mt"

[[pool]]
id    = "b2n8"
label = "Default"

[[pool]]
id    = "q7mt"
label = "Acme"
dirs  = ["~/Code/acme"]
`

// startingIn has commands run with deps start in the directory at rel in
// their home directory.
func startingIn(t *testing.T, deps *cli.Deps, rel string) {
	t.Helper()
	home, err := deps.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	deps.Getwd = func() (string, error) { return filepath.Join(home, rel), nil }
}

// pooledSetup is serve set up with pooledAccounts.
func pooledSetup(t *testing.T, env map[string]string) *serveSetup {
	t.Helper()
	srv := newServeSetup(t, fakeClaudeAPI(t), env)
	srv.accounts = pooledAccounts
	srv.writeConfig(t)
	return srv
}

func TestRunStartsASessionInItsDirectorysPool(t *testing.T) {
	srv := pooledSetup(t, map[string]string{"ANTHROPIC_CUSTOM_HEADERS": "X-Trace: on"})
	srv.start(t)
	tests := []struct {
		name        string
		dir         string
		wantToken   string
		wantHeaders string
		wantPool    string
	}{
		{
			name:        "inside a fence",
			dir:         "Code/acme/api",
			wantToken:   "test-token-side",
			wantHeaders: "X-Trace: on\nX-Switchboard-Dir: ~/Code/acme/api\nX-Switchboard-Pool: q7mt",
			wantPool:    "q7mt",
		},
		{
			name:        "outside every fence",
			dir:         "Code/project",
			wantToken:   "test-token-work",
			wantHeaders: "X-Trace: on\nX-Switchboard-Dir: ~/Code/project\nX-Switchboard-Pool: b2n8",
			wantPool:    "b2n8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := srv.deps
			startingIn(t, &deps, tt.dir)
			handed := recordHandOffs(t, &deps)

			if got := run(t, deps, "run", "--", "--resume"); got != (result{}) {
				t.Errorf("switchboard run = %+v, want exit status 0 and no output", got)
			}
			want := map[string]string{
				"ANTHROPIC_BASE_URL":       "http://" + srv.listen,
				"CLAUDE_CODE_OAUTH_TOKEN":  tt.wantToken,
				"ANTHROPIC_CUSTOM_HEADERS": tt.wantHeaders,
				"SWITCHBOARD_POOL":         tt.wantPool,
			}
			if env := handed.only(t).only(slices.Collect(maps.Keys(want))...); !maps.Equal(env, want) {
				t.Errorf("handed over with\n%q\nwant\n%q", env, want)
			}
			if log := srv.cliLog(t); !hasLine(log, `msg="starting claude" component=launch`, "mode=routed", "pool="+tt.wantPool) {
				t.Errorf("cli.log reads\n%s\nwant the launch logged with its pool", log)
			}
		})
	}
}

func TestRunRefusesWhatAPoolCantServe(t *testing.T) {
	everyFenced := strings.Replace(pooledAccounts, "label = \"Default\"\n", "label = \"Default\"\ndirs  = [\"~/Code/default\"]\n", 1)
	tests := []struct {
		name     string
		accounts string
		dir      string
		args     []string
		want     result
	}{
		{
			name:     "an account of another pool",
			accounts: pooledAccounts,
			dir:      "Code/project",
			args:     []string{"run", "--account", "side"},
			want:     result{stderr: "Error: side is in pool Acme; this directory is in pool Default\n", code: 1},
		},
		{
			name:     "outside every fence, every pool fenced",
			accounts: everyFenced,
			dir:      "Code/project",
			args:     []string{"run"},
			want:     result{stderr: "switchboard: no pool covers ~/Code/project — not starting claude\n", code: 1},
		},
		{
			name:     "on its own login, inside a fence",
			accounts: pooledAccounts,
			dir:      "Code/acme/api",
			args:     []string{"run", "--direct"},
			want:     result{stderr: "switchboard: ~/Code/acme/api is in pool Acme, where claude never starts on its own login — not starting claude\n", code: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), nil)
			srv.accounts = tt.accounts
			srv.writeConfig(t)
			startingIn(t, &srv.deps, tt.dir)
			handed := recordHandOffs(t, &srv.deps)

			if got := run(t, srv.deps, tt.args...); got != tt.want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(tt.args, " "), got, tt.want)
			}
			if len(handed.made) != 0 {
				t.Errorf("handed over %d times, want none", len(handed.made))
			}
		})
	}
}

func TestRunOnItsOwnLoginStartsOutsideEveryFence(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		args []string
	}{
		{name: "outside every fence", dir: "Code/project", args: []string{"--resume"}},
		{name: "a local subcommand, inside a fence", dir: "Code/acme/api", args: []string{"auth", "status"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := pooledSetup(t, nil)
			startingIn(t, &srv.deps, tt.dir)
			handed := recordHandOffs(t, &srv.deps)

			if got := run(t, srv.deps, append([]string{"run", "--direct", "--"}, tt.args...)...); got != (result{}) {
				t.Errorf("switchboard run --direct = %+v, want exit status 0 and no output", got)
			}
			if got := handed.only(t).argv; !slices.Equal(got, append([]string{"claude"}, tt.args...)) {
				t.Errorf("handed over as %q, want claude %q", got, tt.args)
			}
		})
	}
}

// serveHealth answers GET /health with h on the control socket of the
// router commands run with deps ask.
func serveHealth(t *testing.T, deps cli.Deps, h router.Health) {
	t.Helper()
	serveRouter(t, router.SocketPath(stateDir(t, deps)), new(atomic.Int64), map[string]http.HandlerFunc{
		"GET /health": func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(h) },
	})
}

// pooledHealth is the health answer of a router routing by pooledAccounts'
// pools, for commands run with deps, which listens at 127.0.0.1:4747.
func pooledHealth(t *testing.T, deps cli.Deps) router.Health {
	t.Helper()
	home, err := deps.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return router.Health{
		OK: true, Listen: "127.0.0.1:4747", StripsOwnHeaders: true, ReportsPools: true,
		Pools: []router.RoutedPool{
			{ID: "b2n8", Label: "Default", Accounts: []string{"work", "personal"}, Primary: "work"},
			{ID: "q7mt", Label: "Acme", Dirs: []string{filepath.Join(home, "Code", "acme")}, WrittenDirs: []string{"~/Code/acme"}, Accounts: []string{"side"}, Primary: "side"},
		},
	}
}

func TestRunThroughARouterThatDoesntKnowThePools(t *testing.T) {
	tests := []struct {
		name string
		// down is SWITCHBOARD_WHEN_ROUTER_DOWN, "" where it's unset.
		down      string
		want      result
		wantToken string
	}{
		{
			name: "stopping",
			want: result{
				stderr: "switchboard: the router doesn't route by the config's pools (it's from before pools) — not starting claude: " +
					"restart it with switchboard service restart, or go direct this once with SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude\n",
				code: 1,
			},
		},
		{
			name:      "going direct, on the pool's account",
			down:      "direct",
			want:      result{stderr: "switchboard: the router doesn't route by the config's pools (it's from before pools) — connecting directly on side · Side\n"},
			wantToken: "test-token-side",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{}
			if tt.down != "" {
				env["SWITCHBOARD_WHEN_ROUTER_DOWN"] = tt.down
			}
			srv := pooledSetup(t, env)
			// A router from before pools, waited for a moment to take them up.
			serveHealth(t, srv.deps, router.Health{OK: true, Listen: "127.0.0.1:4747", StripsOwnHeaders: true})
			srv.deps.Patience.TakeUp = 10 * time.Millisecond
			startingIn(t, &srv.deps, "Code/acme/api")
			handed := recordHandOffs(t, &srv.deps)

			if got := run(t, srv.deps, "run"); got != tt.want {
				t.Errorf("switchboard run = %+v, want %+v", got, tt.want)
			}
			if tt.wantToken == "" {
				if len(handed.made) != 0 {
					t.Errorf("handed over %d times, want none", len(handed.made))
				}
				return
			}
			env = handed.only(t).only("ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN", "SWITCHBOARD_POOL")
			if want := map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": tt.wantToken, "SWITCHBOARD_POOL": "q7mt"}; !maps.Equal(env, want) {
				t.Errorf("handed over with\n%q\nwant\n%q", env, want)
			}
		})
	}
}

func TestRunWithoutAConfigItCanReadRoutesInThePoolTheRouterSays(t *testing.T) {
	srv := pooledSetup(t, nil)
	srv.start(t)
	invalid := writeConfig(t, invalidConfig)
	startingIn(t, &srv.deps, "Code/acme/api")
	handed := recordHandOffs(t, &srv.deps)

	want := result{stderr: "switchboard: couldn't read the config (invalid config " + invalid + ") — starting claude through the running router\n"}
	if got := run(t, srv.deps, "--config", invalid, "run"); got != want {
		t.Errorf("switchboard run = %+v, want %+v", got, want)
	}
	env := handed.only(t).only("CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_CUSTOM_HEADERS", "SWITCHBOARD_POOL")
	wantEnv := map[string]string{
		"CLAUDE_CODE_OAUTH_TOKEN":  "test-token-side",
		"ANTHROPIC_CUSTOM_HEADERS": "X-Switchboard-Dir: ~/Code/acme/api\nX-Switchboard-Pool: q7mt",
		"SWITCHBOARD_POOL":         "q7mt",
	}
	if !maps.Equal(env, wantEnv) {
		t.Errorf("handed over with\n%q\nwant\n%q", env, wantEnv)
	}
}

// invalidPooled is pooledAccounts in a config otherwise invalid, for a key
// from before the token files; and invalidPools the same, its pools breaking
// the config's rules for them, Acme's two directories overlapping.
var (
	invalidPooled = strings.Replace(pooledAccounts, "label = \"Side\"\n", "label = \"Side\"\ntoken_env = \"CLAUDE_TOKEN_SIDE\"\n", 1)
	invalidPools  = strings.Replace(invalidPooled, `dirs  = ["~/Code/acme"]`, `dirs  = ["~/Code/acme", "~/Code/acme/api"]`, 1)
)

func TestRunWithoutAConfigItCanReadNorTheRouterReadsTheConfigsPools(t *testing.T) {
	tests := []struct {
		name   string
		config string
		dir    string
		// wantSaid is what stderr hears after the config's error, and
		// wantStarted whether claude starts.
		wantSaid    string
		wantStarted bool
	}{
		{
			name:     "inside a fence",
			config:   invalidPooled,
			dir:      "Code/acme/api",
			wantSaid: "~/Code/acme/api is in pool Acme, where claude never starts on its own login — not starting claude\n",
		},
		{
			name:        "outside every fence",
			config:      invalidPooled,
			dir:         "Code/project",
			wantSaid:    "starting claude without it\n",
			wantStarted: true,
		},
		{
			name:        "inside a fence, the pools unreadable even so",
			config:      "[[pool]\n",
			dir:         "Code/acme/api",
			wantSaid:    "starting claude without it\n",
			wantStarted: true,
		},
		{
			name:     "inside a fence of pools that break the config's rules",
			config:   invalidPools,
			dir:      "Code/acme/api",
			wantSaid: "~/Code/acme/api is in pool Acme, where claude never starts on its own login — not starting claude\n",
		},
		{
			name:        "outside every fence of pools that break the config's rules",
			config:      invalidPools,
			dir:         "Code/project",
			wantSaid:    "starting claude without it\n",
			wantStarted: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"SWITCHBOARD_CONFIG": path})
			startingIn(t, &srv.deps, tt.dir)
			handed := recordHandOffs(t, &srv.deps)

			got := run(t, srv.deps, "run")
			if !strings.HasPrefix(got.stderr, "switchboard: couldn't read the config (") || !strings.HasSuffix(got.stderr, ") — "+tt.wantSaid) {
				t.Errorf("switchboard run said %q, want it to say it couldn't read the config, then %q", got.stderr, tt.wantSaid)
			}
			if started := len(handed.made) == 1; started != tt.wantStarted || (got.code == 0) != tt.wantStarted {
				t.Errorf("switchboard run = %+v, handing over %d times; want claude started: %v", got, len(handed.made), tt.wantStarted)
			}
		})
	}
}

func TestRunOnItsOwnLoginWithoutAConfigItCanReadNeverStartsInsideAFence(t *testing.T) {
	const inside = "switchboard: ~/Code/acme/api is in pool Acme, where claude never starts on its own login — not starting claude\n"
	tests := []struct {
		name   string
		config string
		// answering has the router answer with pooledAccounts' pools.
		answering bool
	}{
		{name: "the router answering: its fences, the config file's unreadable", config: "[[pool]\n", answering: true},
		{name: "the router down: the config file's fences", config: invalidPooled},
		{name: "the router down: the fences of pools that break the config's rules", config: invalidPools},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"SWITCHBOARD_CONFIG": writeConfig(t, tt.config)})
			if tt.answering {
				serveHealth(t, srv.deps, pooledHealth(t, srv.deps))
			}
			startingIn(t, &srv.deps, "Code/acme/api")
			handed := recordHandOffs(t, &srv.deps)

			if got, want := run(t, srv.deps, "run", "--direct"), (result{stderr: inside, code: 1}); got != want {
				t.Errorf("switchboard run --direct = %+v, want %+v", got, want)
			}
			if len(handed.made) != 0 {
				t.Errorf("handed over %d times, want none", len(handed.made))
			}
		})
	}
}

func TestRunWithoutFindingTheRouterNeverStartsOnItsOwnLoginInsideAFence(t *testing.T) {
	acme := filepath.Join(t.TempDir(), "acme")
	pooled := strings.Replace(pooledAccounts, `dirs  = ["~/Code/acme"]`, `dirs  = ["`+acme+`"]`, 1)
	env := map[string]string{"SWITCHBOARD_CONFIG": writeConfig(t, pooled), "SWITCHBOARD_WHEN_ROUTER_DOWN": "direct"}
	deps := testDeps(env, t.TempDir())
	handed := recordHandOffs(t, &deps)
	// Without a home directory, the state directory the router's socket and
	// the token files are in can't be located.
	deps.HomeDir = func() (string, error) { return "", errors.New("no home directory") }
	deps.Getwd = func() (string, error) { return filepath.Join(acme, "api"), nil }
	env["PATH"] = filepath.Dir(handed.claude)

	want := result{
		stderr: "switchboard: couldn't find the router (locate state directory: no home directory) — " +
			"pool Acme has no account with a usable token — not starting claude\n",
		code: 1,
	}
	if got := run(t, deps, "run"); got != want {
		t.Errorf("switchboard run = %+v, want %+v", got, want)
	}
	if len(handed.made) != 0 {
		t.Errorf("handed over %d times, want none", len(handed.made))
	}
}

// markPoolsMoved marks the move to pools done in the state directory dir.
func markPoolsMoved(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "pools.moved"), []byte("2026-09-28T13:12:00Z\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunAfterTheMoveToPoolsTakesThePoolsOfAConfigNamingNone(t *testing.T) {
	t.Run("from the router's health answer", func(t *testing.T) {
		srv := newServeSetup(t, fakeClaudeAPI(t), nil)
		markPoolsMoved(t, srv.state)
		serveHealth(t, srv.deps, pooledHealth(t, srv.deps))
		startingIn(t, &srv.deps, "Code/acme/api")
		handed := recordHandOffs(t, &srv.deps)

		if got := run(t, srv.deps, "run"); got != (result{}) {
			t.Errorf("switchboard run = %+v, want exit status 0 and no output", got)
		}
		env := handed.only(t).only("CLAUDE_CODE_OAUTH_TOKEN", "SWITCHBOARD_POOL")
		if want := map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "test-token-side", "SWITCHBOARD_POOL": "q7mt"}; !maps.Equal(env, want) {
			t.Errorf("handed over with\n%q\nwant\n%q", env, want)
		}
	})
	t.Run("from nowhere: refused", func(t *testing.T) {
		srv := newServeSetup(t, fakeClaudeAPI(t), nil)
		markPoolsMoved(t, srv.state)
		srv.start(t)
		handed := recordHandOffs(t, &srv.deps)

		want := result{stderr: "switchboard: the config names no pools, and switchboard can't tell which pool ~/Code/project is in — not starting claude\n", code: 1}
		if got := run(t, srv.deps, "run"); got != want {
			t.Errorf("switchboard run = %+v, want %+v", got, want)
		}
		if len(handed.made) != 0 {
			t.Errorf("handed over %d times, want none", len(handed.made))
		}
	})
	t.Run("on its own login, the router's fences", func(t *testing.T) {
		srv := newServeSetup(t, fakeClaudeAPI(t), nil)
		markPoolsMoved(t, srv.state)
		serveHealth(t, srv.deps, pooledHealth(t, srv.deps))
		startingIn(t, &srv.deps, "Code/acme/api")
		handed := recordHandOffs(t, &srv.deps)

		want := result{stderr: "switchboard: ~/Code/acme/api is in pool Acme, where claude never starts on its own login — not starting claude\n", code: 1}
		if got := run(t, srv.deps, "run", "--direct"); got != want {
			t.Errorf("switchboard run --direct = %+v, want %+v", got, want)
		}
		if len(handed.made) != 0 {
			t.Errorf("handed over %d times, want none", len(handed.made))
		}
	})
}
