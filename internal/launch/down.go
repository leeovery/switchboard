package launch

import (
	"cmp"
	"errors"
	"fmt"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/redact"
)

// whenDownEnv says, in place of the config's when_router_down, what the
// launch it's given to does with the router down: stop or direct. It's taken
// off the environment Claude Code starts in, so it goes no further.
const whenDownEnv = "SWITCHBOARD_WHEN_ROUTER_DOWN"

// ErrNotStarted is what a launch fails with when it starts nothing, the
// router being down: Stderr has heard why, and how to go on.
var ErrNotStarted = errors.New("claude not started: the router is down")

// goingOn says, of a launch that started nothing, how to go on.
const goingOn = "see to it with switchboard service status, or go direct this once with " + whenDownEnv + "=direct claude"

// whenDown is what this launch does with the router down: what whenDownEnv
// says, where the environment sets it, else configured, else Stop. A value
// whenDownEnv doesn't take is Stop, and fails, saying so.
func (l Launcher) whenDown(configured config.RouterDown) (config.RouterDown, error) {
	given := environ(l.Environ).get(whenDownEnv)
	if given == "" {
		return cmp.Or(configured, config.Stop), nil
	}
	if down, ok := config.ParseRouterDown(given); ok {
		return down, nil
	}
	return config.Stop, fmt.Errorf("%s takes %s or %s, not %q", whenDownEnv, config.Stop, config.Direct, redact.Text(given))
}

// goDirect returns nil when, the router down as state says, Claude Code goes
// on without it, as whenDown says of configured. Otherwise Stderr hears why
// it doesn't, and how to go on, and it fails with ErrNotStarted.
func (l Launcher) goDirect(configured config.RouterDown, state routerState) error {
	down, err := l.whenDown(configured)
	if down == config.Direct {
		return nil
	}
	attrs := []any{"mode", "stop", "router", state.name, "reason", state.reason, "asked", state.asked}
	why := "the router " + state.String()
	if err != nil {
		attrs = append(attrs, "error", err)
		why += ", and " + err.Error()
	}
	logger.Warn("not starting claude", attrs...)
	Notice(l.Stderr, why+" — not starting claude: "+goingOn)
	return ErrNotStarted
}
