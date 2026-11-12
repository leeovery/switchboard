package launch_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

const (
	acme1Token = "test-token-acme1"
	acme2Token = "test-token-acme2"
	// acmeDir is the directory Acme is fenced to, and elsewhere one in no
	// fence.
	acmeDir   = homeDir + "/Code/acme"
	elsewhere = homeDir + "/Code/other"
)

// The pools: Default, unfenced; Acme, fenced to acmeDir; and Client, fenced
// to two directories, neither of which a test starts in.
var (
	defaultPool = config.Pool{ID: "b2n8", Label: "Default"}
	acmePool    = config.Pool{ID: "q7mt", Label: "Acme", Dirs: []string{acmeDir}, WrittenDirs: []string{"~/Code/acme"}}
	clientPool  = config.Pool{ID: "h4w2", Label: "Client", Dirs: []string{homeDir + "/Code/client", "/Volumes/Client"}}
)

// poolTokens are the token files there are: personal has none.
var poolTokens = tokenstest.Files{"work": workToken, "side": sideToken, "acme1": acme1Token, "acme2": acme2Token}

// pooledConfig is a config of Default, whose accounts are work, its
// primary, personal and side, and Acme, whose accounts are acme1, its
// primary, and acme2, each pool's primary marked, as Load marks them. With
// the router down, it goes direct.
func pooledConfig() *config.Config {
	return &config.Config{
		Listen:         "127.0.0.1:4747",
		WhenRouterDown: config.Direct,
		Accounts: config.Accounts{
			{ID: "work", Label: "Work", Pool: "b2n8", Primary: true},
			{ID: "personal", Label: "Personal", Pool: "b2n8"},
			{ID: "side", Label: "Side", Pool: "b2n8"},
			{ID: "acme1", Label: "Acme 1", Pool: "q7mt", Primary: true},
			{ID: "acme2", Label: "Acme 2", Pool: "q7mt"},
		},
		Pools: config.Pools{defaultPool, acmePool},
	}
}

// pooledRoute is a launch on cfg through r, pinned to account unless it's
// "", as route is.
func pooledRoute(cfg *config.Config, r launch.Router, account string) launch.Route {
	return launch.Route{
		Config:   cfg,
		Token:    poolTokens.Read,
		Pause:    func(time.Duration) {},
		Router:   r,
		Patience: launch.Patience{Answer: launch.AskTimeout},
		Account:  account,
	}
}

// knowing answers as a healthy router routing by cfg's pools does: each with
// its accounts and primary.
func knowing(cfg *config.Config) *fakeRouter {
	r := healthy()
	r.health.ReportsPools = true
	r.health.Pools = routedPools(cfg)
	return r
}

// routedPools are cfg's pools as a router routing by them reports them.
func routedPools(cfg *config.Config) []router.RoutedPool {
	var pools []router.RoutedPool
	for _, p := range cfg.Pools {
		routed := router.RoutedPool{ID: p.ID, Label: p.Label, Dirs: p.Dirs, WrittenDirs: p.WrittenDirs, Accounts: []string{}}
		for _, a := range cfg.Accounts.InPool(p.ID) {
			routed.Accounts = append(routed.Accounts, a.ID)
		}
		if primary, ok := cfg.Accounts.PrimaryOf(p.ID); ok {
			routed.Primary = primary.ID
		}
		pools = append(pools, routed)
	}
	return pools
}

// headersOf returns Claude Code's custom headers, a line each.
func headersOf(env map[string]string) []string {
	return strings.Split(env["ANTHROPIC_CUSTOM_HEADERS"], "\n")
}

func TestRunStartsASessionInThePoolItsDirectoryIsIn(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		// pools are those configured, Default and Acme where it's nil.
		pools     config.Pools
		wantPool  string
		wantToken string
		wantSaid  string
	}{
		{name: "a fenced pool's directory", dir: acmeDir, wantPool: "q7mt", wantToken: acme1Token},
		{name: "inside it", dir: acmeDir + "/api/src", wantPool: "q7mt", wantToken: acme1Token},
		{name: "inside it, ignoring case", dir: homeDir + "/code/ACME/api", wantPool: "q7mt", wantToken: acme1Token},
		{name: "a Claude Code worktree of a project inside it", dir: acmeDir + "/api/.claude/worktrees/agent-1", wantPool: "q7mt", wantToken: acme1Token},
		{name: "a worktree made outside it, beside it", dir: acmeDir + "-api-worktree", wantPool: "b2n8", wantToken: workToken},
		{name: "outside every fence: the unfenced pool", dir: elsewhere, wantPool: "b2n8", wantToken: workToken},
		{name: "around a fence: the unfenced pool", dir: homeDir + "/Code", wantPool: "b2n8", wantToken: workToken},
		{name: "a directory not known, no pool fenced: the unfenced pool", pools: config.Pools{defaultPool}, wantPool: "b2n8", wantToken: workToken},
		{name: "in the second fence of a pool of two", dir: "/Volumes/Client/site", pools: config.Pools{defaultPool, acmePool, clientPool}, wantPool: "h4w2", wantToken: sideToken},
		{
			name:     "outside every fence, every pool fenced",
			dir:      elsewhere,
			pools:    config.Pools{acmePool, clientPool},
			wantSaid: "switchboard: no pool covers ~/Code/other — not starting claude\n",
		},
		{
			name:     "a directory not known, a pool fenced",
			wantSaid: "switchboard: can't tell which pool the directory claude starts in is in — not starting claude\n",
		},
		{
			name:     "a directory not known, every pool fenced",
			pools:    config.Pools{acmePool, clientPool},
			wantSaid: "switchboard: can't tell which pool the directory claude starts in is in — not starting claude\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, "HOME=/home/tester")
			h.launcher.Dir = tt.dir
			cfg := pooledConfig()
			if tt.pools != nil {
				cfg.Pools = tt.pools
				// side is Client's.
				cfg.Accounts[2].Pool = "h4w2"
			}
			rt := knowing(cfg)

			err := h.launcher.Run(t.Context(), pooledRoute(cfg, rt, ""), nil)
			if tt.wantSaid != "" {
				if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
					t.Errorf("Run() error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
				}
				if said := h.stderr.String(); said != tt.wantSaid {
					t.Errorf("said %q on stderr, want %q", said, tt.wantSaid)
				}
				if rt.asks > 0 {
					t.Errorf("asked the router how it is %d times, want it never asked, the pools refusing", rt.asks)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			env := h.environment(t)
			if env["CLAUDE_CODE_OAUTH_TOKEN"] != tt.wantToken || env["SWITCHBOARD_POOL"] != tt.wantPool {
				t.Errorf("started on %q in pool %q, want %q in pool %q", env["CLAUDE_CODE_OAUTH_TOKEN"], env["SWITCHBOARD_POOL"], tt.wantToken, tt.wantPool)
			}
			if !slices.Contains(headersOf(env), "X-Switchboard-Pool: "+tt.wantPool) {
				t.Errorf("started with the headers %q, want the router told pool %s", env["ANTHROPIC_CUSTOM_HEADERS"], tt.wantPool)
			}
		})
	}
}

