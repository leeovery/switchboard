package views

import (
	"cmp"
	"fmt"
	"iter"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// readHeldBack is the Log's read at now through l, as the watch reads it,
// and the store its rows are told by: today's lines whole as it opens, then
// each day before, newest first, back to the local day with the given date,
// read as ReadDay reads it through the filter through gives of its date,
// each taken in as TakenBack takes it, s telling what's under way, the store
// it reads taken into the one held.
func readHeldBack(l Ledger, through func(date string) LogFilter, from string, s Stream, prices ledger.Table, now time.Time) (LogRead, HeldSessions) {
	read, held := ReadLog(prices), HoldSessions(prices)
	take := func(day LogDay) {
		var fresh *HeldSessions
		if read, fresh = read.TakenBack(l, day, held, s, now); fresh != nil {
			held = held.With(*fresh)
		}
	}
	take(rawDay(l, Today(now).Date))
	for day := dayfile.DayStart(now, -1); dayfile.DateOf(day) >= from; day = dayfile.DayStart(day, -1) {
		date := dayfile.DateOf(day)
		take(ReadDay(l, date, through(date)))
	}
	return read, held
}

// heldWhole is the Log's read at now through l, every day held whole, as
// readHeldBack has it.
func heldWhole(l Ledger, from string, s Stream, prices ledger.Table, now time.Time) LogRead {
	read, _ := readHeldBack(l, func(string) LogFilter { return LogFilter{} }, from, s, prices, now)
	return read
}

// shownThrough is what the Log's Requests built of read through the filter
// f show at now, through l, reaching back to the local day with the given
// date, its oldest kept, s telling what's under way: each of its lines, a
// row, a rule with its figures, as TotalsOn gives them, or the oldest day's
// line, each row with its details; then what today's lines count, and the
// days its rows await a line on. No past day's count is shown.
func shownThrough(l Ledger, read LogRead, f LogFilter, from string, s Stream, prices ledger.Table, now time.Time) []any {
	start, _, _ := dayfile.Day(from)
	table := LogRows(LogSources{
		From: start, Oldest: OldestDay{Day: dayOf(from), Keep: 30 * 24 * time.Hour}, Read: read, Stream: s,
		Totals: TotalsOn(l, dayfile.Span(start, dayfile.DayStart(now, 0)), now, prices), Filter: f, Now: now,
	})
	var shown []any
	for _, line := range table.All() {
		details, _ := line.Details()
		shown = append(shown, LogLine{Row: line.Row, Rule: line.Rule, Oldest: line.Oldest}, details)
	}
	today := Today(now)
	return append(shown, table.Count(today.Date), table.Awaited(today))
}

// widened is a filter r draws that serves f, as serves has it, of the ids
// given: each choice ticking none, as nil or empty, or those f ticks and now
// and then another; keeping those that failed alone only where f does, now
// and then; and of any kinds.
func widened(r *rand.Rand, f LogFilter, ids filterIDs) LogFilter {
	return LogFilter{
		Accounts: widenedChoice(r, f.Accounts, ids.accounts), Models: widenedChoice(r, f.Models, ids.models),
		Sessions: widenedChoice(r, f.Sessions, ids.sessions), Kinds: slices.DeleteFunc(RowKinds(), func(RowKind) bool { return r.IntN(2) == 0 }),
		Failed: f.Failed && r.IntN(2) == 0,
	}
}

// widenedChoice is a choice r draws keeping every one choice keeps, of the
// ids given, as widened has it: none ticked, nil or empty, where choice
// ticks none, and a third of the time besides.
func widenedChoice(r *rand.Rand, choice, ids []string) []string {
	switch {
	case len(choice) == 0 || r.IntN(3) == 0:
		if r.IntN(2) == 0 {
			return []string{}
		}
		return nil
	case r.IntN(2) == 0:
		choice = append(slices.Clone(choice), ids[r.IntN(len(ids))])
	}
	return slices.Compact(slices.Sorted(slices.Values(choice)))
}

// drawnStream is a stream r draws of the chain's lines: one telling nothing
// half the time; else one telling of a request of a day the lines are filed
// on, its line held, and one whose line is yet to come, as toldBack tells
// of them, so that day is read whole.
func (c *digestChain) drawnStream(r *rand.Rand) Stream {
	if r.IntN(2) == 0 {
		return Stream{}
	}
	filed := filedDates(c.lines)
	return toldBack(c.lines, []string{filed[r.IntN(len(filed))]})
}

// The moments a read held through filters meets, each met by some of
// throughWorlds.
const (
	heldDropped  = "a past day held through a filter, dropping lines the read whole holds"
	heldNone     = "a past day held through a filter, holding none of its lines"
	heldTold     = "a past day read whole as the stream tells of a request on it, the filter not keeping every line"
	heldRefilter = "a past day held through a filter read again through its join with another"
)

// heldFilters is how many filters a read is held through at each moment.
const heldFilters = 4

// readsThrough runs check at half an hour past the midnight after the last
// day the chain's lines are filed on, that day's head yet to be built, and
// at half past one, its head built, the chain digested day by day up to
// them, each with a source of random numbers of the chain's seed.
func readsThrough(t *testing.T, check func(c *digestChain, f *ledger.Follower, r *rand.Rand)) {
	t.Helper()
	for _, w := range throughWorlds {
		c := w.chain(t)
		f, filed := c.follower(), filedDates(c.lines)
		last := filed[len(filed)-1]
		r := rand.New(rand.NewPCG(c.seed, 0xb2))
		for _, day := range c.days() {
			after := yesterdayOf(day) == last
			if after {
				c.by(day.Start.Add(30 * time.Minute))
				check(c, f, r)
			}
			c.by(day.Start.Add(90 * time.Minute))
			if after {
				check(c, f, r)
				break
			}
		}
	}
}

// The Log's read with its past days held through a filter, as ReadDay reads
// each through the filter the watch reads it through, as ThroughOn has it,
// shows through any filter that filter serves what the read with every day
// held whole shows: its lines, rules, the oldest day's line, each row's
// details, what today's lines count, and the days its rows await a line
// on. Of throughWorlds at readsThrough's moments, heldFilters a moment, as
// drawnFilter draws what's shown and widened what the days are held
// through, of a stream drawnStream draws.
func TestAReadHeldThroughAFilterShowsThroughOneItServesWhatTheReadWholeShows(t *testing.T) {
	met := make(map[string]bool)
	readsThrough(t, func(c *digestChain, f *ledger.Follower, r *rand.Rand) {
		from, ids, prices := filedDates(c.lines)[0], idsOf(c.lines), changingPrices()
		s := c.drawnStream(r)
		whole := heldWhole(f, from, s, prices, c.at)
		for range heldFilters {
			shows := drawnFilter(r, ids)
			within := widened(r, shows, ids)
			read, _ := readHeldBack(f, func(date string) LogFilter { return within.ThroughOn(s, date) }, from, s, prices, c.at)
			if got, want := shownThrough(f, read, shows, from, s, prices, c.at), shownThrough(f, whole, shows, from, s, prices, c.at); !reflect.DeepEqual(got, want) {
				c.t.Errorf("%s at %v: its past days held through %+v, the Log through %+v shows otherwise than held whole\n%s", c.name, c.at, within, shows, unlike(got, want))
			}
			notedHeld(met, read, whole, within, s)
		}
	})
	for _, moment := range []string{heldDropped, heldNone, heldTold} {
		if !met[moment] {
			t.Errorf("no world meets %s", moment)
		}
	}
}

// notedHeld notes in met the moments a read held through filters meets,
// beside the read whole, its past days read through within, s telling
// what's under way.
func notedHeld(met map[string]bool, read, whole LogRead, within LogFilter, s Stream) {
	if read.Len() < whole.Len() {
		met[heldDropped] = true
	}
	for _, d := range read.days {
		switch {
		case !d.through.whole() && d.lines.len() == 0:
			met[heldNone] = true
		case d.through.whole() && !within.whole() && tellsOfLineOn(s, d.day.Date):
			met[heldTold] = true
		}
	}
}

// The Log's read with its past days held through a filter, those that
// don't serve another read again through the join, as RefilterThrough has
// it, a day at a time, the newest first, and taken in as Refiltered takes
// them, holds what the read afresh holds, each day read through what the
// refiltered day was read through: it shows alike through the filter each
// was held through, the other, and every line; it shows through the other
// what the read whole shows; and it's another read than the one
// refiltered. Of throughWorlds at readsThrough's moments, heldFilters a
// moment, each filter as drawnFilter draws it, of a stream drawnStream
// draws.
func TestAReadRefilteredHoldsWhatTheReadAfreshThroughTheJoinHolds(t *testing.T) {
	met := make(map[string]bool)
	readsThrough(t, func(c *digestChain, f *ledger.Follower, r *rand.Rand) {
		from, ids, prices := filedDates(c.lines)[0], idsOf(c.lines), changingPrices()
		s := c.drawnStream(r)
		whole := heldWhole(f, from, s, prices, c.at)
		for range heldFilters {
			was, shows := drawnFilter(r, ids), drawnFilter(r, ids)
			read, held := readHeldBack(f, func(date string) LogFilter { return was.ThroughOn(s, date) }, from, s, prices, c.at)
			refiltered, through := refilteredAll(f, read, held, shows, s, c.at)
			afresh, _ := readHeldBack(f, func(date string) LogFilter {
				if joined, ok := through[date]; ok {
					return joined
				}
				return was.ThroughOn(s, date)
			}, from, s, prices, c.at)
			if len(through) > 0 {
				met[heldRefilter] = true
			}
			said := fmt.Sprintf("%s at %v: held through %+v, refiltered for %+v", c.name, c.at, was, shows)
			if !refiltered.Serves(shows) || !refiltered.Serves(was) || refiltered.Same(read) != (len(through) == 0) {
				c.t.Errorf("%s: serves it %t, the one it was held through %t, the same read %t; want both served, another read where any day was read again",
					said, refiltered.Serves(shows), refiltered.Serves(was), refiltered.Same(read))
			}
			for _, by := range []LogFilter{shows, was, {}} {
				if got, want := shownThrough(f, refiltered, by, from, s, prices, c.at), shownThrough(f, afresh, by, from, s, prices, c.at); !reflect.DeepEqual(got, want) {
					c.t.Errorf("%s: through %+v it shows otherwise than read afresh\n%s", said, by, unlike(got, want))
				}
			}
			if got, want := shownThrough(f, refiltered, shows, from, s, prices, c.at), shownThrough(f, whole, shows, from, s, prices, c.at); !reflect.DeepEqual(got, want) {
				c.t.Errorf("%s: it shows otherwise than held whole\n%s", said, unlike(got, want))
			}
		}
	})
	if !met[heldRefilter] {
		t.Errorf("no world meets %s", heldRefilter)
	}
}

// refilteredAll is read with each day that doesn't serve the filter f read
// again through l, as Unserved has them, the newest first, a day a read,
// through RefilterThrough, and taken in as Refiltered takes it, the store
// held taken in as each read gives it, s telling what's under way, at now;
// and what each day was read again through, by its date.
func refilteredAll(l Ledger, read LogRead, held HeldSessions, f LogFilter, s Stream, now time.Time) (LogRead, map[string]LogFilter) {
	through := make(map[string]LogFilter)
	for _, day := range read.Unserved(f) {
		through[day.Date] = read.RefilterThrough(day.Date, f, s)
		var fresh *HeldSessions
		if read, fresh, _ = read.Refiltered(l, ReadDay(l, day.Date, through[day.Date]), held, s, now); fresh != nil {
			held = held.With(*fresh)
		}
	}
	return read, through
}

// throughDay is a day of the lines given read through the filter given, as
// ReadDay reads one, of the local day with the given date, every line its
// record.
func throughDay(date string, f LogFilter, lines ...ledger.Held) LogDay {
	day := RecordsOf(lines)
	day.through, day.date = f.through(), date
	return day
}

// heldRequests are the ids of the requests of the lines read holds, in
// order, one without an id as "-".
func heldRequests(read LogRead) []string {
	var ids []string
	for l := range read.lines() {
		ids = append(ids, cmp.Or(l.flight.request, "-"))
	}
	return ids
}

// The Log's read takes in a day read through a filter, as ReadDay reads
// one, keeping of its lines those the filter keeps, as chose keeps their
// rows, and every one of a request the stream tells of: each counted, as
// countedOnce counts it, before the filter drops it, so a line given again
// is held as often as the day held whole holds it, of those kept; and two
// copies of a request saying otherwise of its account hold neither where
// the first is dropped, as the day held whole holds the first, which the
// filter hides. A line read late onto the day, as a read of it again whole
// gives, is kept as the day keeps one. Every line counted, kept or dropped,
// tells when the ledger last wrote, and none passed over as given again
// does, so a line read late that the filter drops changes the read where it
// ended after every line before it.
func TestADayHeldThroughAFilterKeepsWhatItKeepsAndToldLinesEachCountedFirst(t *testing.T) {
	at := time.Date(2026, 10, 7, 9, 0, 0, 0, time.Local)
	date, now := dayfile.DateOf(at), at.Add(48*time.Hour)
	line := func(id, account string, seconds int) ledger.Held {
		return ledger.Held{
			At: at.Add(time.Duration(seconds) * time.Second), Request: id, Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: account, Status: 200,
			TotalMS: 1500,
		}
	}
	lasting := func(l ledger.Held, ms int64) ledger.Held {
		l.TotalMS = ms
		return l
	}
	told := func(l ledger.Held) Stream {
		return Stream{Requests: map[string]Told{l.Request: {Request: l.Request, Session: l.Session, Sent: l.At, Ended: l.At.Add(time.Second)}}}
	}
	ofWork := LogFilter{Accounts: []string{"work"}}
	tests := []struct {
		name        string
		read        []ledger.Held
		stream      Stream
		late        []ledger.Held
		want        []string
		lateChanged bool
		// last is the line counted that ended last, when the ledger last
		// wrote.
		last ledger.Held
	}{
		{name: "the lines its filter keeps", read: []ledger.Held{line("r1", "work", 0), line("r2", "personal", 2)}, want: []string{"r1"},
			last: line("r2", "personal", 2)},
		{name: "a line its filter keeps not, of a request the stream tells of", read: []ledger.Held{line("r1", "work", 0), line("r2", "personal", 2)},
			stream: told(line("r2", "personal", 2)), want: []string{"r1", "r2"}, last: line("r2", "personal", 2)},
		{name: "a line with an id given twice", read: []ledger.Held{line("r1", "work", 0), line("r1", "work", 0)}, want: []string{"r1"}, last: line("r1", "work", 0)},
		{name: "a line without an id given twice", read: []ledger.Held{line("", "work", 0), line("", "work", 0), line("", "personal", 1), line("", "personal", 1)},
			want: []string{"-", "-"}, last: line("", "personal", 1)},
		{name: "two copies of a request, the first of an account its filter keeps not, the second ending later",
			read: []ledger.Held{line("r1", "personal", 0), lasting(line("r1", "work", 0), 3000)}, last: line("r1", "personal", 0)},
		{name: "a line read late onto it, its filter keeping it", read: []ledger.Held{line("r1", "work", 0)},
			late: []ledger.Held{line("r1", "work", 0), line("r3", "work", 3)}, want: []string{"r1", "r3"}, lateChanged: true, last: line("r3", "work", 3)},
		{name: "a line read late onto it, its filter keeping it not", read: []ledger.Held{line("r1", "work", 0)},
			late: []ledger.Held{line("r1", "work", 0), line("r4", "personal", 4)}, want: []string{"r1"}, lateChanged: true, last: line("r4", "personal", 4)},
		{name: "a line read late onto it, its filter keeping it not, ending before the last", read: []ledger.Held{line("r1", "work", 0)},
			late: []ledger.Held{line("r4", "personal", -1)}, want: []string{"r1"}, last: line("r1", "work", 0)},
		{name: "a line read late onto it, its filter keeping it not, of a request the stream tells of", read: []ledger.Held{line("r1", "work", 0)},
			stream: told(line("r4", "personal", 4)), late: []ledger.Held{line("r4", "personal", 4)}, want: []string{"r1", "r4"}, lateChanged: true,
			last: line("r4", "personal", 4)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taking := LogTaking{Held: HoldSessions(ledger.Pricing), Stream: tt.stream, Now: now}
			read := ReadLog(ledger.Pricing).Taken(throughDay(date, ofWork, tt.read...), true, taking)
			got := read
			if tt.late != nil {
				got = read.Taken(RecordsOf(tt.late), true, taking)
				if changed := !got.Same(read); changed != tt.lateChanged {
					t.Errorf("read late onto the day, the read changed %t, want %t", changed, tt.lateChanged)
				}
			}
			if wrote, want := got.WrittenTo(), tt.last.At.Add(time.Duration(tt.last.TotalMS)*time.Millisecond); !wrote.Equal(want) {
				t.Errorf("the ledger last wrote at %v, the read says, want %v, the end of the last line counted", wrote, want)
			}
			if ids := heldRequests(got); !slices.Equal(ids, tt.want) {
				t.Errorf("held through %+v, the day holds %v, want %v", ofWork, ids, tt.want)
			}
			if !got.Serves(ofWork) || got.Serves(LogFilter{}) {
				t.Errorf("the read serves %+v %t, every line %t; want it to serve the one it's held through alone", ofWork, got.Serves(ofWork), got.Serves(LogFilter{}))
			}
		})
	}
}

