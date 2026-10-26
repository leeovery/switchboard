package router

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/handover"
	"github.com/leeovery/switchboard/internal/status"
)

// Why the router restarts itself, as the log gives it.
const (
	restartForConfig  = status.RestartForConfig
	restartForUpgrade = status.RestartForUpgrade
	restartForZone    = status.RestartForZone
	restartAsked      = "asked to"
)

// checkTimeout bounds the router's binary's answer, as it's asked before a
// restart whether it could start: one that hasn't answered by then is
// restarted into all the same.
const checkTimeout = 10 * time.Second

// restarts restarts the router once what it was started from has changed:
// its config file, into another valid config; its binary, which leads to
// another file than the one running, as after an upgrade; or the system's
// time zone, whose file does, as when the Mac is taken to another time zone.
// A restart waits for a moment with no request in flight, and for the config
// file to make a valid config, which the router started again needs; and the
// binary it restarts into is asked whether it could start, the restart held
// while it refuses, as a newer version's rules can refuse a config the
// running version's took, and cleared while the config file and the binary
// stand as they did when it didn't. Only a supervised router, started again
// whenever it exits, restarts: any other logs, once, that a restart is due.
// Either tells of a restart as it falls due, and reports one due, which the
// status document gives. A supervised router also restarts at once when
// asked to, as service restart asks.
type restarts struct {
	config     *configFile
	binary     *ledFile
	zone       *ledFile
	supervised bool
	inFlight   *inFlight
	now        func() time.Time
	// emit hears of each restart as it falls due, and as it's held.
	emit func(Event)
	// ask asks the binary the router restarts into whether it could start,
	// as asking says: nil asks nothing.
	ask func(ctx context.Context) (refusal string, err error)
	// load reads the config file at path, as config.Load does, for
	// restartable to find whether it makes a valid config.
	load func(path string) (*config.Config, error)
	// told is set once the log has said a restart is due.
	told bool
	// held is the binary's refusal to start that holds the restart due: nil
	// while nothing does.
	held *binaryRefusal
	// clearance is the binary's last answer that didn't refuse to start,
	// which clears a restart while the files stand as they did: nil while
	// nothing does.
	clearance *binaryClearance
	// gone is set while the binary wasn't there at the last look, as missing
	// says.
	gone bool
	// restarted closes as the router restarts, reason then saying why.
	restarted chan struct{}
	beginning sync.Once
	reason    string

	mu sync.Mutex
	// pending is the restart due as what the router was started from last
	// looked, held while the binary refuses to start, zero while none is.
	pending status.Restart
}

func newRestarts(file, binary, zone Watched, supervised bool, inFlight *inFlight, now func() time.Time, emit func(Event)) *restarts {
	return &restarts{
		config:     &configFile{path: file.path, seen: file.found, valid: true},
		binary:     &ledFile{path: binary.path, running: binary.found, news: "the binary leads to another file, as after an upgrade"},
		zone:       &ledFile{path: zone.path, running: zone.found, news: "the time zone's file leads to another, as in another time zone"},
		supervised: supervised,
		inFlight:   inFlight,
		now:        now,
		emit:       emit,
		load:       config.Load,
		restarted:  make(chan struct{}),
	}
}

// asking returns what asks the binary of a router built from cfg whether it
// could start, as handover.Ask asks it, by Binary's path, run as Args with
// Environ, as the router restarting would run it, waiting checkTimeout for
// its answer at most: nil when there's nothing to ask, the router given
// nothing to run it with, or not knowing its binary.
func asking(cfg Config) func(ctx context.Context) (string, error) {
	if cfg.Check == nil || cfg.Binary.path == "" {
		return nil
	}
	return func(ctx context.Context) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, checkTimeout)
		defer cancel()
		return handover.Ask(ctx, cfg.Check, cfg.Binary.path, cfg.Args, cfg.Environ)
	}
}

// Watched is a file a router is started from, which it looks after while it
// runs, as Watch found it: its config file, its binary, or the system's time
// zone's. The zero Watched is none.
type Watched struct {
	path  string
	found fileState
}

// Watch finds how the file at path stands now, its links followed, for a
// router started from it to compare it with while it runs. Found before
// anything is read from the file, a change made as the router starts is
// seen. "" is none.
func Watch(path string) Watched {
	return Watched{path: path, found: statFile(path)}
}

