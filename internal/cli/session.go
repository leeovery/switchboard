package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/views"
)

// namingASession says, in the help of the commands that take a session, how
// to name one, and where its id is to be found.
const namingASession = `Name a session by its id, or as much of it as is unique among the sessions
routed in the last hour: switchboard status lists them, Claude Code's /status
shows a session's own, and inside one, $CLAUDE_CODE_SESSION_ID holds its id.`

// findSession returns the id of the session given names among those the
// router has routed in the last hour, as views.NamedRouted names it, listing
// them, where it starts several, as they stand now. Given one that starts no
// such session's id, it returns it as it is, as the whole id of a session
// idle for longer, for the router to say whether it has seen it.
func (a *app) findSession(ctx context.Context, client *router.Client, given string) (string, error) {
	sessions, err := client.Sessions(ctx)
	if err != nil {
		return "", err
	}
	named := views.NamedRouted(given, sessions, a.Now())
	if named.Several != nil {
		return "", several(given, named.Several)
	}
	return named.ID, nil
}

// several is the error of given starting the ids of several sessions,
// listing them, in the order they came, a line each, as lines name them.
func several(given string, lines []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "session %s could be any of these, so give more of its id:", prose.Clean(redact.Text(given)))
	for _, line := range lines {
		fmt.Fprintf(&b, "\n  %s", line)
	}
	return errors.New(b.String())
}
