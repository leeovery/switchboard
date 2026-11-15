package launch_test

import (
	"errors"
	"maps"
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

// processes are the processes running, by id, as ProcessTable tells of them.
type processes map[int]process

// process is a process running: the id of the process that started it,
// when it started, the id of the user it runs as, and switchboard's
// variables of the environment it started in, which can't be read where it's
// unread.
type process struct {
	parent  int
	started time.Time
	owner   int
	environ []string
	unread  bool
}

func (p processes) Process(pid int) (launch.Process, bool) {
	found, ok := p[pid]
	return launch.Process{Parent: found.parent, Started: found.started, Owner: found.owner}, ok
}

func (p processes) Environ(pid int) ([]string, bool) {
	found, ok := p[pid]
	return found.environ, ok && !found.unread
}

// switchboards keeps switchboard's variables of env, as ProcessTable reads
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

// session starts a session in dir, on cfg, from the environment own, as a
// launcher whose process id is pid, and returns the environment its claude
// runs in, whole.
func session(t *testing.T, cfg *config.Config, dir string, own ...string) []string {
	t.Helper()
	h := newHarness(t, append([]string{"HOME=/home/tester"}, own...)...)
	h.launcher.Dir, h.launcher.Project = dir, repositories
	if err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil); err != nil {
		t.Fatalf("starting the session: Run() error = %v; said %q", err, h.stderr.String())
	}
	return h.only(t).inherited()
}

// sessionTable is the processes running for a claude started within the
// session whose claude's environment is sessionEnv: its own, withinPID, the
// shell that started it, and the session's claude, pid, which started a
// second before its mark says.
func sessionTable(sessionEnv []string) processes {
	return processes{
		withinPID: {parent: shellPID, started: launched.Add(time.Minute)},
		shellPID:  {parent: pid, started: launched.Add(time.Second)},
		pid:       {parent: 1, started: launched.Add(-time.Second), environ: switchboards(sessionEnv)},
	}
}

