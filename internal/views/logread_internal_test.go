package views

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// logGrowing is the Log's read grown look by look over a generated run, as
// the watch grows it: the run's lines, as filed; the read and the store its
// rows are told by; its lines tabled through the filter, as each look
// catches them up; which of the run's lines the last look found filed; and
// the days it spans from.
type logGrowing struct {
	t      *testing.T
	seed   uint64
	lines  []ledger.Line
	prices ledger.Table
	read   LogRead
	held   HeldSessions
	tabled LogLines
	filter LogFilter
	filed  []bool
	from   time.Time
}

// look grows the read by a read of the ledger's tail at now: today's lines
// filed since the last look as its gain, or where again says, every one of
// today's, as a read afresh gives them; and each day before today that
// gained a line read again whole. The store is fed them first, wanting the
// sessions of the Log's lines, then the read takes them in, but those it
// holds, as Unheld has them, the feed's gains told once, as the watch's tail
// read takes them. The lines tabled are caught up with the read grown, and
// held to them tabled afresh.
func (g *logGrowing) look(now time.Time, again bool) {
	g.t.Helper()
	today := Today(now)
	var gained []ledger.Held
	days := make(map[string][]ledger.Held)
	late := make(map[string]bool)
	filed := make([]bool, len(g.lines))
	for i := range g.lines {
		l := &g.lines[i]
		if filed[i] = !l.At.After(now) && !ended(l).After(now); !filed[i] {
			continue
		}
		switch date := dayfile.DateOf(l.At); {
		case date < today.Date:
			days[date] = append(days[date], ledger.Held{Line: *l})
			late[date] = late[date] || !g.filed[i]
		case again || !g.filed[i]:
			gained = append(gained, ledger.Held{Line: *l})
		}
	}
	maps.DeleteFunc(days, func(date string, _ []ledger.Held) bool { return !late[date] })
	g.filed = filed
	lines := []LogDay{g.read.Unheld(gained)}
	for _, date := range slices.Sorted(maps.Keys(days)) {
		lines = append(lines, g.read.Unheld(days[date]))
	}
	held, gains := g.held.Fed(filedLedger{lines: g.lines, now: now}, Took(gained, days), SessionsWanted{Log: g.read.Wanted(lines...)}, now)
	g.held = held
	t := LogTaking{Held: held, Gains: gains, Now: now}
	g.read = g.read.Taken(lines[0], again, t)
	t.Gains = nil
	for _, day := range lines[1:] {
		g.read = g.read.Taken(day, true, t)
	}
	src := LogSources{From: g.from, Read: g.read, Filter: g.filter, Now: now}
	g.tabled = LogLinesOf(g.tabled, src)
	if got, want := g.tabled.Rows(src), LogRows(src); !reflect.DeepEqual(shownLines(got), shownLines(want)) {
		g.t.Fatalf("run %d at %v: caught up, the lines tabled are\n%s\nwant them as tabled afresh\n%s\n%s", g.seed, now, linesSaid(got), linesSaid(want),
			unlike(shownLines(got), shownLines(want)))
	}
}

// readAtOnce is the Log's read of every line of the run filed by now, as the
// Log reads them opening at now: each day's whole, today's first, the store
// fed them.
func (g *logGrowing) readAtOnce(now time.Time) LogRead {
	byDay := make(map[string][]ledger.Held)
	for i := range g.lines {
		if l := &g.lines[i]; !l.At.After(now) && !ended(l).After(now) {
			byDay[dayfile.DateOf(l.At)] = append(byDay[dayfile.DateOf(l.At)], ledger.Held{Line: *l})
		}
	}
	today := Today(now).Date
	var gained []ledger.Held
	days := make(map[string][]ledger.Held)
	for date, lines := range byDay {
		if date == today {
			gained = lines
			continue
		}
		days[date] = lines
	}
	read := ReadLog(g.prices)
	held, _ := HoldSessions(g.prices).Fed(filedLedger{lines: g.lines, now: now}, Took(gained, days), SessionsWanted{Log: []string{"s"}}, now)
	t := LogTaking{Held: held, Now: now}
	read = read.Taken(RecordsOf(gained), true, t)
	for _, date := range slices.Backward(slices.Sorted(maps.Keys(days))) {
		read = read.Taken(RecordsOf(days[date]), true, t)
	}
	return read
}

