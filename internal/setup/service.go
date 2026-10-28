package setup

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
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

// Service takes the service step, as the line walk's does, and returns what
// it said and the router's health. It fails as the step does, and with
// errNoAccount where no account is configured, as there's nothing to serve.
func (s *Setup) Service(ctx context.Context) (Serviced, error) {
	switchboard, err := s.LaunchAgent.Binary(s.Install.Executable)
	if err != nil {
		return Serviced{}, err
	}
	registry := s.registry()
	draft, err := config.Edit(registry.ConfigPath)
	switch {
	case err != nil:
		return Serviced{}, err
	case draft.Config() == nil:
		return Serviced{}, errNoAccount
	}
	var out Serviced
	r := &run{Setup: s, switchboard: switchboard, registry: registry, cfg: draft.Config(), tell: func(said Said) { out.Said = append(out.Said, said) }}
	err = r.service(ctx)
	out.Router = r.health
	return out, err
}

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
	installed, err := r.LaunchAgent.Options(ctx)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return r.installFirst(ctx)
	case err != nil:
		return r.reinstallUnread(ctx, err)
	}
	svc, err := r.LaunchAgent.For(installed)
	if err != nil {
		return r.placeless(ctx, r.LaunchAgent, err)
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
	switch {
	case stale && refused != nil:
		return r.unstartable(ctx, svc, why, refused)
	case st.Router == nil && refused != nil:
		const refusedStart = "one couldn't start as the service runs it: " + installAdvice + ", installs it once what stops it is put right"
		r.tell(Said{
			Mark:     Failed,
			Label:    "Router",
			Detail:   "not running, and " + refusedStart,
			Sentence: "Launchd has the service loaded, but no router runs, and " + refusedStart + ".",
		})
		return fmt.Errorf("the router couldn't start: %w", refused)
	}
	// Places are said only of a router that answers: a router started with
	// none answering is launchd's, wherever it answers, which is where its
	// places are judged.
	afresh := st.Router == nil
	var shared bool
	if !afresh {
		if shared, err = r.apart(svc, opts); err != nil {
			return err
		}
	}
	switch {
	case stale:
		if why != (reason{}) {
			r.tell(why.afresh(kept(opts, given)))
		}
		return r.install(ctx, svc, opts, afresh)
	case afresh:
		return r.restart(ctx, svc, opts)
	}
	r.health = st.Router
	said := routerUp(*st.Router)
	said.Sentence = "The service is installed, and the router is up: " + service.Health(*st.Router) + "."
	if r.changed && shared {
		said.Detail += " · takes up setup's changes on its own"
		said.Sentence += " It takes up setup's changes on its own."
	}
	r.tell(said)
	return nil
}

// status is how svc stands, as Status says; but where launchd runs a router
// that doesn't answer, asked again for as long as the launcher waits for one,
// as a router starting under launchd, or handing over in the same process,
// answers within that.
func (r *run) status(ctx context.Context, svc *service.Service) (service.Status, error) {
	st, err := svc.Status(ctx)
	if err != nil || st.Router != nil || !st.Running || svc.Await(ctx, launch.AskTimeout) == nil {
		return st, err
	}
	return svc.Status(ctx)
}

