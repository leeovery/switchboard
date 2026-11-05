package dayfile

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// pruneLook is how often a Writer makes its round, looking at whether a new
// day has come, and its files are to be pruned, with nothing to write.
const pruneLook = time.Hour

// WriterOptions say how a Writer writes to its files, and keeps them.
type WriterOptions struct {
	// Queue is how many of what's noted can wait to be written: past that,
	// what's noted is dropped rather than wait.
	Queue int
	// Keep is how long a day's files are kept, from the end of its day:
	// Forever keeps every day's.
	Keep time.Duration
	// Now reads the clock the days go by.
	Now func() time.Time
	// Items says what the lines hold, as "readings", in what's logged of
	// those dropped or unwritten.
	Items string
	// Round, where it's given, is called with the clock's now on each of the
	// Writer's rounds, on Run's goroutine, before the files are pruned: what
	// else is kept of the files' days is kept on it, while their lines are
	// still there.
	Round func(now time.Time)
	// Pruning, where it's given, is called with the clock's now just before
	// each prune of the files, once a day, on Run's goroutine: what's kept of
	// other days that needs the files' lines, as the request ledger's
	// summaries need the readings history's, is kept on it, while they're
	// still there, and needn't be on every round.
	Pruning func(now time.Time)
	// Spare, where it's given, reports whether the files of the local day
	// with the given date, past keeping, are kept all the same, until a later
	// prune: what's kept of the day needs its lines still, as a request
	// ledger's day without a summary that stands does. It's asked of each day
	// past keeping before each prune, on Run's goroutine, holding nothing of
	// the files.
	Spare func(date string) bool
}

// Writer writes what's noted to it to its files, on Run's goroutine, and
// keeps them, making its round as Run starts, every pruneLook after, and as
// the first write of a day comes, pruning them on the first round of each
// day, after its Round and its Pruning. Noting never waits: what's noted
// queues for Run, and what's past the queue's end is dropped, logged once
// until a write succeeds. A write that fails is logged, once until one
// succeeds, and what it held goes unwritten. T is what's noted at a time, as
// a taking in of readings.
type Writer[T any] struct {
	files     *Files
	linesOf   func(T) Lines
	keep      time.Duration
	now       func() time.Time
	items     string
	onRound   func(now time.Time)
	onPruning func(now time.Time)
	spare     func(date string) bool
	queue     chan T
	// work takes what Do hands Run's goroutine to do, as Run takes it up.
	work chan func()
	// stopped is closed as Run returns, and made afresh as a Run after it
	// starts, as one Writer can be run again; runs guards it.
	stopped chan struct{}
	runs    sync.Mutex
	// dropping is set once what's noted is dropped for the queue being full,
	// until a write succeeds.
	dropping atomic.Bool

	// Only Run's goroutine touches what follows.
	failing bool
	// pruned is the date of the day the files were last pruned on.
	pruned string
}

// NewWriter returns a Writer of files, which writes each thing noted to it as
// the lines linesOf returns of it, as opts say.
func NewWriter[T any](files *Files, linesOf func(T) Lines, opts WriterOptions) *Writer[T] {
	return &Writer[T]{files: files, linesOf: linesOf, keep: opts.Keep, now: opts.Now, items: opts.Items, onRound: opts.Round, onPruning: opts.Pruning,
		spare: opts.Spare, queue: make(chan T, opts.Queue), work: make(chan func()), stopped: make(chan struct{})}
}

// Note queues v for Run to write. It never waits: with the queue full, v is
// dropped.
func (w *Writer[T]) Note(v T) {
	select {
	case w.queue <- v:
	default:
		if !w.dropping.Swap(true) {
			w.files.Logger.Warn(w.files.Name + " fell behind; " + w.items + " dropped from it")
		}
	}
}

// Run writes what's noted, making its round as it starts and every pruneLook
// after, and does what Do hands it, until ctx ends, when it writes what's
// still queued. It makes the files' directory private as it starts. A
// Writer runs one Run at a time.
func (w *Writer[T]) Run(ctx context.Context) {
	defer close(w.starting())
	w.files.makePrivate()
	w.round()
	look := time.NewTicker(pruneLook)
	defer look.Stop()
	for {
		select {
		case v := <-w.queue:
			w.write(v)
		case work := <-w.work:
			work()
		case <-look.C:
			w.round()
		case <-ctx.Done():
			w.drain()
			return
		}
	}
}

// ErrStopped is the error of work handed to a Writer whose Run has returned,
// which nothing takes up.
var ErrStopped = errors.New("the writer has stopped")

