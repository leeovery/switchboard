package launch

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/redact"
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

// StartedBy reports whether the environment getenv reads is one switchboard
// started a claude in, or something that claude started: whether it carries
// the mark, however old.
func StartedBy(getenv func(string) string) bool {
	return getenv(startedEnv) != ""
}

// setEnv marks the environment of a claude run starts with what run set in
// it, so a claude started within its session that switchboard steps aside
// for, or starts unaided, starts as if switchboard weren't there (see
// unmarked). It holds, query-encoded, the value run gave each of markedVars
// it set, under "set:" and the variable's name, and the user's value it
// replaced, under "had:", and each line it added to Claude Code's custom
// headers, under "header", with the user's headers under "had:" where they
// were set to nothing but blank lines.
const setEnv = "SWITCHBOARD_SET"

// The keys of setEnv's mark.
const (
	setKey    = "set:"
	hadKey    = "had:"
	headerKey = "header"
)

// markedVars are the variables setEnv names, of those run sets. The token
// isn't among them: a user's own, which run replaced, can't be written into
// the mark, and without the value run gave it, a token set since, as by a
// script, couldn't be told from run's.
var markedVars = []string{claude.BaseURLEnv, claude.HintHeadersEnv}

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

// get returns key's value, or "" when it's unset.
func (e environ) get(key string) string {
	value, _ := e.lookup(key)
	return value
}

// lookup returns key's value, and whether it's set at all. Of a key set
// twice, the first counts, as it does for getenv.
func (e environ) lookup(key string) (string, bool) {
	for _, variable := range e {
		if k, value, _ := strings.Cut(variable, "="); k == key {
			return value, true
		}
	}
	return "", false
}

