package router

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"iter"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/config"
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
	// historyDay is the layout of a day's date in its file's name.
	historyDay = "2006-01-02"
	// pruneLook is how often the history looks at whether a new day has
	// come, and its files are to be pruned, with no reading to write.
	pruneLook = time.Hour
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

// history is the readings history: each change to an account's windows, as
// the router takes its readings in, a JSON line in a file a day, by local
// date, in the state directory, kept as long as the config says. It's for
// looking back at how the accounts were used, and for the recent rates,
// which the router takes up from it as it starts. Noting a reading never
// holds the router up: the readings queue for run's goroutine, which writes
// them. A write that fails is logged, once until one succeeds, and the
// reading goes unwritten: the history never stands in routing's way.
type history struct {
	now func() time.Time
	// keep is how long a day's file is kept, from the end of its day.
	keep time.Duration
	// dir is where the files are, set once the router has its state
	// directory, before run starts.
	dir    string
	opened atomic.Bool
	queue  chan []reading
	// dropping is set once a reading is dropped for the queue being full,
	// until one is written.
	dropping atomic.Bool

	// Only run's goroutine touches what follows.
	failing bool
	// unmarshalled is set once a reading couldn't be put as a line, which is
	// logged once.
	unmarshalled bool
	// pruned is the day the files were last pruned on.
	pruned string
}

// newHistory returns a history kept as settings say, by now's clock: a zero
// Keep keeps a day's file for config.DefaultKeep.
func newHistory(settings config.History, now func() time.Time) *history {
	return &history{now: now, keep: cmp.Or(settings.Keep, config.DefaultKeep), queue: make(chan []reading, historyQueue)}
}

// open has the history kept in dir from now on.
func (h *history) open(dir string) {
	h.dir = dir
	h.opened.Store(true)
}

// note queues readings for run to write. It never waits: with the history
// not yet opened, or the queue full, they're dropped.
func (h *history) note(readings []reading) {
	if len(readings) == 0 || !h.opened.Load() {
		return
	}
	select {
	case h.queue <- readings:
	default:
		if !h.dropping.Swap(true) {
			logger.Warn("readings history fell behind; readings dropped from it")
		}
	}
}

// run writes the readings queued, pruning the files past keeping as it
// starts and on each day after, until ctx ends, when it writes those still
// queued. It makes the history's directory private as it starts.
func (h *history) run(ctx context.Context) {
	h.makePrivate()
	h.prune()
	look := time.NewTicker(pruneLook)
	defer look.Stop()
	for {
		select {
		case readings := <-h.queue:
			h.write(readings)
		case <-look.C:
			h.pruneDaily()
		case <-ctx.Done():
			h.drain()
			return
		}
	}
}

// makePrivate makes the history's directory, when it isn't there, and makes
// it private, the user's alone, logging why when it can't.
func (h *history) makePrivate() {
	err := os.MkdirAll(h.dir, 0o700)
	if err == nil {
		err = os.Chmod(h.dir, 0o700)
	}
	if err != nil {
		logger.Warn("can't make the readings history private", "dir", h.dir, "error", err)
	}
}

// drain writes the readings still queued.
func (h *history) drain() {
	for {
		select {
		case readings := <-h.queue:
			h.write(readings)
		default:
			return
		}
	}
}

// write appends readings to their day's file, pruning first on a day the
// files haven't been pruned on, and logging a failure once until a write
// succeeds.
func (h *history) write(readings []reading) {
	h.pruneDaily()
	err := h.append(readings)
	switch {
	case err != nil && !h.failing:
		logger.Warn("can't write the readings history; readings go unwritten until it can", "dir", h.dir, "error", err)
	case err == nil && h.failing:
		logger.Info("writing the readings history again", "dir", h.dir)
	}
	h.failing = err != nil
	if err == nil {
		h.dropping.Store(false)
	}
}

// append appends readings, a line each, to the file of the local day each was
// read on, making the directory, private, should it have gone.
func (h *history) append(readings []reading) error {
	if err := os.MkdirAll(h.dir, 0o700); err != nil {
		return err
	}
	for day, lines := range h.byDay(readings) {
		if err := appendLines(filepath.Join(h.dir, historyFile(day)), lines); err != nil {
			return err
		}
	}
	return nil
}

// byDay groups readings, as JSON lines, by the local day each was read on. A
// reading that can't be put as a line, as one of a use that isn't a number,
// goes unwritten, logged once.
func (h *history) byDay(readings []reading) map[string][]byte {
	days := make(map[string][]byte)
	for _, r := range readings {
		line, err := json.Marshal(r)
		if err != nil {
			if !h.unmarshalled {
				logger.Warn("readings history can't hold a reading; it goes unwritten", "account", r.Account, "window", r.Window, "error", err)
			}
			h.unmarshalled = true
			continue
		}
		day := r.At.Local().Format(historyDay)
		days[day] = append(append(days[day], line...), '\n')
	}
	return days
}

