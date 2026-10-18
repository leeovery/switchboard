package views_test

import (
	"fmt"
	"iter"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// countingLedger is a ledger as fakeLedger reads it, its clock as set, each
// line written as its request ended, as the ledger writes it, that counts
// what's asked of it: how often its Sessions is called, and each session's
// id asked of it.
type countingLedger struct {
	fakeLedger
	reads int
	asked map[string]int
}

// Sessions reads the lines written by now, as fakeLedger's Sessions does.
func (c *countingLedger) Sessions(ids []string, whole func(string) bool, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return c.written(ids).Sessions(ids, whole, take)
}

// SessionsOf reads the lines written by now, as fakeLedger's SessionsOf
// does.
func (c *countingLedger) SessionsOf(ids []string, whole func(string) bool, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return c.written(ids).SessionsOf(ids, whole, take)
}

// written counts a read of the sessions with the given ids, and is the
// ledger of the lines written by now.
func (c *countingLedger) written(ids []string) fakeLedger {
	c.reads++
	for _, id := range ids {
		c.asked[id]++
	}
	written := c.fakeLedger
	written.lines = slices.DeleteFunc(slices.Clone(c.lines), func(l ledger.Line) bool { return writtenAt(l).After(c.now) })
	return written
}

// writtenAt is when the ledger wrote l: as its request ended.
func writtenAt(l ledger.Line) time.Time {
	return l.At.Add(time.Duration(l.TotalMS) * time.Millisecond)
}

// countingAt is a counting ledger of the moment's lines, its clock at its
// moment.
func countingAt(hm viewstest.LogMoment) *countingLedger {
	return &countingLedger{fakeLedger: ledgerHolding(hm.Now, hm.Lines...), asked: make(map[string]int)}
}

// readGrowing is the Log's read of the moment's lines through l, as the
// watch grows it, today's in the order the ledger wrote them: the first half
// of today's read as the Log opens, by arrival, as Today gives them; each
// day before taken back, newest first; then the rest of today's taken in a
// line a look, as the tail gains them. The ledger's clock at each look is as
// the last line it gave was written, so a session read early is read short
// of the lines its later looks give, a long request's among them, arriving
// before lines held; and the store the rows are told by is fed at it, but
// never at a time before it was last fed, as a clock set back drops it, nor
// after the moment.
// Each line read is priced, its request's id and arrival counted in priced,
// and placed as the moment's stream tells.
func readGrowing(hm viewstest.LogMoment, l *countingLedger, priced map[string]int) viewstest.LogReading {
	days := spannedDays(hm)
	today := slices.SortedStableFunc(slices.Values(days[0]), func(a, b ledger.Held) int { return writtenAt(a.Line).Compare(writtenAt(b.Line)) })
	half := len(today) / 2
	l.now = hm.Now
	if half > 0 {
		l.now = writtenAt(today[half-1].Line)
	}
	opened := slices.SortedStableFunc(slices.Values(today[:half]), func(a, b ledger.Held) int { return a.At.Compare(b.At) })
	read := viewstest.ReadingLog(ledger.Pricing)
	read.Read = views.CountingPrices(read.Read, priced)
	fed := l.now
	read = read.Took(l, opened, true, hm.Stream, fed)
	for _, day := range days[1:] {
		read = read.Took(l, day, true, hm.Stream, fed)
	}
	for i := half; i < len(today); i++ {
		l.now = writtenAt(today[i].Line)
		fed = minTime(score.Later(fed, l.now), hm.Now)
		read = read.Took(l, today[i:i+1], false, hm.Stream, fed)
	}
	l.now = hm.Now
	return read
}

// minTime is the earlier of two times.
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// TestTheLogsReadServesItsEventsAndItsPanelReadingAndPricingNothingMore
// builds, at each hard moment of the events, the Log's Requests, its Events,
// the card of each event and its panel's sessions all from the one read and
// the store it's told by: none reads the ledger, nor has the read price a
// line again.
func TestTheLogsReadServesItsEventsAndItsPanelReadingAndPricingNothingMore(t *testing.T) {
	for _, hm := range viewstest.EventMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			l, priced := &countingLedger{fakeLedger: ledgerHolding(hm.Now, hm.Lines...), asked: make(map[string]int)}, make(map[string]int)
			read := viewstest.ReadingLog(ledger.Pricing)
			read.Read = views.CountingPrices(read.Read, priced)
			read = read.Took(l, heldOf(hm.Lines...), true, views.Stream{}, hm.Now)
			reads, before := l.reads, maps.Clone(priced)
			requests := views.LogRows(views.LogSources{From: hm.From, Read: read.Read, Now: hm.Now})
			table := views.LogEvents(views.LogEventSources{From: hm.From, Events: eventsRead(hm, filedIn(t, hm)), Held: read.Held, Now: hm.Now})
			for _, row := range table.Rows {
				table.Details(row.Key, read.Read)
			}
			views.LogSessions(views.LogSessionSources{
				Read: read.Read, Held: read.Held, Lines: requests.Count(views.Today(hm.Now).Date).Sessions, Events: table.Today.Sessions, Unlisted: true,
				Now: hm.Now,
			})
			if l.reads != reads || !maps.Equal(priced, before) {
				t.Errorf("the tables, the cards and the panel read the ledger %d times and priced %v, want nothing read or priced but the read's own %v",
					l.reads-reads, priced, before)
			}
		})
	}
}

