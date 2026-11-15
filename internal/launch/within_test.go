package launch_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
)

// The process ids of a chain of launches: a session's claude, started by
// the launcher's own process id, pid, runs a shell, which starts a claude
// within the session, whose shell starts another.
const (
	shellPID       = 6000
	withinPID      = 6001
	innerShellPID  = 7000
	innerWithinPID = 7001
)

// The directories a claude is started in within a session: two of Claude
// Code's scratchpads, in no repository, and a repository outside every
// fence, tool, and one inside Client's fence, site.
const (
	scratchpad      = "/private/tmp/claude-501/-home-tester-Code-acme/0f3a8c21-4b6f-4e1a-9c3d-2f8b7a6e5d4c/scratchpad"
	innerScratchpad = "/private/tmp/claude-501/-private-tmp-scratchpad/7c1e9d2a-1d3b-4c5e-8f9a-0b1c2d3e4f5a/scratchpad"
	tool            = homeDir + "/Code/tool"
	site            = homeDir + "/Code/client/site"
)

// repositories finds the project a directory is in as FindProject does,
// where the repositories are acmeDir, elsewhere, tool and ~/Code/client:
// the one holding it, else the directory itself, in none.
func repositories(dir string) (string, bool) {
	for _, repo := range []string{acmeDir, elsewhere, tool, homeDir + "/Code/client"} {
		if dir == repo || strings.HasPrefix(dir, repo+"/") {
			return repo, true
		}
	}
	return dir, false
}

// processes are the processes running, by id, as FindProcess reads them.
type processes map[int]launch.Process

func (p processes) find(pid int) (launch.Process, bool) {
	found, ok := p[pid]
	return found, ok
}

// switchboards keeps switchboard's variables of env, as FindProcess reads
// a process's environment.
func switchboards(env []string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(variable string) bool { return !strings.HasPrefix(variable, "SWITCHBOARD_") })
}

// inherited returns the environment of the session s starts, whole, as a
// claude started within it inherits it: with the mark of what run set, and
// the mark of the claude, which names the launcher that started it.
func (s started) inherited() []string {
	env := slices.Clone(s.env)
	for _, m := range s.marks {
		env = append(env, markEnv+"="+m)
	}
	for _, set := range s.sets {
		env = append(env, setEnv+"="+set)
	}
	return env
}

// configOf is pooledConfig, but with pools in place of its own where they're
// given, side then Client's.
func configOf(pools config.Pools) *config.Config {
	cfg := pooledConfig()
	if pools != nil {
		cfg.Pools = pools
		cfg.Accounts[2].Pool = clientPool.ID
	}
	return cfg
}

// session starts a session in dir, on cfg, as a launcher whose process id is
// pid, and returns the environment its claude runs in, whole.
func session(t *testing.T, cfg *config.Config, dir string) []string {
	t.Helper()
	h := newHarness(t, "HOME=/home/tester")
	h.launcher.Dir, h.launcher.Project = dir, repositories
	if err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil); err != nil {
		t.Fatalf("starting the session: Run() error = %v; said %q", err, h.stderr.String())
	}
	return h.only(t).inherited()
}

// within is a harness for a claude started in dir, in the environment
// environ, by a shell that the session's claude runs, whose environment is
// sessionEnv: its own process id is withinPID.
func within(t *testing.T, environ, sessionEnv []string, dir string) *harness {
	t.Helper()
	h := newHarness(t, environ...)
	h.launcher.Dir, h.launcher.Project, h.launcher.PID = dir, repositories, withinPID
	h.launcher.Process = processes{
		withinPID: {Parent: shellPID, Started: launched.Add(time.Minute)},
		shellPID:  {Parent: pid, Started: launched.Add(time.Second)},
		pid:       {Parent: 1, Started: launched.Add(-time.Second), Environ: switchboards(sessionEnv)},
	}.find
	return h
}