// linkedTree makes, under a directory of the test's own, which it returns,
// links resolved, the real directories real/acme/api, real/other,
// real/client/site and real/acme/api/shared, and the links the pools of
// linkedConfig are reached through.
func linkedTree(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"real/acme/api/shared", "real/other", "real/client/site"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		// Acme's directory, as the config gives it, a link to where its work
		// is.
		"acme": "real/acme",
		// A link elsewhere to a project inside the fence.
		"shortcut": "real/acme/api",
		// A link inside the fence to a folder outside it.
		"real/acme/api/outside": "../../other",
		// A link inside Acme's fence into Client's.
		"real/acme/api/clientward": "../../client/site",
		// One of Client's directories, as the config gives it, a link into
		// Acme's: apart as written, overlapping once resolved.
		"shared": "real/acme/api/shared",
	}
	for link, target := range links {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// linkedConfig is pooledConfig with Client too, side its account, the pools
// fenced to the links of linkedTree, made under root: Acme to acme, and
// Client to real/client and shared.
func linkedConfig(root string) *config.Config {
	cfg := pooledConfig()
	client := config.Pool{ID: clientPool.ID, Label: clientPool.Label, Dirs: []string{filepath.Join(root, "real", "client"), filepath.Join(root, "shared")}}
	cfg.Pools = append(cfg.Pools, client)
	cfg.Pools[1].Dirs = []string{filepath.Join(root, "acme")}
	cfg.Accounts[2].Pool = client.ID
	return cfg
}

func TestRunFindsThePoolWhereALinkLeads(t *testing.T) {
	root := linkedTree(t)
	inBoth := func(dir string) string {
		return "switchboard: " + dir + " is in pools Acme and Client, as its links lead — not starting claude\n"
	}
	tests := []struct {
		name string
		dir  string
		// wantPool is the pool the session starts in, "" where it's refused,
		// stderr hearing wantSaid.
		wantPool string
		wantSaid string
	}{
		{name: "through the pool's directory, a link", dir: filepath.Join(root, "acme", "api"), wantPool: "q7mt"},
		{name: "where the pool's directory leads", dir: filepath.Join(root, "real", "acme", "api"), wantPool: "q7mt"},
		{name: "through a link elsewhere into the fence", dir: filepath.Join(root, "shortcut"), wantPool: "q7mt"},
		{name: "through a link inside the fence to outside it: still inside", dir: filepath.Join(root, "acme", "api", "outside"), wantPool: "q7mt"},
		{name: "outside the fence", dir: filepath.Join(root, "real", "other"), wantPool: "b2n8"},
		{name: "through a link inside one fence into another", dir: filepath.Join(root, "acme", "api", "clientward"), wantSaid: inBoth(filepath.Join(root, "acme", "api", "clientward"))},
		{name: "where two pools' directories meet, their links resolved", dir: filepath.Join(root, "real", "acme", "api", "shared"), wantSaid: inBoth(filepath.Join(root, "real", "acme", "api", "shared"))},
		{name: "through the link of one pool's directory into another's", dir: filepath.Join(root, "shared"), wantSaid: inBoth(filepath.Join(root, "shared"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.launcher.EvalSymlinks = filepath.EvalSymlinks
			h.launcher.Dir = tt.dir
			cfg := linkedConfig(root)

			err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil)
			if said := h.stderr.String(); said != tt.wantSaid {
				t.Errorf("said %q on stderr, want %q", said, tt.wantSaid)
			}
			if tt.wantPool == "" {
				if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
					t.Errorf("Run() error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := h.environment(t)["SWITCHBOARD_POOL"]; got != tt.wantPool {
				t.Errorf("started in pool %q, want %q", got, tt.wantPool)
			}
		})
	}
}

func TestOnItsOwnLoginClaudeNeverStartsWhereALinkLeadsIntoAFence(t *testing.T) {
	root := linkedTree(t)
	tests := []struct {
		name string
		dir  string
		// wantIn is the pools stderr hears the directory is in, "" where
		// claude starts.
		wantIn string
	}{
		{name: "through a link elsewhere into the fence", dir: filepath.Join(root, "shortcut"), wantIn: "pool Acme"},
		{name: "through a link inside the fence to outside it", dir: filepath.Join(root, "acme", "api", "outside"), wantIn: "pool Acme"},
		{name: "where two pools' directories meet", dir: filepath.Join(root, "real", "acme", "api", "shared"), wantIn: "pools Acme and Client"},
		{name: "outside every fence", dir: filepath.Join(root, "real", "other")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.launcher.EvalSymlinks = filepath.EvalSymlinks
			h.launcher.Dir = tt.dir

			err := h.launcher.Direct(nil, linkedConfig(root).Pools)
			if tt.wantIn == "" {
				if err != nil || len(h.starts) != 1 {
					t.Errorf("Direct() error = %v, starting %q; want claude started", err, h.starts)
				}
				return
			}
			if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
				t.Errorf("Direct() error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
			}
			want := "switchboard: " + tt.dir + " is in " + tt.wantIn + ", where claude never starts on its own login — not starting claude\n"
			if said := h.stderr.String(); said != want {
				t.Errorf("said %q on stderr, want %q", said, want)
			}
		})
	}
}

func TestRunStartsOnAnAccountOfThePool(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		// down has the router not running, where it otherwise routes by the
		// config's pools.
		down bool
		// tokens are the token files there are, poolTokens where nil; and
		// personalPrimary marks personal, without a token, Default's
		// primary, in place of work.
		tokens          tokenstest.Files
		personalPrimary bool
		account         string
		wantToken       string
		wantSaid        string
	}{
		{name: "routed, the pool's primary's", dir: acmeDir, wantToken: acme1Token},
		{name: "routed, the other pool's primary's", dir: elsewhere, wantToken: workToken},
		{
			name:            "routed, the pool's first with a usable token, its primary without one",
			dir:             elsewhere,
			personalPrimary: true,
			wantToken:       workToken,
		},
		{
			name:      "routed, the pool's other account's, its primary without a token",
			dir:       acmeDir,
			tokens:    tokenstest.Files{"work": workToken, "acme2": acme2Token},
			wantToken: acme2Token,
		},
		{
			name:     "routed, none of the pool's accounts with a usable token, another pool's never",
			dir:      acmeDir,
			tokens:   tokenstest.Files{"work": workToken, "side": sideToken},
			wantSaid: "switchboard: pool Acme has no account with a usable token — not starting claude\n",
		},
		{
			name:      "direct, the pool's primary's",
			dir:       acmeDir,
			down:      true,
			wantToken: acme1Token,
			wantSaid:  "switchboard: the router isn't running — connecting directly on acme1 · Acme 1\n",
		},
		{
			name:      "direct and pinned, the pinned account's",
			dir:       acmeDir,
			down:      true,
			account:   "acme2",
			wantToken: acme2Token,
			wantSaid:  "switchboard: the router isn't running — connecting directly on acme2 · Acme 2\n",
		},
		{
			name:      "direct, the pool's first with a usable token, its primary without one",
			dir:       acmeDir,
			down:      true,
			tokens:    tokenstest.Files{"work": workToken, "acme2": acme2Token},
			wantToken: acme2Token,
			wantSaid:  "switchboard: the router isn't running — connecting directly on acme2 · Acme 2\n",
		},
		{
			name:     "direct, none of the pool's accounts with a usable token, another pool's never",
			dir:      acmeDir,
			down:     true,
			tokens:   tokenstest.Files{"work": workToken},
			wantSaid: "switchboard: pool Acme has no account with a usable token — not starting claude\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.launcher.Dir = tt.dir
			cfg := pooledConfig()
			if tt.personalPrimary {
				cfg.Accounts[0].Primary, cfg.Accounts[1].Primary = false, true
			}
			rt := knowing(cfg)
			if tt.down {
				rt = notRunning()
			}
			r := pooledRoute(cfg, rt, tt.account)
			if tt.tokens != nil {
				r.Token = tt.tokens.Read
			}

			err := h.launcher.Run(t.Context(), r, nil)
			if said := h.stderr.String(); said != tt.wantSaid {
				t.Errorf("said %q on stderr, want %q", said, tt.wantSaid)
			}
			if tt.wantToken == "" {
				if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
					t.Errorf("Run() error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
				}
				return
			}
			if got := h.environment(t)["CLAUDE_CODE_OAUTH_TOKEN"]; err != nil || got != tt.wantToken {
				t.Errorf("Run() error = %v, starting on %q; want %q", err, got, tt.wantToken)
			}
		})
	}
}