func TestTheLogsReadGrownIsTheReadAtOnceAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.LogMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			once, grown := logSourcesOf(hm), logSourcesOf(hm)
			grown.Read = readGrowing(hm, countingAt(hm), make(map[string]int)).Read
			if got, want := views.LogRows(grown), views.LogRows(once); !sameTable(got, want) {
				t.Errorf("grown a look at a time, the table is\n%s\nwant it as read at once\n%s", tableLines(got), tableLines(want))
			}
		})
	}
}

// shownOf are what the lines of a table show, in order: each row, rule and
// oldest day's line, as views.LogLine's own fields hold them.
func shownOf(table views.LogTable) []views.LogLine {
	var shown []views.LogLine
	for _, l := range table.All() {
		shown = append(shown, views.LogLine{Row: l.Row, Rule: l.Rule, Oldest: l.Oldest})
	}
	return shown
}

// sameTable reports whether two tables show the same: the same lines, in
// the same order, each day's lines counting alike, and the same days
// awaiting a line.
func sameTable(a, b views.LogTable) bool {
	if !reflect.DeepEqual(shownOf(a), shownOf(b)) || !reflect.DeepEqual(a.Awaited(views.LedgerDay{Start: farOff}), b.Awaited(views.LedgerDay{Start: farOff})) {
		return false
	}
	for _, l := range slices.Concat(shownOf(a), shownOf(b)) {
		if date := views.Today(l.Row.At).Date; l.IsRow() && !reflect.DeepEqual(a.Count(date), b.Count(date)) {
			return false
		}
	}
	return true
}

// farOff is a moment after every one the tests tell of.
var farOff = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// tableLines say a table's lines, as the moments want them, for a failure to
// show.
func tableLines(table views.LogTable) string {
	lines := make([]viewstest.LogLine, 0, table.Len())
	for _, l := range table.All() {
		lines = append(lines, logLineOf(l))
	}
	return logSaid(lines)
}