// appendLines appends lines to the file at path, making it, the user's alone,
// when it isn't there. The file is opened to append, so each write lands
// whole at its end.
func appendLines(path string, lines []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(lines)
	return errors.Join(err, f.Close())
}

// historyFile is the name of the history's file of the local day given, as
// historyDay lays it out.
func historyFile(day string) string {
	return "readings-" + day + ".jsonl"
}

// dayOf returns the local day a history file with the given name holds,
// reporting false for a name that isn't a history file's.
func dayOf(name string) (time.Time, bool) {
	date, ok := strings.CutPrefix(name, "readings-")
	date, dated := strings.CutSuffix(date, ".jsonl")
	if !ok || !dated {
		return time.Time{}, false
	}
	day, err := time.ParseInLocation(historyDay, date, time.Local)
	return day, err == nil
}

// pruneDaily prunes the history's files on a day they haven't been pruned on.
func (h *history) pruneDaily() {
	if h.now().Local().Format(historyDay) != h.pruned {
		h.prune()
	}
}

// prune removes the history's files whose day ended keep or more before now,
// and those of a day after tomorrow, as a clock once set ahead named them,
// which would crowd out the real ones, leaving anything else in the directory
// alone.
func (h *history) prune() {
	now := h.now()
	h.pruned = now.Local().Format(historyDay)
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("can't prune the readings history", "dir", h.dir, "error", err)
		}
		return
	}
	for _, e := range entries {
		day, ok := dayOf(e.Name())
		if !ok || now.Sub(day.AddDate(0, 0, 1)) < h.keep && !afterTomorrow(day, now) {
			continue
		}
		if err := os.Remove(filepath.Join(h.dir, e.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("can't prune the readings history", "file", e.Name(), "error", err)
		}
	}
}

// readBack returns the readings the history's two newest files hold, taken
// at or before now, one at a time, in the order of their lines, each file's
// after the older's: the newest by their names' dates, as a change of time
// zone can put today's file under another date than the clock's, but for
// those of a day after tomorrow. A window's readings since the half hour
// before now, and its baseline before that, are among them unless it has
// been quiet since before the older file, and then it has no recent rate to
// go by. A line that doesn't read as a reading that can be is left out.
func (h *history) readBack(now time.Time) iter.Seq[reading] {
	return func(yield func(reading) bool) {
		skipped := 0
		for _, name := range h.newest(now, 2) {
			bad, more := readFile(filepath.Join(h.dir, name), func(r reading) bool {
				return r.At.After(now) || yield(r)
			})
			skipped += bad
			if !more {
				break
			}
		}
		if skipped > 0 {
			logger.Warn("readings history lines unread", "lines", skipped)
		}
	}
}

// newest returns the names of the history's n newest files at now, by the
// dates their names give, oldest first, but for those of a day after
// tomorrow.
func (h *history) newest(now time.Time, n int) []string {
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("can't read the readings history", "dir", h.dir, "error", err)
		}
		return nil
	}
	var names []string
	for _, e := range entries {
		if day, ok := dayOf(e.Name()); ok && !afterTomorrow(day, now) {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names[max(len(names)-n, 0):]
}

// afterTomorrow reports whether day, a local day as dayOf gives it, is after
// the day after now's: a file of it was named by a clock set ahead.
func afterTomorrow(day, now time.Time) bool {
	y, m, d := now.Local().Date()
	return day.After(time.Date(y, m, d+1, 0, 0, 0, 0, time.Local))
}

// readFile hands take each reading the file at path holds, in the order of
// its lines, until take reports false, and returns how many of its lines
// didn't read as a reading, and whether take wanted more. A line longer than
// historyLineMax is skipped without being held. A file that can't be read
// holds none, which is logged but for one that isn't there.
func readFile(path string, take func(reading) bool) (bad int, more bool) {
	f, err := os.Open(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("can't read the readings history", "file", filepath.Base(path), "error", err)
		}
		return 0, true
	}
	defer func() { _ = f.Close() }()
	lines := bufio.NewReaderSize(f, historyLineMax)
	for {
		line, err := lines.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			bad++
			if err = skipLine(lines); err == nil {
				continue
			}
			line = nil
		}
		if len(line) > 0 {
			var r reading
			switch {
			case json.Unmarshal(line, &r) != nil || !r.valid():
				bad++
			case !take(r):
				return bad, false
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				logger.Warn("readings history read short", "file", filepath.Base(path), "error", err)
			}
			return bad, true
		}
	}
}

// skipLine reads past the rest of a line too long to hold, reporting why it
// stopped short of the line's end, if it did.
func skipLine(lines *bufio.Reader) error {
	for {
		if _, err := lines.ReadSlice('\n'); !errors.Is(err, bufio.ErrBufferFull) {
			return err
		}
	}
}
