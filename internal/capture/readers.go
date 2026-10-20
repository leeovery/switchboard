package capture

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io/fs"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/views"
)

// records are what a world's readers serve, each as the router and the
// ledger make them: the summaries of the days before today's, oldest first;
// every line the ledger keeps, of whatever day, in the order the router wrote
// them, each as it's filed; the readings history, in the order its readings
// were read; and the router's events, every version of each in the order it
// was filed. caps are the accounts' caps today is summarised with.
type records struct {
	days     []ledger.Summary
	lines    []filed
	readings history
	events   []events.Line
	caps     ledger.Caps
}

// recordsOf are the world's records: the year's days of its accounts,
// summarised from its readings history, that history, and its accounts'
// caps; and, where today's traffic of the three is the world's, every line
// the sessions' world holds, or the world's own lines in their place, and
// the router's events.
func (a accountWorlds) recordsOf(w accountWorld) *records {
	r := &records{days: a.year.summaries(w.history, w.accounts), readings: w.history, caps: ledger.CapsOf(w.accounts, claude.SharedWindows())}
	if w.traffic {
		r.lines, r.events = a.traffic()
	}
	if w.lines != nil {
		r.lines = filedOf(heldAsWritten(w.lines(a.now)))
	}
	return r
}

// readers are the records' readers by now's clock, as the watch takes them:
// what each gives is what a real reader gives of files that hold the records,
// read at now.
func (r *records) readers(now func() time.Time) (views.Ledger, views.Readings, views.Events) {
	return heldLedger{records: r, now: now}, heldReadings{records: r, now: now}, heldEvents{records: r, now: now}
}

// heldLedger reads the records' request ledger as a ledger.Follower reads the
// ledger's files, by now's clock.
type heldLedger struct {
	records *records
	now     func() time.Time
}

// filed is a line of the records as a reader finds it: the date of the day
// whose files hold it, and its place among the records' lines.
type filed struct {
	ledger.Held
	date  string
	place int
}

// filedOf are the lines given, in the order they were written, each as the
// ledger files it: under the date of the day its request arrived on.
func filedOf(held []ledger.Held) []filed {
	lines := make([]filed, len(held))
	for i, h := range held {
		lines[i] = filed{Held: h, date: dayfile.DateOf(h.At), place: i}
	}
	return lines
}

// arrived returns the records' lines that arrived by now, in the order they
// were written.
func (l heldLedger) arrived(now time.Time) []filed {
	var lines []filed
	for _, f := range l.records.lines {
		if !f.At.After(now) {
			lines = append(lines, f)
		}
	}
	return lines
}

// byArrival orders lines as a Follower does: by when each arrived, then the
// date of the day's files each is in, and its place there.
func byArrival(a, b filed) int {
	return cmp.Or(a.At.Compare(b.At), strings.Compare(a.date, b.date), cmp.Compare(a.place, b.place))
}

// Days gives the summaries of the days before today's from from's on, as the
// records hold them, and today's, summarised from the lines filed under it
// that arrived by now and the readings history, as a Follower summarises a
// day as it goes.
func (l heldLedger) Days(from time.Time) []ledger.Summary {
	now := l.now()
	first, today := dayfile.DateOf(from), dayfile.DateOf(now)
	var days []ledger.Summary
	for _, s := range l.records.days {
		if s.Day >= first && s.Day < today {
			days = append(days, s)
		}
	}
	lines := func(yield func(ledger.Line) bool) {
		for _, f := range l.arrived(now) {
			if f.date == today && !yield(f.Line) {
				return
			}
		}
	}
	read := heldReadings(l)
	// The date is now's own, so Summarise can't fail.
	summary, _ := ledger.Summarise(today, lines, read.Between, nil, l.records.caps, now)
	return append(days, summary)
}

// DaysOn gives the summaries of the days with the dates given, oldest
// first, as Days gives them from the earliest.
func (l heldLedger) DaysOn(dates []string) []ledger.Summary {
	if len(dates) == 0 {
		return nil
	}
	from, _, _ := dayfile.Day(slices.Min(dates))
	var on []ledger.Summary
	for _, s := range l.Days(from) {
		if slices.Contains(dates, s.Day) {
			on = append(on, s)
		}
	}
	return on
}

// FirstDay gives the start of the first day the records hold a summary of,
// else today's, as a Follower finds the first day its files are of.
func (l heldLedger) FirstDay(now time.Time) (time.Time, error) {
	first := dayfile.DateOf(now)
	if len(l.records.days) > 0 {
		first = min(first, l.records.days[0].Day)
	}
	start, _, _ := dayfile.Day(first)
	return start, nil
}

