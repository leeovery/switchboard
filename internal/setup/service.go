package setup

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

// service installs the LaunchAgent, which starts the router, when it isn't
// installed. Installed, it's seen as the router it runs sees it, in the
// environment it was installed with: setup says where that, or the config
// file it serves, differs from setup's own, and installs it again, keeping
// what it was installed with, when launchd hasn't loaded it, it runs another
// switchboard than this one, or its plist differs from the one this
// switchboard writes. It restarts the router when it doesn't answer, and
// otherwise says it's up, and, when setup has changed the config or a token
// the router reads, that the router takes the change up on its own.
func (r *run) service(ctx context.Context) error {
	opts, err := r.Service.Options()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return r.installFirst(ctx)
	case err != nil:
		return r.reinstall(ctx, r.Service, r.Install, fmt.Sprintf("The service's plist can't be read back (%v)", err))
	}
	opts.Config = cmp.Or(r.Install.Config, opts.Config)
	svc, err := r.Service.For(opts)
	if err != nil {
		return err
	}
	st, err := svc.Status(ctx)
	if err != nil {
		return err
	}
	shared, err := r.apart(svc, opts)
	if err != nil {
		return err
	}
	why, stale, err := r.stale(svc, opts, st)
	switch {
	case err != nil:
		return err
	case stale:
		return r.reinstall(ctx, svc, opts, why)
	case st.Router == nil:
		return r.restart(ctx, svc)
	}
	up := "The service is installed, and the router is up: " + service.Health(*st.Router) + "."
	if r.changed && shared {
		up += " It takes up setup's changes on its own."
	}
	r.Terminal.sayf("%s", up)
	return nil
}

// installFirst installs the LaunchAgent, which isn't installed, as setup's
// options say, in the environment setup runs in, failing where the router
// couldn't start.
func (r *run) installFirst(ctx context.Context) error {
	if err := r.startable(r.Service, r.Install); err != nil {
		return fmt.Errorf("the router couldn't start: %w", err)
	}
	return r.install(ctx, r.Service, r.Install)
}

// reinstall installs svc again, as opts say, first saying why, unless why is
// "", as when launchd hasn't loaded it. Where the router couldn't start so,
// it leaves the service as it is, saying why, and how to install it afresh.
func (r *run) reinstall(ctx context.Context, svc *service.Service, opts service.InstallOptions, why string) error {
	if err := r.startable(svc, opts); err != nil {
		r.Terminal.sayf("%s, but the router couldn't start installed afresh (%v), so the service is left as it is: switchboard service install, run from the shell the router should use, installs it afresh.",
			cmp.Or(why, "launchd hasn't loaded the service"), err)
		return nil
	}
	if why != "" {
		r.Terminal.sayf("%s, so it's installed afresh.", why)
	}
	return r.install(ctx, svc, opts)
}

// startable fails, saying why, where the router svc runs couldn't start,
// installed as opts say.
func (r *run) startable(svc *service.Service, opts service.InstallOptions) error {
	file, err := svc.ConfigFile(opts)
	if err != nil {
		return err
	}
	return r.Startable(svc.Getenv, file)
}

// apart says where the service, installed with opts, and seen as svc,
// decides otherwise than setup: the variables carried this shell sets
// otherwise, and the config file it serves, when it isn't setup's. It
// reports whether the router reads the config file and the token files
// setup writes.
func (r *run) apart(svc *service.Service, opts service.InstallOptions) (bool, error) {
	other, err := r.Service.OtherEnvironment(opts)
	if err != nil {
		return false, err
	}
	if len(other) > 0 {
		var decides []string
		for _, name := range other {
			if d := service.Decides(name); !slices.Contains(decides, d) {
				decides = append(decides, d)
			}
		}
		verb := "differs"
		if len(other) > 1 {
			verb = "differ"
		}
		r.Terminal.sayf("The service's %s %s from this shell's, so the router uses another %s than setup: switchboard service install, run from the shell the router should use, changes the service's.",
			prose.List(other), verb, prose.List(decides))
	}
	served, err := svc.ConfigFile(opts)
	if err != nil {
		return false, err
	}
	mine, err := filepath.Abs(r.Accounts.ConfigPath)
	if err != nil {
		return false, fmt.Errorf("find setup's config file: %w", err)
	}
	if served != mine {
		r.Terminal.sayf("The router serves another config file than the one setup edits, so setup's changes don't reach it: switchboard setup --config, naming the router's, or switchboard service install, run from the shell the router should use, lines them up.")
	}
	// The token files are in the state directory.
	return served == mine && !slices.Contains(other, "XDG_STATE_HOME"), nil
}

// stale says why the service, installed with opts, and seen as svc, is to
// be installed afresh, and whether it is: when it runs another switchboard
// than this one, or its plist differs from the one this switchboard writes;
// or, saying nothing, when launchd hasn't loaded it.
func (r *run) stale(svc *service.Service, opts service.InstallOptions, st service.Status) (string, bool, error) {
	if opts.Executable != r.switchboard {
		return fmt.Sprintf("The service runs %s, not this switchboard, %s", opts.Executable, r.switchboard), true, nil
	}
	drift, err := svc.Drift(opts)
	switch {
	case err != nil:
		return "", false, err
	case len(drift) > 0:
		return "The service's plist differs from the one this switchboard writes, with " + prose.List(drift), true, nil
	}
	return "", !st.Loaded, nil
}

// install installs svc's LaunchAgent as opts says, to run this switchboard,
// and says how the router it starts answers.
func (r *run) install(ctx context.Context, svc *service.Service, opts service.InstallOptions) error {
	opts.Executable, opts.Accounts = r.switchboard, r.cfg.Accounts
	installed, err := svc.Install(ctx, opts)
	if err != nil {
		return err
	}
	r.Terminal.sayf("Installed %s: launchd starts the router now, at every login, and whenever it stops.", svc.Plist())
	for _, warning := range installed.Warnings {
		r.Terminal.sayf("Warning: %s.", warning)
	}
	return r.up(svc, installed.Router)
}

// restart has launchd restart svc's router, which didn't answer, and says
// how the one launchd starts answers. Should the router answer by then after
// all, as one starting might, it's restarted, saying first that it's
// finishing its requests in flight.
func (r *run) restart(ctx context.Context, svc *service.Service) error {
	h, err := svc.Restart(ctx, func(how service.Restarting) {
		r.Terminal.sayf("The router is finishing its requests in flight, then %s.", how)
	})
	if err != nil {
		return err
	}
	r.Terminal.sayf("Restarted the router, as it didn't answer.")
	return r.up(svc, h)
}

// up says how the router svc's launchd started answered, failing, saying
// where to find out why, when it didn't.
func (r *run) up(svc *service.Service, h *router.Health) error {
	if err := svc.Answered(h); err != nil {
		return err
	}
	r.Terminal.sayf("The router is up: %s.", service.Health(*h))
	return nil
}
