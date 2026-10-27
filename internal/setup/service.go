package setup

import (
	"cmp"
	"context"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

// service installs the LaunchAgent, which starts the router, when it isn't
// installed; installs it again, keeping the options it was installed with,
// when launchd hasn't loaded it, it runs another switchboard than this one,
// or its plist differs from the one this switchboard writes; restarts the
// router when it doesn't answer; and otherwise says it's up, and, when setup
// has changed the config or a token, that the router takes the change up on
// its own.
func (r *run) service(ctx context.Context) error {
	st, err := r.Service.Status(ctx)
	if err != nil {
		return err
	}
	if !st.Installed {
		return r.install(ctx, r.Install)
	}
	opts, read, err := r.installed()
	switch {
	case err != nil:
		return err
	case !read || !st.Loaded:
		return r.install(ctx, opts)
	}
	stale, err := r.stale(opts)
	switch {
	case err != nil:
		return err
	case stale:
		return r.install(ctx, opts)
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

// installed returns the options the service was installed with, read back
// from its plist, but for setup's config file where setup is given one, and
// says when the variables carried are set otherwise here than the service's,
// which a reinstall keeps. For a plist it can't read, it says so, and returns
// setup's own options, and false.
func (r *run) installed() (service.InstallOptions, bool, error) {
	opts, err := r.Service.Options()
	if err != nil {
		r.Terminal.sayf("The service's plist can't be read (%v), so it's installed afresh.", err)
		return r.Install, false, nil
	}
	opts.Config = cmp.Or(r.Install.Config, opts.Config)
	other, err := r.Service.OtherEnvironment(opts)
	if err != nil {
		return opts, false, err
	}
	if len(other) > 0 {
		r.Terminal.sayf("The service runs with another %s than this shell's, so setup and the router may read different files: switchboard service install, run from the shell the router should use, changes it.", prose.List(other))
	}
	return opts, true, nil
}

// stale reports whether the service, installed with opts, is to be
// installed afresh, saying why: it runs another switchboard than this one,
// or its plist differs from the one this switchboard writes.
func (r *run) stale(opts service.InstallOptions) (bool, error) {
	if opts.Executable != r.switchboard {
		r.Terminal.sayf("The service runs %s, not this switchboard, %s, so it's installed afresh.", cmp.Or(opts.Executable, "a program its plist doesn't name"), r.switchboard)
		return true, nil
	}
	drift, err := r.Service.Drift(opts)
	if err != nil || len(drift) == 0 {
		return false, err
	}
	r.Terminal.sayf("The service's plist differs from the one this switchboard writes, with %s, so it's installed afresh.", prose.List(drift))
	return true, nil
}

// install installs the LaunchAgent as opts says, to run this switchboard,
// and says how the router it starts answers.
func (r *run) install(ctx context.Context, opts service.InstallOptions) error {
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
// the one launchd starts answers. Should the router answer by then after all,
// as one starting might, it's restarted, saying first that it's finishing its
// requests in flight.
func (r *run) restart(ctx context.Context) error {
	h, err := r.Service.Restart(ctx, func(how service.Restarting) {
		r.Terminal.sayf("The router is finishing its requests in flight, then %s.", how)
	})
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