// within is a harness for a claude started in dir, in the environment
// environ, by a shell that the session's claude runs, whose environment is
// sessionEnv, as sessionTable has them: its own process id is withinPID.
func within(t *testing.T, environ, sessionEnv []string, dir string) *harness {
	t.Helper()
	h := newHarness(t, environ...)
	h.launcher.Dir, h.launcher.Project, h.launcher.PID = dir, repositories, withinPID
	h.launcher.Processes = sessionTable(sessionEnv)
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

// runs runs h's launcher on cfg, routed, failing the test where it doesn't
// start claude.
func runs(t *testing.T, h *harness, cfg *config.Config) {
	t.Helper()
	if err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), ""), nil); err != nil {
		t.Fatalf("Run() error = %v; said %q", err, h.stderr.String())
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
			name:  "in no repository in its own pool's fence, from a fenced pool's session: its pool and project",
			pools: config.Pools{defaultPool, acmePool, clientPool}, from: "/Volumes/Client/notes", dir: "/Volumes/Client/drafts",
			wantPool: clientPool.ID, wantToken: sideToken, wantProject: "/Volumes/Client/notes",
		},
		{
			name: "in a repository in a fence, from the unfenced pool's session: the fence's pool, as a fence keeps its own work", from: elsewhere, dir: acmeDir + "/web",
			wantPool: acmePool.ID, wantToken: acme1Token, wantProject: acmeDir,
		},
		{
			name:  "in no repository in a fence, from the unfenced pool's session: placed as if it weren't within it, the fence's pool, its own project",
			pools: config.Pools{defaultPool, acmePool, clientPool}, from: elsewhere, dir: "/Volumes/Client/drafts",
			wantPool: clientPool.ID, wantToken: sideToken, wantProject: "/Volumes/Client/drafts",
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

// A shell or direnv that names the pool, or the project, already, as run
// would set them, leaves run nothing to change: what run set is marked all
// the same, so a claude started within the session finds them, whether the
// environment of the session's claude is read or, where it can't be, the
// one inherited.
func TestASessionStartedWhereTheEnvironmentNamedItsPoolAlreadyPassesItOn(t *testing.T) {
	cfg := pooledConfig()
	for _, tt := range []struct {
		name string
		own  []string
	}{
		{name: "its pool", own: []string{"SWITCHBOARD_POOL=" + acmePool.ID}},
		{name: "its pool and project", own: []string{"SWITCHBOARD_POOL=" + acmePool.ID, "SWITCHBOARD_PROJECT=" + acmeDir}},
	} {
		for _, read := range []struct {
			name   string
			unread bool
		}{
			{name: "its claude's environment read"},
			{name: "its claude's environment unread, the one inherited read", unread: true},
		} {
			t.Run(tt.name+", "+read.name, func(t *testing.T) {
				sessionEnv := session(t, cfg, acmeDir, tt.own...)
				h := within(t, sessionEnv, sessionEnv, scratchpad)
				table := sessionTable(sessionEnv)
				table[pid] = process{parent: 1, started: launched.Add(-time.Second), environ: switchboards(sessionEnv), unread: read.unread}
				h.launcher.Processes = table

				runs(t, h, cfg)
				placedIn(t, h.environment(t), acmePool.ID, acme1Token, acmeDir)
			})
		}
	}
}

// What the session's claude started with is what counts, which nothing run
// within the session can change: a pool or project changed since in the
// environment the launch inherits counts for nothing.
func TestASessionsPoolChangedSinceInTheEnvironmentInheritedIsStillItsPool(t *testing.T) {
	cfg := configOf(config.Pools{defaultPool, acmePool, clientPool})
	sessionEnv := session(t, cfg, acmeDir)
	changed := slices.DeleteFunc(slices.Clone(sessionEnv), func(v string) bool {
		return strings.HasPrefix(v, "SWITCHBOARD_POOL=") || strings.HasPrefix(v, "SWITCHBOARD_PROJECT=")
	})
	h := within(t, append(changed, "SWITCHBOARD_POOL="+clientPool.ID, "SWITCHBOARD_PROJECT=/Volumes/Client/notes"), sessionEnv, scratchpad)

	runs(t, h, cfg)
	placedIn(t, h.environment(t), acmePool.ID, acme1Token, acmeDir)
}

// Where the environment the session's claude started in can't be read, or
// what's read of it holds no session, as where a process title was written
// over it, the one the launch inherits says what the session is, as run set
// it, judged alike: a pool or project changed since there is none
// of the session's, and a session naming no pool places no launch among
// pools.
func TestASessionsClaudeWhoseEnvironmentCantBeReadIsFoundThroughTheOneInherited(t *testing.T) {
	cfg := pooledConfig()
	sessionEnv := session(t, cfg, acmeDir)
	without := func(names ...string) []string {
		return slices.DeleteFunc(slices.Clone(sessionEnv), func(v string) bool {
			name, _, _ := strings.Cut(v, "=")
			return slices.Contains(names, name)
		})
	}
	started := launched.Add(-time.Second)
	reads := []struct {
		name string
		// claude is the session's claude, as it's read.
		claude process
	}{
		{name: "its claude's environment unread", claude: process{parent: 1, started: started, unread: true}},
		{name: "its claude's environment read as none", claude: process{parent: 1, started: started, environ: []string{}}},
		{name: "its claude's environment read without its mark of the claude", claude: process{parent: 1, started: started, environ: switchboards(without(markEnv))}},
		{
			name:   "its claude's environment read as its mark of the claude alone, what run set taken",
			claude: process{parent: 1, started: started, environ: slices.DeleteFunc(slices.Clone(sessionEnv), func(v string) bool { return !strings.HasPrefix(v, markEnv+"=") })},
		},
		{
			name:   "its claude's environment read with a mark of the claude naming another",
			claude: process{parent: 1, started: started, environ: switchboards(append(without(markEnv), markEnv+"="+mark(sessionPID, launched, "/opt/claude")))},
		},
	}
	tests := []struct {
		name    string
		environ []string
		// inSession is whether it's found within the session started in
		// Acme, rather than placed by its scratchpad alone.
		inSession bool
	}{
		{name: "the session's environment inherited", environ: sessionEnv, inSession: true},
		{name: "an environment cleaned", environ: []string{"HOME=/home/tester"}},
		{name: "the session's environment, its pool changed since", environ: append(without("SWITCHBOARD_POOL"), "SWITCHBOARD_POOL="+defaultPool.ID)},
		{name: "the session's environment, its pool unset since", environ: without("SWITCHBOARD_POOL")},
	}
	for _, read := range reads {
		for _, tt := range tests {
			t.Run(read.name+", "+tt.name, func(t *testing.T) {
				h := within(t, tt.environ, sessionEnv, scratchpad)
				table := sessionTable(sessionEnv)
				table[pid] = read.claude
				h.launcher.Processes = table

				runs(t, h, cfg)
				if tt.inSession {
					placedIn(t, h.environment(t), acmePool.ID, acme1Token, acmeDir)
					return
				}
				placedIn(t, h.environment(t), defaultPool.ID, workToken, scratchpad)
			})
		}
	}
}

func TestALaunchWithItsEnvironmentCleanedIsFoundWithinItsSessionThroughWhatStartedIt(t *testing.T) {
	cfg := pooledConfig()
	sessionEnv := session(t, cfg, acmeDir)
	tests := []struct {
		name string
		// table changes the processes running, as sessionTable gives them,
		// where it isn't nil.
		table func(p processes)
		// inSession is whether it's found within the session started in
		// Acme, rather than placed by its scratchpad alone.
		inSession bool
	}{
		{name: "the session's claude one of the processes that started it", inSession: true},
		{
			name: "the session's claude one of them, a process between holding the session's environment",
			table: func(p processes) {
				p[shellPID] = process{parent: pid, started: launched.Add(time.Second), environ: switchboards(sessionEnv)}
			},
			inSession: true,
		},
		{
			name: "a process that started it holding the environment of a session still running, as a tmux server started within it, but the session's claude not among them",
			table: func(p processes) {
				p[shellPID] = process{parent: 1, started: launched.Add(time.Second), environ: switchboards(sessionEnv)}
			},
		},
		{
			name: "a process that started it holding the environment of a session that has ended, as a tmux server outliving it",
			table: func(p processes) {
				delete(p, pid)
				p[shellPID] = process{parent: 1, started: launched.Add(time.Second), environ: switchboards(sessionEnv)}
			},
		},
		{
			name: "none of the processes that started it within a session",
			table: func(p processes) {
				p[shellPID] = process{parent: 1, started: launched.Add(time.Second)}
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
			table := sessionTable(sessionEnv)
			if tt.table != nil {
				tt.table(table)
			}
			h.launcher.Processes = table

			runs(t, h, cfg)
			env := h.environment(t)
			if tt.inSession {
				placedIn(t, env, acmePool.ID, acme1Token, acmeDir)
				return
			}
			placedIn(t, env, defaultPool.ID, workToken, scratchpad)
		})
	}
}

// The mark of the claude switchboard started gives the second it started,
// its fraction cut off, so a session's claude can have started within the
// second after it says.
func TestASessionsClaudeStartedWithinTheSecondItsMarkSaysIsItsClaude(t *testing.T) {
	cfg := pooledConfig()
	sessionEnv := session(t, cfg, acmeDir)
	for _, tt := range []struct {
		name string
		// after is how long after its mark's second the session's claude
		// started.
		after     time.Duration
		inSession bool
	}{
		{name: "half a second after its mark's second", after: 500 * time.Millisecond, inSession: true},
		{name: "a second and a half after", after: 1500 * time.Millisecond},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := within(t, []string{"HOME=/home/tester"}, sessionEnv, scratchpad)
			table := sessionTable(sessionEnv)
			table[shellPID] = process{parent: pid, started: launched.Add(10 * time.Second)}
			table[pid] = process{parent: 1, started: launched.Add(tt.after), environ: switchboards(sessionEnv)}
			h.launcher.Processes = table

			runs(t, h, cfg)
			if tt.inSession {
				placedIn(t, h.environment(t), acmePool.ID, acme1Token, acmeDir)
				return
			}
			placedIn(t, h.environment(t), defaultPool.ID, workToken, scratchpad)
		})
	}
}

