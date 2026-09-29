package launch

import (
	"slices"
	"strconv"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/router"
)

// startedEnv marks the environment of each claude switchboard starts in its
// own place, as "<pid>:<path>": switchboard's process id, which the claude
// keeps, as exec keeps it, and where the claude is. A switchboard started
// with the mark naming its own process id was started again in that claude's
// place, as by a wrapper named claude that runs switchboard with exec, and
// looks for claude past it rather than start it again, and again. Any other,
// such as one started within a Claude Code session, has a process id of its
// own, and looks everywhere, marking the claude it starts afresh.
const startedEnv = "SWITCHBOARD_STARTED"

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
// starts in, in the place of the switchboard whose process id is pid, in
// place of any mark it held.
func (e environ) startingAt(pid int, path string) environ {
	return e.with(startedEnv, strconv.Itoa(pid)+":"+path)
}

// startedAt returns where the claude is that the switchboard whose process
// id is pid started, as the environment's mark says, when this is that
// switchboard, started again in the claude's place: "" when it isn't.
func (e environ) startedAt(pid int) string {
	id, path, _ := strings.Cut(e.get(startedEnv), ":")
	if id != strconv.Itoa(pid) {
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
	var headers []string
	for line := range strings.Lines(e.get(claude.CustomHeadersEnv)) {
		line = strings.TrimRight(line, "\r\n")
		name, _, _ := strings.Cut(line, ":")
		if strings.TrimSpace(line) != "" && !strings.EqualFold(strings.TrimSpace(name), router.PinHeader) {
			headers = append(headers, line)
		}
	}
	if account != "" {
		headers = append(headers, router.PinHeader+": "+account)
	}
	if len(headers) == 0 {
		return e.without(claude.CustomHeadersEnv)
	}
	return e.with(claude.CustomHeadersEnv, strings.Join(headers, "\n"))
}