// sessionsAsked is a ledger of lines filed by now noting the ids of the
// sessions each read of them asks for.
type sessionsAsked struct {
	filedLedger
	ids *[]string
}

func (a sessionsAsked) Sessions(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	*a.ids = append(*a.ids, ids...)
	return a.filedLedger.Sessions(ids, rest, take)
}

func (a sessionsAsked) SessionsOf(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	*a.ids = append(*a.ids, ids...)
	return a.filedLedger.SessionsOf(ids, rest, take)
}

// The Log wants the store to hold the sessions of the lines its read keeps,
// a day held through a filter keeping those the filter keeps and those of
// the requests the stream tells of, never the sessions of the lines it
// drops: of a day read through a filter, and of a line read late onto it.
// A day read back feeds the store only the sessions it keeps lines of,
// whether the read holds no day yet, or today whole, as the watch's read
// back finds it.
func TestTheLogWantsOnlyTheSessionsOfTheLinesItsReadKeeps(t *testing.T) {
	at := time.Date(2026, 10, 7, 9, 0, 0, 0, time.Local)
	date, now := dayfile.DateOf(at), at.Add(48*time.Hour)
	line := func(id, session string, at time.Time) ledger.Line {
		return ledger.Line{At: at, Request: id, Kind: ledger.KindMessage, Session: session, Model: foldOpus, Account: "work", Status: 200}
	}
	a1, b1, c1, d1 := line("a1", "a", at), line("b1", "b", at.Add(time.Second)), line("c1", "c", at.Add(2*time.Second)), line("d1", "d", now.Add(-time.Hour))
	tellsOf := func(l ledger.Line) Stream {
		return Stream{Requests: map[string]Told{l.Request: {Request: l.Request, Session: l.Session, Sent: l.At}}}
	}
	ofA := LogFilter{Sessions: []string{"a"}}
	late := RecordsOf([]ledger.Held{{Line: c1}})
	tests := []struct {
		name       string
		stream     Stream
		back, late []string
	}{
		{name: "telling of none", back: []string{"a"}, late: []string{"a"}},
		{name: "telling of a request of b, its line dropped", stream: tellsOf(b1), back: []string{"a", "b"}, late: []string{"a", "b"}},
		{name: "telling of a request of c, its line read late", stream: tellsOf(c1), back: []string{"a"}, late: []string{"a", "c"}},
	}
	for _, tt := range tests {
		for _, today := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, today held %t", tt.name, today), func(t *testing.T) {
				var asks, also []string
				l := sessionsAsked{lines: []ledger.Line{a1, b1, c1, d1}, now: now, ids: &asks}
				read, held := ReadLog(ledger.Pricing), HoldSessions(ledger.Pricing)
				if today {
					var fresh *HeldSessions
					read, fresh = read.TakenBack(l, RecordsOf([]ledger.Held{{Line: d1}}), held, tt.stream, now)
					held, also, asks = held.With(*fresh), []string{"d"}, nil
				}
				read, fresh := read.TakenBack(l, throughDay(date, ofA, ledger.Held{Line: a1}, ledger.Held{Line: b1}), held, tt.stream, now)
				if fresh == nil || !slices.Equal(asks, tt.back) {
					t.Errorf("a day read back through %+v reads the store the sessions %v, want %v", ofA, asks, tt.back)
				}
				if got, want := read.Wanted(tt.stream), append(slices.Clone(tt.back), also...); !slices.Equal(got, want) {
					t.Errorf("a day held through %+v wants %v, want %v", ofA, got, want)
				}
				if got, want := read.Wanted(tt.stream, late), append(slices.Clone(tt.late), also...); !slices.Equal(got, want) {
					t.Errorf("a line of c read late onto a day held through %+v, the Log wants %v, want %v", ofA, got, want)
				}
			})
		}
	}
}

