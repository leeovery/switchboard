package setup

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/leeovery/switchboard/internal/launch"
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
		return r.placeless(ctx, r.Service, err)
	}
	opts, given, err := r.configured(svc, installed)
	if err != nil {
		return err
	}
	if _, err := svc.StartsFrom(opts); err != nil {
		return r.placeless(ctx, svc, err)
	}
	st, err := r.status(ctx, svc)
	if err != nil {
		return err
	}
	if left, err := r.leaves(st); left {
		return err
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
	// Places are said only of a router that answers: a router started with
	// none answering is launchd's, wherever it answers, which is where its
	// places are judged.
	afresh := st.Router == nil
	var shared bool
	if !afresh {
		if shared, err = r.apart(svc, served); err != nil {
			return err
		}
	}
	switch {
	case stale && refused != nil:
		return r.unstartable(ctx, svc, why, refused)
	case stale:
		if why != "" {
			r.Terminal.sayf("%s, so it's installed afresh.", why)
		}
		return r.install(ctx, svc, opts, afresh)
	case st.Router == nil && refused != nil:
		r.Terminal.sayf("Launchd has the service loaded, but no router runs, and one couldn't start as the service runs it: %s, installs it once what stops it is put right.", installAdvice)
		return fmt.Errorf("the router couldn't start: %w", refused)
	case st.Router == nil:
		return r.restart(ctx, svc, opts)
	}
	up := "The service is installed, and the router is up: " + service.Health(*st.Router) + "."
	if r.changed && shared {
		up += " It takes up setup's changes on its own."
	}
	r.Terminal.sayf("%s", up)
	return nil
}

// errLeftUnanswered is how the service step fails where it leaves the
// service as it is with no router answering.
var errLeftUnanswered = errors.New("setup left the service as it is, and no router answers: run switchboard setup again once one does")

// status is how svc stands, as Status says; but where launchd runs a router
// that doesn't answer, asked again for as long as the launcher waits for one,
// as a router starting under launchd, or handing over in the same process,
// answers within that.
func (r *run) status(ctx context.Context, svc *service.Service) (service.Status, error) {
	st, err := svc.Status(ctx)
	if err != nil || st.Router != nil || st.PID == 0 || svc.Await(ctx, launch.AskTimeout) == nil {
		return st, err
	}
	return svc.Status(ctx)
}

// leaves reports whether setup leaves the service, standing as st says, as
// it is, having said why, failing where no router answers: where launchd
// hasn't loaded it but a router run some other way answers, which another
// would clash with; where the router that answers may not be the one the
// service runs, as Status's Elsewhere says; and where launchd runs a router
// that doesn't answer.
func (r *run) leaves(st service.Status) (bool, error) {
	switch {
	case !st.Loaded && st.Router != nil:
		r.Terminal.sayf("Launchd hasn't loaded the service, but a router run some other way answers, and another would clash with it, so setup leaves it as it is: once that one's stopped, %s, installs it.", installAdvice)
	case st.Elsewhere:
		r.Terminal.sayf("No router answers where the service's plist keeps its state, but one answers where this shell keeps it, so which the service runs can't be told, and setup leaves it as it is: %s, lines them up.", installAdvice)
	case st.Router == nil && st.PID != 0:
		r.leftRunning("", st.PID)
		return true, errLeftUnanswered
	default:
		return false, nil
	}
	return true, nil
}

// leftRunning says setup leaves the service as it is, launchd running its
// router, pid, which doesn't answer, after what else is so, if anything.
func (r *run) leftRunning(also string, pid int) {
	r.Terminal.sayf("%sLaunchd runs the service's router, pid %d, but it doesn't answer here: it may be starting, or stopping, which can take over half a minute after a bootout, "+
		"or keeping its state somewhere setup doesn't look. Setup leaves it as it is: switchboard service status says how it stands.", also, pid)
}

// placeless says where the service's router finds its state or its config
// file can't be worked out from its plist, as err says, and leaves the
// service as it is while a router svc may run answers, or fails while none
// does, naming service install.
func (r *run) placeless(ctx context.Context, svc *service.Service, err error) error {
	why := fmt.Sprintf("Where the service's router finds its state or its config can't be worked out from its plist (%s)", redact.Text(err.Error()))
	if svc.Answering(ctx) != nil {
		r.Terminal.sayf("%s, but a router answers, so the service is left as it is: %s, installs it afresh.", why, installAdvice)
		return nil
	}
	r.Terminal.sayf("%s, and no router answers: %s, installs it afresh.", why, installAdvice)
	return fmt.Errorf("the service's plist: %w", err)
}