func TestRunWithOnePoolAndNoUsableTokenStartsAsBeforePoolsUnlessItsFenced(t *testing.T) {
	tests := []struct {
		name     string
		pool     config.Pool
		wantSaid string
	}{
		{
			name:     "unfenced: without switchboard, as before pools",
			pool:     defaultPool,
			wantSaid: "switchboard: no account has a usable token (work: " + tokenstest.Missing("work").Error() + ") — starting claude without it\n",
		},
		{
			name:     "fenced: refused",
			pool:     acmePool,
			wantSaid: "switchboard: pool Acme has no account with a usable token — not starting claude\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.launcher.Dir = acmeDir
			cfg := &config.Config{Accounts: config.Accounts{{ID: "work", Label: "Work", Pool: tt.pool.ID, Primary: true}}, Pools: config.Pools{tt.pool}}
			r := pooledRoute(cfg, knowing(cfg), "")
			r.Token = tokenstest.Files{}.Read

			err := h.launcher.Run(t.Context(), r, nil)
			if said := h.stderr.String(); said != tt.wantSaid {
				t.Errorf("said %q on stderr, want %q", said, tt.wantSaid)
			}
			switch {
			case tt.pool.Fenced() && (!errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0):
				t.Errorf("Run() error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
			case !tt.pool.Fenced() && (err != nil || len(h.starts) != 1):
				t.Errorf("Run() error = %v, starting %q; want claude started", err, h.starts)
			}
		})
	}
}

func TestRunRefusesAPoolWithNoAccounts(t *testing.T) {
	h := newHarness(t)
	h.launcher.Dir = "/Volumes/Client/site"
	cfg := pooledConfig()
	cfg.Pools = append(cfg.Pools, clientPool)

	err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil)
	if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
		t.Errorf("Run() error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
	}
	if said, want := h.stderr.String(), "switchboard: pool Client has no accounts — not starting claude\n"; said != want {
		t.Errorf("said %q on stderr, want %q", said, want)
	}
}

func TestRunNamesTheSessionsPool(t *testing.T) {
	// A session started in Default, within whose environment this one is.
	inherited := []string{
		"HOME=/home/tester",
		"SWITCHBOARD_POOL=b2n8",
		"ANTHROPIC_CUSTOM_HEADERS=X-Trace: on\nX-Switchboard-Pool: b2n8\nX-Switchboard-Dir: ~/Code/other",
	}
	oneUnfenced := &config.Config{Accounts: config.Accounts{{ID: "work", Label: "Work", Pool: "b2n8", Primary: true}}, Pools: config.Pools{defaultPool}}
	stripless := healthy()
	stripless.health.StripsOwnHeaders = false
	tests := []struct {
		name   string
		cfg    *config.Config
		router *fakeRouter
		// wantPool is SWITCHBOARD_POOL, "" where it's unset, and
		// wantHeaders Claude Code's custom headers.
		wantPool    string
		wantHeaders string
	}{
		{
			name:        "routed, the router told it",
			cfg:         pooledConfig(),
			router:      knowing(pooledConfig()),
			wantPool:    "q7mt",
			wantHeaders: "X-Trace: on\nX-Switchboard-Dir: ~/Code/acme\nX-Switchboard-Pool: q7mt",
		},
		{
			name:        "routed in one pool, unfenced, the router told it",
			cfg:         oneUnfenced,
			router:      healthy(),
			wantPool:    "b2n8",
			wantHeaders: "X-Trace: on\nX-Switchboard-Dir: ~/Code/acme\nX-Switchboard-Pool: b2n8",
		},
		{
			name:        "routed through a router that sends its own headers on, none told it",
			cfg:         oneUnfenced,
			router:      stripless,
			wantPool:    "b2n8",
			wantHeaders: "X-Trace: on",
		},
		{
			name:        "direct, none told the router",
			cfg:         pooledConfig(),
			router:      notRunning(),
			wantPool:    "q7mt",
			wantHeaders: "X-Trace: on",
		},
		{
			name:        "without pools, none named",
			cfg:         route(healthy(), "").Config,
			router:      healthy(),
			wantHeaders: "X-Trace: on\nX-Switchboard-Dir: ~/Code/acme",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, inherited...)
			h.launcher.Dir = acmeDir
			r := pooledRoute(tt.cfg, tt.router, "")
			r.Token = testTokens.Read
			if tt.cfg.Pools != nil {
				r.Token = poolTokens.Read
			}

			if err := h.launcher.Run(t.Context(), r, nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			env := h.environment(t)
			if pool, named := env["SWITCHBOARD_POOL"]; pool != tt.wantPool || named != (tt.wantPool != "") {
				t.Errorf("started with SWITCHBOARD_POOL %q, set: %v; want %q", pool, named, tt.wantPool)
			}
			if got := env["ANTHROPIC_CUSTOM_HEADERS"]; got != tt.wantHeaders {
				t.Errorf("started with the headers %q, want %q", got, tt.wantHeaders)
			}
		})
	}
}

