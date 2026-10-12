package readings

import (
	"context"
	"encoding/json"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// queue is how many takings in of readings, an answer's or a probe's each,
// can wait to be written. Past that, their readings are dropped from the
// history rather than hold the router up.
const queue = 1024

// Writer writes the readings noted to it to the history's files, on Run's
// goroutine, and keeps them as long as it was opened to: noting readings
// never waits, as a dayfile.Writer says. A reading is written as a line of
// JSON, filed under the local day it was read on.
type Writer struct {
	writer *dayfile.Writer[[]Reading]
	files  *dayfile.Files
	// unwritable is set once a reading couldn't be put as a line, which is
	// logged once. Only Run's goroutine touches it.
	unwritable bool
}

// Open returns a writer of the history's files, which keeps a day's for keep
// once the day has ended, by now's clock, calling pruning, where it's given,
// just before each prune of them, as dayfile.WriterOptions' Pruning says.
func Open(files *dayfile.Files, keep time.Duration, now func() time.Time, pruning func(now time.Time)) *Writer {
	w := &Writer{files: files}
	w.writer = dayfile.NewWriter(files, w.lines, dayfile.WriterOptions{Queue: queue, Keep: keep, Now: now, Items: "readings", Pruning: pruning})
	return w
}

// Note queues taken, the readings of one taking in, for Run to write. It
// never waits: with the queue full, they're dropped.
func (w *Writer) Note(taken []Reading) {
	w.writer.Note(taken)
}

// Run writes the readings noted, and keeps the files, as its writer's Run
// does, until ctx ends, when it writes what's still queued.
func (w *Writer) Run(ctx context.Context) {
	w.writer.Run(ctx)
}

// lines returns taken as the history's lines, each filed under the local day
// it was read on. A reading that can't be put as a line, as one of a use that
// isn't a number, goes unwritten, logged once.
func (w *Writer) lines(taken []Reading) dayfile.Lines {
	lines := make(dayfile.Lines)
	for _, r := range taken {
		line, err := json.Marshal(r)
		if err != nil {
			if !w.unwritable {
				w.files.Logger.Warn("readings history can't hold a reading; it goes unwritten", "account", r.Account, "window", r.Key, "error", err)
			}
			w.unwritable = true
			continue
		}
		lines.Add(r.At, line)
	}
	return lines
}