func TestTheLogsReadReadsEachSessionOnceAndPricesEachLineOnceAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.LogMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			l, priced := countingAt(hm), make(map[string]int)
			read := readGrowing(hm, l, priced)
			for id, n := range l.asked {
				if n != 1 {
					t.Errorf("session %s was read %d times, want once", id[:4], n)
				}
			}
			spanned := slices.Concat(spannedDays(hm)...)
			if len(priced) != len(spanned) || slices.ContainsFunc(slices.Collect(maps.Values(priced)), func(n int) bool { return n != 1 }) {
				t.Errorf("the read priced %v, want each of the %d lines it holds once", priced, len(spanned))
			}
			reads, before := l.reads, maps.Clone(priced)
			src := logSourcesOf(hm)
			src.Read = read.Read
			built := views.LogRows(src)
			for range 2 {
				views.LogRows(src)
			}
			again := read.Took(l, spannedDays(hm)[0], true, hm.Stream, hm.Now)
			src.Read = again.Read
			if l.reads != reads || !maps.Equal(priced, before) {
				t.Errorf("three builds and a look giving today's lines again read the ledger %d times and priced %v, want nothing read or priced",
					l.reads-reads, priced)
			}
			if got := views.LogRows(src); !again.Read.Same(read.Read) || !sameTable(got, built) {
				t.Errorf("a look giving lines the read holds made the table\n%s\nwant it as it was\n%s", tableLines(got), tableLines(built))
			}
		})
	}
}

func TestALookReadsOnlyTheSessionsTheStoreDoesntHold(t *testing.T) {
	// older began yesterday; mover begins today, after the Log opened on
	// older's request. Each look gives the request that came by its time:
	// older's again, as the ledger's read afresh, mover's first, mover's
	// second.
	yesterday := withID("r0", of(ledger.ClassMain, line(older, opus, "work", "new", october(6, 22, 0), hourUsage)))
	olders := withID("r1", of(ledger.ClassMain, line(older, opus, "work", "sticky", october(7, 9, 0), hourUsage)))
	movers := withID("r2", of(ledger.ClassMain, line(mover, opus, "work", "new", october(7, 9, 5), hourUsage)))
	next := withID("r3", of(ledger.ClassMain, line(mover, opus, "work", "sticky", october(7, 9, 10), hourUsage)))
	l := &countingLedger{fakeLedger: ledgerHolding(olders.At, yesterday, olders, movers, next), asked: make(map[string]int)}
	read := viewstest.ReadingLog(ledger.Pricing).Took(l, heldOf(olders), true, views.Stream{}, olders.At)
	tests := []struct {
		name  string
		gives ledger.Line
		reads int
		asked []string
	}{
		{name: "a session it holds reads nothing", gives: olders, asked: []string{older}},
		{name: "a session it doesn't reads that one", gives: movers, reads: 1, asked: []string{older, mover}},
		{name: "that session again reads nothing", gives: next, asked: []string{older, mover}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reads := l.reads
			l.now = tt.gives.At
			read = read.Took(l, heldOf(tt.gives), false, views.Stream{}, l.now)
			if got := slices.Sorted(maps.Keys(l.asked)); l.reads-reads != tt.reads || !slices.Equal(got, tt.asked) {
				t.Errorf("the look read the ledger %d times, sessions asked so far %q, want %d, %q", l.reads-reads, got, tt.reads, tt.asked)
			}
		})
	}
	table := views.LogRows(views.LogSources{From: now, Now: now, Read: read.Read})
	if said, want := tableSaid(table), []string{"r1 lined", "▲ r2 lined", "r3 lined"}; !slices.Equal(said, want) {
		t.Errorf("the table says %q, want %q: older's first request yesterday's, mover's today's", said, want)
	}
}

// A read of the tail that changes one of a session's moves tells again the
// rows of that move's request alone, its settling request new: neither the
// session's other rows, nor those of its other move, which the read leaves
// as it was.
func TestARetellTellsAgainOnlyTheRowsOfTheMoveChanged(t *testing.T) {
	main := func(id, account string, at time.Time) ledger.Line {
		return withID(id, of(ledger.ClassMain, line(mover, opus, account, "sticky", at, hourUsage)))
	}
	moved := func(id, from, to string, at time.Time) ledger.Line {
		return movedFrom(from, withID(id, of("auxiliary", line(mover, opus, to, "moved: "+from+" is at its reserve", at, hourUsage))))
	}
	lines := []ledger.Line{
		main("m0", "work", october(7, 9, 0)), moved("m1", "work", "side", october(7, 9, 10)), main("m2", "side", october(7, 9, 15)),
		moved("n1", "side", "work", october(7, 9, 30)),
	}
	settles := main("n2", "work", october(7, 9, 35))
	read := logReading(writtenAt(lines[3]), views.Stream{}, lines...)
	at := writtenAt(settles)
	grown := read.Took(ledgerHolding(at, append(lines, settles)...), heldOf(settles), false, views.Stream{}, at)
	if got, want := views.Retold(grown.Read), []string{"n1"}; !slices.Equal(got, want) {
		t.Errorf("settling n1's move told again the rows of %v, want %v alone", got, want)
	}
}