func TestASessionsPoolGoesWhereSwitchboardStepsAside(t *testing.T) {
	h := newHarness(t, "HOME=/home/tester")
	h.launcher.Dir = acmeDir
	cfg := pooledConfig()
	if err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	session := append(h.only(t).session(), "ANTHROPIC_API_KEY=test-key")
	tests := []struct {
		name   string
		launch func(l launch.Launcher) error
	}{
		{name: "for a key", launch: func(l launch.Launcher) error { return l.StepAside(nil, "ANTHROPIC_API_KEY") }},
		{name: "on its own login", launch: func(l launch.Launcher) error { return l.Direct(nil, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			within := newHarness(t, session...)

			if err := tt.launch(within.launcher); err != nil {
				t.Fatalf("launch error = %v", err)
			}
			env := within.environment(t)
			if pool, named := env["SWITCHBOARD_POOL"]; named {
				t.Errorf("started with SWITCHBOARD_POOL %q, want it gone with what run set", pool)
			}
			if headers, ok := env["ANTHROPIC_CUSTOM_HEADERS"]; ok {
				t.Errorf("started with the headers %q, want the pool gone with what run set", headers)
			}
		})
	}
}

func TestUnaidedPassesASessionsRoutingOnInItsOwnPoolAlone(t *testing.T) {
	pools := config.Pools{defaultPool, acmePool}
	unreadable := func(_ context.Context, l launch.Launcher) error {
		return l.Unaided(nil, pools, launch.CouldntReadConfig, errUnread)
	}
	tests := []struct {
		name string
		// from is the directory the session claude starts within started in,
		// and dir the one it starts in.
		from string
		dir  string
		// launch starts it unaided, with pools.
		launch func(ctx context.Context, l launch.Launcher) error
		// wantRouted is whether it goes through the session's router, on the
		// session's token.
		wantRouted bool
	}{
		{name: "within a session of its own pool", from: elsewhere, dir: homeDir + "/Code/site", launch: unreadable, wantRouted: true},
		{name: "within a session of a fenced pool, outside its fence", from: acmeDir, dir: elsewhere, launch: unreadable},
		{
			name: "within a session of its own pool, the router found down",
			from: elsewhere,
			dir:  homeDir + "/Code/site",
			launch: func(ctx context.Context, l launch.Launcher) error {
				r := unreadPooledRoute(notRunning(), launch.Written{Pools: pools}, "")
				return l.RunUnread(ctx, r, nil, errUnread)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, "HOME=/home/tester")
			h.launcher.Dir = tt.from
			cfg := pooledConfig()
			if err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			within := newHarness(t, h.only(t).session()...)
			within.launcher.Dir = tt.dir

			if err := tt.launch(t.Context(), within.launcher); err != nil {
				t.Fatalf("launch error = %v", err)
			}
			env := within.environment(t)
			_, routed := env["ANTHROPIC_BASE_URL"]
			_, token := env["CLAUDE_CODE_OAUTH_TOKEN"]
			pool, named := env["SWITCHBOARD_POOL"]
			if routed != tt.wantRouted || token != tt.wantRouted || named != tt.wantRouted {
				t.Errorf("started routed: %v, with a token: %v, in pool %q; want each %v", routed, token, pool, tt.wantRouted)
			}
		})
	}
}

func TestRunRefusesAPinInAnotherPool(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		cfg     *config.Config
		account string
		wantErr string
	}{
		{
			name:    "an account of a fenced pool, from outside it",
			dir:     elsewhere,
			account: "acme1",
			wantErr: "Acme 1 (acme1) is in pool Acme; this directory is in pool Default",
		},
		{
			name:    "an account of the unfenced pool, labelled as its id, from inside a fence",
			dir:     acmeDir,
			account: "work",
			wantErr: "work is in pool Default; this directory is in pool Acme",
		},
		{
			name:    "an account not configured, naming the pool's own with a usable token",
			dir:     elsewhere,
			account: "nope",
			wantErr: `there's no account "nope": pin work or side`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.launcher.Dir = tt.dir
			cfg := pooledConfig()
			rt := knowing(cfg)

			if err := h.launcher.Run(t.Context(), pooledRoute(cfg, rt, tt.account), nil); err == nil || err.Error() != tt.wantErr {
				t.Errorf("Run() error = %v, want %q", err, tt.wantErr)
			}
			if len(h.starts) > 0 {
				t.Errorf("started %q, want nothing started", h.starts)
			}
			if rt.asks > 0 {
				t.Errorf("asked the router how it is %d times, want it never asked, the pin refused", rt.asks)
			}
		})
	}
}

func TestRunRefusesAPinToAnAccountTheRoutersPoolsDontHold(t *testing.T) {
	h := newHarness(t, "HOME=/home/tester")
	h.launcher.Dir = acmeDir
	// The config names no pools once the move is done, so the router's are
	// taken: acme2, added since it started, is in none of them yet.
	rt := otherPools(pooledConfig(), func(ps []router.RoutedPool) []router.RoutedPool {
		ps[1].Accounts = []string{"acme1"}
		return ps
	})
	r := pooledRoute(unpooledConfig(), rt, "acme2")
	r.Moved = true

	err := h.launcher.Run(t.Context(), r, nil)
	if want := "Acme 2 (acme2) is in no pool the router routes yet; this directory is in pool Acme"; err == nil || err.Error() != want {
		t.Errorf("Run() error = %v, want %q", err, want)
	}
	if len(h.starts) > 0 {
		t.Errorf("started %q, want nothing started", h.starts)
	}
}

func TestRunPinsAnAccountOfThePool(t *testing.T) {
	h := newHarness(t)
	h.launcher.Dir = acmeDir
	cfg := pooledConfig()

	if err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), "acme2"), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	env := h.environment(t)
	if env["CLAUDE_CODE_OAUTH_TOKEN"] != acme1Token || !slices.Contains(headersOf(env), "X-Switchboard-Account: acme2") {
		t.Errorf("started on %q with the headers %q, want the primary's token, pinned to acme2", env["CLAUDE_CODE_OAUTH_TOKEN"], env["ANTHROPIC_CUSTOM_HEADERS"])
	}
}

// otherPools answers as a router whose pools are cfg's as change leaves them
// does.
func otherPools(cfg *config.Config, change func([]router.RoutedPool) []router.RoutedPool) *fakeRouter {
	r := knowing(cfg)
	r.health.Pools = change(r.health.Pools)
	return r
}

