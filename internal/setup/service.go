package setup

import (
	"cmp"
	"context"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

// service installs the LaunchAgent, which starts the router, when launchd
// hasn't loaded it, or it runs another switchboard than this one; restarts
// the router when it doesn't answer; and otherwise says it's up, and, when
// setup has changed the config or a token, that the router takes the change
// up on its own.
func (r *run) service(ctx context.Context) error {
	st, err := r.Service.Status(ctx)
	switch {
	case err != nil:
		return err
	case !st.Installed || !st.Loaded:
		return r.install(ctx)
	case st.Binary != r.switchboard:
		r.Terminal.sayf("The service runs %s, not this switchboard, %s, so it's installed afresh.", cmp.Or(st.Binary, "a program its plist doesn't name"), r.switchboard)
		return r.install(ctx)
	case st.Router == nil:
		return r.restart(ctx)
	}
	up := "The service is installed, and the router is up: " + service.Health(*st.Router) + "."
	if r.changed {
		up += " It takes up setup's changes on its own."
	}
	r.Terminal.sayf("%s", up)
	return nil
}

// install installs the LaunchAgent to run this switchboard, and says how the
// router it starts answers.
func (r *run) install(ctx context.Context) error {
	opts := r.Install
	opts.Executable, opts.Accounts = r.switchboard, r.cfg.Accounts
	installed, err := r.Service.Install(ctx, opts)
	if err != nil {
		return err
	}
	r.Terminal.sayf("Installed %s: launchd starts the router now, at every login, and whenever it stops.", r.Service.Plist())
	for _, warning := range installed.Warnings {
		r.Terminal.sayf("Warning: %s.", warning)
	}
	return r.up(installed.Router)
}

// restart has launchd restart the router, which didn't answer, and says how
// the one it starts answers.
func (r *run) restart(ctx context.Context) error {
	h, err := r.Service.Restart(ctx)
	if err != nil {
		return err
	}
	r.Terminal.sayf("Restarted the router, as it didn't answer.")
	return r.up(h)
}

// up says how the router launchd started answered, failing, saying where to
// find out why, when it didn't.
func (r *run) up(h *router.Health) error {
	if err := r.Service.Answered(h); err != nil {
		return err
	}
	r.Terminal.sayf("The router is up: %s.", service.Health(*h))
	return nil
}
