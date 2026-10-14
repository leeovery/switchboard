package views_test

import (
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
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
func (c *countingLedger) Sessions(ids []string, take func(string, ledger.Line)) map[string][]ledger.Line {
	c.reads++
	for _, id := range ids {
		c.asked[id]++
	}
	written := c.fakeLedger
	written.lines = slices.DeleteFunc(slices.Clone(c.lines), func(l ledger.Line) bool { return writtenAt(l).After(c.now) })
	return written.Sessions(ids, take)
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
// line a look. The ledger's clock at each look is as the last line it gave
// was written, so a session read early is read short of the lines its later
// looks give, a long request's among them, arriving before lines held. Each
// line read is priced, its request's id and arrival counted in priced.
func readGrowing(hm viewstest.LogMoment, l *countingLedger, priced map[string]int) views.LogRead {
	days := spannedDays(hm)
	today := slices.SortedStableFunc(slices.Values(days[0]), func(a, b ledger.Held) int { return writtenAt(a.Line).Compare(writtenAt(b.Line)) })
	half := len(today) / 2
	l.now = hm.Now
	if half > 0 {
		l.now = writtenAt(today[half-1].Line)
	}
	opened := slices.SortedStableFunc(slices.Values(today[:half]), func(a, b ledger.Held) int { return a.At.Compare(b.At) })
	read := views.CountingPrices(views.ReadLogHeld(l, nil, ledger.Pricing, hm.Now), priced).Grown(l, opened, hm.Now)
	for _, day := range days[1:] {
		read = read.Back(l, day, hm.Now)
	}
	for i := half; i < len(today); i++ {
		l.now = writtenAt(today[i].Line)
		read = read.Grown(l, today[i:i+1], hm.Now)
	}
	l.now = hm.Now
	return read
}

// TestTheLogsReadServesItsEventsAndItsPanelReadingAndPricingNothingMore
// builds, at each hard moment of the events, the Log's Requests, its Events,
// the card of each event and its panel's sessions all from the one read:
// none reads the ledger, nor has the read price a line again.
func TestTheLogsReadServesItsEventsAndItsPanelReadingAndPricingNothingMore(t *testing.T) {
	for _, hm := range viewstest.EventMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			l, priced := &countingLedger{fakeLedger: ledgerHolding(hm.Now, hm.Lines...), asked: make(map[string]int)}, make(map[string]int)
			read := views.CountingPrices(views.ReadLogHeld(l, nil, ledger.Pricing, hm.Now), priced).Grown(l, heldOf(hm.Lines...), hm.Now)
			reads, before := l.reads, maps.Clone(priced)
			requests := views.LogRows(views.LogSources{From: hm.From, Read: read, Now: hm.Now})
			table := views.LogEvents(views.LogEventSources{From: hm.From, Events: eventsRead(hm, filedIn(t, hm)), Read: read, Now: hm.Now})
			for _, row := range table.Rows {
				table.Details(row.Key, read)
			}
			views.LogSessions(views.LogSessionSources{
				Read: read, Lines: requests.Count(views.Today(hm.Now).Date).Sessions, Events: table.Today.Sessions, Unlisted: true, Now: hm.Now,
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
			grown.Read = readGrowing(hm, countingAt(hm), make(map[string]int))
			if got, want := views.LogRows(grown), views.LogRows(once); !reflect.DeepEqual(got, want) {
				t.Errorf("grown a look at a time, the table is\n%s\nwant it as read at once\n%s", tableLines(got), tableLines(want))
			}
		})
	}
}

// tableLines say a table's lines, as the moments want them, for a failure to
// show.
func tableLines(table views.LogTable) string {
	lines := make([]viewstest.LogLine, len(table.Lines))
	for i, l := range table.Lines {
		lines[i] = logLineOf(l)
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
			src.Read = read
			built := views.LogRows(src)
			for range 2 {
				views.LogRows(src)
			}
			src.Read = read.Grown(l, spannedDays(hm)[0], hm.Now)
			again := views.LogRows(src)
			if l.reads != reads || !maps.Equal(priced, before) {
				t.Errorf("three builds and a look giving today's lines again read the ledger %d times and priced %v, want nothing read or priced",
					l.reads-reads, priced)
			}
			if !reflect.DeepEqual(again, built) {
				t.Errorf("a look giving lines the read holds made the table\n%s\nwant it as it was\n%s", tableLines(again), tableLines(built))
			}
		})
	}
}

func TestALookReadsOnlyTheSessionsTheReadDoesntHold(t *testing.T) {
	// older began yesterday; mover begins today, after the Log opened on
	// older's request. Each look gives the request that came by its time:
	// older's again, as the ledger's read afresh, mover's first, mover's
	// second.
	yesterday := withID("r0", of(ledger.ClassMain, line(older, opus, "work", "new", october(6, 22, 0), hourUsage)))
	olders := withID("r1", of(ledger.ClassMain, line(older, opus, "work", "sticky", october(7, 9, 0), hourUsage)))
	movers := withID("r2", of(ledger.ClassMain, line(mover, opus, "work", "new", october(7, 9, 5), hourUsage)))
	next := withID("r3", of(ledger.ClassMain, line(mover, opus, "work", "sticky", october(7, 9, 10), hourUsage)))
	l := &countingLedger{fakeLedger: ledgerHolding(olders.At, yesterday, olders, movers, next), asked: make(map[string]int)}
	read := views.ReadLogHeld(l, heldOf(olders), ledger.Pricing, now)
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
			read = read.Grown(l, heldOf(tt.gives), now)
			if got := slices.Sorted(maps.Keys(l.asked)); l.reads-reads != tt.reads || !slices.Equal(got, tt.asked) {
				t.Errorf("the look read the ledger %d times, sessions asked so far %q, want %d, %q", l.reads-reads, got, tt.reads, tt.asked)
			}
		})
	}
	table := views.LogRows(views.LogSources{From: now, Now: now, Read: read})
	if said, want := tableSaid(table), []string{"r1 lined", "▲ r2 lined", "r3 lined"}; !slices.Equal(said, want) {
		t.Errorf("the table says %q, want %q: older's first request yesterday's, mover's today's", said, want)
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
	before := views.ReadLogHeld(l, heldOf(first, between, settles), ledger.Pricing, now)
	src := views.LogSources{From: now, Now: now, Read: before}
	was := views.LogRows(src)
	grown := before.Grown(l, heldOf(moved), now)
	if got := views.LogRows(src); !reflect.DeepEqual(got, was) {
		t.Errorf("growing the read changed the read it grew from: its table is\n%s\nwant\n%s", tableLines(got), tableLines(was))
	}
	src.Read = grown
	got := views.LogRows(src)
	if said, want := tableSaid(got), []string{"▲ d0 lined", "d3 lined", "▸ d1 lined", "d2 lined"}; !slices.Equal(said, want) || got.Lines[2].Row.Routing.Cost == nil {
		t.Errorf("grown, the table says %q, d1's move costing %v, want %q, the move's cost settled by d2", said, got.Lines[2].Row.Routing.Cost, want)
	}
}