// placedIn checks that the launch env was started in is in wantPool, on
// wantToken, and runs in wantProject, the router told both.
func placedIn(t *testing.T, env map[string]string, wantPool, wantToken, wantProject string) {
	t.Helper()
	headers := headersOf(env)
	if env["SWITCHBOARD_POOL"] != wantPool || !slices.Contains(headers, "X-Switchboard-Pool: "+wantPool) || env["CLAUDE_CODE_OAUTH_TOKEN"] != wantToken {
		t.Errorf("started on %q in pool %q, its headers %q; want %q in pool %s, the router told it", env["CLAUDE_CODE_OAUTH_TOKEN"], env["SWITCHBOARD_POOL"], headers, wantToken, wantPool)
	}
	shown := wantProject
	if rest, ok := strings.CutPrefix(wantProject, homeDir+"/"); ok {
		shown = filepath.Join("~", rest)
	}
	if !slices.Contains(headers, "X-Switchboard-Project: "+shown) || env["SWITCHBOARD_PROJECT"] != wantProject {
		t.Errorf("started in the project %q, its headers %q; want %q, the router told it %s", env["SWITCHBOARD_PROJECT"], headers, wantProject, shown)
	}
}

func TestALaunchWithinASessionIsInItsPoolAndRunsInItsProject(t *testing.T) {
	tests := []struct {
		name string
		// pools are those configured, Default and Acme where it's nil; from
		// is where the session starts, and dir where the claude within it
		// does.
		pools     config.Pools
		from, dir string
		// link is a directory that's a link, and leads where it leads, where
		// it isn't "".
		link, leads string
		// wantPool, wantToken and wantProject are where the claude within
		// starts, or wantSaid why it doesn't.
		wantPool, wantToken, wantProject string
		wantSaid                         string
	}{
		{
			name: "in a scratchpad, from a fenced pool's session: its pool and project", from: acmeDir, dir: scratchpad,
			wantPool: acmePool.ID, wantToken: acme1Token, wantProject: acmeDir,
		},
		{
			name: "in a scratchpad, from the unfenced pool's session: its pool and project", from: elsewhere, dir: scratchpad,
			wantPool: defaultPool.ID, wantToken: workToken, wantProject: elsewhere,
		},
		{
			name: "in a repository outside every fence, from a fenced pool's session: its pool, the repository's project", from: acmeDir, dir: tool + "/cmd",
			wantPool: acmePool.ID, wantToken: acme1Token, wantProject: tool,
		},
		{
			name: "in a repository outside every fence, from the unfenced pool's session: its pool, the repository's project", from: elsewhere, dir: tool,
			wantPool: defaultPool.ID, wantToken: workToken, wantProject: tool,
		},
		{
			name: "in a repository in its own pool's fence, from a fenced pool's session", from: acmeDir, dir: acmeDir + "/web",
			wantPool: acmePool.ID, wantToken: acme1Token, wantProject: acmeDir,
		},
		{
			name: "in a repository in a fence, from the unfenced pool's session: the fence's pool, as a fence keeps its own work", from: elsewhere, dir: acmeDir + "/web",
			wantPool: acmePool.ID, wantToken: acme1Token, wantProject: acmeDir,
		},
		{
			name:  "in no repository in a fence, from the unfenced pool's session: the fence's pool, the session's project",
			pools: config.Pools{defaultPool, acmePool, clientPool}, from: elsewhere, dir: "/Volumes/Client/drafts",
			wantPool: clientPool.ID, wantToken: sideToken, wantProject: elsewhere,
		},
		{
			name:  "in a scratchpad, from a fenced pool's session, every pool fenced: its pool, where none covers the scratchpad",
			pools: config.Pools{acmePool, clientPool}, from: acmeDir, dir: scratchpad,
			wantPool: acmePool.ID, wantToken: acme1Token, wantProject: acmeDir,
		},
		{
			name:  "in another pool's fence, from a fenced pool's session: refused, naming both",
			pools: config.Pools{defaultPool, acmePool, clientPool}, from: acmeDir, dir: site,
			wantSaid: "switchboard: ~/Code/client/site is in pool Client, but the session it's started within is in pool Acme — not starting claude\n",
		},
		{
			name:  "in another pool's fence through its links, from a fenced pool's session: refused, as without one",
			pools: config.Pools{defaultPool, acmePool, clientPool}, from: acmeDir, dir: acmeDir + "/client", link: acmeDir + "/client", leads: "/Volumes/Client",
			wantSaid: "switchboard: ~/Code/acme/client is in pools Acme and Client, as its links lead — not starting claude\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configOf(tt.pools)
			sessionEnv := session(t, cfg, tt.from)
			h := within(t, sessionEnv, sessionEnv, tt.dir)
			if tt.link != "" {
				resolve := h.launcher.EvalSymlinks
				h.launcher.EvalSymlinks = func(path string) (string, error) {
					if rest, ok := strings.CutPrefix(path, tt.link); ok {
						return tt.leads + rest, nil
					}
					return resolve(path)
				}
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
				t.Fatalf("Run() error = %v; said %q", err, h.stderr.String())
			}
			placedIn(t, h.environment(t), tt.wantPool, tt.wantToken, tt.wantProject)
		})
	}
}

