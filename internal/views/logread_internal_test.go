package views

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

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
// sessions of the Log's lines, then the read takes them in, the feed's gains
// told once, as the watch's tail read takes them. The lines tabled are
// caught up with the read grown, and held to them tabled afresh.
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
	lines := [][]ledger.Held{gained}
	for _, date := range slices.Sorted(maps.Keys(days)) {
		lines = append(lines, days[date])
	}
	held, gains := g.held.Fed(filedLedger{lines: g.lines, now: now}, Took(gained, days), SessionsWanted{Log: g.read.Wanted(lines...)}, now)
	g.held = held
	t := LogTaking{Held: held, Gains: gains, Now: now}
	g.read = g.read.Taken(gained, again, t)
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
	read = read.Taken(gained, true, t)
	for _, date := range slices.Backward(slices.Sorted(maps.Keys(days))) {
		read = read.Taken(days[date], true, t)
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