// look looks at the config file, the binary and the time zone's file again,
// notes whether the binary is there, lifts a hold either of the first two has
// moved on from, tells of a restart they've made due as it falls due, and
// logs it.
func (r *restarts) look() {
	r.config.look()
	r.binary.look()
	r.zone.look()
	r.gone = r.missing(statFile(r.binary.path))
	r.reconsider()
	why := r.due()
	if r.note(why) {
		r.emit(r.fallen(why))
	}
	if why == "" || r.told {
		return
	}
	r.told = true
	if r.supervised {
		logger.Info("restart due; restarting once no request is in flight", "reason", why)
		return
	}
	logger.Info("restart due; run switchboard serve again to take it up", "reason", why)
}

// fallen is the event telling of a restart falling due for why: held, as the
// binary's refusal to start still holds it, as when the config file has been
// gone a while and comes back as it was, else due.
func (r *restarts) fallen(why string) Event {
	if r.held != nil {
		return RestartHeld{Reason: why, Refusal: r.held.why}
	}
	return RestartDue{Reason: why}
}

// note notes why a restart is due, "" for none, for report to give: since it
// was first found due, while it has been due since. It reports whether the
// restart has fallen due just now.
func (r *restarts) note(why string) (fell bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case why == "":
		r.pending = status.Restart{}
	case !r.pending.Due():
		r.pending = status.Restart{Reason: why, Since: r.now().UTC(), ByHand: !r.supervised, Held: r.held.reason()}
		return true
	default:
		r.pending.Reason = why
	}
	return false
}

// report is the restart due as what the router was started from last looked,
// with the requests in flight now, or zero while none is.
func (r *restarts) report() status.Restart {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending := r.pending
	if pending.Due() {
		pending.InFlight = r.inFlight.requests()
	}
	return pending
}

// due says why a restart is due, as the config file, the binary and the time
// zone's file last looked, or is "" while none is.
func (r *restarts) due() string {
	switch {
	case !r.config.valid:
		return ""
	case r.config.changed:
		return restartForConfig
	case r.binary.moved:
		return restartForUpgrade
	case r.zone.moved:
		return restartForZone
	}
	return ""
}

// configured returns the accounts the last valid config the config file has
// made since the router started configures, which the router takes up as it
// restarts: none while it has made none.
func (r *restarts) configured() config.Accounts {
	return r.config.accounts
}

// ready returns what's closed once a supervised router, with a restart due,
// has no request in flight, or nil while it's not to restart, as while its
// config file made no valid config at the last look, which the next settles,
// while its binary wasn't there at the last look, or while its binary refuses
// to start, until a look finds the config file or the binary moved on.
func (r *restarts) ready() <-chan struct{} {
	if !r.supervised || r.due() == "" || r.config.missed || r.gone || r.held != nil {
		return nil
	}
	return r.inFlight.quiet()
}

// restart restarts the router, once a look at what it was started from finds
// a restart still due, its config file making a valid config, and its binary
// clear to restart into, as clear says, and no request in flight, and reports
// whether it did: the config file may have changed since it was last looked
// at, into a config the router couldn't start again from.
func (r *restarts) restart(ctx context.Context) bool {
	r.look()
	why := r.due()
	if why == "" || r.config.missed || !r.clear(ctx, why) {
		return false
	}
	// Asking the binary takes a while, in which a request can begin: the
	// clearance carries the restart over to the next quiet moment.
	if r.inFlight.requests() > 0 {
		return false
	}
	r.begin(why, r.clearance)
	return true
}

// clear reports whether the binary is clear for the router to restart into
// for why: while the config file and the binary stand as they did when it
// last answered without refusing to start, it is, unasked; while it isn't
// there, it isn't; otherwise it's asked, as asked does, until ctx ends, and
// is unless ctx ends first, either file changes as it's asked, which drops
// its answer for the next quiet moment to ask again, or it refuses, which
// holds the restart.
func (r *restarts) clear(ctx context.Context, why string) bool {
	stood := r.files()
	switch {
	case r.clearance != nil && r.clearance.stood.same(stood):
		return true
	case r.missing(stood.binary):
		return false
	}
	refusal, silent, err := r.asked(ctx)
	switch {
	case err != nil, !r.files().same(stood):
		return false
	case refusal != "":
		r.hold(stood, why, refusal)
		return false
	}
	r.clearance = &binaryClearance{stood: stood, silent: silent}
	return true
}

// missing reports whether the binary, standing as binary, isn't there, as for
// a moment while an upgrade moves its link on, which nothing can restart
// into: never for a router that doesn't know its binary, which asks nothing.
func (r *restarts) missing(binary fileState) bool {
	return r.binary.path != "" && !binary.there()
}

// hold holds the restart due for why, its binary refusing to start, saying
// why, as the config file and the binary stood as it was asked: the log says
// why at warn, an event tells of it, and the restart due reports it, until a
// look finds either moved on.
func (r *restarts) hold(stood checked, why, refusal string) {
	r.held = &binaryRefusal{stood: stood, why: refusal}
	r.mu.Lock()
	r.pending.Held = refusal
	r.mu.Unlock()
	logger.Warn("restart held, as its binary refuses to start; carrying on as it is", "reason", why, "path", r.binary.path, "refusal", refusal)
	r.emit(RestartHeld{Reason: why, Refusal: refusal})
}

