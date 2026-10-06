package launch

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/router"
)

// startedEnv marks the environment of each claude switchboard starts in its
// own place, as "<pid>:<time>:<path>": switchboard's process id, which the
// claude keeps, as exec keeps it, when, in Unix seconds, and where the claude
// is, last, as a path can hold a colon. A switchboard started with the mark
// naming its own process id, within startedFor of it, was started again in
// that claude's place, as by a wrapper named claude that runs switchboard
// with exec, and looks for claude past it rather than start it again, and
// again. Any other, such as one started within a Claude Code session, has a
// process id of its own, and looks everywhere, marking the claude it starts
// afresh.
const startedEnv = "SWITCHBOARD_STARTED"

// startedFor is how long a mark counts. A switchboard started again in the
// place of the claude it started comes within moments of it: milliseconds
// for a wrapper that runs switchboard with exec, a few seconds on a machine
// hard at work. A mark older than that is another process's, whatever its
// id: one that took the id of a claude long gone, the mark living on in the
// environment of something started within that claude's session, such as a
// tmux server, or one running claude again in its own place much later.
const startedFor = 30 * time.Second

// environ is an environment as os.Environ gives it: "key=value", a variable
// each. Its methods return a changed copy, leaving it as it was.
type environ []string

// with returns the environment with key set to value, in place of any value
// it had.
func (e environ) with(key, value string) environ {
	return append(e.without(key), key+"="+value)
}

// without returns the environment without the keys given.
func (e environ) without(keys ...string) environ {
	return slices.DeleteFunc(slices.Clone(e), func(variable string) bool {
		key, _, _ := strings.Cut(variable, "=")
		return slices.Contains(keys, key)
	})
}

// get returns key's value, or "" when it's unset. Of a key set twice, the
// first counts, as it does for getenv.
func (e environ) get(key string) string {
	for _, variable := range e {
		if k, value, _ := strings.Cut(variable, "="); k == key {
			return value
		}
	}
	return ""
}

// startingAt returns the environment marked as the one the claude at path
// starts in at now, in the place of the switchboard whose process id is pid,
// in place of any mark it held.
func (e environ) startingAt(pid int, now time.Time, path string) environ {
	return e.with(startedEnv, strconv.Itoa(pid)+":"+strconv.FormatInt(now.Unix(), 10)+":"+path)
}

// startedAt returns where the claude is that the switchboard whose process
// id is pid started, as the environment's mark says, when this is that
// switchboard, started again in the claude's place, at now, within
// startedFor of starting it: "" when it isn't, or the mark can't be read.
func (e environ) startedAt(pid int, now time.Time) string {
	id, rest, _ := strings.Cut(e.get(startedEnv), ":")
	at, path, _ := strings.Cut(rest, ":")
	started, err := strconv.ParseInt(at, 10, 64)
	if id != strconv.Itoa(pid) || err != nil {
		return ""
	}
	if age := now.Sub(time.Unix(started, 0)); age < 0 || age > startedFor {
		return ""
	}
	return path
}

// pinnedTo returns the environment with Claude Code's custom headers pinning
// its requests to the account with the given id, or to none for "". A pin the
// environment holds already, such as a session's that this one is started
// from within, goes: what this launch pins is the only pin. Any other header
// stays.
func (e environ) pinnedTo(account string) environ {
	return e.withHeader(router.PinHeader, account)
}

// workingIn returns the environment with Claude Code's custom headers telling
// the router the directory it was started in, dir, or none for "". A
// directory the environment names already, such as a session's that this one
// is started from within, goes: what this launch tells is the only directory.
// Any other header stays.
func (e environ) workingIn(dir string) environ {
	return e.withHeader(router.DirHeader, router.EncodeDir(dir))
}

// withHeader returns the environment with Claude Code's custom headers
// holding the header key with value, in place of any line of it they held,
// or without it for "". Any other header stays.
func (e environ) withHeader(key, value string) environ {
	var headers []string
	for line := range strings.Lines(e.get(claude.CustomHeadersEnv)) {
		line = strings.TrimRight(line, "\r\n")
		name, _, _ := strings.Cut(line, ":")
		if strings.TrimSpace(line) != "" && !strings.EqualFold(strings.TrimSpace(name), key) {
			headers = append(headers, line)
		}
	}
	if value != "" {
		headers = append(headers, key+": "+value)
	}
	if len(headers) == 0 {
		return e.without(claude.CustomHeadersEnv)
	}
	return e.with(claude.CustomHeadersEnv, strings.Join(headers, "\n"))
}
