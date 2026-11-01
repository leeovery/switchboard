package setup

import "github.com/leeovery/switchboard/internal/sessionhook"

// SessionHook installs the session hook in Claude Code's settings, running
// this switchboard, or puts it right, and says which it did, if either. It
// never fails: a hook that can't be installed is said to be, with a warning,
// and setup carries on.
func (s *Setup) SessionHook() Said {
	switchboard, err := s.Binary()
	if err != nil {
		logger.Warn("can't find switchboard's binary for the session hook", "error", err)
		return withoutHook(err)
	}
	outcome, err := sessionhook.Install(s.Settings, switchboard)
	does := "Claude Code says so when a session doesn't start through switchboard · " + s.fromHome(s.Settings)
	switch {
	case err != nil:
		logger.Warn("can't install the session hook", "path", s.Settings, "error", err)
		return withoutHook(err)
	case outcome == sessionhook.Installed:
		logger.Info("installed the session hook", "path", s.Settings)
		return Said{Mark: Done, Label: "Session hook", Detail: does}
	case outcome == sessionhook.PutRight:
		logger.Info("put the session hook right", "path", s.Settings)
		return Said{Mark: Done, Label: "Session hook", Detail: "put right: " + does}
	}
	return Said{Mark: Done, Label: "Session hook", Detail: "in place: " + does}
}

// withoutHook is what setup says of carrying on without the session hook,
// which err stopped.
func withoutHook(err error) Said {
	return Said{
		Mark:   Warning,
		Label:  "Session hook",
		Detail: "setup carries on without it, so a session that doesn't go through switchboard won't say so: " + err.Error(),
	}
}