// heldThrough is a read at now holding today whole, and each of the past
// days given held through its filter, as ReadDay reads one, each a line of
// its own arriving at noon on it, of the session s on the account work.
func heldThrough(now time.Time, days map[string]LogFilter) LogRead {
	line := func(at time.Time) ledger.Held {
		return ledger.Held{At: at, Request: "r-" + dayfile.DateOf(at), Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: "work", Status: 200}
	}
	taking := LogTaking{Held: HoldSessions(ledger.Pricing), Now: now}
	read := ReadLog(ledger.Pricing).Taken(RecordsOf([]ledger.Held{line(now.Add(-time.Hour))}), true, taking)
	for date, f := range days {
		start, _, _ := dayfile.Day(date)
		read = read.Taken(throughDay(date, f, line(start.Add(12*time.Hour))), true, taking)
	}
	return read
}

// A read serves a filter where each day it holds serves it, as the day
// held through its filter serves it, today held whole serving every one;
// Unserved are those that don't, the newest first; and the Requests are
// built through the filter shown where the read serves it, else through
// the one they were last built through. A filter ticking none, nil or
// empty, ticks the same.
func TestAReadServesAFilterEachDayItHoldsServes(t *testing.T) {
	now := time.Date(2026, 10, 9, 9, 0, 0, 0, time.Local)
	work, personal, s := []string{"work"}, []string{"personal"}, []string{"s"}
	read := heldThrough(now, map[string]LogFilter{
		"2026-10-07": {Sessions: s},
		"2026-10-08": {Accounts: work, Models: []string{}, Failed: true},
	})
	was := LogFilter{Sessions: s, Accounts: work, Failed: true}
	tests := []struct {
		name     string
		f        LogFilter
		unserved []string
	}{
		{"one within both", LogFilter{Sessions: s, Accounts: work, Models: []string{foldOpus}, Kinds: []RowKind{RowCheck}, Failed: true}, nil},
		{"one within both, ticking no model as empty", LogFilter{Sessions: s, Accounts: work, Models: []string{}, Failed: true}, nil},
		{"one keeping more than the failed", LogFilter{Sessions: s, Accounts: work}, []string{"2026-10-08"}},
		{"another account's", LogFilter{Sessions: s, Accounts: personal, Failed: true}, []string{"2026-10-08"}},
		{"every line", LogFilter{}, []string{"2026-10-08", "2026-10-07"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var unserved []string
			for _, d := range read.Unserved(tt.f) {
				unserved = append(unserved, d.Date)
			}
			if !slices.Equal(unserved, tt.unserved) || read.Serves(tt.f) != (len(tt.unserved) == 0) {
				t.Errorf("Unserved(%+v) = %v, Serves %t; want %v", tt.f, unserved, read.Serves(tt.f), tt.unserved)
			}
			want := was
			if len(tt.unserved) == 0 {
				want = tt.f
			}
			if got := read.Through(tt.f, was); !got.Equal(want) {
				t.Errorf("Through(%+v, %+v) = %+v, want %+v", tt.f, was, got, want)
			}
		})
	}
}