// leaves reports whether setup leaves the service, standing as st says, as
// it is, having said why: where a router answers that isn't launchd's, as
// handRun says, failing; where the router that answers may not be the one
// the service runs, as Status's Elsewhere says; and where launchd runs a
// router that doesn't answer, failing.
func (r *run) leaves(st service.Status) (bool, error) {
	switch {
	case st.Router != nil && !launchdAnswers(st):
		return true, r.handRun(st, reason{}, "")
	case st.Elsewhere:
		const untold = "no router answers where the service's plist keeps its state, but one answers where this shell keeps it, so which the service runs can't be told"
		const lineUp = installAdvice + ", lines them up"
		r.tell(Said{
			Mark:     Warning,
			Label:    "Left as it is",
			Detail:   untold + ": " + lineUp,
			Sentence: capital(untold) + ", and setup leaves it as it is: " + lineUp + ".",
		})
		return true, nil
	case st.Router == nil && st.Running:
		const unanswered = "it may be starting, or stopping, which can take over half a minute after a bootout, or keeping its state somewhere setup doesn't look"
		const again = "if it doesn't answer soon, as switchboard service status shows, switchboard service install installs it again: give it the --config and --log-level it ran with, if any"
		r.tell(Said{
			Mark:     Failed,
			Label:    "Router",
			Detail:   fmt.Sprintf("launchd runs it, %s, but it doesn't answer here: %s; setup leaves it as it is: %s", pidOf(st.PID), unanswered, again),
			Sentence: fmt.Sprintf("Launchd runs the service's router, %s, but it doesn't answer here: %s. Setup leaves it as it is: %s.", pidOf(st.PID), unanswered, again),
		})
		return true, fmt.Errorf("launchd's router, %s, doesn't answer: if it doesn't soon, run switchboard service install", pidOf(st.PID))
	}
	return false, nil
}

// noService is how setup says there's no service, its plist gone and launchd
// not having loaded it.
const noService = "No service is installed"

// handRun says a router run some other way answers, as st says, after
// why, the plist's state where setup can't use it, and what launchd has of
// the service, where why doesn't say it, as noService does; and fails, short
// naming the plist's state, as no router could start beside it, and only
// service install, once it's stopped, puts the service right.
func (r *run) handRun(st service.Status, why reason, short string) error {
	const answers = "a router run some other way answers"
	said, detail, failed := capital(answers), answers, "a router run by hand answers"
	if why.lead != "" {
		said, detail, failed = why.lead+", and "+answers, why.detail+", and "+answers, short+", and a router run by hand answers"
	}
	while, left := ", while "+launchdHas(st), "them as they are"
	if why.lead == noService {
		while, left = "", "it as it is"
	}
	const stop = "stop the router run by hand, then run switchboard service install"
	r.tell(Said{
		Mark:     Failed,
		Label:    "Router run by hand",
		Detail:   fmt.Sprintf("%s, %s%s: setup leaves %s; %s", detail, pidOf(st.Router.PID), while, left, stop),
		Sentence: fmt.Sprintf("%s, %s%s: setup leaves %s. %s.", said, pidOf(st.Router.PID), while, left, capital(stop)),
	})
	return fmt.Errorf("%s: stop it, then run switchboard service install", failed)
}

// placeless says where the service's router finds its state or its config
// file can't be worked out from its plist, as err says, and fails, as the
// plist can't start a router again: leaving a router that answers as it
// is, as handRun says where it isn't launchd's, and naming service install.
func (r *run) placeless(ctx context.Context, svc *service.Service, err error) error {
	unplaced := redact.Text(err.Error())
	why := reason{
		lead:   fmt.Sprintf("Where the service's router finds its state or its config can't be worked out from its plist (%s)", unplaced),
		detail: fmt.Sprintf("where the service's router finds its state or its config can't be worked out from its plist (%s)", unplaced),
	}
	short := "the service's plist: " + unplaced
	const afresh = installAdvice + ", installs it afresh"
	// placeless tells of the plist, what follows why being stands.
	placeless := func(stands string) {
		r.tell(Said{
			Mark:     Failed,
			Label:    "Plist",
			Detail:   why.detail + stands,
			Sentence: why.lead + stands + ".",
		})
	}
	st, err := r.status(ctx, svc)
	switch {
	case err != nil:
		unasked := redact.Text(err.Error())
		r.tell(Said{
			Mark:     Failed,
			Label:    "Plist",
			Detail:   fmt.Sprintf("%s: %s · launchd couldn't be asked about the service: %s", why.detail, afresh, unasked),
			Sentence: fmt.Sprintf("%s: %s. Launchd couldn't be asked about the service: %s", why.lead, afresh, unasked),
		})
		return fmt.Errorf("%s: run switchboard service install: %s", short, unasked)
	case st.Router != nil && !launchdAnswers(st):
		return r.handRun(st, why, short)
	case st.Router != nil:
		r.health = st.Router
		placeless(fmt.Sprintf(", though launchd runs its router, %s, which answers: setup leaves it as it is, but the plist can't start a router again, so %s", pidOf(st.PID), afresh))
	case st.Running:
		placeless(fmt.Sprintf(", and no router answers, while %s: %s", launchdHas(st), afresh))
	default:
		placeless(", and no router answers: " + afresh)
	}
	return fmt.Errorf("%s: run switchboard service install", short)
}