// reconsider lifts the hold on the restart due once the config file or the
// binary has moved on since the binary refused to start, as movedTo says, for
// it to be asked again.
func (r *restarts) reconsider() {
	if r.held == nil || !r.held.stood.movedTo(r.files()) {
		return
	}
	r.held = nil
	r.mu.Lock()
	r.pending.Held = ""
	r.mu.Unlock()
	logger.Info("the config file or the binary has changed since the restart was held; asking the binary again")
}

// files returns how the config file and the binary stand now.
func (r *restarts) files() checked {
	return checked{config: statFile(r.config.path), binary: statFile(r.binary.path)}
}

// restartable returns what clears the router to restart at once, as it's
// asked to, for atOnce to restart it with, or fails, saying why, when it
// can't: it isn't supervised, so nothing would start it again; its binary
// isn't there; its config file doesn't make a valid config, which the router
// started again couldn't start from; its binary, asked until ctx ends,
// refuses to start; or either changes as the config file is read or the
// binary asked. A router restarting already can, cleared by nothing. It reads
// the config file afresh, and is safe for concurrent use.
func (r *restarts) restartable(ctx context.Context) (*binaryClearance, error) {
	if !r.supervised {
		return nil, errors.New("the router was run by hand, with switchboard serve, so nothing would start it again: stop it, and run it again")
	}
	if r.restarting() {
		return nil, nil
	}
	// The files are found as they stand before the config file is read, so a
	// save landing as it's read is caught, as one landing as the binary is
	// asked is.
	stood := r.files()
	if r.missing(stood.binary) {
		return nil, fmt.Errorf("the router's binary, %s, isn't there, as for a moment while an upgrade moves it on: try again once it is", r.binary.path)
	}
	if r.config.path != "" {
		if _, err := r.load(r.config.path); err != nil {
			return nil, fmt.Errorf("the router's config file doesn't make a valid config, which it couldn't start again from: %w", err)
		}
	}
	refusal, silent, err := r.asked(ctx)
	now := r.files()
	switch {
	case err != nil:
		return nil, err
	case !now.same(stood):
		return nil, fmt.Errorf("the router's %s changed as it checked whether it could restart: try again", r.changed(stood, now))
	case refusal != "":
		return nil, fmt.Errorf("the router's binary refuses to start, so the router started again would stop: %s", refusal)
	}
	return &binaryClearance{stood: stood, silent: silent}, nil
}

// changed names what changed from stood to now, by its path: "config file,
// /test/config.toml,", "binary, /test/switchboard,", or both.
func (r *restarts) changed(stood, now checked) string {
	var named []string
	if !now.config.same(stood.config) {
		named = append(named, "config file, "+r.config.path+",")
	}
	if !now.binary.same(stood.binary) {
		named = append(named, "binary, "+r.binary.path+",")
	}
	return strings.Join(named, " and its ")
}

// asked asks the binary whether it could start, until ctx ends, and returns
// why it refuses, if it does, and why it couldn't answer, if it couldn't, as
// one from before the check can't, which refuses nothing, as before there was
// one to ask; or fails with ctx's error as it ends first. Nothing to ask
// refuses nothing.
func (r *restarts) asked(ctx context.Context) (refusal string, silent, err error) {
	if r.ask == nil {
		return "", nil, nil
	}
	refusal, silent = r.ask(ctx)
	if ctx.Err() != nil {
		return "", nil, ctx.Err()
	}
	return refusal, silent, nil
}

// restarting reports whether the router is restarting.
func (r *restarts) restarting() bool {
	select {
	case <-r.restarted:
		return true
	default:
		return false
	}
}

// atOnce restarts the router, whatever is in flight, once restartable has
// found it can, cleared as it returned.
func (r *restarts) atOnce(cleared *binaryClearance) {
	r.begin(restartAsked, cleared)
}

// begin restarts the router, for the reason given, unless it's restarting
// already, logging first that its binary couldn't answer, where the answer
// that cleared it was a silence: once, whoever begins it.
func (r *restarts) begin(why string, cleared *binaryClearance) {
	r.beginning.Do(func() {
		r.reason = why
		if silent := cleared.silence(); silent != nil {
			logger.Info("its binary couldn't answer whether it could start; restarting all the same", "path", r.binary.path, "error", silent)
		}
		logger.Info("restarting", "reason", why)
		close(r.restarted)
	})
}