func TestGrowingTheLogsReadLeavesTheReadItGrewAsItWas(t *testing.T) {
	// d1, a side request, moves mover, and runs long; d2, its conversation's
	// next, arrives after it, but ends first, so its line comes a look before
	// d1's. d2 settles what d1's move cost only once d1's line has come.
	first := withID("d0", of(ledger.ClassMain, line(mover, opus, "work", "new", october(7, 9, 0), hourUsage)))
	between := withID("d3", of(ledger.ClassMain, line(mover, opus, "work", "sticky", october(7, 9, 30), hourUsage)))
	moved := movedFrom("work", withID("d1", of("auxiliary", line(mover, opus, "side", "moved: work is at its reserve", october(7, 10, 0), hourUsage))))
	moved.TotalMS = (10 * time.Minute).Milliseconds()
	settles := withID("d2", of(ledger.ClassMain, line(mover, opus, "side", "sticky", october(7, 10, 5), hourUsage)))
	l := &countingLedger{fakeLedger: ledgerHolding(writtenAt(settles), first, between, moved, settles), asked: make(map[string]int)}
	before := viewstest.ReadingLog(ledger.Pricing).Took(l, heldOf(first, between, settles), true, views.Stream{}, writtenAt(settles))
	src := views.LogSources{From: now, Now: now, Read: before.Read}
	was := views.LogRows(src)
	l.now = writtenAt(moved)
	grown := before.Took(l, heldOf(moved), false, views.Stream{}, l.now)
	if got := views.LogRows(src); !sameTable(got, was) {
		t.Errorf("growing the read changed the read it grew from: its table is\n%s\nwant\n%s", tableLines(got), tableLines(was))
	}
	src.Read = grown.Read
	got := views.LogRows(src)
	if said, want := tableSaid(got), []string{"▲ d0 lined", "d3 lined", "▸ d1 lined", "d2 lined"}; !slices.Equal(said, want) || got.Line(2).Row.Routing.Cost == nil {
		t.Errorf("grown, the table says %q, d1's move costing %v, want %q, the move's cost settled by d2", said, got.Line(2).Row.Routing.Cost, want)
	}
}

// A read of the tail gaining k lines has the lines tabled catch up copying
// a chunk of the rows for each at most, however many the table holds: never
// the table.
func TestCatchingUpALookCopiesAChunkALineWhateverTheTableHolds(t *testing.T) {
	for _, n := range []int{300, 3000} {
		lines := make([]ledger.Line, n)
		for i := range lines {
			lines[i] = withID(fmt.Sprintf("r%05d", i), line(fmt.Sprintf("s%05d", i), opus, "work", "new", october(7, 0, 0).Add(time.Duration(i)*5*time.Second), hourUsage))
		}
		reading := logReading(now, views.Stream{}, lines...)
		src := views.LogSources{From: now, Now: now, Read: reading.Read}
		tabled := views.LogLinesOf(views.LogLines{}, src)
		for _, k := range []int{1, 4, 16} {
			var gained []ledger.Line
			for i := range k {
				at := october(7, 0, 0).Add(time.Duration(i*n/k)*5*time.Second + time.Second)
				gained = append(gained, withID(fmt.Sprintf("g%02d%03d", k, i), line(fmt.Sprintf("g%02d%03d", k, i), opus, "work", "new", at, hourUsage)))
			}
			lines = append(lines, gained...)
			reading = reading.Took(ledgerHolding(now, lines...), heldOf(gained...), false, views.Stream{}, now)
			src.Read = reading.Read
			caught := views.LogLinesOf(tabled, src)
			if copied := caught.Copied() - tabled.Copied(); copied > k {
				t.Errorf("of %d rows, a look gaining %d lines copied %d chunks, want %d at most", n, k, copied, k)
			}
			if got, want := caught.Rows(src), views.LogRows(src); !sameTable(got, want) {
				t.Errorf("of %d rows, caught up with %d lines, the table is\n%s\nwant it tabled afresh\n%s", n, k, tableLines(got), tableLines(want))
			}
			tabled = caught
		}
	}
}