// installFirst installs the LaunchAgent, which isn't installed, as setup's
// options say, in the environment setup runs in, failing where the router
// couldn't start; but not where unusable leaves it.
func (r *run) installFirst(ctx context.Context) error {
	if left, err := r.unusable(ctx, nil); left {
		return err
	}
	svc, opts, err := r.LaunchAgent.Planned(ctx, r.Install)
	if err != nil {
		return err
	}
	if err := r.startable(svc, opts); err != nil {
		return fmt.Errorf("the router couldn't start: %w", err)
	}
	return r.install(ctx, svc, opts, false)
}

// unusable judges the service where setup has no plist it can use: none, or
// one it can't read back, as unread says. Where launchd runs a router, or
// one answers, or launchd has loaded a plist it can't read back, it leaves
// both as they are, saying truly what runs, and fails: a plist launchd can't
// read, or none, can't start the router at login, so the step isn't done,
// and only service install, which writes the plist afresh, puts that right.
// It reports whether it left the service so; where it didn't, setup installs
// it afresh.
func (r *run) unusable(ctx context.Context, unread error) (bool, error) {
	st, err := r.status(ctx, r.LaunchAgent)
	switch {
	case err != nil:
		return true, err
	// Nothing answers and nothing runs, so there's nothing to swap out; but
	// with a plist it can't read, setup can't keep the options it holds.
	case st.Router == nil && !st.Running && (!st.Loaded || unread == nil):
		return false, nil
	}
	plist, short := "The service's plist is gone", "the service's plist is gone"
	detail := short
	switch {
	case unread != nil:
		plist = fmt.Sprintf("The service's plist can't be read back (%s)", redact.Text(unread.Error()))
		short = "the service's plist can't be read back"
		detail = fmt.Sprintf("the service's plist can't be read back (%s)", redact.Text(unread.Error()))
	case !st.Loaded:
		plist, short, detail = noService, "no service is installed", "no service is installed"
	}
	// unusable tells of the plist, what follows plist being stands.
	unusable := func(stands string) {
		const leaves = ": setup leaves it as it is, and switchboard service install writes the plist afresh"
		r.tell(Said{Mark: Failed, Label: "Plist", Detail: detail + stands + leaves, Sentence: plist + stands + leaves + "."})
	}
	switch {
	case launchdAnswers(st):
		r.health = st.Router
		unusable(fmt.Sprintf(", though launchd runs its router, %s, which answers", pidOf(st.PID)))
		return true, fmt.Errorf("%s: run switchboard service install", short)
	case st.Router != nil:
		return true, r.handRun(st, reason{lead: plist, detail: detail}, short)
	}
	unusable(", and no router answers, while " + launchdHas(st))
	return true, fmt.Errorf("%s, and no router answers: run switchboard service install", short)
}

// launchdAnswers reports whether the router that answers, as st says, is
// launchd's: launchd runs one, or print didn't say whether it does, and,
// where both the router and launchd say its pid, it's that one.
func launchdAnswers(st service.Status) bool {
	return st.Router != nil && (st.Running || st.StateUnread) && (st.Router.PID == 0 || st.PID == 0 || st.Router.PID == st.PID)
}

// launchdHas says what launchd has of the service, as st says, of a router
// that doesn't answer where setup looks.
func launchdHas(st service.Status) string {
	switch {
	case st.Running:
		return fmt.Sprintf("launchd runs a router, %s, which doesn't answer where setup looks", pidOf(st.PID))
	case st.Loaded:
		return "launchd has the service loaded, but runs no router"
	}
	return "launchd hasn't loaded the service"
}