func TestRunComparesTheRoutersPoolsWithTheConfigs(t *testing.T) {
	// The pools Default and Acme are, in the config's order, as a router
	// routing by the config reports them.
	const (
		inDefault = 0
		inAcme    = 1
	)
	tests := []struct {
		name string
		// change makes the pools the router routes for of the config's.
		change func([]router.RoutedPool) []router.RoutedPool
		// old has the router answer as one from before pools, and stripless
		// as one that sends switchboard's own headers on to the API.
		old       bool
		stripless bool
		// wantDiffers is how the router's pools differ, "" where they match
		// and the session is routed.
		wantDiffers string
	}{
		{name: "the same", change: func(ps []router.RoutedPool) []router.RoutedPool { return ps }},
		{
			name:        "the same, from a router that would send the pool on to the API",
			change:      func(ps []router.RoutedPool) []router.RoutedPool { return ps },
			stripless:   true,
			wantDiffers: "it would pass switchboard's own headers on to the API",
		},
		{
			name: "the same, in another order",
			change: func(ps []router.RoutedPool) []router.RoutedPool {
				return []router.RoutedPool{ps[inAcme], ps[inDefault]}
			},
		},
		{name: "a router from before pools", old: true, wantDiffers: "it's from before pools"},
		{
			name:        "a pool missing",
			change:      func(ps []router.RoutedPool) []router.RoutedPool { return ps[:1] },
			wantDiffers: "its pools aren't the config's",
		},
		{
			name: "a pool more",
			change: func(ps []router.RoutedPool) []router.RoutedPool {
				return append(ps, router.RoutedPool{ID: "h4w2", Label: "Client", Dirs: clientPool.Dirs})
			},
			wantDiffers: "its pools aren't the config's",
		},
		{
			name: "a pool another",
			change: func(ps []router.RoutedPool) []router.RoutedPool {
				ps[inDefault].ID = "h4w2"
				return ps
			},
			wantDiffers: "its pools aren't the config's",
		},
		{
			name: "a pool's directories others",
			change: func(ps []router.RoutedPool) []router.RoutedPool {
				ps[inAcme].Dirs = []string{homeDir + "/Code/acme-old"}
				return ps
			},
			wantDiffers: "pool Acme's directories aren't the config's",
		},
		{
			name: "a pool's directories more",
			change: func(ps []router.RoutedPool) []router.RoutedPool {
				ps[inAcme].Dirs = append(ps[inAcme].Dirs, homeDir+"/Code/acme-site")
				return ps
			},
			wantDiffers: "pool Acme's directories aren't the config's",
		},
		{
			name: "an account routed in another pool",
			change: func(ps []router.RoutedPool) []router.RoutedPool {
				ps[inDefault].Accounts = append(ps[inDefault].Accounts, "acme2")
				ps[inAcme].Accounts = []string{"acme1"}
				return ps
			},
			wantDiffers: "pool Default's accounts aren't the config's",
		},
		{
			name: "an account routed that the config has removed",
			change: func(ps []router.RoutedPool) []router.RoutedPool {
				ps[inAcme].Accounts = append(ps[inAcme].Accounts, "acme3")
				return ps
			},
			wantDiffers: "pool Acme's accounts aren't the config's",
		},
		{
			name: "an account the config has added, waiting for the router's restart",
			change: func(ps []router.RoutedPool) []router.RoutedPool {
				ps[inAcme].Accounts = []string{"acme1"}
				return ps
			},
		},
		{
			name: "a primary another",
			change: func(ps []router.RoutedPool) []router.RoutedPool {
				ps[inAcme].Primary = "acme2"
				return ps
			},
			wantDiffers: "pool Acme's primary isn't the config's",
		},
		{
			name: "a primary the config has added, not compared",
			change: func(ps []router.RoutedPool) []router.RoutedPool {
				ps[inAcme].Accounts, ps[inAcme].Primary = []string{"acme2"}, "acme2"
				return ps
			},
		},
		{
			name: "a new pool, none of whose accounts the router routes till it restarts",
			change: func(ps []router.RoutedPool) []router.RoutedPool {
				ps[inAcme].Accounts, ps[inAcme].Primary = []string{}, ""
				return ps
			},
			wantDiffers: "it routes none of pool Acme's accounts till it restarts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness(t)
				h.launcher.Dir = acmeDir
				cfg := pooledConfig()
				r := knowing(cfg)
				if tt.old {
					r.health.ReportsPools, r.health.Pools = false, nil
				} else {
					r = otherPools(cfg, tt.change)
				}
				r.health.StripsOwnHeaders = !tt.stripless
				began := time.Now()

				if err := h.launcher.Run(t.Context(), pooledRoute(cfg, r, ""), nil); err != nil {
					t.Fatalf("Run() error = %v", err)
				}

				_, routed := h.environment(t)["ANTHROPIC_BASE_URL"]
				if routed != (tt.wantDiffers == "") {
					t.Errorf("started routed: %v, want %v", routed, tt.wantDiffers == "")
				}
				wantSaid, wantWaited := "", time.Duration(0)
				if tt.wantDiffers != "" {
					wantSaid = "switchboard: the router doesn't route by the config's pools (" + tt.wantDiffers + ") — connecting directly on acme1 · Acme 1\n"
					wantWaited = 5 * time.Second
				}
				if said := h.stderr.String(); said != wantSaid {
					t.Errorf("said %q on stderr, want %q", said, wantSaid)
				}
				if waited := time.Since(began); waited != wantWaited {
					t.Errorf("waited %v for the router to take the pools up, want %v", waited, wantWaited)
				}
			})
		})
	}
}

func TestRunWithOnePoolComparesNothing(t *testing.T) {
	h := newHarness(t)
	h.launcher.Dir = acmeDir
	cfg := &config.Config{Accounts: config.Accounts{{ID: "work", Label: "Work", Pool: "q7mt", Primary: true}}, Pools: config.Pools{acmePool}}

	// A router from before pools.
	if err := h.launcher.Run(t.Context(), pooledRoute(cfg, healthy(), ""), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, routed := h.environment(t)["ANTHROPIC_BASE_URL"]; !routed || h.stderr.Len() > 0 {
		t.Errorf("started routed: %v, saying %q; want routed, saying nothing", routed, h.stderr.String())
	}
}

func TestRunNeedsTheRouterToRouteAnAccountOfItsOwnPoolAlone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, "HOME=/home/tester")
		h.launcher.Dir = elsewhere
		cfg := pooledConfig()
		// Acme is new: the router routes none of its accounts till it restarts.
		r := otherPools(cfg, func(ps []router.RoutedPool) []router.RoutedPool {
			ps[1].Accounts, ps[1].Primary = []string{}, ""
			return ps
		})
		began := time.Now()

		if err := h.launcher.Run(t.Context(), pooledRoute(cfg, r, ""), nil); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		env := h.environment(t)
		if _, routed := env["ANTHROPIC_BASE_URL"]; !routed || env["SWITCHBOARD_POOL"] != "b2n8" || h.stderr.Len() > 0 {
			t.Errorf("started routed: %v, in pool %q, saying %q; want routed in Default, saying nothing", routed, env["SWITCHBOARD_POOL"], h.stderr.String())
		}
		if waited := time.Since(began); waited != 0 {
			t.Errorf("waited %v for the router to take the pools up, want no wait", waited)
		}
	})
}

func TestARouterThatDoesntKnowThePoolsStopsALaunchAsOneDown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		h := newHarness(t)
		h.launcher.Dir = acmeDir
		cfg := pooledConfig()
		cfg.WhenRouterDown = config.Stop

		err := h.launcher.Run(t.Context(), pooledRoute(cfg, healthy(), ""), nil)
		if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
			t.Errorf("Run() error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
		}
		want := "switchboard: the router doesn't route by the config's pools (it's from before pools) — not starting claude: " +
			"restart it with switchboard service restart, or go direct this once with SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude\n"
		if said := h.stderr.String(); said != want {
			t.Errorf("said %q on stderr, want %q", said, want)
		}
		wantLog := []string{"level=WARN", `msg="not starting claude"`, "mode=stop", `router="other pools"`, `reason="it's from before pools"`, "asked=5s"}
		if !log.Has(wantLog...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, wantLog)
		}
	})
}

// changingRouter answers as a router does that answers with before until at,
// from began, and with after from then.
type changingRouter struct {
	began         time.Time
	at            time.Duration
	before, after router.Health
}

func (r *changingRouter) Health(context.Context) (router.Health, error) {
	if time.Since(r.began) < r.at {
		return r.before, nil
	}
	return r.after, nil
}

func TestRunWaitsForTheRouterToTakeThePoolsUp(t *testing.T) {
	tests := []struct {
		name string
		// at is when the router takes the config's pools up.
		at         time.Duration
		wantRouted bool
		wantWaited time.Duration
	}{
		{name: "taken up at 1.5s, asked again every 200ms", at: 1500 * time.Millisecond, wantRouted: true, wantWaited: 1600 * time.Millisecond},
		{name: "taken up at 4.7s", at: 4700 * time.Millisecond, wantRouted: true, wantWaited: 4800 * time.Millisecond},
		{name: "never taken up, 5s at most", at: never, wantWaited: 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness(t)
				h.launcher.Dir = acmeDir
				cfg := pooledConfig()
				r := &changingRouter{began: time.Now(), at: tt.at, after: knowing(cfg).health}
				r.before = otherPools(cfg, func(ps []router.RoutedPool) []router.RoutedPool {
					ps[1].Dirs = []string{homeDir + "/Code/acme-old"}
					return ps
				}).health

				if err := h.launcher.Run(t.Context(), pooledRoute(cfg, r, ""), nil); err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				if _, routed := h.environment(t)["ANTHROPIC_BASE_URL"]; routed != tt.wantRouted {
					t.Errorf("started routed: %v, want %v", routed, tt.wantRouted)
				}
				if waited := time.Since(r.began); waited != tt.wantWaited {
					t.Errorf("waited %v, want %v", waited, tt.wantWaited)
				}
			})
		})
	}
}