// configFile is the config file the router was started from, as it was last
// looked at.
type configFile struct {
	path string
	seen fileState
	// valid is set while the file, as last looked at, makes a valid config,
	// which a router started again can start from: one look finding it
	// doesn't, as a file caught while it's saved, is missed, and changes
	// nothing until the next finds it so too.
	valid, missed bool
	// changed is set once the file has made another valid config since the
	// router started, and accounts are those the last it made configures.
	changed  bool
	accounts config.Accounts
}

// look looks at the config file again, and reads it once it has changed:
// a valid config calls for a restart, and one that isn't valid at two looks
// in a row is refused, logged at warn, the router carrying on with the config
// it has. A single look finding it invalid changes nothing, noted at debug:
// an editor saving the file can leave it so for a moment.
func (c *configFile) look() {
	now := statFile(c.path)
	if now.same(c.seen) {
		c.missed = false
		return
	}
	cfg, err := config.Load(c.path)
	switch {
	case err != nil && !c.missed:
		c.missed = true
		logger.Debug("config file makes no valid config; looking again before refusing it", "path", c.path, "error", err)
		return
	case err != nil:
		c.seen, c.valid, c.missed = now, false, false
		logger.Warn("config change refused; carrying on with the config as it was", "path", c.path, "error", err)
		return
	}
	c.seen, c.missed = now, false
	c.valid, c.changed, c.accounts = true, true, cfg.Accounts
	logger.Info("config changed", "path", c.path)
}

// ledFile is a file the router was started from by a path that can come to
// lead to another: its binary, by the path it was run by, such as the
// Homebrew link the service runs, or the system's time zone, by
// /etc/localtime, which a program reads once, as it starts. running is the
// file the path led to as the router started, which it runs on.
type ledFile struct {
	path    string
	running fileState
	// news is what the log says once the path leads to another file.
	news string
	// moved is set once it does.
	moved bool
}

// look looks at where the path leads now: to another file than the one
// running, it has moved, as after an upgrade. A path that leads nowhere, as
// it may for a moment while an upgrade moves it on, hasn't.
func (f *ledFile) look() {
	if f.moved || !f.running.there() || !f.running.movedTo(statFile(f.path)) {
		return
	}
	f.moved = true
	logger.Info(f.news, "path", f.path)
}

// binaryRefusal is the binary's refusal to start, which holds a restart: why,
// and how the config file and the binary stood as it was asked.
type binaryRefusal struct {
	stood checked
	why   string
}

// reason is why the binary refuses to start: "" for no hold at all.
func (h *binaryRefusal) reason() string {
	if h == nil {
		return ""
	}
	return h.why
}

// binaryClearance is the binary's answer that didn't refuse to start, which
// clears a restart: how the config file and the binary stood as it was asked,
// and why it couldn't answer, where it couldn't, for the restart to log as it
// begins.
type binaryClearance struct {
	stood  checked
	silent error
}

// silence is why the binary couldn't answer: nil where it answered, or
// nothing cleared the restart.
func (c *binaryClearance) silence() error {
	if c == nil {
		return nil
	}
	return c.silent
}

// checked is how the config file and the binary stand, as the binary is
// asked whether it could start.
type checked struct {
	config, binary fileState
}

// movedTo reports whether either file has moved on from s to t, as
// fileState's movedTo says.
func (s checked) movedTo(t checked) bool {
	return s.config.movedTo(t.config) || s.binary.movedTo(t.binary)
}

// same reports whether s and t stand alike: each file the same, unchanged, or
// not there in both.
func (s checked) same(t checked) bool {
	return s.config.same(t.config) && s.binary.same(t.binary)
}

// fileState is how a file stands, its links followed: which file it is, and
// when it last changed. The zero fileState is a file that isn't there.
type fileState struct {
	info fs.FileInfo
}

// statFile returns how the file at path stands now.
func statFile(path string) fileState {
	info, err := os.Stat(path)
	if err != nil {
		return fileState{}
	}
	return fileState{info: info}
}

// there reports whether the file is there.
func (f fileState) there() bool {
	return f.info != nil
}

// movedTo reports whether the path f was found at has moved on to g: g is
// there, and another file than f, or f changed. A path that leads nowhere, as
// it may for a moment while an upgrade moves a link on, or an editor saves a
// file, hasn't.
func (f fileState) movedTo(g fileState) bool {
	return g.there() && !g.same(f)
}

// same reports whether f and g are the same file, unchanged, or both not
// there.
func (f fileState) same(g fileState) bool {
	if !f.there() || !g.there() {
		return f.there() == g.there()
	}
	return os.SameFile(f.info, g.info) && f.info.ModTime().Equal(g.info.ModTime())
}