func TestALaunchWithinASessionDirectIsInItsPoolAndRunsInItsProject(t *testing.T) {
	cfg := pooledConfig()
	sessionEnv := session(t, cfg, acmeDir)
	h := within(t, sessionEnv, sessionEnv, scratchpad)

	if err := h.launcher.Run(t.Context(), pooledRoute(cfg, notRunning(), ""), nil); err != nil {
		t.Fatalf("Run() error = %v; said %q", err, h.stderr.String())
	}
	env := h.environment(t)
	if env["SWITCHBOARD_POOL"] != acmePool.ID || env["CLAUDE_CODE_OAUTH_TOKEN"] != acme1Token || env["SWITCHBOARD_PROJECT"] != acmeDir {
		t.Errorf("started direct on %q in pool %q and project %q, want %q in pool %s and project %s",
			env["CLAUDE_CODE_OAUTH_TOKEN"], env["SWITCHBOARD_POOL"], env["SWITCHBOARD_PROJECT"], acme1Token, acmePool.ID, acmeDir)
	}
}

func TestALaunchWithItsEnvironmentCleanedIsFoundWithinItsSessionThroughWhatStartedIt(t *testing.T) {
	cfg := pooledConfig()
	sessionEnv := session(t, cfg, acmeDir)
	tests := []struct {
		name string
		// table changes the processes running, as within gives them, where
		// it isn't nil.
		table func(p processes)
		// inSession is whether it's found within the session started in
		// Acme, rather than placed by its scratchpad alone.
		inSession bool
	}{
		{name: "the session's claude one of the processes that started it", inSession: true},
		{
			name: "the session's claude one of them, a process between holding the session's environment",
			table: func(p processes) {
				p[shellPID] = launch.Process{Parent: pid, Started: launched.Add(time.Second), Environ: switchboards(sessionEnv)}
			},
			inSession: true,
		},
		{
			name: "a process that started it holding the environment of a session still running, as a tmux server started within it, but the session's claude not among them",
			table: func(p processes) {
				p[shellPID] = launch.Process{Parent: 1, Started: launched.Add(time.Second), Environ: switchboards(sessionEnv)}
			},
		},
		{
			name: "a process that started it holding the environment of a session that has ended, as a tmux server outliving it",
			table: func(p processes) {
				delete(p, pid)
				p[shellPID] = launch.Process{Parent: 1, Started: launched.Add(time.Second), Environ: switchboards(sessionEnv)}
			},
		},
		{
			name: "none of the processes that started it within a session",
			table: func(p processes) {
				p[shellPID] = launch.Process{Parent: 1, Started: launched.Add(time.Second)}
			},
		},
		{
			name: "the processes that started it unknown",
			table: func(p processes) {
				delete(p, withinPID)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := within(t, []string{"HOME=/home/tester"}, sessionEnv, scratchpad)
			table := processes{}
			for _, id := range []int{withinPID, shellPID, pid} {
				table[id], _ = h.launcher.Process(id)
			}
			if tt.table != nil {
				tt.table(table)
			}
			h.launcher.Process = table.find

			if err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil); err != nil {
				t.Fatalf("Run() error = %v; said %q", err, h.stderr.String())
			}
			env := h.environment(t)
			if tt.inSession {
				placedIn(t, env, acmePool.ID, acme1Token, acmeDir)
				return
			}
			placedIn(t, env, defaultPool.ID, workToken, scratchpad)
		})
	}
}