func TestOnItsOwnLoginClaudeNeverStartsInsideAFence(t *testing.T) {
	pools := config.Pools{defaultPool, acmePool}
	launches := []struct {
		name   string
		launch func(l launch.Launcher) error
		// said is what stderr hears ahead of why a launch is refused.
		said string
	}{
		{name: "--direct", launch: func(l launch.Launcher) error { return l.Direct(nil, pools) }, said: "switchboard: "},
		{
			name:   "without a config it can read",
			launch: func(l launch.Launcher) error { return l.Unaided(nil, pools, launch.CouldntReadConfig, errUnread) },
			said:   unreadSaid,
		},
	}
	places := []struct {
		name string
		dir  string
		// wantWhy is why stderr hears it's refused, "" where claude starts.
		wantWhy string
	}{
		{name: "inside a fence", dir: acmeDir + "/api", wantWhy: "~/Code/acme/api is in pool Acme, where claude never starts on its own login"},
		{name: "outside every fence", dir: elsewhere},
		{name: "a directory not known", wantWhy: "can't tell which pool the directory claude starts in is in"},
	}
	for _, tt := range launches {
		for _, at := range places {
			t.Run(tt.name+", "+at.name, func(t *testing.T) {
				h := newHarness(t, "HOME=/home/tester")
				h.launcher.Dir = at.dir

				err := tt.launch(h.launcher)
				if at.wantWhy == "" {
					if err != nil || len(h.starts) != 1 {
						t.Errorf("launch error = %v, starting %q; want claude started", err, h.starts)
					}
					return
				}
				if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
					t.Errorf("launch error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
				}
				if said, want := h.stderr.String(), tt.said+at.wantWhy+" — not starting claude\n"; said != want {
					t.Errorf("said %q on stderr, want %q", said, want)
				}
			})
		}
	}
}

func TestOnItsOwnLoginClaudeStartsAnywhereWithoutAFence(t *testing.T) {
	h := newHarness(t, "HOME=/home/tester")

	// A directory not known, the one pool unfenced.
	if err := h.launcher.Direct(nil, config.Pools{defaultPool}); err != nil || len(h.starts) != 1 {
		t.Errorf("Direct() error = %v, starting %q; want claude started", err, h.starts)
	}
}

func TestRouterUnfoundGoesDirectOnAnAccountOfThePool(t *testing.T) {
	const unfound = "locate state directory: no home directory"
	tests := []struct {
		name    string
		dir     string
		account string
		tokens  tokenstest.Files
		down    config.RouterDown
		// pools are those configured, Default and Acme where nil.
		pools config.Pools
		// wantStarted is whether claude starts, straight to the API, on
		// wantToken, "" for none; and wantSaid what stderr hears.
		wantStarted bool
		wantToken   string
		wantSaid    string
	}{
		{
			name:        "inside a fence, on its primary",
			dir:         acmeDir + "/api",
			down:        config.Direct,
			wantStarted: true,
			wantToken:   acme1Token,
			wantSaid:    "switchboard: the router couldn't be found (" + unfound + ") — connecting directly on acme1 · Acme 1\n",
		},
		{
			name:        "outside every fence, on the unfenced pool's primary",
			dir:         elsewhere,
			down:        config.Direct,
			wantStarted: true,
			wantToken:   workToken,
			wantSaid:    "switchboard: the router couldn't be found (" + unfound + ") — connecting directly on work · Work\n",
		},
		{
			name:        "pinned, on the pin's",
			dir:         acmeDir,
			account:     "acme2",
			down:        config.Direct,
			wantStarted: true,
			wantToken:   acme2Token,
			wantSaid:    "switchboard: the router couldn't be found (" + unfound + ") — connecting directly on acme2 · Acme 2\n",
		},
		{
			name:     "none of the pool's accounts with a usable token: refused, never on its own login",
			dir:      acmeDir,
			tokens:   tokenstest.Files{"work": workToken},
			down:     config.Direct,
			wantSaid: "switchboard: couldn't find the router (" + unfound + ") — pool Acme has no account with a usable token — not starting claude\n",
		},
		{
			name:        "one pool, unfenced, without a usable token: without switchboard, as before pools",
			dir:         acmeDir,
			tokens:      tokenstest.Files{},
			down:        config.Direct,
			pools:       config.Pools{defaultPool},
			wantStarted: true,
			wantSaid:    "switchboard: couldn't find the router (" + unfound + ") — starting claude without it\n",
		},
		{
			name:     "no pool covering its directory: refused",
			dir:      elsewhere,
			down:     config.Direct,
			pools:    config.Pools{acmePool, clientPool},
			wantSaid: "switchboard: no pool covers ~/Code/other — not starting claude\n",
		},
		{
			name: "told to stop: stopped",
			dir:  acmeDir,
			down: config.Stop,
			wantSaid: "switchboard: the router couldn't be found (" + unfound + ") — not starting claude: " +
				"go direct this once with SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, "HOME=/home/tester")
			h.launcher.Dir = tt.dir
			cfg := pooledConfig()
			cfg.WhenRouterDown = tt.down
			if tt.pools != nil {
				cfg.Pools = tt.pools
			}
			r := pooledRoute(cfg, nil, tt.account)
			if tt.tokens != nil {
				r.Token = tt.tokens.Read
			}

			err := h.launcher.RouterUnfound(r, nil, errors.New(unfound))
			if said := h.stderr.String(); said != tt.wantSaid {
				t.Errorf("said %q on stderr, want %q", said, tt.wantSaid)
			}
			if !tt.wantStarted {
				if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
					t.Errorf("RouterUnfound() error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
				}
				return
			}
			if err != nil {
				t.Fatalf("RouterUnfound() error = %v", err)
			}
			env := h.environment(t)
			if _, routed := env["ANTHROPIC_BASE_URL"]; routed || env["CLAUDE_CODE_OAUTH_TOKEN"] != tt.wantToken {
				t.Errorf("started routed: %v, on %q; want straight to the API on %q", routed, env["CLAUDE_CODE_OAUTH_TOKEN"], tt.wantToken)
			}
		})
	}
}

// reporting returns r, its health answer reporting the pools of cfg.
func (r *fakeRouter) reporting(cfg *config.Config) *fakeRouter {
	r.health.ReportsPools = true
	r.health.Pools = routedPools(cfg)
	return r
}

// unreadPooledRoute is a launch through r where the config can't be read,
// pinned to account unless it's "", the config file's pools written.
func unreadPooledRoute(r *fakeRouter, written launch.Written, account string) launch.Route {
	route := unreadRoute(r, account)
	route.Written = written
	route.Token = poolTokens.Read
	return route
}