// A day held through a filter is read again through the join of its
// filter and the one shown, as joined has it; whole where the stream tells
// of a request whose line may be filed on it, as ThroughOn has it; and of a
// day held whole, whole. A day the read holds none of is read through the
// one shown, as ThroughOn has it: whole, too, where the stream tells of a
// request on it.
func TestADayIsRefilteredThroughTheJoinOfItsFilterAndTheOneShown(t *testing.T) {
	now := time.Date(2026, 10, 9, 9, 0, 0, 0, time.Local)
	work, personal := []string{"work"}, []string{"personal"}
	read := heldThrough(now, map[string]LogFilter{"2026-10-07": {Accounts: work}})
	start, _, _ := dayfile.Day("2026-10-07")
	told := Stream{Requests: map[string]Told{"r1": {Request: "r1", Session: "s", Sent: start.Add(12 * time.Hour)}}}
	shows := LogFilter{Accounts: personal, Kinds: []RowKind{RowCount}}
	tests := []struct {
		name, date string
		s          Stream
		want       LogFilter
	}{
		{"a day held through an account's", "2026-10-07", Stream{}, LogFilter{Accounts: []string{"work", "personal"}}},
		{"a day held through an account's, the stream telling of a request on it", "2026-10-07", told, LogFilter{}},
		{"today, held whole", "2026-10-09", Stream{}, LogFilter{}},
		{"a day not held", "2026-10-08", Stream{}, LogFilter{Accounts: personal}},
		{"a day not held, the stream telling of a request on it", "2026-10-08", Stream{Requests: map[string]Told{"r2": {Request: "r2", Session: "s", Sent: start.Add(36 * time.Hour)}}},
			LogFilter{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := read.RefilterThrough(tt.date, shows, tt.s); !got.Equal(tt.want) {
				t.Errorf("RefilterThrough(%s, %+v) = %+v, want %+v", tt.date, shows, got, tt.want)
			}
		})
	}
}