// idless is lines with two of them, and a line given again beside one,
// without a request's id, as a ledger from before the router wrote ids
// holds them: two at the same moment, alike, as the ledger's files hold them
// twice.
func idless(r *rand.Rand, lines []ledger.Line) []ledger.Line {
	i := r.IntN(len(lines))
	lines[i].Request = ""
	j := r.IntN(len(lines))
	lines[j].Request = ""
	return slices.Insert(lines, j+1, lines[j])
}

// shownLines are what a table's lines show, in order: each row, rule and
// oldest day's line; and what each day's lines count.
func shownLines(table LogTable) []any {
	return shownOf(table, tableLinesOf(table))
}

// tiesSorted are what a table's lines show, as shownLines has them, those of
// rows that arrived at the same moment in the order of their requests' ids:
// a read grown places lines alike in the order it took them in, one read at
// once in the order a day's files give them.
func tiesSorted(table LogTable) []any {
	lines := tableLinesOf(table)
	for from := 0; from < len(lines); {
		to := from + 1
		for to < len(lines) && lines[from].IsRow() && lines[to].IsRow() && lines[to].Row.At.Equal(lines[from].Row.At) {
			to++
		}
		slices.SortStableFunc(lines[from:to], func(a, b LogLine) int { return strings.Compare(a.Row.Request, b.Row.Request) })
		from = to
	}
	return shownOf(table, lines)
}

// tableLinesOf are a table's lines, each as its row, rule and oldest day's line.
func tableLinesOf(table LogTable) []LogLine {
	var lines []LogLine
	for _, l := range table.All() {
		lines = append(lines, LogLine{Row: l.Row, Rule: l.Rule, Oldest: l.Oldest})
	}
	return lines
}

// shownOf are what lines of a table show, each followed, where it's a row,
// by what its day's lines count.
func shownOf(table LogTable, lines []LogLine) []any {
	shown := make([]any, 0, 2*len(lines))
	for _, l := range lines {
		shown = append(shown, l)
		if l.IsRow() {
			shown = append(shown, table.Count(Today(l.Row.At).Date))
		}
	}
	return shown
}

// unlike says the first of what two tables show that differs, for a failure
// to show.
func unlike(got, want []any) string {
	for i := range min(len(got), len(want)) {
		if !reflect.DeepEqual(got[i], want[i]) {
			return fmt.Sprintf("the first unlike is\n%+v\nwant\n%+v", got[i], want[i])
		}
	}
	return fmt.Sprintf("%d shown, want %d", len(got), len(want))
}

// linesSaid say a table's lines, for a failure to show.
func linesSaid(table LogTable) string {
	var said []string
	for _, l := range table.All() {
		switch {
		case l.Rule != nil:
			said = append(said, "── "+l.Rule.Ended.Date)
		case l.IsRow():
			said = append(said, l.Row.At.Format(time.StampMilli)+" "+l.Row.Request+" "+l.Row.Account+" "+l.Row.Model)
		default:
			said = append(said, "oldest")
		}
	}
	return strings.Join(said, "\n")
}