// A read from a day holds none of the days before it: its table, from any
// day before, is the table of the read whole from that day, and the lines
// tabled of the read whole, caught up, are those tabled of it afresh; the
// read it was made from holds what it held.
func TestAReadFromADayHoldsNoneOfTheDaysBefore(t *testing.T) {
	yesterday := withID("y0", line(older, opus, "work", "new", october(6, 22, 0), hourUsage))
	today := withID("t0", line(mover, opus, "work", "new", october(7, 9, 0), hourUsage))
	whole := logReading(now, views.Stream{}, yesterday, today).Read
	spanning := views.LogSources{From: october(6, 0, 0), Now: now, Read: whole}
	was := views.LogRows(spanning)
	from := whole.From(views.Today(now))
	src := spanning
	src.Read = from
	if got, want := views.LogRows(src), views.LogRows(views.LogSources{From: now, Now: now, Read: whole}); !sameTable(got, want) {
		t.Errorf("read from today, the table from yesterday is\n%s\nwant today's alone\n%s", tableLines(got), tableLines(want))
	}
	if got := views.LogLinesOf(views.LogLinesOf(views.LogLines{}, spanning), src).Rows(src); !sameTable(got, views.LogRows(src)) {
		t.Errorf("caught up with the read from today, the lines tabled are\n%s\nwant them tabled afresh\n%s", tableLines(got), tableLines(views.LogRows(src)))
	}
	if got := views.LogRows(spanning); !sameTable(got, was) || from.Len() != 1 || whole.Len() != 2 {
		t.Errorf("read from today, it holds %d lines and the read it was made from %d, its table\n%s\nwant 1 and 2, the table as it was", from.Len(), whole.Len(), tableLines(got))
	}
}

// listedEvents are the router's events, as its files give them, each as it
// was filed.
type listedEvents []events.Line

func (l listedEvents) Between(from, to time.Time) iter.Seq[events.Line] {
	return func(yield func(events.Line) bool) {
		for _, e := range l {
			if !e.At.Before(from) && e.At.Before(to) && !yield(e) {
				return
			}
		}
	}
}

func (l listedEvents) FirstDay() (time.Time, error) {
	return views.Today(l[0].At).Start, nil
}

// Events read from a day hold none of those before EventsFrom of it, and
// those after as they stand, as events read from that day hold them; and the
// read they were made from holds what it held. Read from a day before, they
// hold what they held.
func TestEventsReadFromADayHoldNoneBeforeIt(t *testing.T) {
	run := october(1, 0, 0)
	filed := listedEvents{
		{ID: 1, At: october(5, 20, 0), Kind: status.EventPin, Accounts: []string{"work"}, Run: run},
		{ID: 2, At: october(6, 22, 0), Kind: status.EventPin, Accounts: []string{"side"}, Run: run},
		{ID: 3, At: october(7, 9, 0), Kind: status.EventPin, Accounts: []string{"work"}, Run: run},
	}
	whole := views.ReadEvents(filed, october(5, 0, 0), now)
	from := whole.From(views.Today(now))
	if got, want := from.Events(), views.ReadEvents(filed, now, now).Events(); !reflect.DeepEqual(got, want) || !from.Since().Equal(views.EventsFrom(now)) {
		t.Errorf("read from today, the events are %v, since %v, want %v, as read from today, since %v", got, from.Since(), want, views.EventsFrom(now))
	}
	if got := len(whole.Events()); got != 3 {
		t.Errorf("the events read from 5 October hold %d events after the read from today was made, want the 3 they held", got)
	}
	if again := from.From(views.Today(october(6, 0, 0))); !reflect.DeepEqual(again.Events(), from.Events()) {
		t.Errorf("read from yesterday, the events read from today hold %v, want them as they stand, %v", again.Events(), from.Events())
	}
}

