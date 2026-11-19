package launch

import (
	"strings"

	"github.com/leeovery/switchboard/internal/ledger"
)

// The variables that say which terminal session a launch runs in.
const (
	// zmxSessionEnv names the zmx session a process runs in, as zmx sets it.
	zmxSessionEnv = "ZMX_SESSION"
	// tmuxEnv is set inside tmux, as tmux sets it: the tmux session's name
	// isn't in it, so tmux is asked.
	tmuxEnv = "TMUX"
)

// terminalName is the name of the terminal session the launch runs in, as
// the router is told it, read once, as Claude Code starts, so a rename since
// isn't followed: ZMX_SESSION's, where it's set; else, inside tmux, the tmux
// session's, as TmuxSession asks it; else none, as where tmux can't say. Each
// is read as namedBy reads it.
func (l Launcher) terminalName() string {
	env := environ(l.Environ)
	if name := namedBy(env.get(zmxSessionEnv)); name != "" {
		return name
	}
	if env.get(tmuxEnv) == "" || l.TmuxSession == nil {
		return ""
	}
	name, err := l.TmuxSession()
	if err != nil {
		logger.Debug("tmux didn't name its session; the session goes unnamed", "error", err)
		return ""
	}
	return namedBy(name)
}

// namedBy is the name given, cleaned and cut as ledger.LineName has it, as
// it's text from outside; none where that's all digits, as tmux numbers the
// sessions it isn't given a name for, 0, 1 and on: a number names no
// session, and the fallbacks name it better.
func namedBy(name string) string {
	name = ledger.LineName(name)
	if strings.Trim(name, "0123456789") == "" {
		return ""
	}
	return name
}

// toldName is the name of the terminal session the router in state is told
// Claude Code starts in, as terminalName reads it, where the router takes
// every header of switchboard's own off what it sends upstream: none for one
// that doesn't, which would send it on to the API, as told has it of the
// directory.
func (l Launcher) toldName(state routerState) string {
	if !state.strips {
		return ""
	}
	return l.terminalName()
}