// TestTheLogGrownLookByLookIsTheLogReadAtOnceOnGeneratedRuns grows the Log's
// read look by look over generated runs, as the watch grows it, through a
// filter or none: of sessions whose requests end out of the order they came
// in, come to the millisecond together, move, run past midnight, and land
// behind their session's horizon; with lines without an id, two alike at
// the same moment; today's given again, and a day before read again whole.
// At each look the lines tabled as they're caught up are those tabled
// afresh; and at the end the table is the one read at once, what each day
// counts alike, rows that arrived together in either order. Prices hold
// across the run, as each line is priced at those of the day it's taken in
// on.
func TestTheLogGrownLookByLookIsTheLogReadAtOnceOnGeneratedRuns(t *testing.T) {
	prices := ledger.Pricing
	filters := []LogFilter{{}, LogOpening(), {Models: []string{foldOpus}}, {Accounts: []string{"work"}}}
	for seed := range uint64(120) {
		r := rand.New(rand.NewPCG(seed, 6))
		lines := generatedRun(seed)
		if seed%3 == 0 {
			lines = idless(r, lines)
		}
		g := &logGrowing{
			t: t, seed: seed, lines: lines, prices: prices, read: ReadLog(prices), held: HoldSessions(prices),
			filter: filters[seed%uint64(len(filters))], filed: make([]bool, len(lines)), from: lines[0].At.AddDate(0, 0, -1),
		}
		at := lines[0].At.Add(-time.Minute)
		g.look(at, true)
		for i := 0; i < len(lines); i += 1 + r.IntN(4) {
			at = later(instantOf(at), instantOf(ended(&lines[i]))).time().Add(time.Duration(r.IntN(5000)) * time.Millisecond)
			g.look(at, r.IntN(10) == 0)
		}
		at = ended(&lines[len(lines)-1]).Add(time.Minute)
		g.look(at, false)
		src := LogSources{From: g.from, Read: g.read, Filter: g.filter, Now: at}
		once := src
		once.Read = g.readAtOnce(at)
		if got, want := LogRows(src), LogRows(once); !reflect.DeepEqual(tiesSorted(got), tiesSorted(want)) {
			t.Fatalf("run %d: grown look by look, the table is\n%s\nwant it as read at once\n%s\n%s", seed, linesSaid(got), linesSaid(want), unlike(tiesSorted(got), tiesSorted(want)))
		}
	}
}

// A read's Unheld are the lines given, each its record, but those with ids
// it holds already, on the day each arrived: of an id it holds, one that
// arrived at another moment is another request's, kept, and so is every
// line without an id, which Taken knows by its record's sum. So a day read
// again whole decodes only what it gained, and the read takes in of it what
// it takes in of every line.
func TestUnheldLeavesOutOnlyTheLinesWithIDsTheReadHolds(t *testing.T) {
	at := time.Date(2026, 10, 7, 9, 0, 0, 0, time.Local)
	line := func(id string, seconds int) ledger.Held {
		return ledger.Held{At: at.Add(time.Duration(seconds) * time.Second), Request: id, Kind: ledger.KindMessage, Session: "s", Status: 200}
	}
	day := []ledger.Held{line("r1", 0), line("", 1), line("r2", 2)}
	taking := LogTaking{Held: HoldSessions(ledger.Pricing), Now: at.Add(time.Hour)}
	read := ReadLog(ledger.Pricing).Taken(RecordsOf(day), true, taking)
	again := append(slices.Clone(day), line("r3", 3), line("r1", 4), line("", 5))
	unheld := read.Unheld(again)
	var kept []string
	for _, r := range unheld.records {
		kept = append(kept, fmt.Sprintf("%q at %v", r.request, r.arrived().Sub(at)))
	}
	if want := []string{`"" at 1s`, `"r3" at 3s`, `"r1" at 4s`, `"" at 5s`}; !slices.Equal(kept, want) {
		t.Errorf("of the day read again, Unheld keeps %v, want %v", kept, want)
	}
	got, want := read.Taken(unheld, true, taking), read.Taken(RecordsOf(again), true, taking)
	if !reflect.DeepEqual(slices.Collect(got.lines()), slices.Collect(want.lines())) {
		t.Errorf("the read takes in of Unheld's lines\n%s\nwant what it takes in of every line\n%s", readSaid(got), readSaid(want))
	}
	if !read.Taken(read.Unheld(day), true, taking).Same(read) {
		t.Error("the read takes in a line of Unheld's of the day it holds whole, want none")
	}
}

// readSaid is what r holds, a line to each of its lines.
func readSaid(r LogRead) string {
	var said strings.Builder
	for l := range r.lines() {
		fmt.Fprintf(&said, "%d %q at %v\n", l.seq, l.flight.request, l.flight.arrived().Format(time.TimeOnly))
	}
	return said.String()
}