// hinting returns the environment with Claude Code sending the router the
// headers telling of each request that it sends the API directly, such as the
// prompt a request serves, which the request ledger keeps. Where it says
// already whether to send them, as set to 0, which sends them nowhere, it's
// left as it is. Set to "", it says nothing: Claude Code reads it as unset.
func (e environ) hinting() environ {
	if e.get(claude.HintHeadersEnv) != "" {
		return e
	}
	return e.with(claude.HintHeadersEnv, "1")
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

// markedAgainst returns the environment marked with what it sets that own,
// the environment run started from, doesn't (see setEnv), in place of any
// mark it held: each of markedVars it gives a value own didn't, with own's
// value where own had one, and each custom header line own didn't hold, with
// own's custom headers where they were set to nothing but blank lines. The
// base URL is marked even where own held the same value, as its mark is what
// says the environment is still routed (see routedByRun); restoring it then
// changes nothing. The mark sits in the environment of everything the session
// runs, which never held own's replaced values before, so nothing secret is
// written into it: a value run set holding anything token-shaped goes
// unmarked, and stays, and a value of own's holding a secret, as
// redact.HoldsSecret finds one, such as a gateway's URL with credentials in
// it, isn't kept, its variable marked as set where the user had none.
func (e environ) markedAgainst(own environ) environ {
	mark := url.Values{}
	for _, key := range markedVars {
		set, isSet := e.lookup(key)
		had, hadIt := own.lookup(key)
		unchanged := hadIt && had == set && key != claude.BaseURLEnv
		if !isSet || unchanged || redact.HoldsToken(set) {
			continue
		}
		mark.Set(setKey+key, set)
		if hadIt {
			keepHad(mark, key, had)
		}
	}
	ownLines := own.headerLines()
	for _, line := range e.headerLines() {
		if !slices.Contains(ownLines, line) && !redact.HoldsToken(line) {
			mark.Add(headerKey, line)
		}
	}
	if blank, isSet := own.lookup(claude.CustomHeadersEnv); isSet && len(ownLines) == 0 && mark.Has(headerKey) {
		keepHad(mark, claude.CustomHeadersEnv, blank)
	}
	if len(mark) == 0 {
		return e.without(setEnv)
	}
	return e.with(setEnv, mark.Encode())
}

// keepHad writes into mark the user's value of key that run replaced, unless
// it holds a secret.
func keepHad(mark url.Values, key, had string) {
	if !redact.HoldsSecret(had) {
		mark.Set(hadKey+key, had)
	}
}

// unmarked returns the environment without what run set in it, as its mark
// says (see setEnv), and without the mark: each of markedVars still holding
// the value run gave it back to the user's, or unset where the user had none,
// and each custom header line run added taken off, every other staying (see
// restoredHeaders). A variable changed since, as by a script pointing claude
// at a gateway of its own, stays as it is. Without a mark, as from a
// switchboard before marks, or with one that can't be read, nothing else
// changes.
func (e environ) unmarked() environ {
	if _, marked := e.lookup(setEnv); !marked {
		return e
	}
	env := e.without(setEnv)
	mark, ok := e.mark()
	if !ok {
		return env
	}
	for _, key := range markedVars {
		env = env.restored(key, mark)
	}
	return env.restoredHeaders(mark)
}

// restoredHeaders returns the environment with Claude Code's custom headers
// without the lines run added, as mark has them, every other staying. Where
// taking them off leaves none, and the user's were set to nothing but blank
// lines, they go back to that, rather than unset.
func (e environ) restoredHeaders(mark url.Values) environ {
	env := e.withoutHeaderLines(mark[headerKey])
	_, held := e.lookup(claude.CustomHeadersEnv)
	_, holds := env.lookup(claude.CustomHeadersEnv)
	if held && !holds && mark.Has(hadKey+claude.CustomHeadersEnv) {
		return env.with(claude.CustomHeadersEnv, mark.Get(hadKey+claude.CustomHeadersEnv))
	}
	return env
}

// routedByRun reports whether the environment still sends Claude Code to the
// router run sent it to, as its mark says: false without a mark, or where the
// base URL has been changed since, as by a script pointing claude at a gateway
// of its own.
func (e environ) routedByRun() bool {
	mark, ok := e.mark()
	if !ok || !mark.Has(setKey+claude.BaseURLEnv) {
		return false
	}
	value, isSet := e.lookup(claude.BaseURLEnv)
	return isSet && value == mark.Get(setKey+claude.BaseURLEnv)
}

// mark returns what run set in the environment, as its mark says (see
// setEnv): false without a mark, or with one that can't be read.
func (e environ) mark() (url.Values, bool) {
	raw, marked := e.lookup(setEnv)
	if !marked {
		return nil, false
	}
	mark, err := url.ParseQuery(raw)
	return mark, err == nil
}

// restored returns the environment with key back to the user's value, as
// mark has it, or unset where the user had none, while it holds the value
// mark says run gave it.
func (e environ) restored(key string, mark url.Values) environ {
	value, isSet := e.lookup(key)
	if !isSet || !mark.Has(setKey+key) || value != mark.Get(setKey+key) {
		return e
	}
	if mark.Has(hadKey + key) {
		return e.with(key, mark.Get(hadKey+key))
	}
	return e.without(key)
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
	lines := slices.DeleteFunc(e.headerLines(), func(line string) bool {
		name, _, _ := strings.Cut(line, ":")
		return strings.EqualFold(strings.TrimSpace(name), key)
	})
	if value != "" {
		lines = append(lines, key+": "+value)
	}
	return e.withHeaderLines(lines)
}

// withoutHeaderLines returns the environment with Claude Code's custom
// headers without the lines given, every other staying. Where it holds none
// of them, it's left as it is.
func (e environ) withoutHeaderLines(gone []string) environ {
	lines := e.headerLines()
	kept := slices.DeleteFunc(slices.Clone(lines), func(line string) bool {
		return slices.Contains(gone, line)
	})
	if len(kept) == len(lines) {
		return e
	}
	return e.withHeaderLines(kept)
}

// headerLines returns Claude Code's custom headers, a line each, blank lines
// left out.
func (e environ) headerLines() []string {
	var lines []string
	for line := range strings.Lines(e.get(claude.CustomHeadersEnv)) {
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// withHeaderLines returns the environment with Claude Code's custom headers
// the lines given, or without them for none.
func (e environ) withHeaderLines(lines []string) environ {
	if len(lines) == 0 {
		return e.without(claude.CustomHeadersEnv)
	}
	return e.with(claude.CustomHeadersEnv, strings.Join(lines, "\n"))
}