// A read holds the line of a request the stream tells of on a day before
// the one given where the request was sent on that day or after, its line
// filed on the day before, as one sent just after midnight: letting go of
// that day would have its row told again, awaiting the line there.
func TestAReadHoldsTheLineOfARequestToldOfFiledOnADayBefore(t *testing.T) {
	midnight := october(7, 0, 0)
	crossing := withID("x0", line(mover, opus, "work", "new", midnight.Add(-2*time.Second), hourUsage))
	crossing.TotalMS = (12 * time.Second).Milliseconds()
	earlier := withID("e0", line(older, opus, "work", "new", october(6, 22, 0), hourUsage))
	sent := func(l ledger.Line, at time.Time) views.Stream {
		return views.Stream{Live: true, Requests: map[string]views.Told{l.Request: {Request: l.Request, Session: l.Session, Sent: at}}}
	}
	tests := []struct {
		name   string
		lines  []ledger.Line
		stream views.Stream
		holds  bool
	}{
		{name: "sent after midnight, its line filed the day before", lines: []ledger.Line{crossing}, stream: sent(crossing, midnight.Add(time.Second)), holds: true},
		{name: "the stream telling of it no longer", lines: []ledger.Line{crossing}},
		{name: "its line not read", lines: []ledger.Line{earlier}, stream: sent(crossing, midnight.Add(time.Second))},
		{name: "sent the day before", lines: []ledger.Line{earlier}, stream: sent(earlier, earlier.At.Add(time.Second))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			read := logReading(midnight.Add(time.Minute), views.Stream{}, tt.lines...).Read
			if got := read.HoldsTold(tt.stream, views.Today(midnight)); got != tt.holds {
				t.Errorf("the read holds before today the line of a request told of %t, want %t", got, tt.holds)
			}
		})
	}
}

// A session's line without an id told again, as a later line settles what
// its move cost, is its alone: another session's line without an id that
// arrived at the same moment keeps its own story, so the read grown is the
// read at once.
func TestRetellingALineWithoutAnIDRetellsItsSessionsAlone(t *testing.T) {
	at := october(7, 10, 0)
	first := withID("d0", of(ledger.ClassMain, line(mover, opus, "work", "new", october(7, 9, 0), hourUsage)))
	moved := movedFrom("work", withID("", of("auxiliary", line(mover, opus, "side", "moved: work is at its reserve", at, hourUsage))))
	alongside := withID("", of(ledger.ClassMain, line(older, opus, "work", "new", at, hourUsage)))
	settles := withID("d2", of(ledger.ClassMain, line(mover, opus, "side", "sticky", october(7, 10, 5), hourUsage)))
	read, looked := october(7, 10, 2), october(7, 10, 6)
	all := ledgerHolding(looked, first, moved, alongside, settles)
	grown := logReading(read, views.Stream{}, first, moved, alongside).Took(all, heldOf(settles), false, views.Stream{}, looked)
	src := views.LogSources{From: looked, Now: looked, Read: grown.Read}
	once := src
	once.Read = logRead(looked, first, moved, alongside, settles)
	if got, want := views.LogRows(src), views.LogRows(once); !sameTable(got, want) {
		t.Errorf("grown, the table is\n%s\nwant it as read at once\n%s", tableLines(got), tableLines(want))
	}
}
