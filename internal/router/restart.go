package router

import (
	"io/fs"
	"os"

	"github.com/leeovery/switchboard/internal/config"
)

// Why the router restarts itself, as the log gives it.
const (
	restartForConfig  = "config changed"
	restartForUpgrade = "upgraded"
)

// restarts restarts the router once what it was started from has changed:
// its config file, into another valid config, or its binary, which leads to
// another file than the one running, as after an upgrade. A restart waits
// for a moment with no request in flight, and for the config file to make a
// valid config, which the router started again needs. Only a supervised
// router, started again whenever it exits, restarts: any other logs, once,
// that a restart is due.
type restarts struct {
	config     *configFile
	binary     *binaryFile
	supervised bool
	inFlight   *inFlight
	// told is set once the log has said a restart is due.
	told bool
	// restarted closes as the router restarts.
	restarted chan struct{}
}

func newRestarts(configPath, binaryPath string, supervised bool, inFlight *inFlight) *restarts {
	return &restarts{
		config:     &configFile{path: configPath, seen: statFile(configPath), valid: true},
		binary:     &binaryFile{path: binaryPath, running: statFile(binaryPath)},
		supervised: supervised,
		inFlight:   inFlight,
		restarted:  make(chan struct{}),
	}
}

// look looks at the config file and the binary again, and logs a restart
// they've made due.
func (r *restarts) look() {
	r.config.look()
	r.binary.look()
	why := r.due()
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

// due says why a restart is due, as the config file and the binary last
// looked, or is "" while none is.
func (r *restarts) due() string {
	switch {
	case !r.config.valid:
		return ""
	case r.config.changed:
		return restartForConfig
	case r.binary.upgraded:
		return restartForUpgrade
	}
	return ""
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
	logger.Info("restarting", "reason", why)
	close(r.restarted)
	return true
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
	// router started.
	changed bool
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
	if _, err := config.Load(c.path); err != nil {
		c.valid = false
		logger.Warn("config change refused; carrying on with the config as it was", "path", c.path, "error", err)
		return
	}
	c.valid, c.changed = true, true
	logger.Info("config changed", "path", c.path)
}

// binaryFile is the switchboard binary the router was started as: the path
// it was run by, such as the Homebrew link the service runs, and the file
// that led to as it started, which is the one running.
type binaryFile struct {
	path    string
	running fileState
	// upgraded is set once the path leads to another file.
	upgraded bool
}

// look looks at where the binary's path leads now: to another file than the
// one running, it's an upgrade. A path that leads nowhere, as it may for a
// moment while an upgrade moves it on, isn't one.
func (b *binaryFile) look() {
	if b.upgraded || !b.running.there() {
		return
	}
	now := statFile(b.path)
	if !now.there() || now.same(b.running) {
		return
	}
	b.upgraded = true
	logger.Info("the binary leads to another file, as after an upgrade", "path", b.path)
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
