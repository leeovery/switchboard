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
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/skill"
)

// installAdvice is how setup says to install the service from the shell the
// router should use.
const installAdvice = "switchboard service install, run from the shell the router should use"

// service installs the LaunchAgent, which starts the router, when it isn't
// installed. Installed, it's seen as the router it runs sees it, through its
// plist: setup installs it again, keeping what it was installed with, when
// launchd hasn't loaded it, it runs another switchboard than this one, setup
// is given another config file than the router serves, or the plist differs
// from the one this switchboard writes, once it has checked the router
// could start so. It says where the router finds its state, its config or
// Claude Code's config elsewhere than setup, restarts the router when none
// answers and none runs, once it has checked it could start, and otherwise
// says it's up, and, when setup has changed the config or a token the router
// reads, that the router takes the change up on its own. Where it can't tell
// the router the service runs, as leaves says, it leaves the service as it
// is.
func (r *run) service(ctx context.Context) error {
	installed, err := r.Service.Options(ctx)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return r.installFirst(ctx)
	case err != nil:
		return r.reinstallUnread(ctx, err)
	}
	svc, err := r.Service.For(installed)
	if err != nil {
		return err
	}
	opts, given, err := r.configured(svc, installed)
	if err != nil {
		return err
	}
	st, err := svc.Status(ctx)
	if err != nil {
		return err
	}
	if r.leaves(st) {
		return nil
	}
	why, stale, err := r.stale(ctx, svc, opts, given, st)
	if err != nil {
		return err
	}
	var refused error
	if stale || st.Router == nil {
		refused = r.startable(svc, opts)
	}
	served := opts
	if refused != nil {
		served = installed
	}
	shared, err := r.apart(svc, served)
	if err != nil {
		return err
	}
	switch {
	case stale && refused != nil:
		return r.unstartable(ctx, svc, why, refused)
	case stale:
		if why != "" {
			r.Terminal.sayf("%s, so it's installed afresh.", why)
		}
		return r.install(ctx, svc, opts)
	case st.Router == nil && refused != nil:
		r.Terminal.sayf("Launchd has the service loaded, but no router runs, and one couldn't start as the service runs it: %s, installs it once what stops it is put right.", installAdvice)
		return fmt.Errorf("the router couldn't start: %w", refused)
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

// leaves reports whether setup leaves the service, standing as st says, as
// it is, having said why: where launchd hasn't loaded it but a router run
// some other way answers, which another would clash with; where the router
// that answers may not be the one the service runs, as Status's Elsewhere
// says; and where launchd runs a router that answers nowhere setup looks.
func (r *run) leaves(st service.Status) bool {
	switch {
	case !st.Loaded && st.Router != nil:
		r.Terminal.sayf("Launchd hasn't loaded the service, but a router run some other way answers, and another would clash with it, so setup leaves it as it is: once that one's stopped, %s, installs it.", installAdvice)
	case st.Elsewhere:
		r.Terminal.sayf("No router answers where the service's plist keeps its state, but one answers where this shell keeps it, so which the service runs can't be told, and setup leaves it as it is: %s, lines them up.", installAdvice)
	case st.Router == nil && st.PID != 0:
		r.Terminal.sayf("Launchd runs the service's router, pid %d, but it answers neither where the service's plist keeps its state nor where this shell keeps it, so setup leaves it as it is: switchboard service status says how it stands.", st.PID)
	default:
		return false
	}
	return true
}

// installFirst installs the LaunchAgent, which isn't installed, as setup's
// options say, in the environment setup runs in, failing where the router
// couldn't start.
func (r *run) installFirst(ctx context.Context) error {
	svc, opts, err := r.Service.Planned(ctx, r.Install)
	if err != nil {
		return err
	}
	if err := r.startable(svc, opts); err != nil {
		return fmt.Errorf("the router couldn't start: %w", err)
	}
	return r.install(ctx, svc, opts)
}

// reinstallUnread installs the LaunchAgent afresh, as setup's options say,
// in the environment setup runs in, its plist being one setup couldn't read
// back, as err says, once it has checked the router could start so; but
// leaves it as it is while launchd has it loaded, or a router answers,
// either of which may run as the plist says.
func (r *run) reinstallUnread(ctx context.Context, unread error) error {
	why := fmt.Sprintf("The service's plist can't be read back (%s)", redact.Text(unread.Error()))
	st, err := r.Service.Status(ctx)
	switch {
	case err != nil:
		return err
	case st.Loaded:
		r.Terminal.sayf("%s, but launchd has it loaded, so the service is left as it is: %s, installs it afresh.", why, installAdvice)
		return nil
	case st.Router != nil:
		r.Terminal.sayf("%s, but a router answers, so the service is left as it is: %s, installs it afresh.", why, installAdvice)
		return nil
	}
	svc, opts, err := r.Service.Planned(ctx, r.Install)
	if err != nil {
		return err
	}
	if err := r.startable(svc, opts); err != nil {
		return r.unstartable(ctx, svc, why, err)
	}
	r.Terminal.sayf("%s, so it's installed afresh.", why)
	return r.install(ctx, svc, opts)
}

// unstartable says, why it was to be, that the service isn't installed
// afresh, as the router couldn't start so, err saying why, and how to
// install it once that's put right; and leaves it as it is while its router
// answers, or fails while none does.
func (r *run) unstartable(ctx context.Context, svc *service.Service, why string, err error) error {
	why = cmp.Or(why, "Launchd hasn't loaded the service")
	if svc.Answering(ctx) == nil {
		r.Terminal.sayf("%s, but the router couldn't start installed afresh, and none is running: %s, installs it once what stops it is put right.", why, installAdvice)
		return fmt.Errorf("the router couldn't start: %w", err)
	}
	r.Terminal.sayf("%s, but the router couldn't start installed afresh, so the service is left as it is, its router running: "+
		"%s, installs it afresh once what stops it is put right. It couldn't start as:", why, installAdvice)
	r.Terminal.sayf("%s", redact.Text(err.Error()))
	return nil
}

// startable fails, saying why, where the router svc runs couldn't start,
// installed as opts say.
func (r *run) startable(svc *service.Service, opts service.InstallOptions) error {
	start, err := svc.StartsFrom(opts)
	if err != nil {
		return err
	}
	return r.Startable(start)
}

// configured is the options the service, installed with installed, and
// seen as svc, is to be installed with, and whether setup's config file is
// given in place of the one the router serves: those, but for setup's config
// file where setup is given one other than the one the router serves.
func (r *run) configured(svc *service.Service, installed service.InstallOptions) (service.InstallOptions, bool, error) {
	if r.Install.Config == "" {
		return installed, false, nil
	}
	served, err := svc.ConfigFile(installed)
	if err != nil {
		return service.InstallOptions{}, false, err
	}
	given, err := filepath.Abs(r.Install.Config)
	if err != nil {
		return service.InstallOptions{}, false, fmt.Errorf("find the config file: %w", err)
	}
	if service.SameFile(given, served) {
		return installed, false, nil
	}
	opts := installed
	opts.Config = given
	return opts, true, nil
}

// places are where setup finds what it reads and writes: the token files
// and the config file of its accounts, and where it writes the skill.
func (r *run) places() (service.Places, error) {
	file, err := filepath.Abs(r.Accounts.ConfigPath)
	if err != nil {
		return service.Places{}, fmt.Errorf("find the config file: %w", err)
	}
	return service.Places{Tokens: r.Accounts.Tokens.Dir(), ConfigFile: file, ClaudeDir: skill.ConfigDirOf(r.Skill)}, nil
}

// apart says where the router the service, installed with opts, and seen as
// svc, runs, finds its state, its config or Claude Code's config elsewhere
// than setup does, and reports whether it finds setup's token files and
// setup's config file.
func (r *run) apart(svc *service.Service, opts service.InstallOptions) (bool, error) {
	routers, err := svc.Places(opts)
	if err != nil {
		return false, err
	}
	setups, err := r.places()
	if err != nil {
		return false, err
	}
	apart := routers.Apart(setups)
	if len(apart) > 0 {
		advice := installAdvice + ", lines them up."
		if slices.Contains(apart, "config file") {
			advice += " For the config file, switchboard setup --config, naming the router's, does too."
		}
		r.Terminal.sayf("The router uses another %s than setup: %s", prose.List(apart), advice)
	}
	return routers.Shared(setups), nil
}

// stale says why the service, installed with opts, and seen as svc, is to
// be installed afresh, and whether it is: when it runs another switchboard
// than this one, setup is given another config file than the router serves,
// as given says, or its plist differs from the one this switchboard writes;
// or, saying nothing, when launchd hasn't loaded it.
func (r *run) stale(ctx context.Context, svc *service.Service, opts service.InstallOptions, given bool, st service.Status) (string, bool, error) {
	if opts.Executable != r.switchboard {
		return fmt.Sprintf("The service runs %s, not this switchboard, %s", opts.Executable, r.switchboard), true, nil
	}
	if given {
		return "Setup is given another config file than the one the router serves", true, nil
	}
	drift, err := svc.Drift(ctx, opts)
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
	routers, err := svc.Places(opts)
	if err != nil {
		return err
	}
	setups, err := r.places()
	if err != nil {
		return err
	}
	if routers.SharesConfig(setups) {
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