func TestAStaleMarkOfASessionIsNone(t *testing.T) {
	cfg := pooledConfig()
	sessionEnv := session(t, cfg, acmeDir)
	tests := []struct {
		name string
		// change changes the launch within the session, as within gives it.
		change func(h *harness)
	}{
		{
			name: "its claude has ended, the shell it ran outliving it",
			change: func(h *harness) {
				h.launcher.Process = processes{withinPID: {Parent: shellPID}, shellPID: {Parent: 1}}.find
			},
		},
		{
			name: "its claude's process id taken since by a later process, one that started it",
			change: func(h *harness) {
				h.launcher.Process = processes{withinPID: {Parent: pid}, pid: {Parent: 1, Started: launched.Add(2 * time.Second)}}.find
			},
		},
		{
			name: "its claude running, but not among the processes that started it, as where they're a tmux server's started within the session",
			change: func(h *harness) {
				h.launcher.Process = processes{
					withinPID: {Parent: shellPID},
					shellPID:  {Parent: 1},
					pid:       {Parent: 1, Started: launched.Add(-time.Second)},
				}.find
			},
		},
		{
			name: "naming this process, a switchboard started again in its own claude's place",
			change: func(h *harness) {
				h.launcher.PID = pid
				h.launcher.Process = processes{pid: {Parent: 1, Started: launched.Add(-time.Second)}}.find
			},
		},
		{
			name:   "no way to tell which processes run",
			change: func(h *harness) { h.launcher.Process = nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := within(t, sessionEnv, sessionEnv, scratchpad)
			tt.change(h)

			if err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil); err != nil {
				t.Fatalf("Run() error = %v; said %q", err, h.stderr.String())
			}
			placedIn(t, h.environment(t), defaultPool.ID, workToken, scratchpad)
		})
	}
}

func TestASessionsPoolChangedSinceRunSetItIsNoneOfItsOwn(t *testing.T) {
	cfg := pooledConfig()
	sessionEnv := session(t, cfg, acmeDir)
	changed := slices.DeleteFunc(slices.Clone(sessionEnv), func(v string) bool { return strings.HasPrefix(v, "SWITCHBOARD_POOL=") })
	h := within(t, append(changed, "SWITCHBOARD_POOL="+clientPool.ID), sessionEnv, scratchpad)

	if err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil); err != nil {
		t.Fatalf("Run() error = %v; said %q", err, h.stderr.String())
	}
	placedIn(t, h.environment(t), defaultPool.ID, workToken, acmeDir)
}

func TestAChainOfSessionsPassesOnWhatEachWasGiven(t *testing.T) {
	tests := []struct {
		name string
		// from is where the first session starts, and dir where the one
		// within it does, within which the last starts in a scratchpad.
		from, dir                        string
		wantPool, wantToken, wantProject string
	}{
		{
			name: "each in a scratchpad: the first's pool and project", from: acmeDir, dir: scratchpad,
			wantPool: acmePool.ID, wantToken: acme1Token, wantProject: acmeDir,
		},
		{
			name: "the second in a fence, from the unfenced pool's session: the fence's pool, kept down the chain", from: elsewhere, dir: acmeDir + "/web",
			wantPool: acmePool.ID, wantToken: acme1Token, wantProject: acmeDir,
		},
		{
			name: "the second in a repository of its own: the first's pool, the second's project", from: acmeDir, dir: tool,
			wantPool: acmePool.ID, wantToken: acme1Token, wantProject: tool,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := pooledConfig()
			first := session(t, cfg, tt.from)
			second := within(t, first, first, tt.dir)
			if err := second.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil); err != nil {
				t.Fatalf("starting the second: Run() error = %v; said %q", err, second.stderr.String())
			}
			secondEnv := second.only(t).inherited()
			last := newHarness(t, secondEnv...)
			last.launcher.Dir, last.launcher.Project, last.launcher.PID = innerScratchpad, repositories, innerWithinPID
			last.launcher.Process = processes{
				innerWithinPID: {Parent: innerShellPID, Started: launched.Add(2 * time.Minute)},
				innerShellPID:  {Parent: withinPID, Started: launched.Add(time.Minute)},
				withinPID:      {Parent: shellPID, Started: launched.Add(-time.Second), Environ: switchboards(secondEnv)},
				shellPID:       {Parent: pid, Started: launched.Add(-2 * time.Second)},
				pid:            {Parent: 1, Started: launched.Add(-3 * time.Second), Environ: switchboards(first)},
			}.find

			if err := last.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil); err != nil {
				t.Fatalf("starting the last: Run() error = %v; said %q", err, last.stderr.String())
			}
			placedIn(t, last.environment(t), tt.wantPool, tt.wantToken, tt.wantProject)
		})
	}
}

