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
	var matches []S
	listed := make(map[string]bool)
	for _, s := range sessions {
		switch sid := id(s); {
		case sid == given:
			return given, nil
		case strings.HasPrefix(sid, given) && !listed[sid]:
			listed[sid] = true
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		return given, nil
	case 1:
		return id(matches[0]), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "session %s could be any of these, so give more of its id:", status.Clean(redact.Text(given)))
	for _, s := range matches {
		fmt.Fprintf(&b, "\n  %s", line(s))
	}
	return "", errors.New(b.String())
}
