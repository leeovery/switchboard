package launch

import (
	"time"

	"github.com/leeovery/switchboard/internal/config"
)

// Processes tells of the processes running, as ProcessTable does.
type Processes interface {
	// Process returns what's known of the running process with the given
	// id: false where none runs.
	Process(pid int) (Process, bool)
	// Environ returns switchboard's variables of the environment the process
	// with the given id started in, as os.Environ gives each, and never
	// another, which can hold a token: false where it can't be read.
	Environ(pid int) ([]string, bool)
}

// ProcessTable is the system's processes, as Processes tells of them: only
// macOS's are read.
type ProcessTable struct{}

// Process is what's known of a running process: the id of the process that
// started it, when it started, and the id of the user it runs as.
type Process struct {
	Parent  int
	Started time.Time
	Owner   int
}

// session is a session a launch is started within, as sessionOf finds it:
// the process id of its claude, and the id of the pool it's in and the
// project it runs in, each "" where it names none. The zero session is none.
type session struct {
	pid           int
	pool, project string
}

// attrs are what the log says of s, where it's a session: the process id of
// its claude.
func (s session) attrs() []any {
	if s.pid == 0 {
		return nil
	}
	return []any{"within", s.pid}
}

// outerSessions are the sessions a launch is started within, as findOuter
// finds them: the nearest, and the nearest naming a pool, each none where
// there's no such session.
type outerSessions struct {
	nearest, pooled session
}

// with returns o knowing s, the next session out from those o knows.
func (o outerSessions) with(s session) outerSessions {
	if o.nearest.pid == 0 {
		o.nearest = s
	}
	if o.pooled.pid == 0 && s.pool != "" {
		o.pooled = s
	}
	return o
}

// among returns the session a launch among pools is started within: the
// nearest naming a pool, where there are pools, as one naming none can't
// place it among them; else the nearest.
func (o outerSessions) among(pools config.Pools) session {
	if len(pools) > 0 {
		return o.pooled
	}
	return o.nearest
}

// markRounding is how much later than its mark says a claude switchboard
// started can have started: the mark gives the second, its fraction cut off.
const markRounding = time.Second

// ancestorsMost bounds the walk up from a launch through the processes that
// started it: deeper than any real tree, so a walk through a process table
// that changes as it's read still ends.
const ancestorsMost = 64

// findOuter returns the sessions a launch is started within, as Processes
// tells of the processes running: those whose claude is among the processes
// that started it, up from its parent, as sessionOf finds them, until one
// names a pool. A session whose claude isn't among them is none, though its
// mark lives on, as in a tmux server started within it, which every claude
// started in it would inherit. A process another user runs is never a
// session's claude, as only the environment the launch inherits could name
// it, as through sudo -E. The walk stops at a process that started after the
// one it started, as no parent does: its id was taken, as the walk went, by
// a later process. None without Processes.
func (l Launcher) findOuter() outerSessions {
	var outer outerSessions
	if l.Processes == nil {
		return outer
	}
	self, ok := l.Processes.Process(l.PID)
	child := self
	for range ancestorsMost {
		if !ok || child.Parent <= 1 || outer.pooled.pid != 0 {
			break
		}
		pid := child.Parent
		parent, running := l.Processes.Process(pid)
		if !running || parent.Started.After(child.Started) {
			break
		}
		if s, found := l.sessionOf(pid, parent, self.Owner); found {
			outer = outer.with(s)
		}
		child = parent
	}
	return outer
}

// sessionOf returns the session whose claude is p, the process whose id is
// pid, and whether it's one: none where a user other than owner runs p; else
// as sessionIn finds it in the environment p started in, which nothing the
// session runs can change; else in the one the launch starts from, as
// inherited, which also gives the pool where the one p started in names
// none. What's read of a process is its memory as it is, which the process
// can write over, as Claude Code's title does under Node, so a read that
// holds no session, as one whose title took what run set, is passed over as
// one that can't be read is, and one that lost the pool alone, as one whose
// title took the variables ahead of the project, takes the inherited one's.
func (l Launcher) sessionOf(pid int, p Process, owner int) (session, bool) {
	if p.Owner != owner {
		return session{}, false
	}
	inherited, inherits := sessionIn(environ(l.Environ), pid, p)
	s, found := l.ownSession(pid, p)
	switch {
	case !found:
		return inherited, inherits
	case s.pool == "" && inherits:
		s.pool = inherited.pool
	}
	return s, true
}

// ownSession returns the session sessionIn finds in the environment p, the
// process whose id is pid, started in: false where it holds none, or can't
// be read.
func (l Launcher) ownSession(pid int, p Process) (session, bool) {
	own, read := l.Processes.Environ(pid)
	if !read {
		return session{}, false
	}
	return sessionIn(environ(own), pid, p)
}

// sessionIn returns the session env says p, the process whose id is pid, is
// the claude of, and whether it's one: where env's mark of the claude
// switchboard started (see startedEnv) names it, and it started no later
// than the mark says, so a process that took the id of a claude since ended
// isn't; where env's mark of what run set was made for that claude (see
// markedFor), as a claude started since in the session's environment, as
// Local starts one, carries on a mark made for another, though it took the
// id of the one it was made for; and where env names the pool or project run
// set (see setByRun).
func sessionIn(env environ, pid int, p Process) (session, bool) {
	m, marked := env.started()
	if !marked || m.pid != pid || p.Started.After(m.at.Add(markRounding)) || !env.markedFor(m) {
		return session{}, false
	}
	s := session{pid: pid, pool: env.setByRun(PoolEnv), project: env.setByRun(projectEnv)}
	return s, s.pool != "" || s.project != ""
}

// outerPool returns the session a launch among pools is started within, as
// among has it, and its pool of pools: false where it's within none, or that
// pool isn't one of pools, as where it's been removed since.
func (l Launcher) outerPool(pools config.Pools) (session, config.Pool, bool) {
	s := l.outer.among(pools)
	if s.pool == "" {
		return session{}, config.Pool{}, false
	}
	p, ok := pools.ByID(s.pool)
	return s, p, ok
}
