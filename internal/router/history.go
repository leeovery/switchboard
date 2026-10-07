package router

import (
	"cmp"
	"context"
	"encoding/json"
	"iter"
	"slices"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/readings"
)

// historyQueue is how many takings in of readings, an answer's or a probe's
// each, can wait to be written. Past that, their readings are dropped from
// the history rather than hold the router up.
const historyQueue = 1024

// history is the readings history: each change to an account's windows, as
// the router takes its readings in, a JSON line in a file a day, by local
// date, in the state directory, compressed once its day ended two days
// before, and kept as long as the config says, as dayfile keeps them, in the
// format internal/readings owns. It's for looking back at how the accounts
// were used, and for the recent rates, which the router takes up from it as
// it starts. Noting a reading never holds the router up: the readings queue
// for run's goroutine, which writes them. A write that fails is logged, once
// until one succeeds, and the reading goes unwritten: the history never
// stands in routing's way.
type history struct {
	now func() time.Time
	// keep is how long a day's file is kept, from the end of its day.
	keep time.Duration
	// round, where it's given, is called on each of the writer's rounds,
	// before the history's files are pruned: the request ledger summarises
	// on it the days that have ended, while the readings their summaries
	// need are still there.
	round func(now time.Time)
	// files are the history's files, and writer what writes the readings
	// noted to them: both are made as the history is opened, once the router
	// has its state directory, before run starts.
	files  *dayfile.Files
	writer *dayfile.Writer[[]readings.Reading]
	opened atomic.Bool
	// unmarshalled is set once a reading couldn't be put as a line, which is
	// logged once. Only run's goroutine touches it.
	unmarshalled bool
}

// newHistory returns a history kept as settings say, by now's clock: a zero
// Keep keeps a day's file for config.DefaultHistoryKeep.
func newHistory(settings config.History, now func() time.Time) *history {
	return &history{now: now, keep: cmp.Or(settings.Keep, config.DefaultHistoryKeep)}
}

// open has the history kept in dir from now on.
func (h *history) open(dir string) {
	h.files = readings.Files(dir, logger)
	h.writer = dayfile.NewWriter(h.files, h.lines, dayfile.WriterOptions{Queue: historyQueue, Keep: h.keep, Now: h.now, Items: "readings", Round: h.round})
	h.opened.Store(true)
}

// note queues taken, the readings of one taking in, for run to write. It
// never waits: with the history not yet opened, or the queue full, they're
// dropped.
func (h *history) note(taken []readings.Reading) {
	if len(taken) == 0 || !h.opened.Load() {
		return
	}
	h.writer.Note(taken)
}

// run writes the readings queued, and keeps the history's files, as its
// writer's Run does, until ctx ends.
func (h *history) run(ctx context.Context) {
	h.writer.Run(ctx)
}

// lines returns taken as the history's lines, each filed under the local day
// it was read on. A reading that can't be put as a line, as one of a use that
// isn't a number, goes unwritten, logged once.
func (h *history) lines(taken []readings.Reading) dayfile.Lines {
	lines := make(dayfile.Lines)
	for _, r := range taken {
		line, err := json.Marshal(r)
		if err != nil {
			if !h.unmarshalled {
				logger.Warn("readings history can't hold a reading; it goes unwritten", "account", r.Account, "window", r.Key, "error", err)
			}
			h.unmarshalled = true
			continue
		}
		lines.Add(r.At, line)
	}
	return lines
}

// readBack returns the readings the history's two newest days hold, taken at
// or before now, one at a time, in the order they were read, whichever day's
// file each is in, as readings.Read reads them. The newest days are by the
// dates the files' names give, as a change of time zone can put today's file
// under another date than the clock's, but for those of a day after
// tomorrow. A window's readings since the half hour before now, and its
// baseline before that, are among them unless it has been quiet since before
// the older day, and then it has no recent rate to go by. A line that doesn't
// read as a reading that can be is left out.
func (h *history) readBack(now time.Time) iter.Seq[readings.Reading] {
	return func(yield func(readings.Reading) bool) {
		unread := readings.Read(h.files, h.files.Newest(now, 2), func(r readings.Reading) bool {
			return r.At.After(now) || yield(r)
		})
		if unread > 0 {
			logger.Warn("readings history lines unread", "lines", unread)
		}
	}
}

// readings returns the readings the history holds of times from from up to
// to, as readings.Between gives them, as the request ledger summarises a day
// with them: none before the history is opened.
func (h *history) readings(from, to time.Time) iter.Seq[readings.Reading] {
	if !h.opened.Load() {
		return func(func(readings.Reading) bool) {}
	}
	return readings.Between(h.files, from, to)
}

// windowReadings returns the readings the history holds of the window with
// the given key, of the accounts with the given ids, by account, in the order
// they were read: those of the local days from the day before from's to the
// day after to's, as dayfile.Dates gives them. It holds the files from
// pruning and compressing while it reads them, and reads none for no account,
// or before the history is opened.
func (h *history) windowReadings(key string, ids []string, from, to time.Time) map[string][]readings.Reading {
	if len(ids) == 0 || !h.opened.Load() {
		return nil
	}
	read := make(map[string][]readings.Reading)
	readings.Read(h.files, dayfile.Dates(from, to), func(r readings.Reading) bool {
		if r.Key == key && slices.Contains(ids, r.Account) {
			read[r.Account] = append(read[r.Account], r)
		}
		return true
	})
	return read
}