// A process another user runs is never a session's claude, though the
// environment the launch inherits names it, as through sudo -E, where what
// that process started with can't be read.
func TestAProcessAnotherUserRunsIsNoSessionsClaude(t *testing.T) {
	const user, other = 501, 502
	cfg := pooledConfig()
	sessionEnv := session(t, cfg, acmeDir)
	for _, tt := range []struct {
		name string
		// owner runs the session's claude, and user the rest.
		owner     int
		inSession bool
	}{
		{name: "the session's claude run by the launch's user", owner: user, inSession: true},
		{name: "the session's claude run by another user", owner: other},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := within(t, sessionEnv, sessionEnv, scratchpad)
			h.launcher.Processes = processes{
				withinPID: {parent: shellPID, started: launched.Add(time.Minute), owner: user},
				shellPID:  {parent: pid, started: launched.Add(time.Second), owner: user},
				pid:       {parent: 1, started: launched.Add(-time.Second), owner: tt.owner, unread: true},
			}

			runs(t, h, cfg)
			if tt.inSession {
				placedIn(t, h.environment(t), acmePool.ID, acme1Token, acmeDir)
				return
			}
			placedIn(t, h.environment(t), defaultPool.ID, workToken, scratchpad)
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
				h.launcher.Processes = processes{withinPID: {parent: shellPID}, shellPID: {parent: 1}}
			},
		},
		{
			name: "its claude's process id taken since by a later process, one that started it",
			change: func(h *harness) {
				h.launcher.Processes = processes{
					withinPID: {parent: pid, started: launched.Add(time.Minute)},
					pid:       {parent: 1, started: launched.Add(2 * time.Second), environ: switchboards(sessionEnv)},
				}
			},
		},
		{
			name: "the id of a process that started it taken, as the processes were read, by its claude, which started after the process it's read as starting",
			change: func(h *harness) {
				h.launcher.Processes = processes{
					withinPID: {parent: shellPID, started: launched.Add(time.Minute)},
					shellPID:  {parent: pid, started: launched.Add(-2 * time.Second)},
					pid:       {parent: 1, started: launched.Add(-time.Second), environ: switchboards(sessionEnv)},
				}
			},
		},
		{
			name: "its claude running, but not among the processes that started it, as where they're a tmux server's started within the session",
			change: func(h *harness) {
				h.launcher.Processes = processes{
					withinPID: {parent: shellPID},
					shellPID:  {parent: 1},
					pid:       {parent: 1, started: launched.Add(-time.Second), environ: switchboards(sessionEnv)},
				}
			},
		},
		{
			name: "naming this process, a switchboard started again in its own claude's place",
			change: func(h *harness) {
				h.launcher.PID = pid
				h.launcher.Processes = processes{pid: {parent: 1, started: launched.Add(-time.Second), environ: switchboards(sessionEnv)}}
			},
		},
		{
			name:   "no way to tell which processes run",
			change: func(h *harness) { h.launcher.Processes = nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := within(t, sessionEnv, sessionEnv, scratchpad)
			tt.change(h)

			runs(t, h, cfg)
			placedIn(t, h.environment(t), defaultPool.ID, workToken, scratchpad)
		})
	}
}

