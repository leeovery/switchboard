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
// installed. Installed, it's seen as the router it runs sees it, through its
// plist: setup says where that router finds its state, its config or Claude
// Code's config elsewhere than setup, and installs the service again,
// keeping what it was installed with, when launchd hasn't loaded it, it runs
// another switchboard than this one, or its plist differs from the one this
// switchboard writes. It restarts the router when none answers, and
// otherwise says it's up, and, when setup has changed the config or a token
// the router reads, that the router takes the change up on its own.
func (r *run) service(ctx context.Context) error {
	installed, err := r.Service.Options()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return r.installFirst(ctx)
	case err != nil:
		return r.reinstall(ctx, r.Service, r.Install, fmt.Sprintf("The service's plist can't be read back (%v)", err))
	}
	svc, err := r.Service.For(installed)
	if err != nil {
		return err
	}
	opts, err := r.configured(svc, installed)
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
// it leaves the service as it is while a router answers, saying why, and
// fails, saying why, while none does.
func (r *run) reinstall(ctx context.Context, svc *service.Service, opts service.InstallOptions, why string) error {
	if err := r.startable(svc, opts); err != nil {
		return r.unstartable(ctx, svc, why, err)
	}
	if why != "" {
		r.Terminal.sayf("%s, so it's installed afresh.", why)
	}
	return r.install(ctx, svc, opts)
}

// unstartable says, why it was to be, that the service isn't installed
// afresh, as the router couldn't start so, err saying why, and leaves it as
// it is while its router answers, or fails while none does.
func (r *run) unstartable(ctx context.Context, svc *service.Service, why string, err error) error {
	why = cmp.Or(why, "Launchd hasn't loaded the service")
	if svc.Answering(ctx) == nil {
		r.Terminal.sayf("%s, but the router couldn't start installed afresh, and none is running.", why)
		return fmt.Errorf("the router couldn't start: %w", err)
	}
	r.Terminal.sayf("%s, but the router couldn't start installed afresh, so the service is left as it is, its router running: "+
		"switchboard service install, run from the shell the router should use, installs it afresh once what stops it is put right. It couldn't start as:", why)
	r.Terminal.sayf("%v", err)
	return nil
}

// startable fails, saying why, where the router svc runs couldn't start,
// installed as opts say.
func (r *run) startable(svc *service.Service, opts service.InstallOptions) error {
	file, err := svc.ConfigFile(opts)
	if err != nil {
		return err
	}
	return r.Startable(svc.Getenv, file, opts.LogLevel)
}

// configured is the options the service, installed with installed, and
// seen as svc, is to be installed with: those, but for setup's config file
// where setup is given one other than the one the router serves.
func (r *run) configured(svc *service.Service, installed service.InstallOptions) (service.InstallOptions, error) {
	if r.Install.Config == "" {
		return installed, nil
	}
	served, err := svc.ConfigFile(installed)
	if err != nil {
		return service.InstallOptions{}, err
	}
	given, err := filepath.Abs(r.Install.Config)
	if err != nil {
		return service.InstallOptions{}, fmt.Errorf("find the config file: %w", err)
	}
	if service.SameFile(given, served) {
		return installed, nil
	}
	opts := installed
	opts.Config = given
	return opts, nil
}

// apart says where the router the service, installed with opts, and seen as
// svc, runs, finds its state, its config or Claude Code's config elsewhere
// than setup does, and reports whether it finds setup's state directory,
// with the token files, and setup's config file.
func (r *run) apart(svc *service.Service, opts service.InstallOptions) (bool, error) {
	routers, err := svc.Places(opts)
	if err != nil {
		return false, err
	}
	setups, err := r.Service.Places(r.Install)
	if err != nil {
		return false, err
	}
	apart := routers.Apart(setups)
	if len(apart) > 0 {
		advice := "switchboard service install, run from the shell the router should use, lines them up."
		if slices.Contains(apart, "config file") {
			advice += " For the config file, switchboard setup --config, naming the router's, does too."
		}
		r.Terminal.sayf("The router uses another %s than setup: %s", prose.List(apart), advice)
	}
	return routers.Shared(setups), nil
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
// and says how the router it starts answers. It warns of the accounts
// without a usable token only where the router serves setup's config file,
// whose accounts setup has.
func (r *run) install(ctx context.Context, svc *service.Service, opts service.InstallOptions) error {
	opts.Executable = r.switchboard
	served, err := svc.ConfigFile(opts)
	if err != nil {
		return err
	}
	setups, err := r.Service.ConfigFile(r.Install)
	if err != nil {
		return err
	}
	if service.SameFile(served, setups) {
		opts.Accounts = r.cfg.Accounts
	}
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