func TestRunUnreadStartsInThePoolTheRouterRoutesItsDirectoryIn(t *testing.T) {
	tests := []struct {
		name      string
		dir       string
		wantToken string
		wantPool  string
	}{
		{name: "inside a fence", dir: acmeDir + "/api", wantToken: acme1Token, wantPool: "q7mt"},
		{name: "outside every fence", dir: elsewhere, wantToken: workToken, wantPool: "b2n8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, "HOME=/home/tester")
			h.launcher.Dir = tt.dir
			// The config file writes no pools: the router's are the ones.
			r := unreadPooledRoute(healthy().reporting(pooledConfig()), launch.Written{}, "")

			if err := h.launcher.RunUnread(t.Context(), r, nil, errUnread); err != nil {
				t.Fatalf("RunUnread() error = %v", err)
			}
			env := h.environment(t)
			if env["CLAUDE_CODE_OAUTH_TOKEN"] != tt.wantToken || env["SWITCHBOARD_POOL"] != tt.wantPool || !slices.Contains(headersOf(env), "X-Switchboard-Pool: "+tt.wantPool) {
				t.Errorf("started on %q in pool %q with the headers %q, want %q in pool %s, the router told it", env["CLAUDE_CODE_OAUTH_TOKEN"], env["SWITCHBOARD_POOL"], env["ANTHROPIC_CUSTOM_HEADERS"], tt.wantToken, tt.wantPool)
			}
			if said := h.stderr.String(); said != throughRunning {
				t.Errorf("said %q on stderr, want %q", said, throughRunning)
			}
		})
	}
}

func TestRunUnreadDecidesThePoolAsRunDoes(t *testing.T) {
	everyFenced := pooledConfig()
	everyFenced.Pools = config.Pools{acmePool, clientPool}
	everyFenced.Accounts[1].Pool, everyFenced.Accounts[2].Pool = "h4w2", "h4w2"
	everyFenced.Accounts = everyFenced.Accounts[1:]
	tests := []struct {
		name    string
		dir     string
		router  *fakeRouter
		account string
		tokens  tokenstest.Files
		wantErr string
		// wantSaid is what stderr hears, "" for nothing.
		wantSaid string
	}{
		{
			name:     "outside every fence, every pool fenced",
			dir:      elsewhere,
			router:   healthy().reporting(everyFenced),
			wantSaid: "switchboard: no pool covers ~/Code/other — not starting claude\n",
		},
		{
			name:    "pinned to an account in another pool, named by its id, its label not known",
			dir:     elsewhere,
			router:  healthy().reporting(pooledConfig()),
			account: "acme2",
			wantErr: "acme2 is in pool Acme; this directory is in pool Default",
		},
		{
			name:     "none of the pool's accounts with a usable token",
			dir:      acmeDir,
			router:   healthy().reporting(pooledConfig()),
			tokens:   tokenstest.Files{"work": workToken},
			wantSaid: "switchboard: pool Acme has no account with a usable token — not starting claude\n",
		},
		{
			name: "none of the pool's accounts routed till the router restarts",
			dir:  acmeDir,
			router: otherPools(pooledConfig(), func(ps []router.RoutedPool) []router.RoutedPool {
				ps[1].Accounts, ps[1].Primary = []string{}, ""
				return ps
			}),
			wantSaid: "switchboard: the router routes no account of pool Acme yet: any the config gives it waits for the router to restart — not starting claude\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.launcher.Dir = tt.dir
			r := unreadPooledRoute(tt.router, launch.Written{}, tt.account)
			if tt.tokens != nil {
				r.Token = tt.tokens.Read
			}

			err := h.launcher.RunUnread(t.Context(), r, nil, errUnread)
			switch {
			case tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr):
				t.Errorf("RunUnread() error = %v, want %q", err, tt.wantErr)
			case tt.wantErr == "" && !errors.Is(err, launch.ErrNotStarted):
				t.Errorf("RunUnread() error = %v, want %v", err, launch.ErrNotStarted)
			}
			if len(h.starts) > 0 {
				t.Errorf("started %q, want nothing started", h.starts)
			}
			if said := h.stderr.String(); said != tt.wantSaid {
				t.Errorf("said %q on stderr, want %q", said, tt.wantSaid)
			}
		})
	}
}

func TestRunUnreadRoutesByTheConfigFilesPoolsAlone(t *testing.T) {
	written := launch.Written{Pools: config.Pools{defaultPool, acmePool}}
	unchecked := launch.Written{Pools: written.Pools, Unchecked: true}
	unrouted := unreadSaid + "the running router doesn't route by the config's pools"
	insideFence := unrouted + " — ~/Code/acme is in pool Acme, where claude never starts on its own login — not starting claude\n"
	// moved answers as a router whose Acme is fenced where the config file's
	// was before, and stripless as one that sends switchboard's own headers
	// on to the API.
	moved := func() *fakeRouter {
		return otherPools(pooledConfig(), func(ps []router.RoutedPool) []router.RoutedPool {
			ps[1].Dirs = []string{homeDir + "/Code/acme-old"}
			return ps
		})
	}
	stripless := func() *fakeRouter {
		r := healthy().reporting(pooledConfig())
		r.health.StripsOwnHeaders = false
		return r
	}
	tests := []struct {
		name    string
		router  *fakeRouter
		written launch.Written
		dir     string
		// wantSaid is what stderr hears; wantStarted whether claude starts,
		// and wantPool the pool its environment names, "" for none.
		wantSaid    string
		wantStarted bool
		wantPool    string
	}{
		{
			name:        "a router reporting the pools written: through it",
			router:      healthy().reporting(pooledConfig()),
			written:     written,
			dir:         acmeDir,
			wantSaid:    throughRunning,
			wantStarted: true,
			wantPool:    "q7mt",
		},
		{
			name:        "a router reporting the pools written, which break the config's rules: through it, by its pools",
			router:      healthy().reporting(pooledConfig()),
			written:     unchecked,
			dir:         acmeDir,
			wantSaid:    throughRunning,
			wantStarted: true,
			wantPool:    "q7mt",
		},
		{
			name:     "a router whose fences aren't those written, inside a written fence: refused",
			router:   moved(),
			written:  written,
			dir:      acmeDir,
			wantSaid: insideFence,
		},
		{
			name:        "a router whose fences aren't those written, outside every written fence: without switchboard",
			router:      moved(),
			written:     written,
			dir:         homeDir + "/Code/acme-old",
			wantSaid:    unrouted + " — starting claude without it\n",
			wantStarted: true,
		},
		{
			name:     "a router reporting several pools that would send the pool on to the API: refused inside a fence",
			router:   stripless(),
			dir:      acmeDir,
			wantSaid: insideFence,
		},
		{
			name:     "a router from before pools, several written, inside a fence: refused",
			router:   healthy().naming("work", accounts...),
			written:  written,
			dir:      acmeDir,
			wantSaid: insideFence,
		},
		{
			name:        "a router from before pools, several written, outside every fence: without switchboard",
			router:      healthy().naming("work", accounts...),
			written:     written,
			dir:         elsewhere,
			wantSaid:    unrouted + " — starting claude without it\n",
			wantStarted: true,
		},
		{
			name:     "the router down, several written, inside a fence: refused",
			router:   notRunning(),
			written:  written,
			dir:      acmeDir,
			wantSaid: unreadSaid + "~/Code/acme is in pool Acme, where claude never starts on its own login — not starting claude\n",
		},
		{
			name:        "the router down, several written, outside every fence: without switchboard",
			router:      notRunning(),
			written:     written,
			dir:         elsewhere,
			wantSaid:    unreadUnaided,
			wantStarted: true,
		},
		{
			name:        "a router from before pools, one written: through it, in that pool",
			router:      healthy().naming("work", accounts...),
			written:     launch.Written{Pools: config.Pools{acmePool}},
			dir:         acmeDir,
			wantSaid:    throughRunning,
			wantStarted: true,
			wantPool:    "q7mt",
		},
		{
			name:     "a router from before pools, one written, fenced, outside it: refused",
			router:   healthy().naming("work", accounts...),
			written:  launch.Written{Pools: config.Pools{acmePool}},
			dir:      elsewhere,
			wantSaid: "switchboard: no pool covers ~/Code/other — not starting claude\n",
		},
		{
			name:     "a router from before pools, one written that breaks the config's rules, inside it: refused, never routed by it",
			router:   healthy().naming("work", accounts...),
			written:  launch.Written{Pools: config.Pools{acmePool}, Unchecked: true},
			dir:      acmeDir,
			wantSaid: insideFence,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, "HOME=/home/tester")
			h.launcher.Dir = tt.dir

			err := h.launcher.RunUnread(t.Context(), unreadPooledRoute(tt.router, tt.written, ""), nil, errUnread)
			if said := h.stderr.String(); said != tt.wantSaid {
				t.Errorf("said %q on stderr, want %q", said, tt.wantSaid)
			}
			if !tt.wantStarted {
				if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
					t.Errorf("RunUnread() error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
				}
				return
			}
			if err != nil {
				t.Fatalf("RunUnread() error = %v", err)
			}
			if pool := h.environment(t)["SWITCHBOARD_POOL"]; pool != tt.wantPool {
				t.Errorf("started in pool %q, want %q", pool, tt.wantPool)
			}
		})
	}
}

