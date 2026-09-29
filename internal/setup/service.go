package setup

import (
	"context"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

// service installs the LaunchAgent, which starts the router, when launchd
// hasn't loaded it; restarts the router when this run has changed what it
// reads as it starts, or it doesn't answer; and otherwise says it's up.
func (r *run) service(ctx context.Context) error {
	st, err := r.Service.Status(ctx)
	switch {
	case err != nil:
		return err
	case !st.Installed || !st.Loaded:
		return r.install(ctx)
	case r.changed:
		return r.restart(ctx, "to read the config and the tokens afresh")
	case st.Router == nil:
		return r.restart(ctx, "as it didn't answer")
	}
	r.Terminal.sayf("The service is installed, and the router is up: %s.", service.Health(*st.Router))
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

// restart has launchd restart the router, why says why, and says how it
// answers.
func (r *run) restart(ctx context.Context, why string) error {
	h, err := r.Service.Restart(ctx)
	if err != nil {
		return err
	}
	r.Terminal.sayf("Restarted the router, %s.", why)
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