func TestOnItsOwnLoginClaudeNeverStartsWithinASessionOfAFencedPool(t *testing.T) {
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
	sessions := []struct {
		name string
		from string
		// wantWhy is why stderr hears it's refused, "" where claude starts.
		wantWhy string
	}{
		{name: "within a fenced pool's session", from: acmeDir, wantWhy: scratchpad + " is in pool Acme, through the session it's started within, where claude never starts on its own login"},
		{name: "within the unfenced pool's session", from: elsewhere},
	}
	for _, tt := range launches {
		for _, s := range sessions {
			t.Run(tt.name+", "+s.name, func(t *testing.T) {
				sessionEnv := session(t, pooledConfig(), s.from)
				h := within(t, sessionEnv, sessionEnv, scratchpad)

				err := tt.launch(h.launcher)
				if s.wantWhy == "" {
					if err != nil || len(h.starts) != 1 {
						t.Errorf("launch error = %v, starting %q; want claude started", err, h.starts)
					}
					return
				}
				if !errors.Is(err, launch.ErrNotStarted) || len(h.starts) > 0 {
					t.Errorf("launch error = %v, starting %q; want %v, nothing started", err, h.starts, launch.ErrNotStarted)
				}
				if said, want := h.stderr.String(), tt.said+s.wantWhy+" — not starting claude\n"; said != want {
					t.Errorf("said %q on stderr, want %q", said, want)
				}
			})
		}
	}
}

func TestASessionsProjectIsNamedRoutedOrDirectAndGoesWhereSwitchboardStepsAside(t *testing.T) {
	for _, tt := range []struct {
		name   string
		router *fakeRouter
	}{
		{name: "routed", router: knowing(pooledConfig())},
		{name: "direct", router: notRunning()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, "HOME=/home/tester", "SWITCHBOARD_PROJECT=/home/tester/Code/mine")
			h.launcher.Dir, h.launcher.Project = acmeDir+"/web", repositories
			if err := h.launcher.Run(t.Context(), pooledRoute(pooledConfig(), tt.router, ""), nil); err != nil {
				t.Fatalf("Run() error = %v; said %q", err, h.stderr.String())
			}
			if got := h.environment(t)["SWITCHBOARD_PROJECT"]; got != acmeDir {
				t.Errorf("started with SWITCHBOARD_PROJECT %q, want %q, the repository's path", got, acmeDir)
			}

			aside := newHarness(t, append(h.only(t).session(), "ANTHROPIC_API_KEY=test-key")...)
			if err := aside.launcher.StepAside(nil, "ANTHROPIC_API_KEY"); err != nil {
				t.Fatalf("StepAside() error = %v", err)
			}
			if got := aside.environment(t)["SWITCHBOARD_PROJECT"]; got != "/home/tester/Code/mine" {
				t.Errorf("started stepping aside with SWITCHBOARD_PROJECT %q, want the user's own back, %q", got, "/home/tester/Code/mine")
			}
			own := envMap(launch.OwnLogin(h.only(t).session()))
			if got, named := own["SWITCHBOARD_PROJECT"]; named {
				t.Errorf("started on its own login with SWITCHBOARD_PROJECT %q, want none", got)
			}
		})
	}
}
