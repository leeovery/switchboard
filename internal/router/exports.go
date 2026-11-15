package router

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/status"
)

// exportEvery is how often the router exports the history into the folder
// its config names.
const exportEvery = time.Hour

// refusingOps are the operations on a path macOS refuses a program, with
// EPERM, in a folder it guards that the user hasn't let the program into:
// opening a file, a new one included, making a folder, and looking at or
// listing one.
var refusingOps = []string{"open", "mkdir", "stat", "lstat", "fdopendir", "readdir", "readdirent"}

// exports are the router's exports of the history into the folder the
// config's [export] dir names, each Mac into a folder of its own in it, as
// export <dir> writes it: as the router starts, once it has first looked at
// what it was started from, and every exportEvery after, each once the
// history's writers have compressed what's due, so a day compressed goes as
// its final file. An export never holds a request up: each runs on a
// goroutine of its own, which the router stopping never waits on, as a write
// into a folder macOS guards waits until the user answers its prompt, though
// a restart in place does, as restarts says; and a round that comes while
// one into the same folder runs starts none. A failure is logged once until
// an export succeeds. What the router keeps of them, for the status
// document, is how they stand since it started: the folder, when an export
// into it last succeeded, when the one under way began, and why the last
// failed, until one succeeds.
type exports struct {
	every   time.Duration
	now     func() time.Time
	homeDir func() (string, error)
	// exportInto exports the history into dir, as export.Exporter's Export
	// does.
	exportInto func(dir string) (export.Result, error)
	// tend has the history's writers compress and prune their files where
	// that's due, returning once they have, or failing once ctx has ended or
	// a writer has stopped.
	tend func(ctx context.Context) error
	// restarting reports whether the router is to restart itself, and moving
	// whether the history waits on its move onto the accounts' ids, which
	// an export would copy as it stands meanwhile.
	restarting, moving func() bool
	// supervised is set where the router is started again whenever it exits,
	// as the service's is, rather than run by hand from a terminal, which
	// macOS counts the router as part of.
	supervised bool

	mu sync.Mutex
	// configured is the [export] the exports go by, and folder the folder it
	// names, or its dir as given where that can't be found, as unfound says
	// why.
	configured config.Export
	folder     string
	unfound    error
	// at is when an export into folder last succeeded, in UTC, and why why the
	// last failed, until one succeeds: zero, and "", for none.
	at  time.Time
	why string
	// unread are the days' files the last export that succeeded couldn't
	// read, by what they are, and unremoved the files writes cut short left
	// that it couldn't remove, by their paths.
	unread, unremoved map[string]bool
	// running are the folders exports run into, each by when the export into
	// it began, in UTC, and underWay counts them, for a restart in place to
	// wait until none runs.
	running  map[string]time.Time
	underWay *inFlight
	// heldOff counts the restarts asked for that hold every export off from
	// beginning, as holdOff says.
	heldOff int
}

// newExports returns the exports of a router built from cfg, into the folder
// its Export names, from its state directory, by its clock, each once tend
// has had the history's writers compress what's due, and none while
// restarting reports the router is to restart, or moving that the history
// waits on its move onto the accounts' ids.
func newExports(cfg Config, tend func(ctx context.Context) error, restarting, moving func() bool) *exports {
	x := &exports{
		every:      exportEvery,
		now:        cfg.Now,
		homeDir:    cfg.HomeDir,
		exportInto: export.Exporter{StateDir: cfg.StateDir, Now: cfg.Now, Logger: logger, Accounts: cfg.Accounts, Pools: cfg.Pools, NoParents: true}.Export,
		tend:       tend,
		restarting: restarting,
		moving:     moving,
		supervised: cfg.Supervised,
		running:    make(map[string]time.Time),
		underWay:   newInFlight(),
	}
	x.configure(cfg.Export)
	return x
}

// configure has the exports go by e from their next round: a folder other
// than the one they went by, or none, starts them afresh, keeping nothing of
// the one before.
func (x *exports) configure(e config.Export) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if e == x.configured {
		return
	}
	dir, err := e.Folder(x.homeDir)
	x.configured, x.folder, x.unfound = e, cmp.Or(dir, e.Dir), err
	x.at, x.why, x.unread, x.unremoved = time.Time{}, "", nil, nil
}