// FirstLinesDay gives the start of the first day the records' lines that
// arrived by now are filed under, as a Follower finds the first day its
// files hold lines of, failing with fs.ErrNotExist where none has.
func (l heldLedger) FirstLinesDay(now time.Time) (time.Time, error) {
	lines := l.arrived(now)
	if len(lines) == 0 {
		return time.Time{}, fmt.Errorf("the records hold no day's lines: %w", fs.ErrNotExist)
	}
	first := slices.MinFunc(lines, func(a, b filed) int { return strings.Compare(a.date, b.date) })
	start, _, _ := dayfile.Day(first.date)
	return start, nil
}

// Today gives every line that arrived from today's start to now, oldest
// first, as a Follower orders them, none of its JSON held, as a Follower
// gives them, afresh, whatever mark says, as a Follower gives them where
// today's files were replaced: a ledger.Mark is the ledger package's alone
// to make, so the zero Mark is all it can give back.
func (l heldLedger) Today(ledger.Mark) (lines []ledger.Held, next ledger.Mark, afresh bool) {
	now := l.now()
	start := dayfile.DayStart(now.Local(), 0)
	var today []filed
	for _, f := range l.arrived(now) {
		if !f.At.Before(start) {
			today = append(today, f)
		}
	}
	slices.SortStableFunc(today, byArrival)
	for _, f := range today {
		lines = append(lines, ledger.Held{Line: f.Line})
	}
	return lines, ledger.Mark{}, true
}

// Day gives the lines that arrived on the local day with the given date, by
// now, oldest first, as a Follower orders them, none of its JSON held, as a
// Follower gives them: whichever day's files hold them, as a Follower reads
// the day's files and its neighbours'.
func (l heldLedger) Day(date string) []ledger.Held {
	bounds, ok := dayfile.DayBounds(date)
	if !ok {
		return nil
	}
	var day []filed
	for _, f := range l.arrived(l.now()) {
		if bounds.Holds(f.At) {
			day = append(day, f)
		}
	}
	slices.SortStableFunc(day, byArrival)
	lines := make([]ledger.Held, len(day))
	for i, f := range day {
		lines[i] = ledger.Held{Line: f.Line}
	}
	return lines
}

// DaySide gives no side, whatever the day, as a Follower of a ledger that
// holds no day's head gives it: the find reads each day's lines, as Day
// gives them.
func (heldLedger) DaySide(string, int) (json.RawMessage, bool) {
	return nil, false
}

// DayRows gives no rows, whatever the day, as a Follower of a ledger that
// holds no day's head gives them: the Log reads each day's lines, as Day
// gives them.
func (heldLedger) DayRows(string, int) (ledger.DayRows, bool) {
	return ledger.DayRows{}, false
}

// RowChunk fails with fs.ErrNotExist, whatever the chunk, as a Follower of a
// ledger that holds no day's rows file fails.
func (heldLedger) RowChunk(date string, _ ledger.RowsChunk) ([]byte, error) {
	return nil, fmt.Errorf("a chunk of the rows of %s: %w", date, fs.ErrNotExist)
}

// Session gives the lines of the session with the given id that arrived by
// now, newest first, as a Follower orders them, as far as the caller goes on:
// of each day that names it, as its summary would, as a Follower reads them,
// so none of a day it made only quota checks on; and of those, the days
// ledger.Spanning gives of the spans given alone, as a Follower reads them.
func (l heldLedger) Session(id string, within ...ledger.Span) iter.Seq[ledger.Held] {
	return func(yield func(ledger.Held) bool) {
		lines := l.arrived(l.now())
		named, spanned := naming(lines, map[string]bool{id: true}), ledger.Spanning(within)
		lines = slices.DeleteFunc(lines, func(f filed) bool { return f.Session != id || !named[f.date] || !spanned(f.date) })
		slices.SortStableFunc(lines, func(a, b filed) int { return byArrival(b, a) })
		for _, f := range lines {
			if !yield(f.Held) {
				return
			}
		}
	}
}

// Sessions gives today's sessions, those with a request that arrived today,
// and those with the given ids, by each one's id, as a Follower of a ledger
// that holds no digest reads them: each read whole, resting on no base,
// whatever rest says, its lines those that arrived by now, oldest first:
// the days either side of today's, and today's, read whole, then each
// earlier day that names a session it gives, as its summary would. take,
// where it's given, is handed each line of every day read, with the day's
// date, those either side of today's and today's first.
func (l heldLedger) Sessions(ids []string, _ func(id string) ledger.Rest, take func(date string, line ledger.Line)) map[string]ledger.SessionFrom {
	return ledger.SessionsWhole(l.sessions(ids, true, take))
}

// SessionsOf gives the sessions with the given ids alone, as Sessions gives
// them, but none of today's others, as a Follower reads them.
func (l heldLedger) SessionsOf(ids []string, _ func(id string) ledger.Rest, take func(date string, line ledger.Line)) map[string]ledger.SessionFrom {
	return ledger.SessionsWhole(l.sessions(ids, false, take))
}

