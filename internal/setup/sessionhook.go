package setup

import (
	"context"

	"github.com/leeovery/switchboard/internal/sessionhook"
)

// sessionHook installs the session hook in Claude Code's settings, running
// this switchboard, or puts it right. It never stops setup: one that can't
// be installed is said to be, and setup carries on.
func (r *run) sessionHook(context.Context) {
	outcome, err := sessionhook.Install(r.Settings, r.switchboard)
	switch {
	case err != nil:
		logger.Warn("can't install the session hook", "path", r.Settings, "error", err)
		r.Terminal.sayf("Setup carries on without the session hook, so a session that doesn't go through switchboard won't say so: %v.", err)
	case outcome == sessionhook.Installed:
		logger.Info("installed the session hook", "path", r.Settings)
		r.Terminal.sayf("Installed the session hook, which tells a session that doesn't go through switchboard so: %s", r.Settings)
	case outcome == sessionhook.PutRight:
		logger.Info("put the session hook right", "path", r.Settings)
		r.Terminal.sayf("Put the session hook right: %s", r.Settings)
	default:
		r.Terminal.sayf("The session hook is in place: %s", r.Settings)
	}
}