// A claude a local subcommand runs, as Local starts it, carries on the
// session's mark of what run set, made for the session's claude: it's never
// a session's claude itself, whether it's started within a session, which
// is found past it, or where a session's mark lives on without its claude,
// as in a tmux server started within it.
func TestAClaudeStartedForALocalSubcommandIsNoSessionsClaude(t *testing.T) {
	const (
		localPID  = 8001
		nestedPID = 9001
	)
	cfg := pooledConfig()
	sessionEnv := session(t, cfg, acmeDir)
	local := newHarness(t, sessionEnv...)
	local.launcher.PID = localPID
	if err := local.launcher.Local([]string{"mcp", "serve"}); err != nil {
		t.Fatalf("Local() error = %v", err)
	}
	localEnv := local.only(t).inherited()
	tests := []struct {
		name string
		// above is the processes above the local claude.
		above     processes
		inSession bool
	}{
		{
			name: "within the session",
			above: processes{
				shellPID: {parent: pid, started: launched.Add(-time.Second)},
				pid:      {parent: 1, started: launched.Add(-time.Minute), environ: switchboards(sessionEnv)},
			},
			inSession: true,
		},
		{
			name: "in a tmux server's pane, the server started within the session",
			above: processes{
				shellPID: {parent: 1, started: launched.Add(-time.Second), environ: switchboards(sessionEnv)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, localEnv...)
			h.launcher.Dir, h.launcher.Project, h.launcher.PID = scratchpad, repositories, nestedPID
			table := processes{
				nestedPID: {parent: localPID, started: launched.Add(time.Minute)},
				localPID:  {parent: shellPID, started: launched, environ: switchboards(localEnv)},
			}
			maps.Copy(table, tt.above)
			h.launcher.Processes = table

			runs(t, h, cfg)
			if tt.inSession {
				placedIn(t, h.environment(t), acmePool.ID, acme1Token, acmeDir)
				return
			}
			placedIn(t, h.environment(t), defaultPool.ID, workToken, scratchpad)
		})
	}
}

