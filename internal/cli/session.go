package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// namingASession says, in the help of the commands that take a session, how
// to name one, and where its id is to be found.
const namingASession = `Name a session by its id, or as much of it as is unique among the sessions
routed in the last hour: switchboard status lists them, Claude Code's /status
shows a session's own, and inside one, $CLAUDE_CODE_SESSION_ID holds its id.`

// findSession returns the id of the session given names: its id, or as much
// of it as is unique among the sessions the router has routed in the last
// hour. Given one that starts no such session's id, it returns it as it is,
// as the whole id of a session idle for longer, for the router to say whether
// it has seen it. It fails, listing them, when given starts the ids of
// several.
func (a *app) findSession(ctx context.Context, client *router.Client, given string) (string, error) {
	sessions, err := client.Sessions(ctx)
	if err != nil {
		return "", err
	}
	var matches []status.Session
	for _, s := range sessions {
		switch {
		case s.ID == given:
			return given, nil
		case strings.HasPrefix(s.ID, given):
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		return given, nil
	case 1:
		return matches[0].ID, nil
	}
	return "", ambiguous(given, matches, a.Now())
}

// ambiguous is the error for given, which starts the id of each of sessions,
// listing them as they stand at now.
func ambiguous(given string, sessions []status.Session, now time.Time) error {
	var b strings.Builder
	fmt.Fprintf(&b, "session %s could be any of these, so give more of its id:", status.Clean(given))
	for _, s := range sessions {
		fmt.Fprintf(&b, "\n  %s", s.Line(now))
	}
	return errors.New(b.String())
}
