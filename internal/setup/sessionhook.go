package setup

import (
	"context"

	"github.com/leeovery/switchboard/internal/sessionhook"
)

// SessionHook installs the session hook in Claude Code's settings, running
// this switchboard, or puts it right, and says which it did, if either. It
// never fails: a hook that can't be installed is said to be, with a warning,
// and setup carries on.
func (s *Setup) SessionHook() Said {
	switchboard, err := s.LaunchAgent.Binary(s.Install.Executable)
	if err != nil {
		logger.Warn("can't install the session hook", "path", s.Settings, "error", err)
		return withoutHook(err)
	}
	return (&run{Setup: s, switchboard: switchboard}).hook()
}

// sessionHook is the line walk's session hook, which the claude link step
// takes last.
func (r *run) sessionHook(context.Context) {
	r.Terminal.tell(r.hook())
}

// hook installs the session hook, as SessionHook says.
func (r *run) hook() Said {
	outcome, err := sessionhook.Install(r.Settings, r.switchboard)
	does := "Claude Code says so when a session doesn't start through switchboard · " + r.fromHome(r.Settings)
	switch {
	case err != nil:
		logger.Warn("can't install the session hook", "path", r.Settings, "error", err)
		return withoutHook(err)
	case outcome == sessionhook.Installed:
		logger.Info("installed the session hook", "path", r.Settings)
		return Said{Mark: Done, Label: "Session hook", Detail: does, Sentence: "Installed the session hook, which tells a session that doesn't go through switchboard so: " + r.Settings}
	case outcome == sessionhook.PutRight:
		logger.Info("put the session hook right", "path", r.Settings)
		return Said{Mark: Done, Label: "Session hook", Detail: "put right: " + does, Sentence: "Put the session hook right: " + r.Settings}
	}
	return Said{Mark: Done, Label: "Session hook", Detail: "in place: " + does, Sentence: "The session hook is in place: " + r.Settings}
}

// withoutHook is what setup says of carrying on without the session hook,
// which err stopped.
func withoutHook(err error) Said {
	const wontSay = ", so a session that doesn't go through switchboard won't say so: "
	return Said{
		Mark:     Warning,
		Label:    "Session hook",
		Detail:   "setup carries on without it" + wontSay + err.Error(),
		Sentence: "Setup carries on without the session hook" + wontSay + err.Error() + ".",
	}
}
