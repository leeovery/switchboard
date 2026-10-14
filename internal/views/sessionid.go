package views

import (
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// SessionNamed is the session an id given names among sessions, by as much
// of its id as is unique among theirs, a session coming as often as it may:
// what every verb that takes a session names it by.
type SessionNamed struct {
	// ID is the session's whole id: given, where it's one of theirs whole,
	// or starts none of theirs, as the whole id of a session they don't
	// hold; else the one alone whose id it starts. It's "" where it starts
	// several.
	ID string
	// Several name each session whose id given starts, where it starts
	// several, in the order they first came: none where it names one.
	Several []string
}

// NamedAmong is the session given names among those with the given ids,
// each named, where it starts several, by its id, cleaned.
func NamedAmong(given string, ids []string) SessionNamed {
	return namedAmong(given, ids, func(id string) string { return id }, prose.Clean)
}

// NamedRouted is the session given names among the sessions the router
// lists, each named, where it starts several, by its line, as status's text
// lists it at now.
func NamedRouted(given string, sessions []status.Session, now time.Time) SessionNamed {
	return namedAmong(given, sessions,
		func(s status.Session) string { return s.ID },
		func(s status.Session) string { return s.Line(now) })
}

// Routed is the session with the given id among those the router lists:
// nil where it doesn't list it.
func Routed(running []status.Session, id string) *status.Session {
	i := slices.IndexFunc(running, func(s status.Session) bool { return s.ID == id })
	if i < 0 {
		return nil
	}
	return &running[i]
}

// NamedLines is the session given names among those lines are of, as
// NamedAmong names it among their ids, and its lines, in order: none where
// given starts several. It reads lines once, holding the lines of one
// session at most, as lineHold does.
func NamedLines(lines iter.Seq[ledger.Held], given string) (SessionNamed, []ledger.Held) {
	h := lineHold{sessionNaming: newSessionNaming(given)}
	for l := range lines {
		h.add(l)
	}
	if id, ok := h.named(); ok {
		return SessionNamed{ID: id}, h.held
	}
	several := make([]string, len(h.ids))
	for i, id := range h.ids {
		several[i] = prose.Clean(id)
	}
	return SessionNamed{Several: several}, nil
}

// namedAmong is the session given names among sessions, by their ids as id
// gives them, each named, where given starts several, as line names it.
func namedAmong[S any](given string, sessions []S, id, line func(S) string) SessionNamed {
	n := newSessionNaming(given)
	var matched []S
	for _, s := range sessions {
		if n.note(id(s)) {
			matched = append(matched, s)
		}
	}
	if named, ok := n.named(); ok {
		return SessionNamed{ID: named}
	}
	several := make([]string, len(matched))
	for i, s := range matched {
		several[i] = line(s)
	}
	return SessionNamed{Several: several}
}

// sessionNaming is what's known, as sessions come, of the session an id
// given names among them: the ids of those whose ids it starts, each once,
// in the order they came.
type sessionNaming struct {
	given string
	seen  map[string]bool
	ids   []string
}

// newSessionNaming returns what's known of the session given names before
// any session comes.
func newSessionNaming(given string) *sessionNaming {
	return &sessionNaming{given: given, seen: make(map[string]bool)}
}

// note notes the session with the given id, reporting whether given starts
// it, and it hadn't come before.
func (n *sessionNaming) note(id string) bool {
	if n.seen[id] || !strings.HasPrefix(id, n.given) {
		return false
	}
	n.seen[id] = true
	n.ids = append(n.ids, id)
	return true
}

// named returns the id of the session given names among those noted: given,
// where it's one of their ids whole, or starts none of them; else the one
// alone it starts; and false where it starts several.
func (n *sessionNaming) named() (string, bool) {
	switch {
	case n.seen[n.given] || len(n.ids) == 0:
		return n.given, true
	case len(n.ids) == 1:
		return n.ids[0], true
	}
	return "", false
}

// lineHold holds, as lines are read, the lines of the session given names
// among theirs so far, as sessionNaming says: those of one session at most,
// those of the session given names once every line is read.
type lineHold struct {
	*sessionNaming
	holding string
	held    []ledger.Held
}

// add takes the line l in, holding it where it's of the session named so
// far, and letting go of those held where that's no longer the session
// they're of.
func (h *lineHold) add(l ledger.Held) {
	h.note(l.Session)
	if id, _ := h.named(); id != h.holding {
		h.holding, h.held = id, nil
	}
	if l.Session == h.holding {
		h.held = append(h.held, l)
	}
}