// A read refiltered holds the day read again in place of the day it held,
// through the filter it was read through, its other days as they were; and
// is another read, so lines tabled of the read before are tabled of it
// afresh. A day read again whole of no line is held no longer, as no day
// read whole is held of none.
func TestARefilteredReadHoldsTheDayReadAgainInPlaceOfTheDayItHeld(t *testing.T) {
	now := time.Date(2026, 10, 9, 9, 0, 0, 0, time.Local)
	start, _, _ := dayfile.Day("2026-10-07")
	line := func(id, account string) ledger.Line {
		return ledger.Line{At: start.Add(12 * time.Hour), Request: id, Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: account, Status: 200}
	}
	work, both := LogFilter{Accounts: []string{"work"}}, LogFilter{Accounts: []string{"work", "personal"}}
	lines := []ledger.Held{{Line: line("r1", "work")}, {Line: line("r2", "personal")}}
	l := filedLedger{lines: []ledger.Line{lines[0].Line, lines[1].Line}, now: now}
	read := heldThrough(now, map[string]LogFilter{"2026-10-08": work})
	read, _ = read.TakenBack(l, throughDay("2026-10-07", work, lines...), HoldSessions(ledger.Pricing), Stream{}, now)
	src := LogSources{From: start, Read: read, Filter: work, Now: now}
	tabled := LogLinesOf(LogLines{}, src)
	tests := []struct {
		name    string
		day     LogDay
		want    []string
		through []LogFilter
	}{
		{"read again through two accounts'", throughDay("2026-10-07", both, lines...), []string{"r1", "r2", "r-2026-10-08", "r-2026-10-09"},
			[]LogFilter{both, work, {}}},
		{"read again whole", throughDay("2026-10-07", LogFilter{}, lines...), []string{"r1", "r2", "r-2026-10-08", "r-2026-10-09"},
			[]LogFilter{{}, work, {}}},
		{"read again whole, of no line", throughDay("2026-10-07", LogFilter{}), []string{"r-2026-10-08", "r-2026-10-09"}, []LogFilter{work, {}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, ok := read.Refiltered(l, tt.day, HoldSessions(ledger.Pricing), Stream{}, now)
			if !ok {
				t.Fatal("refiltering a day the read holds, it refused")
			}
			var through []LogFilter
			for _, d := range got.days {
				through = append(through, d.through)
			}
			if ids := heldRequests(got); !slices.Equal(ids, tt.want) || !slices.EqualFunc(through, tt.through, LogFilter.Equal) {
				t.Errorf("refiltered, the read holds %v through %+v, want %v through %+v", ids, through, tt.want, tt.through)
			}
			if got.Same(read) {
				t.Error("refiltered, the read is the same read, want another")
			}
			refiltered := src
			refiltered.Read = got
			if caught, afresh := LogLinesOf(tabled, refiltered), linesOf(got, work, Today(start)); !reflect.DeepEqual(caught, afresh) {
				t.Errorf("the lines tabled of the read before, caught up with it refiltered, are\n%+v\nwant them tabled afresh\n%+v", caught, afresh)
			}
		})
	}
}