// pidOf says a router's pid, where it says one.
func pidOf(pid int) string {
	if pid == 0 {
		return "its pid unknown"
	}
	return fmt.Sprintf("pid %d", pid)
}

// reinstallUnread installs the LaunchAgent afresh, as setup's options say,
// in the environment setup runs in, its plist being one setup couldn't read
// back, as err says, once it has checked the router could start so; but not
// where unusable leaves it.
func (r *run) reinstallUnread(ctx context.Context, unread error) error {
	if left, err := r.unusable(ctx, unread); left {
		return err
	}
	unreadable := redact.Text(unread.Error())
	why := reason{
		lead:   fmt.Sprintf("The service's plist can't be read back (%s)", unreadable),
		detail: fmt.Sprintf("its plist couldn't be read back (%s)", unreadable),
	}
	svc, opts, err := r.LaunchAgent.Planned(ctx, r.Install)
	if err != nil {
		return err
	}
	if err := r.startable(svc, opts); err != nil {
		return r.unstartable(ctx, svc, why, err)
	}
	r.tell(why.afresh(""))
	return r.install(ctx, svc, opts, false)
}

// unstartable says, why it was to be, that the service isn't installed
// afresh, as the router couldn't start so, err saying why, and how to
// install it once that's put right; and leaves it as it is while its router
// answers, or fails while none does.
func (r *run) unstartable(ctx context.Context, svc *service.Service, why reason, err error) error {
	why = cmp.Or(why, reason{lead: "Launchd hasn't loaded the service", detail: "launchd hasn't loaded it"})
	h := svc.Answering(ctx)
	if h == nil {
		const none = ", but the router couldn't start installed afresh, and none is running: " + installAdvice + ", installs it once what stops it is put right"
		r.tell(Said{Mark: Failed, Label: "Router", Detail: why.detail + none, Sentence: why.lead + none + "."})
		return fmt.Errorf("the router couldn't start: %w", err)
	}
	r.health = h
	const left = ", but the router couldn't start installed afresh, so the service is left as it is, its router running: " + installAdvice + ", installs it afresh once what stops it is put right"
	refused := redact.Text(err.Error())
	r.tell(Said{
		Mark:     Warning,
		Label:    "Left as it is",
		Detail:   why.detail + left + " · it couldn't start as: " + refused,
		Sentence: why.lead + left + ". It couldn't start as:\n" + refused,
	})
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
	return service.Places{Tokens: r.Accounts.Tokens.Dir(), ConfigFile: file, ClaudeDir: skill.ConfigDirOf(r.SkillFile)}, nil
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
		const skillless = "the router finds no Claude Code config directory, as its plist sets HOME empty, so it won't keep the skill up to date: " + installAdvice + ", lines that up"
		r.tell(Said{Mark: Warning, Label: "Skill", Detail: skillless, Sentence: capital(skillless) + "."})
	}
	apart := routers.Apart(setups)
	if len(apart) > 0 {
		detail := fmt.Sprintf("the router uses another %s than setup: %s, lines them up", prose.List(apart), installAdvice)
		sentence := capital(detail) + "."
		if slices.Contains(apart, "config file") {
			const named = "for the config file, switchboard setup --config, naming the router's, does too"
			detail += " · " + named
			sentence += " " + capital(named) + "."
		}
		r.tell(Said{Mark: Warning, Label: "Elsewhere", Detail: detail, Sentence: sentence})
	}
	return routers.Shared(setups), nil
}

