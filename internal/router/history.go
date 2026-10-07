package router

import (
	"cmp"
	"context"
	"encoding/json"
	"iter"
	"math"
	"slices"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

const (
	// historyDirName is the readings history's directory in the state
	// directory.
	historyDirName = "history"
	// historyQueue is how many takings in of readings, an answer's or a
	// probe's each, can wait to be written. Past that, their readings are
	// dropped from the history rather than hold the router up.
	historyQueue = 1024
	// historyLineMax is the longest line of the history read back: a
	// reading's is a couple of hundred bytes, and a longer line, which no
	// reading makes, is skipped without being held.
	historyLineMax = 4096
)

// source is where a reading came from, as the readings history gives it.
type source string

const (
	fromAnswer source = "answer"
	fromProbe  source = "probe"
	fromPrime  source = "prime"
)

// reading is a line of the readings history: a window of an account as read
// when its reading changed, and where the reading came from. It holds the
// account's id and nothing else of the account: never its token or label.
// Fields may be added to it, never renamed.
type reading struct {
	At          time.Time    `json:"at"`
	Account     string       `json:"account"`
	Window      string       `json:"window"`
	Utilization float64      `json:"utilization"`
	ResetsAt    time.Time    `json:"resets_at,omitzero"`
	Status      quota.Status `json:"status,omitempty"`
	Source      source       `json:"source"`
}

// readingsOf are windows of the account with the given id, read at a time
// from a source, as lines of the readings history.
func readingsOf(id string, windows []quota.Window, at time.Time, from source) []reading {
	readings := make([]reading, len(windows))
	for i, w := range windows {
		readings[i] = reading{At: at, Account: id, Window: w.Key, Utilization: w.Utilization, ResetsAt: w.ResetsAt, Status: w.Status, Source: from}
	}
	return readings
}

// window is the reading as the window it read.
func (r reading) window() quota.Window {
	return quota.Window{Key: r.Window, Utilization: r.Utilization, ResetsAt: r.ResetsAt, Status: r.Status}
}

// valid reports whether the reading, as the history holds it, can be taken
// up: it names an account and a window, and reads a use that can be.
func (r reading) valid() bool {
	return r.Account != "" && r.Window != "" && !r.At.IsZero() &&
		!math.IsNaN(r.Utilization) && !math.IsInf(r.Utilization, 0) && r.Utilization >= 0
}

// readingIn returns the reading a line of the history holds, reporting false
// for one that doesn't read as a reading that can be taken up.
func readingIn(line []byte) (reading, bool) {
	var r reading
	if json.Unmarshal(line, &r) != nil || !r.valid() {
		return reading{}, false
	}
	return r, true
}

// history is the readings history: each change to an account's windows, as
// the router takes its readings in, a JSON line in a file a day, by local
// date, in the state directory, compressed once its day ended two days
// before, and kept as long as the config says, as dayfile keeps them. It's
// for looking back at how the accounts were used, and for the recent rates,
// which the router takes up from it as it starts. Noting a reading never
// holds the router up: the readings queue for run's goroutine, which writes
// them. A write that fails is logged, once until one succeeds, and the
// reading goes unwritten: the history never stands in routing's way.
type history struct {
	now func() time.Time
	// keep is how long a day's file is kept, from the end of its day.
	keep time.Duration
	// files are the history's files, and writer what writes the readings
	// noted to them: both are made as the history is opened, once the router
	// has its state directory, before run starts.
	files  *dayfile.Files
	writer *dayfile.Writer[[]reading]
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
	h.files = &dayfile.Files{Dir: dir, Prefix: "readings", Name: "readings history", LineMax: historyLineMax, Logger: logger}
	h.writer = dayfile.NewWriter(h.files, h.lines, dayfile.WriterOptions{Queue: historyQueue, Keep: h.keep, Now: h.now, Items: "readings"})
	h.opened.Store(true)
}

// note queues readings for run to write. It never waits: with the history
// not yet opened, or the queue full, they're dropped.
func (h *history) note(readings []reading) {
	if len(readings) == 0 || !h.opened.Load() {
		return
	}
	h.writer.Note(readings)
}

// run writes the readings queued, and keeps the history's files, as its
// writer's Run does, until ctx ends.
func (h *history) run(ctx context.Context) {
	h.writer.Run(ctx)
}

// lines returns readings as the history's lines, each filed under the local
// day it was read on. A reading that can't be put as a line, as one of a use
// that isn't a number, goes unwritten, logged once.
func (h *history) lines(readings []reading) dayfile.Lines {
	lines := make(dayfile.Lines)
	for _, r := range readings {
		line, err := json.Marshal(r)
		if err != nil {
			if !h.unmarshalled {
				logger.Warn("readings history can't hold a reading; it goes unwritten", "account", r.Account, "window", r.Window, "error", err)
			}
			h.unmarshalled = true
			continue
		}
		lines.Add(r.At, line)
	}
	return lines
}

// readBack returns the readings the history's two newest days hold, taken at
// or before now, one at a time, in the order they came: each day's after the
// older's, as dayfile.Read reads them. The newest days are by the dates the
// files' names give, as a change of time zone can put today's file under
// another date than the clock's, but for those of a day after tomorrow. A
// window's readings since the half hour before now, and its baseline before
// that, are among them unless it has been quiet since before the older day,
// and then it has no recent rate to go by. A line that doesn't read as a
// reading that can be is left out.
func (h *history) readBack(now time.Time) iter.Seq[reading] {
	return func(yield func(reading) bool) {
		unread := dayfile.Read(h.files, h.files.Newest(now, 2), readingIn, func(r reading) bool {
			return r.At.After(now) || yield(r)
		})
		if unread > 0 {
			logger.Warn("readings history lines unread", "lines", unread)
		}
	}
}

// readings returns the readings the history holds of times from from up to
// to, one at a time, in the order they came, as the request ledger summarises
// a day with them: none before the history is opened. It holds the files
// from pruning and compressing while it reads them.
func (h *history) readings(from, to time.Time) iter.Seq[ledger.Reading] {
	return func(yield func(ledger.Reading) bool) {
		if !h.opened.Load() {
			return
		}
		dayfile.Read(h.files, dayfile.Dates(from, to), readingIn, func(r reading) bool {
			if r.At.Before(from) || !r.At.Before(to) {
				return true
			}
			return yield(ledger.Reading{At: r.At, Account: r.Account, Window: r.window()})
		})
	}
}

// windowReadings returns the readings the history holds of the window with
// the given key, of the accounts with the given ids, by account, in the order
// they came: those of the local days from the day before from's to the day
// after to's, as dayfile.Dates gives them. It holds the files from pruning
// and compressing while it reads them, and reads none for no account, or
// before the history is opened.
func (h *history) windowReadings(key string, ids []string, from, to time.Time) map[string][]reading {
	if len(ids) == 0 || !h.opened.Load() {
		return nil
	}
	readings := make(map[string][]reading)
	dayfile.Read(h.files, dayfile.Dates(from, to), readingIn, func(r reading) bool {
		if r.Window == key && slices.Contains(ids, r.Account) {
			readings[r.Account] = append(readings[r.Account], r)
		}
		return true
	})
	return readings
}