// A read refuses to refilter a day it doesn't hold, which only a read back
// adds: it's the read it was, reading no store, whatever the day gives.
func TestARefilteredReadRefusesADayItDoesntHold(t *testing.T) {
	now := time.Date(2026, 10, 9, 9, 0, 0, 0, time.Local)
	start, _, _ := dayfile.Day("2026-10-06")
	line := ledger.Line{At: start.Add(12 * time.Hour), Request: "r0", Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: "work", Status: 200}
	var asks []string
	l := sessionsAsked{lines: []ledger.Line{line}, now: now, ids: &asks}
	read := heldThrough(now, map[string]LogFilter{"2026-10-08": {Accounts: []string{"work"}}})
	for _, day := range []LogDay{throughDay("2026-10-06", LogFilter{}, ledger.Held{Line: line}), RecordsOf([]ledger.Held{{Line: line}})} {
		got, fresh, ok := read.Refiltered(l, day, HoldSessions(ledger.Pricing), Stream{}, now)
		if ok || !got.Same(read) || fresh != nil || len(asks) > 0 || !slices.Equal(heldRequests(got), heldRequests(read)) {
			t.Errorf("refiltering a day of %q it doesn't hold, the read took it %t, is the same read %t, read the store %v, holds %v; want it refused, as it was",
				day.date, ok, got.Same(read), asks, heldRequests(got))
		}
	}
}

// A filter keeps a line's record as it chooses the row the line's record
// makes, of generated runs' lines through the filters drawnFilter draws.
func TestAFilterKeepsARecordAsItChoosesItsRow(t *testing.T) {
	for seed := range uint64(60) {
		lines := generatedRun(seed)
		r := rand.New(rand.NewPCG(seed, 0xb2))
		for range 10 {
			f := drawnFilter(r, idsOf(lines))
			for i := range lines {
				rec := lineRecordOf(&lines[i])
				row := lineFacts(&rec.rowRecord, nil)
				if f.keeps(&rec) != f.chose(&row) {
					t.Fatalf("run %d: %+v keeps the record of %s %t, chooses its row %t", seed, f, lines[i].Request, f.keeps(&rec), f.chose(&row))
				}
			}
		}
	}
}