// stale says why the service, installed with opts, and seen as svc, is to
// be installed afresh, and whether it is: when it runs another switchboard
// than this one, setup is given another config file than the router serves,
// as given says, or its plist differs from the one this switchboard writes;
// or, saying nothing, when launchd hasn't loaded it.
func (r *run) stale(ctx context.Context, svc *service.Service, opts service.InstallOptions, given bool, st service.Status) (reason, bool, error) {
	if opts.Executable != r.switchboard {
		return reason{
			lead:   fmt.Sprintf("The service runs %s, not this switchboard, %s", opts.Executable, r.switchboard),
			detail: fmt.Sprintf("it ran %s, not this switchboard, %s", opts.Executable, r.switchboard),
		}, true, nil
	}
	if given {
		return reason{
			lead:   "Setup is given another config file than the one the router serves",
			detail: "setup is given another config file than the one the router served",
		}, true, nil
	}
	drift, err := svc.Drift(ctx, opts)
	switch {
	case err != nil:
		return reason{}, false, err
	case len(drift) > 0:
		return reason{
			lead:   "The service's plist differs from the one this switchboard writes, with " + prose.List(drift),
			detail: "its plist had " + prose.List(drift),
		}, true, nil
	}
	return reason{}, !st.Loaded, nil
}

// reason is why the service is installed afresh: as the line walk's
// sentence leads with it, and as a Said's detail words it.
type reason struct {
	lead, detail string
}

// afresh says the service is installed afresh, as why says, keeping the
// router's flags kept names, "" for none.
func (why reason) afresh(kept string) Said {
	detail := "reinstalled: " + why.detail
	if kept != "" {
		detail += " · kept its " + kept
	}
	return Said{Mark: Warning, Label: "Updated", Detail: detail, Sentence: why.lead + ", so it's installed afresh."}
}

// kept names the router's flags the service, installed afresh with opts,
// keeps from its plist, never their values: the config file, but for one
// setup is given in its place, as given says, and the log level.
func kept(opts service.InstallOptions, given bool) string {
	var flags []string
	if opts.Config != "" && !given {
		flags = append(flags, "--config")
	}
	if opts.LogLevel != "" {
		flags = append(flags, "--log-level")
	}
	return prose.List(flags)
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
	r.tell(Said{
		Mark:     Done,
		Label:    "Installed",
		Detail:   r.fromHome(svc.Plist()),
		Sentence: fmt.Sprintf("Installed %s: launchd starts the router now, at every login, and whenever it stops.", svc.Plist()),
	})
	for _, warning := range installed.Warnings {
		r.tell(Said{Mark: Warning, Label: "Warning", Detail: warning, Sentence: fmt.Sprintf("Warning: %s.", warning)})
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
		finishing := fmt.Sprintf("finishing its requests in flight, then %s", how)
		r.tell(Said{Mark: Warning, Label: "Restarting", Detail: finishing, Sentence: "The router is " + finishing + "."})
	})
	if err != nil {
		return err
	}
	r.tell(Said{Mark: Warning, Label: "Restarted", Detail: "the router didn't answer", Sentence: "Restarted the router, as it didn't answer."})
	return r.up(ctx, svc, opts, h, true)
}

// placesOfStarted says where the router launchd started, svc's, installed
// with opts, finds its state, its config or Claude Code's config elsewhere
// than setup, as apart does, seen where it answers: saying so first where
// that's where this shell keeps its state, not where the plist does.
func (r *run) placesOfStarted(ctx context.Context, svc *service.Service, opts service.InstallOptions) error {
	running, elsewhere := svc.Found(ctx)
	if elsewhere {
		const started = "launchd started the router where this shell keeps its state, not where the service's plist does: " + installAdvice + ", lines them up"
		r.tell(Said{Mark: Warning, Label: "Elsewhere", Detail: started, Sentence: capital(started) + "."})
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
	r.health = h
	said := routerUp(*h)
	said.Sentence = "The router is up: " + service.Health(*h) + "."
	r.tell(said)
	return nil
}

// routerUp is the Said, but for its sentence, of the service's router up,
// answering as h says.
func routerUp(h router.Health) Said {
	said := Said{Mark: Done, Label: "Router", Detail: routerDetail(h)}
	if !h.OK {
		said.Mark = Warning
	}
	return said
}

// routerDetail words a router's answer as the service page shows it: how it
// stands, the address it listens on and its version, each where it says
// one, such as "healthy · 127.0.0.1:4747 · v0.2.10".
func routerDetail(h router.Health) string {
	parts := []string{service.Healthy(h)}
	for _, part := range []string{h.Listen, prose.Version(h.Version)} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " · ")
}