// unpooledConfig is pooledConfig's accounts naming no pools, as a config put
// back by hand once the move to pools is done names none, its first the
// primary.
func unpooledConfig() *config.Config {
	cfg := pooledConfig()
	cfg.Pools = nil
	for i := range cfg.Accounts {
		cfg.Accounts[i].Pool, cfg.Accounts[i].Primary = "", i == 0
	}
	return cfg
}

func TestRunAfterTheMoveToPoolsNeverLaunchesAConfigNamingNoneAsItIs(t *testing.T) {
	// The record keeps Acme's primary as acme2, and the router acme1.
	kept := func(cfg *config.Config) (*config.Config, bool) {
		pooled := pooledConfig()
		pooled.WhenRouterDown = cfg.WhenRouterDown
		pooled.Accounts[3].Primary, pooled.Accounts[4].Primary = false, true
		return pooled, true
	}
	tests := []struct {
		name   string
		moved  bool
		router *fakeRouter
		kept   func(*config.Config) (*config.Config, bool)
		// wantPool is the pool the session is started in, "" for none, and
		// wantToken its token, or "" where nothing starts.
		wantPool  string
		wantToken string
		wantSaid  string
	}{
		{name: "before the move: as before pools", router: healthy(), wantToken: workToken},
		{name: "before the move, a router reporting pools: as before pools", router: healthy().reporting(pooledConfig()), kept: kept, wantToken: workToken},
		{name: "the router's pools", moved: true, router: healthy().reporting(pooledConfig()), wantPool: "q7mt", wantToken: acme1Token},
		{name: "the router's pools before the record's", moved: true, router: healthy().reporting(pooledConfig()), kept: kept, wantPool: "q7mt", wantToken: acme1Token},
		{
			name:      "a router from before pools: the record's, the router counting as down",
			moved:     true,
			router:    healthy(),
			kept:      kept,
			wantPool:  "q7mt",
			wantToken: acme2Token,
			wantSaid:  "switchboard: the router doesn't route by the config's pools (it's from before pools) — connecting directly on acme2 · Acme 2\n",
		},
		{
			name:      "the router down: the record's, direct on the pool's primary",
			moved:     true,
			router:    notRunning(),
			kept:      kept,
			wantPool:  "q7mt",
			wantToken: acme2Token,
			wantSaid:  "switchboard: the router isn't running — connecting directly on acme2 · Acme 2\n",
		},
		{
			name:     "neither: refused",
			moved:    true,
			router:   healthy(),
			kept:     func(*config.Config) (*config.Config, bool) { return nil, false },
			wantSaid: "switchboard: the config names no pools, and switchboard can't tell which pool ~/Code/acme is in — not starting claude\n",
		},
		{
			name:     "no record to ask: refused",
			moved:    true,
			router:   notRunning(),
			wantSaid: "switchboard: the config names no pools, and switchboard can't tell which pool ~/Code/acme is in — not starting claude\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness(t, "HOME=/home/tester")
				h.launcher.Dir = acmeDir
				r := pooledRoute(unpooledConfig(), tt.router, "")
				r.Moved, r.Kept = tt.moved, tt.kept

				err := h.launcher.Run(t.Context(), r, nil)
				if said := h.stderr.String(); said != tt.wantSaid {
					t.Errorf("said %q on stderr, want %q", said, tt.wantSaid)
				}
				if tt.wantToken == "" {
					if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
						t.Errorf("Run() error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
					}
					return
				}
				if err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				env := h.environment(t)
				if env["CLAUDE_CODE_OAUTH_TOKEN"] != tt.wantToken || env["SWITCHBOARD_POOL"] != tt.wantPool {
					t.Errorf("started on %q in pool %q, want %q in pool %q", env["CLAUDE_CODE_OAUTH_TOKEN"], env["SWITCHBOARD_POOL"], tt.wantToken, tt.wantPool)
				}
			})
		})
	}
}

func TestAConfigWithPoolsNeverAsksTheRecord(t *testing.T) {
	h := newHarness(t)
	h.launcher.Dir = acmeDir
	cfg := pooledConfig()
	r := pooledRoute(cfg, knowing(cfg), "")
	r.Moved = true
	r.Kept = func(*config.Config) (*config.Config, bool) {
		t.Error("asked the record for the pools of a config naming them")
		return nil, false
	}

	if err := h.launcher.Run(t.Context(), r, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRoutedPools(t *testing.T) {
	cfg := pooledConfig()
	tests := []struct {
		name   string
		health router.Health
		want   config.Pools
	}{
		{name: "a router from before pools", health: router.Health{OK: true}},
		{name: "a router of no pools", health: router.Health{OK: true, ReportsPools: true}},
		{name: "a router of pools", health: knowing(cfg).health, want: config.Pools{defaultPool, acmePool}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reported := launch.RoutedPools(tt.health)
			if !slices.EqualFunc(got, tt.want, samePool) || reported != (tt.want != nil) {
				t.Errorf("RoutedPools() = %+v, %v; want %+v", got, reported, tt.want)
			}
		})
	}
}

// samePool reports whether a and b are the same pool, labelled alike, with
// the same directories.
func samePool(a, b config.Pool) bool {
	return a.ID == b.ID && a.Label == b.Label && slices.Equal(a.Dirs, b.Dirs) && slices.Equal(a.WrittenDirs, b.WrittenDirs)
}