// Lines tabled through a filter say they're tabled through it; those never
// tabled, through the zero LogFilter.
func TestLinesTabledSayTheFilterTheyreTabledThrough(t *testing.T) {
	f := LogFilter{Sessions: []string{"s"}, Kinds: []RowKind{RowRequest}}
	if got := LogLinesOf(LogLines{}, LogSources{Read: ReadLog(ledger.Pricing), Filter: f, Now: time.Now()}).Filter(); !got.Equal(f) {
		t.Errorf("lines tabled through %+v say %+v", f, got)
	}
	if got := (LogLines{}).Filter(); !got.Equal(LogFilter{}) {
		t.Errorf("lines never tabled say %+v, want the zero LogFilter", got)
	}
}

// eventFiles are the router's files of events holding those given, oldest
// first, each as it was last filed.
type eventFiles []events.Line

func (f eventFiles) Between(from, to time.Time) iter.Seq[events.Line] {
	return func(yield func(events.Line) bool) {
		for _, l := range f {
			if !l.At.Before(from) && l.At.Before(to) && !yield(l) {
				return
			}
		}
	}
}

func (f eventFiles) FirstDay() (time.Time, error) {
	return Today(f[0].At).Start, nil
}

// eventsShown is what the Log's Events built of src show: their rows, their
// lines and today's count; then the card of each event they read, those
// the filter hides among them, as src's Read tells it.
func eventsShown(src LogEventSources) []any {
	table := LogEvents(src)
	shown := []any{table.Rows, table.Lines, table.Today}
	for _, e := range table.told {
		details, _ := table.Details(e.row.Key, src.Read)
		shown = append(shown, details)
	}
	return shown
}

// The Log's Events read only the days the Log's read holds whole after the
// newest it holds through a filter, as its wholeFrom has them: of a read
// holding yesterday through a filter, they show, cards and all, what they
// show of the read without yesterday or any day before; and once yesterday
// is refiltered whole, what they show of the read with every day whole. A
// move's row on a day held whole tells of its session's story.
func TestTheLogsEventsReadOnlyTheDaysItsReadHoldsWhole(t *testing.T) {
	now := time.Date(2026, 10, 9, 9, 0, 0, 0, time.Local)
	today, yesterday, before := Today(now), Today(dayfile.DayStart(now, -1)), Today(dayfile.DayStart(now, -2))
	capped := today.Start.Add(30 * time.Minute)
	line := func(id, session string, at time.Time) ledger.Line {
		return ledger.Line{
			At: at, Request: id, Kind: ledger.KindMessage, Session: session, Model: foldOpus, Account: "work", Status: 200, Dir: "/work/" + session,
		}
	}
	lines := []ledger.Line{
		line("a5", "a", before.Start.Add(12*time.Hour)),
		line("a7", "a", capped.Add(-55*time.Minute)), line("b7", "b", capped.Add(-50*time.Minute)),
		line("c9", "c", capped.Add(-20*time.Minute)),
	}
	event := func(id int, e status.Event) events.Line {
		e.ID = id
		return events.Line{Event: e, Run: before.Start}
	}
	files := eventFiles{
		event(1, status.Event{At: before.Start.Add(13 * time.Hour), Kind: status.EventPressure, Account: "work"}),
		event(2, status.Event{At: yesterday.Start.Add(20 * time.Hour), Kind: status.EventLimit, Account: "personal"}),
		event(3, status.Event{At: capped, Kind: status.EventCap, Account: "work"}),
		event(4, status.Event{At: capped.Add(5 * time.Minute), Kind: status.EventMoved, Session: "c", From: "work", To: "personal"}),
	}
	l := filedLedger{lines: lines, now: now}
	on := func(day LedgerDay, f LogFilter) LogDay {
		var held []ledger.Held
		for _, line := range lines {
			if day.Holds(line.At) {
				held = append(held, ledger.Held{Line: line})
			}
		}
		return throughDay(day.Date, f, held...)
	}
	readBack := func(days ...LogDay) (LogRead, HeldSessions) {
		read, held := ReadLog(ledger.Pricing), HoldSessions(ledger.Pricing)
		for _, day := range days {
			var fresh *HeldSessions
			if read, fresh = read.TakenBack(l, day, held, Stream{}, now); fresh != nil {
				held = held.With(*fresh)
			}
		}
		return read, held
	}
	read := ReadEvents(files, before.Start, now)
	shown := func(from LedgerDay, r LogRead, held HeldSessions) []any {
		return eventsShown(LogEventSources{
			From: from.Start, Events: read, Held: held, Read: r, Oldest: OldestDay{Day: before, Keep: 30 * 24 * time.Hour}, Now: now,
		})
	}
	ofA := LogFilter{Sessions: []string{"a"}}
	through, throughHeld := readBack(on(today, LogFilter{}), on(yesterday, ofA), on(before, LogFilter{}))
	todays, todaysHeld := readBack(on(today, LogFilter{}))
	whole, wholeHeld := readBack(on(today, LogFilter{}), on(yesterday, LogFilter{}), on(before, LogFilter{}))
	got := shown(before, through, throughHeld)
	if want := shown(today, todays, todaysHeld); !reflect.DeepEqual(got, want) {
		t.Errorf("of a read holding yesterday through %+v, the Events show otherwise than of the read without it\n%s", ofA, unlike(got, want))
	}
	if rows := got[0].([]LogEvent); len(rows) != 2 || rows[1].Dir != "/work/c" {
		t.Errorf("of a read holding yesterday through %+v, the Events show %+v, want the cap and the move of c, its directory told", ofA, rows)
	}
	refiltered, fresh, ok := through.Refiltered(l, on(yesterday, LogFilter{}), throughHeld, Stream{}, now)
	if !ok {
		t.Fatalf("refiltering yesterday, the read held through %+v refused it", ofA)
	}
	if fresh != nil {
		throughHeld = throughHeld.With(*fresh)
	}
	got, want := shown(before, refiltered, throughHeld), shown(before, whole, wholeHeld)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("yesterday refiltered whole, the Events show otherwise than of the read whole\n%s", unlike(got, want))
	}
	if card := want[len(want)-2].(EventDetails); len(want[0].([]LogEvent)) != len(files) || card.Then == nil || card.Then.Sessions != 3 {
		t.Errorf("of the read whole, the Events show %+v, the cap's card %+v; want every event, its card counting a, b and c", want[0], card)
	}
}