// A day taken into the Log's read whole copies a chunk of its lines for each
// chunkRows of them, as a day built whole has them, never one for each line;
// and a read of the tail gaining a few lines after copies one. Each line is
// held in the order taken in.
func TestADayTakenInWholeCopiesAChunkForEachChunkOfItsLines(t *testing.T) {
	at := time.Date(2026, 10, 7, 9, 0, 0, 0, time.Local)
	lines := make([]ledger.Held, 3000)
	for i := range lines {
		lines[i] = ledger.Held{At: at.Add(time.Duration(i) * time.Second), Request: fmt.Sprintf("r%05d", i), Kind: ledger.KindMessage, Session: "s", Status: 200}
	}
	taking := LogTaking{Held: HoldSessions(ledger.Pricing), Now: at.Add(time.Hour)}
	whole, tail := lines[:2990], lines[2990:]
	read := ReadLog(ledger.Pricing).Taken(RecordsOf(whole), true, taking)
	if copied, most := read.days[0].lines.copied, (len(whole)+chunkRows-1)/chunkRows; copied > most {
		t.Errorf("a day of %d lines taken in whole copied %d chunks, want %d at most, one for each %d lines", len(whole), copied, most, chunkRows)
	}
	grown := read.Taken(RecordsOf(tail), false, taking)
	if copied := grown.days[0].lines.copied - read.days[0].lines.copied; copied > 1 {
		t.Errorf("a read of the tail gaining %d lines copied %d chunks, want one at most", len(tail), copied)
	}
	var got, want []string
	for _, l := range grown.days[0].lines.all() {
		got = append(got, l.flight.request)
	}
	for _, h := range lines {
		want = append(want, h.Request)
	}
	if !slices.Equal(got, want) {
		t.Errorf("the read holds %d lines, want the %d taken in, in order", len(got), len(want))
	}
}

// Lines taken into the Log's read either side of a collection share one
// copy of each word they name alike, as interned holds it: a session's id
// and directory, its model, account and reason, and its answer's model.
func TestLinesTakenInEitherSideOfACollectionShareTheirWords(t *testing.T) {
	at := time.Date(2026, 10, 7, 9, 0, 0, 0, time.Local)
	lineOf := func(id string, at time.Time) ledger.Held {
		l := ledger.Line{At: at, Request: id, Kind: ledger.KindMessage, Session: fmt.Sprint("s-", 7), Dir: fmt.Sprint("/code/", "x"), Model: fmt.Sprint("model-", 7),
			Account: fmt.Sprint("acct-", 7), Reason: fmt.Sprint("reason-", 7), Status: 200}
		l.Answer.Model = fmt.Sprint("served-", 7)
		return ledger.Held{Line: l}
	}
	taking := LogTaking{Held: HoldSessions(ledger.Pricing), Now: at.Add(time.Hour)}
	read := ReadLog(ledger.Pricing).Taken(RecordsOf([]ledger.Held{lineOf("r1", at)}), false, taking)
	runtime.GC()
	runtime.GC()
	read = read.Taken(RecordsOf([]ledger.Held{lineOf("r2", at.Add(time.Second))}), false, taking)
	lines := slices.Collect(read.lines())
	if len(lines) != 2 {
		t.Fatalf("the read holds %d lines, want 2", len(lines))
	}
	a, b := lines[0], lines[1]
	words := map[string][2]string{
		"session": {a.row.Session, b.row.Session}, "directory": {a.row.Dir, b.row.Dir}, "model": {a.row.Model, b.row.Model},
		"account": {a.row.Account, b.row.Account}, "reason": {a.routed.reason, b.routed.reason}, "served": {a.shown.served, b.shown.served},
	}
	for name, w := range words {
		if unsafe.StringData(w[0]) != unsafe.StringData(w[1]) {
			t.Errorf("the lines' %s, %q, are two copies, want one shared", name, w[0])
		}
	}
}
