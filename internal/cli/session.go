package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// namingASession says, in the help of the commands that take a session, how
// to name one, and where its id is to be found.
const namingASession = `Name a session by its id, or as much of it as is unique among the sessions
routed in the last hour: switchboard status lists them, Claude Code's /status
shows a session's own, and inside one, $CLAUDE_CODE_SESSION_ID holds its id.`

// findSession returns the id of the session given names among those the
// router has routed in the last hour, as sessionNamed says, listing them,
// where it starts several, as they stand now. Given one that starts no such
// session's id, it returns it as it is, as the whole id of a session idle for
// longer, for the router to say whether it has seen it.
func (a *app) findSession(ctx context.Context, client *router.Client, given string) (string, error) {
	sessions, err := client.Sessions(ctx)
	if err != nil {
		return "", err
	}
	now := a.Now()
	return sessionNamed(given, sessions,
		func(s status.Session) string { return s.ID },
		func(s status.Session) string { return s.Line(now) })
}

// sessionNamed returns the id of the session given names among sessions, by
// their ids as id gives them, a session coming as often as it may: its id,
// or as much of it as is unique among theirs. Given one that starts none of
// their ids, it returns it as it is. It fails, listing the sessions whose ids
// it starts, in the order they first come, a line each as line gives it,
// when it starts several.
func sessionNamed[S any](given string, sessions []S, id, line func(S) string) (string, error) {
	n := newNaming(given)
	var matches []S
	for _, s := range sessions {
		if n.note(id(s)) {
			matches = append(matches, s)
		}
	}
	if named, ok := n.named(); ok {
		return named, nil
	}
	return "", several(given, matches, line)
}

// naming is what's known, as sessions come, of the session given names among
// them: the ids of those whose ids it starts, each once, in the order they
// came.
type naming struct {
	given string
	seen  map[string]bool
	ids   []string
}

// newNaming returns what's known of the session given names before any
// session comes.
func newNaming(given string) *naming {
	return &naming{given: given, seen: make(map[string]bool)}
}

// note notes the session with the given id, reporting whether given starts
// it, and it hadn't come before.
func (n *naming) note(id string) bool {
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
func (n *naming) named() (string, bool) {
	switch {
	case n.seen[n.given] || len(n.ids) == 0:
		return n.given, true
	case len(n.ids) == 1:
		return n.ids[0], true
	}
	return "", false
}

// several is the error of given starting the ids of several sessions, listing
// them, in the order they came, a line each as line gives it.
func several[S any](given string, sessions []S, line func(S) string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "session %s could be any of these, so give more of its id:", status.Clean(redact.Text(given)))
	for _, s := range sessions {
		fmt.Fprintf(&b, "\n  %s", line(s))
	}
	return errors.New(b.String())
}
