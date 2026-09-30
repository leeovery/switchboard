package router

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/status"
)

// Why the router restarts itself, as the log gives it.
const (
	restartForConfig  = "config changed"
	restartForUpgrade = "upgraded"
	restartForZone    = "time zone changed"
	restartAsked      = "asked to"
)

// restarts restarts the router once what it was started from has changed:
// its config file, into another valid config; its binary, which leads to
// another file than the one running, as after an upgrade; or the system's
// time zone, whose file does, as when the Mac is taken to another time zone.
// A restart waits for a moment with no request in flight, and for the config
// file to make a valid config, which the router started again needs. Only a
// supervised router, started again whenever it exits, restarts: any other
// logs, once, that a restart is due. Either reports a restart due, which the
// status document gives. A supervised router also restarts at once when
// asked to, as service restart asks.
type restarts struct {
	config     *configFile
	binary     *ledFile
	zone       *ledFile
	supervised bool
	inFlight   *inFlight
	now        func() time.Time
	// told is set once the log has said a restart is due.
	told bool
	// restarted closes as the router restarts, reason then saying why.
	restarted  chan struct{}
	restarting sync.Once
	reason     string

	mu sync.Mutex
	// pending is the restart due as what the router was started from last
	// looked, zero while none is.
	pending status.Restart
}

func newRestarts(config, binary, zone Watched, supervised bool, inFlight *inFlight, now func() time.Time) *restarts {
	return &restarts{
		config:     &configFile{path: config.path, seen: config.found, valid: true},
		binary:     &ledFile{path: binary.path, running: binary.found, news: "the binary leads to another file, as after an upgrade"},
		zone:       &ledFile{path: zone.path, running: zone.found, news: "the time zone's file leads to another, as in another time zone"},
		supervised: supervised,
		inFlight:   inFlight,
		now:        now,
		restarted:  make(chan struct{}),
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
// and logs a restart they've made due.
func (r *restarts) look() {
	r.config.look()
	r.binary.look()
	r.zone.look()
	why := r.due()
	r.note(why)
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

// note notes why a restart is due, "" for none, for report to give: since it
// was first found due, while it has been due since.
func (r *restarts) note(why string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case why == "":
		r.pending = status.Restart{}
	case !r.pending.Due():
		r.pending = status.Restart{Reason: why, Since: r.now().UTC(), ByHand: !r.supervised}
	default:
		r.pending.Reason = why
	}
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
// has no request in flight, or nil while it's not to restart.
func (r *restarts) ready() <-chan struct{} {
	if !r.supervised || r.due() == "" {
		return nil
	}
	return r.inFlight.quiet()
}

// restart restarts the router, once a look at what it was started from finds
// a restart still due, and reports whether it did: the config file may have
// changed since it was last looked at, into a config the router couldn't
// start again from.
func (r *restarts) restart() bool {
	r.look()
	why := r.due()
	if why == "" {
		return false
	}
	r.begin(why)
	return true
}

// restartable fails, saying why, when the router can't restart at once, as
// it's asked to: it isn't supervised, so nothing would start it again, or its
// config file doesn't make a valid config, which the router started again
// couldn't start from. It reads the config file afresh, and is safe for
// concurrent use.
func (r *restarts) restartable() error {
	if !r.supervised {
		return errors.New("the router was run by hand, with switchboard serve, so nothing would start it again: stop it, and run it again")
	}
	if r.config.path == "" {
		return nil
	}
	if _, err := config.Load(r.config.path); err != nil {
		return fmt.Errorf("the router's config file doesn't make a valid config, which it couldn't start again from: %w", err)
	}
	return nil
}

// atOnce restarts the router, whatever is in flight, once restartable
// has found it can.
func (r *restarts) atOnce() {
	r.begin(restartAsked)
}

// begin restarts the router, for the reason given, unless it's restarting
// already.
func (r *restarts) begin(why string) {
	r.restarting.Do(func() {
		r.reason = why
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
	// which a router started again can start from.
	valid bool
	// changed is set once the file has made another valid config since the
	// router started, and accounts are those the last it made configures.
	changed  bool
	accounts config.Accounts
}

// look looks at the config file again, and reads it once it has changed:
// a valid config calls for a restart, and an invalid one is refused, logged
// at warn, the router carrying on with the config it has.
func (c *configFile) look() {
	now := statFile(c.path)
	if now.same(c.seen) {
		return
	}
	c.seen = now
	cfg, err := config.Load(c.path)
	if err != nil {
		c.valid = false
		logger.Warn("config change refused; carrying on with the config as it was", "path", c.path, "error", err)
		return
	}
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
	if f.moved || !f.running.there() {
		return
	}
	now := statFile(f.path)
	if !now.there() || now.same(f.running) {
		return
	}
	f.moved = true
	logger.Info(f.news, "path", f.path)
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

// same reports whether f and g are the same file, unchanged, or both not
// there.
func (f fileState) same(g fileState) bool {
	if !f.there() || !g.there() {
		return f.there() == g.there()
	}
	return os.SameFile(f.info, g.info) && f.info.ModTime().Equal(g.info.ModTime())
}
