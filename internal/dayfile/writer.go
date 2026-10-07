package dayfile

import (
	"context"
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
}

// Writer writes what's noted to it to its files, on Run's goroutine, and
// keeps them, making its round as Run starts, every pruneLook after, and as
// the first write of a day comes, pruning them on the first round of each
// day, after its Round. Noting never waits: what's noted queues for Run, and
// what's past the queue's end is dropped, logged once until a write
// succeeds. A write that fails is logged, once until one succeeds, and what
// it held goes unwritten. T is what's noted at a time, as a taking in of
// readings.
type Writer[T any] struct {
	files   *Files
	linesOf func(T) Lines
	keep    time.Duration
	now     func() time.Time
	items   string
	onRound func(now time.Time)
	queue   chan T
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
	return &Writer[T]{files: files, linesOf: linesOf, keep: opts.Keep, now: opts.Now, items: opts.Items, onRound: opts.Round, queue: make(chan T, opts.Queue)}
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
// after, until ctx ends, when it writes what's still queued. It makes the
// files' directory private as it starts.
func (w *Writer[T]) Run(ctx context.Context) {
	w.files.makePrivate()
	w.round()
	look := time.NewTicker(pruneLook)
	defer look.Stop()
	for {
		select {
		case v := <-w.queue:
			w.write(v)
		case <-look.C:
			w.round()
		case <-ctx.Done():
			w.drain()
			return
		}
	}
}

// round has the Writer's Round, where it has one, keep what it keeps of the
// files' days, then prunes the files on a day they haven't been pruned on,
// noting the day it did: no prune comes but after the Round.
func (w *Writer[T]) round() {
	now := w.now()
	if w.onRound != nil {
		w.onRound(now)
	}
	if !w.prunedOn(now) {
		w.pruned = dateOf(now)
		w.files.prune(now, w.keep)
	}
}

// prunedOn reports whether the files have been pruned on the local day now
// falls on.
func (w *Writer[T]) prunedOn(now time.Time) bool {
	return dateOf(now) == w.pruned
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