// A claude started in a session's environment, as Local starts one, or as an
// unaided launch passes the session's on, that took the id of the session's
// claude since ended, carries on a mark of what run set made for that claude,
// which started at another second: it's none of the session's.
func TestAClaudeThatTookTheIdOfASessionsClaudeSinceEndedIsNoSessionsClaude(t *testing.T) {
	const nestedPID = 9001
	later := launched.Add(time.Hour)
	tests := []struct {
		name string
		// from is where the session starts, and start how the claude that
		// took the id of its claude starts, in its environment.
		from  string
		start func(l launch.Launcher) error
	}{
		{name: "for a local subcommand", from: acmeDir, start: func(l launch.Launcher) error { return l.Local([]string{"mcp", "serve"}) }},
		{
			name: "unaided, the session's environment passed on", from: elsewhere,
			start: func(l launch.Launcher) error {
				return l.Unaided(nil, config.Pools{defaultPool, acmePool}, launch.CouldntReadConfig, errUnread)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := pooledConfig()
			sessionEnv := session(t, cfg, tt.from)
			again := newHarness(t, sessionEnv...)
			again.launcher.Dir, again.launcher.Now = tt.from, func() time.Time { return later }
			if err := tt.start(again.launcher); err != nil {
				t.Fatalf("starting the claude that took the id: error = %v; said %q", err, again.stderr.String())
			}
			againEnv := again.only(t).inherited()
			h := newHarness(t, againEnv...)
			h.launcher.Dir, h.launcher.Project, h.launcher.PID = scratchpad, repositories, nestedPID
			h.launcher.Processes = processes{
				nestedPID: {parent: pid, started: later.Add(time.Minute)},
				pid:       {parent: shellPID, started: later, environ: switchboards(againEnv)},
				shellPID:  {parent: 1, started: launched.Add(30 * time.Minute)},
			}

			runs(t, h, cfg)
			placedIn(t, h.environment(t), defaultPool.ID, workToken, scratchpad)
		})
	}
}

// A session's mark of what run set names its claude by the second its mark
// of the claude does, though the clock moves on as run starts it.
func TestASessionsMarksNameItsClaudeAlike(t *testing.T) {
	cfg := pooledConfig()
	h := newHarness(t, "HOME=/home/tester")
	h.launcher.Dir, h.launcher.Project = acmeDir, repositories
	now := launched
	h.launcher.Now = func() time.Time {
		now = now.Add(time.Second)
		return now
	}
	runs(t, h, cfg)
	sessionEnv := h.only(t).inherited()
	nested := within(t, sessionEnv, sessionEnv, scratchpad)
	table := sessionTable(sessionEnv)
	table[pid] = process{parent: 1, started: launched, environ: switchboards(sessionEnv)}
	nested.launcher.Processes = table

	runs(t, nested, cfg)
	placedIn(t, nested.environment(t), acmePool.ID, acme1Token, acmeDir)
}

// counted is processes, counting how many processes it's asked about, and
// finding none past most, so a walk that never ends fails rather than hangs.
type counted struct {
	processes
	asked, most int
}

func (c *counted) Process(pid int) (launch.Process, bool) {
	c.asked++
	if c.asked > c.most {
		return launch.Process{}, false
	}
	return c.processes.Process(pid)
}

// The walk up through the processes that started a launch ends, 64 past the
// launch's own at most, though what's read of them runs in a circle, as a
// process table that changes as it's read can.
func TestTheWalkThroughTheProcessesThatStartedALaunchEnds(t *testing.T) {
	cfg := pooledConfig()
	sessionEnv := session(t, cfg, acmeDir)
	h := within(t, sessionEnv, sessionEnv, scratchpad)
	table := &counted{processes: processes{
		withinPID:     {parent: shellPID},
		shellPID:      {parent: innerShellPID},
		innerShellPID: {parent: shellPID},
	}, most: 1000}
	h.launcher.Processes = table

	runs(t, h, cfg)
	placedIn(t, h.environment(t), defaultPool.ID, workToken, scratchpad)
	if table.asked != 65 {
		t.Errorf("asked of %d processes, want 65: the launch's own, and 64 that started it", table.asked)
	}
}

