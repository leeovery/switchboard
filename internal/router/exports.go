package router

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
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
// export <dir> writes it: as the router starts, and every exportEvery after,
// each once the history's writers have compressed what's due, so a day
// compressed goes as its final file. An export never holds the router up:
// each runs on a goroutine of its own, which nothing waits on, the router
// stopping included, as a write into a folder macOS guards waits until the
// user answers its prompt; and a round that comes while one into the same
// folder runs starts none. A failure is logged once until an export
// succeeds. What the router keeps of them, for the status document, is how
// they stand since it started: the folder, when an export into it last
// succeeded, when the one under way began, and why the last failed, until
// one succeeds.
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
	// restarting reports whether the router is to restart itself.
	restarting func() bool

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
	// read, by what they are.
	unread map[string]bool
	// running are the folders exports run into, each by when the export into
	// it began, in UTC.
	running map[string]time.Time
}

// newExports returns the exports of a router built from cfg, into the folder
// its Export names, from its state directory, by its clock, each once tend
// has had the history's writers compress what's due, and none while
// restarting reports the router is to restart.
func newExports(cfg Config, tend func(ctx context.Context) error, restarting func() bool) *exports {
	x := &exports{
		every:      exportEvery,
		now:        cfg.Now,
		homeDir:    cfg.HomeDir,
		exportInto: export.Exporter{StateDir: cfg.StateDir, Now: cfg.Now, Logger: logger, NoParents: true}.Export,
		tend:       tend,
		restarting: restarting,
		running:    make(map[string]time.Time),
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
	x.at, x.why, x.unread = time.Time{}, "", nil
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

// run makes a round at once, then every x.every, until ctx ends, leaving the
// exports still running then to run on, unwaited.
func (x *exports) run(ctx context.Context) {
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
// the folder, reporting false, beginning none, where none is configured, or
// one into it runs still. One into another folder, as the one configured
// before, held as a write macOS guards is, holds none up.
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
	}
	x.running[x.folder] = x.now().UTC()
	return target, true
}

// end notes the export into dir ended.
func (x *exports) end(dir string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	delete(x.running, dir)
}

// export has the history's writers compress what's due, then exports into
// target, noting how it went, as note does: nothing once ctx has ended, or a
// writer has stopped, as the router stops; nor while the router is to
// restart itself, as when the config naming the folder has just changed, as
// the router it becomes exports as it starts. Begun now, an export macOS
// holds on its prompt would still be waiting in the kernel as the router
// replaces itself, as nothing knows yet whether that cuts the wait short.
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
// success after logged too; and each day's file it couldn't read, as
// noteUnread says, which fails no export.
func (x *exports) note(dir string, r export.Result, err error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if dir != x.folder {
		return
	}
	failing := x.why != ""
	if err != nil {
		x.why = plainly(dir, err)
		if !failing {
			logger.Warn("can't export the history; trying again at each round", "dir", dir, "error", x.why)
		}
		return
	}
	if failing {
		logger.Info("exporting the history again", "dir", dir)
	}
	x.at, x.why = x.now().UTC(), ""
	x.noteUnread(r.Unread)
	logger.Debug("exported the history", "folder", r.Folder, "written", r.Written, "as_they_were", r.AsTheyWere)
}

// noteUnread warns of each of unread, the days' files an export passed over
// as it couldn't read them, that the export before read, so one is warned of
// once until an export reads it.
func (x *exports) noteUnread(unread []export.DayError) {
	was := x.unread
	x.unread = make(map[string]bool, len(unread))
	for _, u := range unread {
		x.unread[u.What] = true
		if !was[u.What] {
			logger.Warn("couldn't export "+u.What+"; passed over", "error", u.Err)
		}
	}
}

// plainly is why an export into dir failed, as err says, in plain words:
// macOS refusing the router the folder, as refusedByMacOS says, saying where
// to allow it; else err's own words, which say where the folder isn't there.
func plainly(dir string, err error) string {
	if refusedByMacOS(dir, err) {
		return "macOS refused access to " + dir + ": allow switchboard in System Settings → Privacy & Security"
	}
	return err.Error()
}

// refusedByMacOS reports whether err is macOS refusing the router the folder
// dir: an EPERM from one of refusingOps on a path in it. One from anything
// else, as a rename onto a file the Finder has locked, or setting the time of
// a file made already, says its own cause.
func refusedByMacOS(dir string, err error) bool {
	failed, ok := errors.AsType[*fs.PathError](err)
	return ok && errors.Is(failed.Err, syscall.EPERM) && slices.Contains(refusingOps, failed.Op) && inFolder(dir, failed.Path)
}

// inFolder reports whether path is the folder dir, or a path in it.
func inFolder(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