// report is how the exports stand, for the status document: zero where the
// config names no folder.
func (x *exports) report() status.Export {
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.configured.On() {
		return status.Export{}
	}
	return status.Export{Dir: x.folder, At: x.at, Running: x.running[x.folder], Error: x.why}
}

// run makes a round once looked closes, as once the router has first looked
// at what it was started from, so a router that starts with a restart due
// starts no export, then every x.every, until ctx ends, leaving the exports
// still running then to run on, unwaited.
func (x *exports) run(ctx context.Context, looked <-chan struct{}) {
	select {
	case <-ctx.Done():
		return
	case <-looked:
	}
	tick := time.NewTicker(x.every)
	defer tick.Stop()
	for {
		x.round(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// round starts an export into the folder configured, on a goroutine of its
// own, as export makes it, where begin begins one.
func (x *exports) round(ctx context.Context) {
	target, began := x.begin()
	if !began {
		return
	}
	go func() {
		defer x.end(target.dir)
		x.export(ctx, target)
	}()
}

// exportTarget is the folder an export goes into: dir, or the config's dir
// as given where unfound says why its folder can't be found.
type exportTarget struct {
	dir     string
	unfound error
}

// begin notes an export into the folder configured beginning, and returns
// the folder, reporting false, beginning none, where none is configured, one
// into it runs still, a restart is coming, or asked for, as holdOff says,
// or the history waits on its move onto the accounts' ids. One into another folder, as the one configured before, held as a
// write macOS guards is, holds none up.
func (x *exports) begin() (exportTarget, bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	target := exportTarget{dir: x.folder, unfound: x.unfound}
	switch _, running := x.running[x.folder]; {
	case !x.configured.On():
		return target, false
	case running:
		logger.Debug("the last export is still running; this round exports nothing", "dir", x.folder)
		return target, false
	case x.heldOff > 0 || x.restarting():
		logger.Debug("a restart is coming; this round exports nothing", "dir", x.folder)
		return target, false
	case x.moving():
		logger.Debug("the history waits on its move onto the accounts' ids; this round exports nothing", "dir", x.folder)
		return target, false
	}
	x.running[x.folder] = x.now().UTC()
	x.underWay.begin()
	return target, true
}

// end notes the export into dir ended.
func (x *exports) end(dir string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	delete(x.running, dir)
	x.underWay.end()
}

// quiet returns what's closed once no export runs, into any folder.
func (x *exports) quiet() <-chan struct{} {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.underWay.quiet()
}

// runningInto returns the folder an export runs into, as into says.
func (x *exports) runningInto() string {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.into()
}

// into returns the folder an export runs into: the one configured where
// that's one, else the first by name, as the one configured before; "" while
// none runs. Its caller holds mu.
func (x *exports) into() string {
	if _, into := x.running[x.folder]; into {
		return x.folder
	}
	if len(x.running) == 0 {
		return ""
	}
	return slices.Min(slices.Collect(maps.Keys(x.running)))
}

// holdOff holds every export off from beginning, for a restart asked for to
// begin with none running, until release lets them begin again, as where
// that restart is refused; or, where one runs, holds nothing off, and
// returns the folder it runs into, as into says.
func (x *exports) holdOff() (running string, ok bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if dir := x.into(); dir != "" {
		return dir, false
	}
	x.heldOff++
	return "", true
}

// release lets exports begin again, as one holdOff held them off for.
func (x *exports) release() {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.heldOff--
}

// export has the history's writers compress what's due, then exports into
// target, noting how it went, as note does: nothing once ctx has ended, or a
// writer has stopped, as the router stops; nor where a restart has come due
// meanwhile, as when the config naming the folder has just changed, as the
// router it becomes exports as it starts, and an export begun now would
// hold the restart off until it ended, as one macOS holds on its prompt does
// until that's answered.
func (x *exports) export(ctx context.Context, target exportTarget) {
	if target.unfound != nil {
		x.note(target.dir, export.Result{}, target.unfound)
		return
	}
	if err := x.tend(ctx); err != nil {
		return
	}
	if x.restarting() {
		logger.Debug("a restart is coming; the router it becomes exports as it starts", "dir", target.dir)
		return
	}
	r, err := x.exportInto(target.dir)
	x.note(target.dir, r, err)
}

// note notes how the export into dir went, r saying what it did, or err why
// it failed, where dir is the folder configured still: a failure, logged once
// until an export succeeds, as one failing already has a why, and the first
// success after logged too; and each day's file it couldn't read, and each
// file writes cut short left that it couldn't remove, neither of which fails
// an export, each warned of once until an export no longer meets it, as
// warnedOnce says.
func (x *exports) note(dir string, r export.Result, err error) {
	// Saying why can look at the folder, which a disk that has stopped
	// answering holds up: never while holding mu, which report waits on.
	var why string
	if err != nil {
		why = x.plainly(dir, err)
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if dir != x.folder {
		return
	}
	failing := x.why != ""
	if err != nil {
		x.why = why
		if !failing {
			logger.Warn("can't export the history; trying again at each round", "dir", dir, "error", x.why)
		}
		return
	}
	if failing {
		logger.Info("exporting the history again", "dir", dir)
	}
	x.at, x.why = x.now().UTC(), ""
	x.unread = warnedOnce(x.unread, r.Unread, func(u export.DayError) string { return u.What }, func(u export.DayError) {
		logger.Warn("couldn't export "+u.What+"; passed over", "error", u.Err)
	})
	x.unremoved = warnedOnce(x.unremoved, r.Unremoved, func(l export.Leftover) string { return l.Path }, func(l export.Leftover) {
		logger.Warn("can't remove a file an export cut short left; trying again at each round", "path", l.Path, "error", l.Err)
	})
	logger.Debug("exported the history", "folder", r.Folder, "written", r.Written, "as_they_were", r.AsTheyWere)
}

// warnedOnce warns of each of met, what an export met that it's to warn of,
// as warn does, that was, what the export before met, by key, doesn't hold,
// and returns what this one met, by key: so each is warned of once until an
// export no longer meets it.
func warnedOnce[T any](was map[string]bool, met []T, key func(T) string, warn func(T)) map[string]bool {
	now := make(map[string]bool, len(met))
	for _, m := range met {
		now[key(m)] = true
		if !was[key(m)] {
			warn(m)
		}
	}
	return now
}

// plainly is why an export into dir failed, as err says, in plain words: an
// EPERM refusing a change to a path in it, as refusedChange finds one, where
// a file or folder is locked, as lockedBy finds it, saying so; else where
// macOS could have refused the router the folder, saying where to allow it,
// as refusedByMacOS words it; else err's own words, which say where the
// folder isn't there.
func (x *exports) plainly(dir string, err error) string {
	path, byMacOS := refusedChange(dir, err)
	if path == "" {
		return err.Error()
	}
	if locked, ok := lockedBy(path); ok {
		return locked + " is locked: unlock it in the Finder, in Get Info"
	}
	if byMacOS {
		return x.refusedByMacOS(dir)
	}
	return err.Error()
}

// refusedByMacOS is what an export into dir that macOS refused says: where to
// allow the router the folder, switchboard, or, run by hand, the terminal app
// it runs in, which macOS counts it as part of.
func (x *exports) refusedByMacOS(dir string) string {
	if !x.supervised {
		return "macOS refused access to " + dir + ": allow the terminal app the router runs in, in System Settings → Privacy & Security"
	}
	return "macOS refused access to " + dir + ": allow switchboard in System Settings → Privacy & Security"
}

// refusedChange returns the path in dir that err is an EPERM refusing a
// change to, "" for none, and reports whether macOS refusing the router a
// folder it guards could have given it: one from one of refusingOps could;
// one from a rename onto the path, which only a lock gives, or from setting
// the time of a file made already, couldn't.
func refusedChange(dir string, err error) (path string, byMacOS bool) {
	if !errors.Is(err, syscall.EPERM) {
		return "", false
	}
	if failed, ok := errors.AsType[*os.LinkError](err); ok && inFolder(dir, failed.New) {
		return failed.New, false
	}
	if failed, ok := errors.AsType[*fs.PathError](err); ok && inFolder(dir, failed.Path) {
		return failed.Path, slices.Contains(refusingOps, failed.Op)
	}
	return "", false
}

// lockedBy returns what refused a change to path as locked, as locked says:
// path itself, or the folder it's in, which refuses a file or folder made in
// it; reporting false where neither is locked.
func lockedBy(path string) (string, bool) {
	for _, p := range []string{path, filepath.Dir(path)} {
		if locked(p) {
			return p, true
		}
	}
	return "", false
}

// inFolder reports whether path is the folder dir, or a path in it.
func inFolder(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