// sessions gives the lines of the sessions with the given ids, and where
// todays is set, of today's, as Sessions gives them.
func (l heldLedger) sessions(ids []string, todays bool, take func(date string, line ledger.Line)) map[string][]ledger.Line {
	now := l.now()
	lines := l.arrived(now)
	near := dayfile.Dates(now, now)
	start := dayfile.DayStart(now.Local(), 0)
	wanted := make(map[string]bool)
	for _, id := range ids {
		wanted[id] = true
	}
	for _, f := range lines {
		if todays && slices.Contains(near, f.date) && f.Kind == ledger.KindMessage && f.Session != "" && !f.At.Before(start) {
			wanted[f.Session] = true
		}
	}
	named := naming(lines, wanted)
	read := func(f filed) bool { return slices.Contains(near, f.date) }
	more := func(f filed) bool { return f.date < near[0] && named[f.date] }
	sessions := make(map[string][]filed)
	for _, reads := range []func(filed) bool{read, more} {
		for _, f := range lines {
			if !reads(f) {
				continue
			}
			if take != nil {
				take(f.date, f.Line)
			}
			if wanted[f.Session] {
				sessions[f.Session] = append(sessions[f.Session], f)
			}
		}
	}
	given := make(map[string][]ledger.Line, len(sessions))
	for id, held := range sessions {
		slices.SortStableFunc(held, byArrival)
		for _, f := range held {
			given[id] = append(given[id], f.Line)
		}
	}
	return given
}

// naming are the dates of the days whose lines name a session wanted, as
// their summaries would: one a line counts toward, as Line.CountedSession
// says.
func naming(lines []filed, wanted map[string]bool) map[string]bool {
	named := make(map[string]bool)
	for _, f := range lines {
		if wanted[f.CountedSession()] {
			named[f.date] = true
		}
	}
	return named
}

// heldReadings reads the records' readings history as a readings.Reader
// reads its files, by now's clock.
type heldReadings struct {
	records *records
	now     func() time.Time
}

// Between gives the readings of times from from up to to read by now, in the
// order they were read.
func (h heldReadings) Between(from, to time.Time) iter.Seq[readings.Reading] {
	return func(yield func(readings.Reading) bool) {
		now := h.now()
		for r := range h.records.readings.Between(from, to) {
			if r.At.After(now) || !yield(r) {
				return
			}
		}
	}
}

// Tail gives a read of the readings read by now, as readings.TailOf reads
// them: each call reads them whole, as they're held, not filed.
func (h heldReadings) Tail(from, start, end time.Time) func() []readings.Reading {
	return readings.TailOf(h.Between, from, start, end)
}

// FirstDay gives the start of the local day of the first reading read by
// now: fs.ErrNotExist where none was.
func (h heldReadings) FirstDay() (time.Time, error) {
	if len(h.records.readings) == 0 || h.records.readings[0].At.After(h.now()) {
		return time.Time{}, fs.ErrNotExist
	}
	return dayfile.DayStart(h.records.readings[0].At.Local(), 0), nil
}

// heldEvents reads the records' router's events as an events.Reader reads
// their files, by now's clock, each filed on the day it happened.
type heldEvents struct {
	records *records
	now     func() time.Time
}

// eventKey tells an event apart from every other, as an events.Reader does:
// its run, and its id.
type eventKey struct {
	run int64
	id  int
}

// Between gives the events that happened by now, from from up to to, each as
// its last version filed stands, oldest first, by when each happened, then
// by run and id.
func (h heldEvents) Between(from, to time.Time) iter.Seq[events.Line] {
	return func(yield func(events.Line) bool) {
		now := h.now()
		last := make(map[eventKey]events.Line)
		for _, e := range h.records.events {
			if !e.At.Before(from) && e.At.Before(to) && !e.At.After(now) {
				last[eventKey{run: e.Run.UnixNano(), id: e.ID}] = e
			}
		}
		happened := make([]events.Line, 0, len(last))
		for _, e := range last {
			happened = append(happened, e)
		}
		slices.SortFunc(happened, func(a, b events.Line) int {
			return cmp.Or(a.At.Compare(b.At), a.Run.Compare(b.Run), cmp.Compare(a.ID, b.ID))
		})
		for _, e := range happened {
			if !yield(e) {
				return
			}
		}
	}
}

// FirstDay gives the start of the first local day an event happened on by
// now: fs.ErrNotExist where none had.
func (h heldEvents) FirstDay() (time.Time, error) {
	now := h.now()
	var first time.Time
	for _, e := range h.records.events {
		if !e.At.After(now) && (first.IsZero() || e.At.Before(first)) {
			first = e.At
		}
	}
	if first.IsZero() {
		return time.Time{}, fs.ErrNotExist
	}
	return dayfile.DayStart(first.Local(), 0), nil
}