// chain is the processes running for a claude started in a scratchpad within
// a session, second, whose environment is secondEnv, that's started within
// another, first, whose environment is firstEnv.
func chain(firstEnv, secondEnv []string) processes {
	return processes{
		innerWithinPID: {parent: innerShellPID, started: launched.Add(2 * time.Minute)},
		innerShellPID:  {parent: withinPID, started: launched.Add(time.Minute)},
		withinPID:      {parent: shellPID, started: launched.Add(-time.Second), environ: switchboards(secondEnv)},
		shellPID:       {parent: pid, started: launched.Add(-2 * time.Second)},
		pid:            {parent: 1, started: launched.Add(-3 * time.Second), environ: switchboards(firstEnv)},
	}
}

// last is a harness for a claude started in a scratchpad within second, as
// chain has it.
func last(t *testing.T, firstEnv, secondEnv []string) *harness {
	t.Helper()
	h := newHarness(t, secondEnv...)
	h.launcher.Dir, h.launcher.Project, h.launcher.PID = innerScratchpad, repositories, innerWithinPID
	h.launcher.Processes = chain(firstEnv, secondEnv)
	return h
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
			runs(t, second, cfg)
			h := last(t, first, second.only(t).inherited())

			runs(t, h, cfg)
			placedIn(t, h.environment(t), tt.wantPool, tt.wantToken, tt.wantProject)
		})
	}
}

// A session that names no pool, as one started while the config had none,
// places no launch among pools: one started within it on a config with pools
// is placed through the session around it that names one, its pool and
// project alike; on a config without, it runs in the nearest's project.
func TestASessionNamingNoPoolPlacesNoLaunchAmongPools(t *testing.T) {
	cfg := pooledConfig()
	first := session(t, cfg, acmeDir)
	second := within(t, first, first, tool)
	if err := second.launcher.Run(t.Context(), route(healthy(), ""), nil); err != nil {
		t.Fatalf("starting the second without pools: Run() error = %v; said %q", err, second.stderr.String())
	}
	secondEnv := second.only(t).inherited()

	t.Run("on a config with pools", func(t *testing.T) {
		h := last(t, first, secondEnv)
		runs(t, h, cfg)
		placedIn(t, h.environment(t), acmePool.ID, acme1Token, acmeDir)
	})
	t.Run("on a config without", func(t *testing.T) {
		h := last(t, first, secondEnv)
		if err := h.launcher.Run(t.Context(), route(healthy(), ""), nil); err != nil {
			t.Fatalf("Run() error = %v; said %q", err, h.stderr.String())
		}
		if got := h.environment(t)["SWITCHBOARD_PROJECT"]; got != tool {
			t.Errorf("started in the project %q, want the nearest session's, %q", got, tool)
		}
	})
}

// A pin to an account of another pool is refused, saying where the session's
// pool comes from: through the session it's started within, where that
// session alone decides it, its pool fenced and no fence holding the
// directory.
func TestAPinWithinASessionToAnotherPoolsAccountSaysWhereItsPoolComesFrom(t *testing.T) {
	cfg := pooledConfig()
	for _, tt := range []struct {
		// from is where the session starts, dir where the claude within it
		// does, pinned to account.
		name, from, dir, account, wantErr string
	}{
		{
			name: "in a scratchpad, within a fenced pool's session", from: acmeDir, dir: scratchpad, account: "work",
			wantErr: "work is in pool Default; this directory is in pool Acme, through the session it's started within",
		},
		{
			name: "in its pool's fence, within a fenced pool's session", from: acmeDir, dir: acmeDir + "/web", account: "work",
			wantErr: "work is in pool Default; this directory is in pool Acme",
		},
		{
			name: "in a scratchpad, within the unfenced pool's session, where it would be anyway", from: elsewhere, dir: scratchpad, account: "acme1",
			wantErr: "Acme 1 (acme1) is in pool Acme; this directory is in pool Default",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sessionEnv := session(t, cfg, tt.from)
			h := within(t, sessionEnv, sessionEnv, tt.dir)
			if err := h.launcher.Run(t.Context(), pooledRoute(cfg, knowing(cfg), tt.account), nil); err == nil || err.Error() != tt.wantErr {
				t.Errorf("Run() error = %v, want %q", err, tt.wantErr)
			}
			if len(h.starts) > 0 {
				t.Errorf("started %q, want nothing started", h.starts)
			}
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
