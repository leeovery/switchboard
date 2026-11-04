package router

import (
	"cmp"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/status"
)

// exportEvery is how often the router exports the history into the folder
// its config names.
const exportEvery = time.Hour

// exports are the router's exports of the history into the folder the
// config's [export] dir names, each Mac into a folder of its own in it, as
// export <dir> writes it: as the router starts, and every exportEvery after,
// each once the history's writers have made their rounds, so a day a round
// compresses goes as its final file. An export never holds the router up:
// each runs on a goroutine of its own, which nothing waits on, the router
// stopping included, as a write into a folder macOS guards waits until the
// user answers its prompt; and a round that comes while one runs starts
// none. A failure is logged once until an export succeeds. What the router
// keeps of them, for the status document, is how they stand since it
// started: the folder, when an export into it last succeeded, and why the
// last failed, until one succeeds.
type exports struct {
	every   time.Duration
	now     func() time.Time
	homeDir func() (string, error)
	// exportInto exports the history into dir, as export.Exporter's Export
	// does.
	exportInto func(dir string) (export.Result, error)
	// rounds has the history's writers make their rounds at once, returning
	// once they have, or failing once ctx has ended or a writer has stopped.
	rounds func(ctx context.Context) error
	// running is set while an export runs.
	running atomic.Bool

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
}

// newExports returns the exports of a router built from cfg, into the folder
// its Export names, from its state directory, by its clock, once rounds has
// had the history's writers make their rounds.
func newExports(cfg Config, rounds func(ctx context.Context) error) *exports {
	x := &exports{
		every:      exportEvery,
		now:        cfg.Now,
		homeDir:    cfg.HomeDir,
		exportInto: export.Exporter{StateDir: cfg.StateDir, Now: cfg.Now, Logger: logger, NoParents: true}.Export,
		rounds:     rounds,
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
	return status.Export{Dir: x.folder, At: x.at, Error: x.why}
}

// run makes a round at once, then every x.every, until ctx ends, leaving an
// export still running then to run on, unwaited.
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
// own, as exportNow makes it: none where no folder is configured, nor while
// one runs.
func (x *exports) round(ctx context.Context) {
	target := x.target()
	switch {
	case !target.on:
		return
	case !x.running.CompareAndSwap(false, true):
		logger.Debug("the last export is still running; this round exports nothing", "dir", target.dir)
		return
	}
	go func() {
		defer x.running.Store(false)
		x.exportNow(ctx)
	}()
}

// exportTarget is the folder the exports go by: dir, or its dir as given
// where unfound says why it can't be found; and whether one is configured.
type exportTarget struct {
	dir     string
	unfound error
	on      bool
}

// target is the folder configured.
func (x *exports) target() exportTarget {
	x.mu.Lock()
	defer x.mu.Unlock()
	return exportTarget{dir: x.folder, unfound: x.unfound, on: x.configured.On()}
}

// exportNow has the history's writers make their rounds, then exports into
// the folder configured, noting how it went, as note does: nothing where none
// is configured, nor once ctx has ended, or a writer has stopped, as the
// router stops.
func (x *exports) exportNow(ctx context.Context) {
	target := x.target()
	switch {
	case !target.on:
		return
	case target.unfound != nil:
		x.note(target.dir, export.Result{}, target.unfound)
		return
	}
	if err := x.rounds(ctx); err != nil {
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
// macOS refusing the router the folder, as it refuses a program the user
// hasn't allowed into a folder it guards, saying where to allow it; else
// err's own words, which say where the folder isn't there.
func plainly(dir string, err error) string {
	if errors.Is(err, syscall.EPERM) {
		return "macOS refused access to " + dir + ": allow switchboard in System Settings → Privacy & Security"
	}
	return err.Error()
}
