package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// namingASession says, in the help of the commands that take a session, how
// to name one, and where its id is to be found.
const namingASession = `Name a session by its id, or as much of it as is unique among the sessions
routed in the last hour: switchboard status lists them, Claude Code's /status
shows a session's own, and inside one, $CLAUDE_CODE_SESSION_ID holds its id.`

// findSession returns the id of the session given names, as lookUpSession
// finds it.
func (a *app) findSession(ctx context.Context, client *router.Client, names status.Names, given string) (string, error) {
	id, _, err := a.lookUpSession(ctx, client, names, given)
	return id, err
}

// lookUpSession returns the id of the session given names, and the session
// as the router answers for it, where it has seen a session of that whole
// id, in whatever pool; else nil, and the id of the one given names among
// those routed in the last hour in every pool, as views.NamedRouted names
// it, listing them, where it starts several, as they stand now, their
// accounts as names has them. Given one that starts no such session's id, it
// returns it as it is, as the whole id of a session idle for longer, for the
// router to say whether it has seen it.
func (a *app) lookUpSession(ctx context.Context, client *router.Client, names status.Names, given string) (string, *status.Session, error) {
	s, err := client.Session(ctx, given)
	switch {
	case err == nil:
		return given, &s, nil
	case !errors.Is(err, router.ErrUnknownSession):
		return "", nil, err
	}
	sessions, err := client.SessionsInEveryPool(ctx)
	if err != nil {
		return "", nil, err
	}
	named := views.NamedRouted(given, sessions, a.Now(), names)
	if named.Several != nil {
		return "", nil, several(given, named.Several)
	}
	return named.ID, nil, nil
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