// A read serves a filter from the day after the newest day it holds that
// doesn't serve it, whatever it holds before it or after, as a read part-way
// refiltered holds them: of every line, the day after the newest held
// through a filter; of a filter every day serves, no day.
func TestAReadServesAFilterFromTheDayAfterTheNewestDayThatDoesnt(t *testing.T) {
	now := time.Date(2026, 10, 9, 9, 0, 0, 0, time.Local)
	ofA, ofB, failed, whole := LogFilter{Sessions: []string{"a"}}, LogFilter{Sessions: []string{"b"}}, LogFilter{Failed: true}, LogFilter{}
	tests := []struct {
		name string
		days map[string]LogFilter
		f    LogFilter
		from string
	}{
		{"every day whole, a's", map[string]LogFilter{"2026-10-07": whole, "2026-10-08": whole}, ofA, ""},
		{"yesterday through a's, a's", map[string]LogFilter{"2026-10-07": whole, "2026-10-08": ofA}, ofA, ""},
		{"yesterday through a's, every line", map[string]LogFilter{"2026-10-07": whole, "2026-10-08": ofA}, whole, "2026-10-09"},
		{"a's either side of a whole day, b's", map[string]LogFilter{"2026-10-06": ofA, "2026-10-07": whole, "2026-10-08": ofA}, ofB, "2026-10-09"},
		{"a's before a whole day, b's", map[string]LogFilter{"2026-10-06": ofA, "2026-10-07": ofA, "2026-10-08": whole}, ofB, "2026-10-08"},
		{"the failed alone between whole days, a's", map[string]LogFilter{"2026-10-06": whole, "2026-10-07": failed, "2026-10-08": whole}, ofA, "2026-10-08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			day, ok := heldThrough(now, tt.days).ServedFrom(tt.f)
			if day.Date != tt.from || ok != (tt.from != "") {
				t.Errorf("a read holding %v serves %+v from %q, %t; want from %q", tt.days, tt.f, day.Date, ok, tt.from)
			}
		})
	}
}

// The Log's Events show from the day after the newest the Log's read holds
// through a filter, whatever it holds before it or after, as a read part-way
// refiltered holds them; else from the day the table starts on.
func TestTheEventsShowFromTheDayAfterTheNewestHeldThroughAFilter(t *testing.T) {
	now := time.Date(2026, 10, 9, 9, 0, 0, 0, time.Local)
	from, _, _ := dayfile.Day("2026-10-06")
	ofA, whole := LogFilter{Sessions: []string{"a"}}, LogFilter{}
	tests := []struct {
		name  string
		days  map[string]LogFilter
		first string
	}{
		{"every day whole", map[string]LogFilter{"2026-10-06": whole, "2026-10-07": whole, "2026-10-08": whole}, "2026-10-06"},
		{"yesterday through a filter", map[string]LogFilter{"2026-10-06": whole, "2026-10-07": whole, "2026-10-08": ofA}, "2026-10-09"},
		{"two days through a filter, a whole day newer than both", map[string]LogFilter{"2026-10-06": ofA, "2026-10-07": ofA, "2026-10-08": whole}, "2026-10-08"},
		{"days through a filter either side of a whole day", map[string]LogFilter{"2026-10-06": ofA, "2026-10-07": whole, "2026-10-08": ofA}, "2026-10-09"},
		{"a day through the failed alone between whole days", map[string]LogFilter{"2026-10-06": whole, "2026-10-07": {Failed: true}, "2026-10-08": whole}, "2026-10-08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := LogEvents(LogEventSources{From: from, Read: heldThrough(now, tt.days), Now: now})
			if table.first.Date != tt.first {
				t.Errorf("the Events of a read holding %v show from %s, want %s", tt.days, table.first.Date, tt.first)
			}
		})
	}
}