// starting returns what a Run starting closes as it returns: the Writer's
// stopped, made afresh where a Run before closed it.
func (w *Writer[T]) starting() chan struct{} {
	w.runs.Lock()
	defer w.runs.Unlock()
	select {
	case <-w.stopped:
		w.stopped = make(chan struct{})
	default:
	}
	return w.stopped
}

// stoppedRun returns what's closed once Run has returned, until a Run after
// it starts.
func (w *Writer[T]) stoppedRun() <-chan struct{} {
	w.runs.Lock()
	defer w.runs.Unlock()
	return w.stopped
}

// Do does work on Run's goroutine, between its writes and its rounds, so
// nothing else is done with the files while it runs, and returns nil once
// it's done. It waits for Run to take the work up as long as ctx lets it,
// Run not yet started included, returning ctx's error once ctx has ended
// and ErrStopped at once where Run has returned, either having done none of
// it. Once Run has taken it up, the work runs to its end, but ctx ending
// first returns ctx's error.
func (w *Writer[T]) Do(ctx context.Context, work func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	done := make(chan struct{})
	select {
	case w.work <- func() {
		work()
		close(done)
	}:
	case <-w.stoppedRun():
		return ErrStopped
	case <-ctx.Done():
		return ctx.Err()
	}
	return doneBy(ctx, done)
}

// doneBy waits for done to close, returning nil, or for ctx to end,
// returning its error: nil where both have, so work that's done is never
// reported undone.
func doneBy(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
	case <-ctx.Done():
	}
	select {
	case <-done:
		return nil
	default:
		return ctx.Err()
	}
}

// RoundNow makes the Writer's round at once, as it makes one every pruneLook,
// on Run's goroutine, and returns as Do does.
func (w *Writer[T]) RoundNow(ctx context.Context) error {
	return w.Do(ctx, w.round)
}

// TendNow makes the Writer's round at once where the files are due their
// prune, as on its first round of a day, compressing and pruning them, on
// Run's goroutine, and returns as Do does. Where they've been pruned on the
// day already, it does nothing, so its Round keeps what it keeps no more
// often than its own rounds do, nor warns of more.
func (w *Writer[T]) TendNow(ctx context.Context) error {
	return w.Do(ctx, func() {
		if !w.prunedOn(w.now()) {
			w.round()
		}
	})
}

// round has the Writer's Round, where it has one, keep what it keeps of the
// files' days, then prunes the files on a day they haven't been pruned on,
// noting the day it did, after its Pruning, where it has one, sparing the
// days spared says: no prune comes but after both.
func (w *Writer[T]) round() {
	now := w.now()
	if w.onRound != nil {
		w.onRound(now)
	}
	if w.prunedOn(now) {
		return
	}
	if w.onPruning != nil {
		w.onPruning(now)
	}
	w.pruned = DateOf(now)
	w.files.prune(now, w.keep, w.spared(now))
}

// spared returns what reports whether the files of a day past keeping at now
// are spared the prune, as the Writer's Spare says, asked before it, so the
// prune asks it nothing of the files: none where there's no Spare, and every
// one where the files can't be listed to ask.
func (w *Writer[T]) spared(now time.Time) func(date string) bool {
	if w.spare == nil || w.keep == Forever {
		return nil
	}
	past, err := w.files.dates(func(l listed) bool { return pastKeepingAt(l.day, now, w.keep) })
	if err != nil {
		return func(string) bool { return true }
	}
	spared := make(map[string]bool)
	for _, date := range past {
		spared[date] = w.spare(date)
	}
	return func(date string) bool { return spared[date] }
}

// prunedOn reports whether the files have been pruned on the local day now
// falls on.
func (w *Writer[T]) prunedOn(now time.Time) bool {
	return DateOf(now) == w.pruned
}

// drain writes what's still queued.
func (w *Writer[T]) drain() {
	for {
		select {
		case v := <-w.queue:
			w.write(v)
		default:
			return
		}
	}
}

// write appends v's lines to the files, logging a failure once until a write
// succeeds. On a day the files haven't been pruned on, it makes its round
// first: a sleep holds the hourly round back, so a day's first write can come
// before it.
func (w *Writer[T]) write(v T) {
	if !w.prunedOn(w.now()) {
		w.round()
	}
	err := w.files.Append(w.linesOf(v))
	switch {
	case err != nil && !w.failing:
		w.files.Logger.Warn("can't write the "+w.files.Name+"; "+w.items+" go unwritten until it can", "dir", w.files.Dir, "error", err)
	case err == nil && w.failing:
		w.files.Logger.Info("writing the "+w.files.Name+" again", "dir", w.files.Dir)
	}
	w.failing = err != nil
	if err == nil {
		w.dropping.Store(false)
	}
}
