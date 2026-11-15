package launch

import (
	"time"

	"github.com/leeovery/switchboard/internal/config"
)

// Process is what's known of a running process, as FindProcess reads it.
type Process struct {
	// Parent is the id of the process that started it, and Started when it
	// started.
	Parent  int
	Started time.Time
	// Environ is switchboard's variables of the environment it started in,
	// as os.Environ gives each: never another, which can hold a token.
	Environ []string
}

// session is the session a launch is started within, as sessionWithin finds
// it: the process id of its claude, and the id of the pool it's in and the
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

// markRounding is how much later than its mark says a claude switchboard
// started can have started: the mark gives the second, its fraction cut off.
const markRounding = time.Second

// ancestorsMost bounds the walk up from a launch through the processes that
// started it: deeper than any real tree, so a walk through a process table
// that changes as it's read still ends.
const ancestorsMost = 64

// withinSession returns l knowing the session it's started within, as
// sessionWithin finds it, so a launch looks once, and every rule it places
// the session by sees the same.
func (l Launcher) withinSession() Launcher {
	l.within = l.sessionWithin()
	return l
}

// sessionWithin returns the session a launch is started within, Process
// telling of the processes running: that of the nearest of the processes
// that started it, up from its parent, that's a session's claude. A process
// is one where an environment's mark of the claude switchboard started (see
// startedEnv) names it, by its id, and it started no later than the mark
// says, so a process that took the id of a claude since ended isn't; and
// where that environment holds the session's pool or project as run set
// them (see setByRun). The environment the launch starts from is looked at
// first, then the process's own, as it started, so a claude started with its
// environment cleaned, as a test harness may start it, is found within the
// session the harness runs in. A session whose claude isn't among the
// processes that started the launch is none, though its mark lives on, as in
// a tmux server started within it, which every claude started in it would
// inherit. None without Process.
func (l Launcher) sessionWithin() session {
	if l.Process == nil {
		return session{}
	}
	inherited := environ(l.Environ)
	p, ok := l.Process(l.PID)
	for range ancestorsMost {
		if !ok || p.Parent <= 1 {
			break
		}
		pid := p.Parent
		if p, ok = l.Process(pid); !ok {
			break
		}
		for _, env := range []environ{inherited, environ(p.Environ)} {
			if s, found := sessionOf(pid, p, env); found {
				return s
			}
		}
	}
	return session{}
}

// sessionOf returns the session env names, where p, the process whose id is
// pid, is its claude, as sessionWithin says, and whether it is.
func sessionOf(pid int, p Process, env environ) (session, bool) {
	m, ok := env.started()
	if !ok || m.pid != pid || p.Started.After(m.at.Add(markRounding)) {
		return session{}, false
	}
	s := session{pid: pid, pool: env.setByRun(PoolEnv), project: env.setByRun(projectEnv)}
	return s, s.pool != "" || s.project != ""
}

// withinPool returns the pool of pools the session the launch is started
// within is in: false where it's within none, or that pool isn't one of
// pools, as where it's been removed since.
func (l Launcher) withinPool(pools config.Pools) (config.Pool, bool) {
	if l.within.pool == "" {
		return config.Pool{}, false
	}
	return pools.ByID(l.within.pool)
}
