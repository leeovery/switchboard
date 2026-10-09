package events

import (
	"cmp"
	"iter"
	"log/slog"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// ChangesFor is how many days after it began an event can change, and be
// filed again: a week's limit ends by its reset. The router changes none
// later, and a reader looks no further on for an event's versions.
const ChangesFor = 8

// Reader reads the router's events back where they lie, in the request
// ledger's directory of the state directory, with no router.
type Reader struct {
	files  *dayfile.Files
	now    func() time.Time
	logger *slog.Logger
}

// NewReader returns a reader of the router's events in the state directory
// stateDir, by now's clock, what it can't read logged to logger.
func NewReader(stateDir string, now func() time.Time, logger *slog.Logger) *Reader {
	return &Reader{files: Files(ledger.Dir(stateDir), logger), now: now, logger: logger}
}

// Between returns the events that happened from from up to to, each as it
// last stands, oldest first, by when each happened, then by run and id. An
// event's versions are merged by its run and id, the last line read winning:
// the days are read oldest first, each day's lines in the order they were
// filed, plain or compressed, as dayfile.ReadFiled reads them. A version is
// filed under the day it's written on, as late as ChangesFor days after its
// event, so a day's events are read from its own file and the next
// ChangesFor days', and a day either side, as a change of time zone files a
// line under a day beside its own; and each event is handed on once no file
// left to read can hold a version of it, so what's held is never more than
// those days' events. A line that doesn't read as an event is passed over,
// and a damaged file read up to the damage: how many lines couldn't be read
// is logged.
func (r *Reader) Between(from, to time.Time) iter.Seq[Line] {
	return func(yield func(Line) bool) {
		for h := range r.HeldBetween(from, to) {
			if !yield(h.Line) {
				return
			}
		}
	}
}

// HeldBetween returns the events that happened from from up to to, as
// Between does, each with its last version's JSON, as it was filed.
func (r *Reader) HeldBetween(from, to time.Time) iter.Seq[Held] {
	return func(yield func(Held) bool) {
		last := to.AddDate(0, 0, ChangesFor)
		if now := r.now(); now.Before(last) {
			last = now
		}
		m := merge{from: from, to: to, held: make(map[key]Held), yield: yield}
		unread := dayfile.ReadFiled(r.files, dayfile.Dates(from, last), HeldIn, m.hold, m.dayRead)
		if !m.stopped {
			m.handOn(func(Held) bool { return true })
		}
		if unread > 0 {
			r.logger.Warn("lines of the router's events unread", "lines", unread)
		}
	}
}

// key tells an event apart from every other: its run and its id.
type key struct {
	run int64
	id  int
}

// keyOf is the key of the event line is a version of.
func keyOf(line Line) key {
	return key{run: line.Run.UnixNano(), id: line.ID}
}

// merge merges the versions of the events of a span of time, from up to to,
// as they're read, holding each event's last until it's settled, and hands
// on those settled, oldest first, to yield.
type merge struct {
	from, to time.Time
	held     map[key]Held
	yield    func(Held) bool
	// settled is when the events handed on happened before: no file left to
	// read can hold a version of one.
	settled time.Time
	// stopped is set once yield reports false.
	stopped bool
}

// hold holds line, in place of any version of its event held before, where
// its event happened in the span and isn't settled: a version of a settled
// event, filed further after it than an event changes, is passed over, as its
// event has been handed on. It always wants more.
func (m *merge) hold(h Held) bool {
	if h.At.Before(m.from) || !h.At.Before(m.to) || h.At.Before(m.settled) {
		return true
	}
	m.held[keyOf(h.Line)] = h
	return true
}

// dayRead hands on the events settled once the local day with the given date
// is read, and reports whether yield wants more. A later day's file is
// written from the day after's start, less a day for a change of time zone,
// so it holds no version of an event that happened ChangesFor days before
// the day's start.
func (m *merge) dayRead(date string) bool {
	start, _, ok := dayfile.Day(date)
	if !ok {
		return true
	}
	m.settled = dayfile.DayStart(start, -ChangesFor)
	return m.handOn(func(h Held) bool { return h.At.Before(m.settled) })
}

// handOn hands yield the events held that settled reports true of, oldest
// first, and lets them go, reporting whether yield wants more.
func (m *merge) handOn(settled func(Held) bool) bool {
	var ready []Held
	for k, h := range m.held {
		if settled(h) {
			ready = append(ready, h)
			delete(m.held, k)
		}
	}
	slices.SortFunc(ready, oldestFirst)
	for _, h := range ready {
		if !m.yield(h) {
			m.stopped = true
			return false
		}
	}
	return true
}

// oldestFirst orders lines by when their events happened, then by their runs
// and ids.
func oldestFirst(a, b Held) int {
	return cmp.Or(a.At.Compare(b.At), a.Run.Compare(b.Run), cmp.Compare(a.ID, b.ID))
}

// Empty reads as the router's events where there are none, with no files to
// read: for one who reads them where there's none to find.
type Empty struct{}

// Between returns no event.
func (Empty) Between(time.Time, time.Time) iter.Seq[Line] {
	return func(func(Line) bool) {}
}