// installFirst installs the LaunchAgent, which isn't installed, as setup's
// options say, in the environment setup runs in, failing where the router
// couldn't start.
//
// With no plist, it leaves a router launchd runs, its plist gone, and one
// run some other way that answers, which another would clash with, failing
// all the same: with the service not installed, nothing brings the router
// back after a restart.
func (r *run) installFirst(ctx context.Context) error {
	st, err := r.status(ctx, r.Service)
	switch {
	case err != nil:
		return err
	case st.Loaded && st.Router != nil:
		r.Terminal.sayf("The service's plist is gone, though launchd still runs its router, which answers: setup leaves it as it is, and switchboard service install writes the plist again.")
		return errors.New("the service's plist is gone: run switchboard service install to write it again")
	case st.Router != nil:
		r.Terminal.sayf("No service is installed, and a router run some other way answers: another would clash with it, so setup leaves it; stop it, then run setup again to install the service.")
		return errors.New("no service is installed, so nothing brings the router back after a restart: stop the router run by hand, then run switchboard setup again")
	case st.PID != 0:
		r.leftRunning("The service's plist is gone. ", st.PID)
		return errLeftUnanswered
	}
	svc, opts, err := r.Service.Planned(ctx, r.Install)
	if err != nil {
		return err
	}
	if err := r.startable(svc, opts); err != nil {
		return fmt.Errorf("the router couldn't start: %w", err)
	}
	return r.install(ctx, svc, opts, false)
}

// reinstallUnread installs the LaunchAgent afresh, as setup's options say,
// in the environment setup runs in, its plist being one setup couldn't read
// back, as err says, once it has checked the router could start so; but
// leaves it as it is while launchd has it loaded, or a router answers,
// either of which may run as the plist says.
func (r *run) reinstallUnread(ctx context.Context, unread error) error {
	why := fmt.Sprintf("The service's plist can't be read back (%s)", redact.Text(unread.Error()))
	st, err := r.status(ctx, r.Service)
	switch {
	case err != nil:
		return err
	case st.Loaded:
		r.Terminal.sayf("%s, but launchd has it loaded, so the service is left as it is: %s, installs it afresh.", why, installAdvice)
		if st.Router == nil {
			return errors.New("the service's plist can't be read back, and no router answers: run switchboard service install")
		}
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
	return r.install(ctx, svc, opts, false)
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
// file where setup is given one other than the one the router serves, or
// one where the one it serves can't be worked out.
func (r *run) configured(svc *service.Service, installed service.InstallOptions) (service.InstallOptions, bool, error) {
	if r.Install.Config == "" {
		return installed, false, nil
	}
	given, err := filepath.Abs(r.Install.Config)
	if err != nil {
		return service.InstallOptions{}, false, fmt.Errorf("find the config file: %w", err)
	}
	// One the router serves that can't be worked out, as under a HOME its
	// plist sets empty, is another than the one given.
	if served, err := svc.ConfigFile(installed); err == nil && service.SameFile(given, served) {
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
	if routers.ClaudeDir == "" {
		r.Terminal.sayf("The router finds no Claude Code config directory, as its plist sets HOME empty, so it won't keep the skill up to date: %s, lines that up.", installAdvice)
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
// and says how the router it starts answers, and, with placesAfter set,
// where it finds what setup does elsewhere, as placesOfStarted says. It
// warns of the accounts without a usable token only where the router serves
// setup's config file, whose accounts setup has.
func (r *run) install(ctx context.Context, svc *service.Service, opts service.InstallOptions, placesAfter bool) error {
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
	return r.up(ctx, svc, opts, installed.Router, placesAfter)
}

// restart has launchd restart svc's router, installed with opts, which
// didn't answer, and says how the one launchd starts answers, and where it
// finds what setup does elsewhere, as placesOfStarted says. Should the
// router answer by then after all, as one starting might, it's restarted,
// saying first that it's finishing its requests in flight.
func (r *run) restart(ctx context.Context, svc *service.Service, opts service.InstallOptions) error {
	h, err := svc.Restart(ctx, func(how service.Restarting) {
		r.Terminal.sayf("The router is finishing its requests in flight, then %s.", how)
	})
	if err != nil {
		return err
	}
	r.Terminal.sayf("Restarted the router, as it didn't answer.")
	return r.up(ctx, svc, opts, h, true)
}

// placesOfStarted says where the router launchd started, svc's, installed
// with opts, finds its state, its config or Claude Code's config elsewhere
// than setup, as apart does, seen where it answers: saying so first where
// that's where this shell keeps its state, not where the plist does.
func (r *run) placesOfStarted(ctx context.Context, svc *service.Service, opts service.InstallOptions) error {
	running, elsewhere := svc.Found(ctx)
	if elsewhere {
		r.Terminal.sayf("Launchd started the router where this shell keeps its state, not where the service's plist does: %s, lines them up.", installAdvice)
	}
	_, err := r.apart(running, opts)
	return err
}

// up says how the router svc's launchd started, installed with opts,
// answered, failing, saying where to find out why, when it didn't; and, with
// placesAfter set, once it answers, where it finds what setup does
// elsewhere, as placesOfStarted says.
func (r *run) up(ctx context.Context, svc *service.Service, opts service.InstallOptions, h *router.Health, placesAfter bool) error {
	if err := svc.Answered(h); err != nil {
		return err
	}
	if placesAfter {
		if err := r.placesOfStarted(ctx, svc, opts); err != nil {
			return err
		}
	}
	r.Terminal.sayf("The router is up: %s.", service.Health(*h))
	return nil
}
