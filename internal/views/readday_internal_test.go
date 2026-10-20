package views

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// rowsNoted is a ledger as a day back reads it, noting the dates of the days
// whose rows DayRows gave, standing, and of those whose lines Day read.
type rowsNoted struct {
	Ledger
	rows, raw map[string]bool
}

// readingRows is l read as rowsNoted notes it, nothing read yet.
func readingRows(l Ledger) *rowsNoted {
	return &rowsNoted{Ledger: l, rows: make(map[string]bool), raw: make(map[string]bool)}
}

func (r *rowsNoted) DayRows(date string, body int) (ledger.DayRows, bool) {
	rows, ok := r.Ledger.DayRows(date, body)
	if ok {
		r.rows[date] = true
	}
	return rows, ok
}

func (r *rowsNoted) Day(date string) []ledger.Held {
	r.raw[date] = true
	return r.Ledger.Day(date)
}

// fromRows reports whether the day with the given date was read from its
// rows alone: they stood, and its lines were never read.
func (r *rowsNoted) fromRows(date string) bool {
	return r.rows[date] && !r.raw[date]
}

// rawDay is the LogDay of the local day with the given date read raw,
// through l, as ReadDay reads a day whose rows don't stand: of its lines, as
// Day gives them, holding the day's date.
func rawDay(l Ledger, date string) LogDay {
	day := RecordsOf(l.Day(date))
	day.date = date
	return day
}

// wholeDay is the LogDay of the local day with the given date read back
// whole through l, as ReadDay reads it through the zero LogFilter.
func wholeDay(l Ledger, date string) LogDay {
	return ReadDay(l, date, LogFilter{})
}

// logBack is what the Log's Requests show at now, read through l as the
// watch reads them: today's lines as it opens, then each day before, newest
// first, back to the local day with the given date, as read reads it, each
// taken in as TakenBack takes a day back, the store it reads fresh taken
// into the one held, as the watch takes it in. What it shows is the table's
// lines, each row with what its day's lines count, as shownLines has them,
// each day's rule with its figures, as TotalsOn gives them; every row's
// details; what each day's lines count; and the days its rows await a line
// on.
func logBack(l Ledger, read func(Ledger, string) LogDay, from string, s Stream, prices ledger.Table, now time.Time) []any {
	today := Today(now)
	days, dates := []LogDay{rawDay(l, today.Date)}, []string{today.Date}
	for day := dayfile.DayStart(now, -1); dayfile.DateOf(day) >= from; day = dayfile.DayStart(day, -1) {
		days, dates = append(days, read(l, dayfile.DateOf(day))), append(dates, dayfile.DateOf(day))
	}
	logRead, held := ReadLog(prices), HoldSessions(prices)
	for _, day := range days {
		var fresh *HeldSessions
		if logRead, fresh = logRead.TakenBack(l, day, held, s, now); fresh != nil {
			held = held.With(*fresh)
		}
	}
	slices.Reverse(dates)
	start, _, _ := dayfile.Day(from)
	table := LogRows(LogSources{From: start, Read: logRead, Stream: s, Totals: TotalsOn(l, dates, now, prices), Now: now})
	shown := shownLines(table)
	for _, line := range table.All() {
		d, _ := line.Details()
		shown = append(shown, d)
	}
	for _, date := range dates {
		shown = append(shown, table.Count(date))
	}
	return append(shown, table.Awaited(today))
}

// toldBack is a stream telling of the last request with an id that arrived
// on each of the local days with the given dates, as lines have them, ended,
// so its line is held as its row; and of one sent a second before the first
// of them ended, ended, its line yet to come, so the Log's rows await it on
// that day.
func toldBack(lines []ledger.Line, dates []string) Stream {
	s := Stream{Requests: make(map[string]Told)}
	for _, date := range dates {
		var last *ledger.Line
		for i := range lines {
			if l := &lines[i]; l.Request != "" && isRequest(l) && dayfile.DateOf(l.At) == date && (last == nil || !l.At.Before(last.At)) {
				last = l
			}
		}
		if last != nil {
			s.Requests[last.Request] = toldOf(last)
		}
	}
	_, end, _ := dayfile.Day(dates[0])
	sent := end.Add(-time.Second)
	s.Requests["awaited"] = Told{
		Request: "awaited", Session: "awaited", Model: "claude-opus-4-1", Account: "work", Class: ledger.ClassMain, Sent: sent, Ended: sent.Add(time.Millisecond),
		Status: 200, Heard: sent.Add(time.Millisecond),
	}
	return s
}

// toldOf is what the stream tells of l's request, ended, its line held: sent
// as it arrived, and heard as it ended.
func toldOf(l *ledger.Line) Told {
	return Told{
		Request: l.Request, Session: l.Session, Model: l.Model, Account: l.Account, Class: l.Class, Sent: l.At, Ended: ended(l), Status: l.Status,
		Heard: ended(l),
	}
}

// follower is a Follower of the chain's ledger, by its clock, resting on
// its digests.
func (c *digestChain) follower() *ledger.Follower {
	return ledger.NewFollower(c.state, func() time.Time { return c.at }, ledger.CapsOf(nil, nil), DigestVersion, quiet)
}

// sameDay fails the test where the day with the given date, read back
// through l as ReadDay reads it, at the chain's clock, is read from its rows
// alone otherwise than rows says, its lines read where it isn't; or where
// its records are otherwise than read raw through f, as Day gives its lines.
// It reports whether it was read from its rows alone.
func (c *digestChain) sameDay(l Ledger, f *ledger.Follower, date string, rows bool) bool {
	c.t.Helper()
	noted := readingRows(l)
	if got, want := wholeDay(noted, date), rawDay(f, date); !reflect.DeepEqual(got, want) {
		c.t.Errorf("%s at %v: %s reads back as\n%+v\nwant, as its lines read raw give\n%+v", c.name, c.at, date, got.records, want.records)
	}
	if noted.fromRows(date) != rows || !rows && !noted.raw[date] {
		c.t.Errorf("%s at %v: %s read back from its rows %t, its rows standing %t, its lines read %t; want it read from its rows alone %t",
			c.name, c.at, date, noted.fromRows(date), noted.rows[date], noted.raw[date], rows)
	}
	return noted.fromRows(date)
}

// sameDayBack fails the test as sameDay does of the day with the given
// date; and where what the Log read back to it through l shows, as logBack
// has it, of a stream telling of the day's requests, as toldBack has it, is
// otherwise than read back through f over the days read raw.
func (c *digestChain) sameDayBack(l Ledger, f *ledger.Follower, date string, rows bool) {
	c.t.Helper()
	c.sameDay(l, f, date, rows)
	c.sameLogBack(l, f, date, toldBack(c.lines, []string{date}))
}

// sameLogBack fails the test where what the Log read back through l, at the
// chain's clock, to the day with the given date, each day as ReadDay reads
// it, shows, as logBack has it, of what the stream s tells, is otherwise
// than read back through f over the days read raw; or where its rows await
// no line on that day, as the stream's request whose line is yet to come,
// as toldBack tells it, has them.
func (c *digestChain) sameLogBack(l Ledger, f *ledger.Follower, from string, s Stream) {
	c.t.Helper()
	got, want := logBack(l, wholeDay, from, s, changingPrices(), c.at), logBack(f, rawDay, from, s, changingPrices(), c.at)
	if !reflect.DeepEqual(got, want) {
		c.t.Errorf("%s at %v: the Log read back to %s over rows shows otherwise than over the days read raw\n%s", c.name, c.at, from, unlike(got, want))
	}
	if awaited := got[len(got)-1].([]string); !slices.Contains(awaited, from) {
		c.t.Errorf("%s at %v: the Log read back to %s awaits lines on %v, want %s among them", c.name, c.at, from, awaited, from)
	}
}

// readBackSeeds is how many seeds of each of readBackWorlds the Log is read
// back over: fewer than the chained digests', whose rows' tests hold each
// day's rows to its lines over every one, as the Log is built of the day's
// records alone. Among them, the busy days' of 1 and 2 run to more than one
// chunk a day.
const readBackSeeds = 3

// readBackWorlds are the worlds the Log is read back over: the rows', and a
// generated session's run again three days on, so the Log reads back across
// a day of no lines, which has no head.
var readBackWorlds = append(slices.Clip(rowsWorlds), struct {
	name  string
	lines func(seed uint64) []ledger.Line
}{name: "three days apart", lines: threeDaysApart})

// threeDaysApart is a generated session's lines, as generatedSession makes
// them, and again three days on, as movedOn moves them, each request of its
// own, in the order the ledger files them: a session runs for less than a
// day, so a day or more between the two holds no lines.
func threeDaysApart(seed uint64) []ledger.Line {
	lines := generatedSession(seed)
	for _, l := range slices.Clone(lines) {
		l = movedOn(l, 3)
		if l.Request != "" {
			l.Request += "-again"
		}
		lines = append(lines, l)
	}
	return lines
}

// The Log read back over a real ledger's days, each past day read as
// ReadDay reads it, shows what it shows read back over the same days raw,
// as Day gives their lines: its table, each day's rule with its figures and
// what its lines count, every row's details, and the days its rows await a
// line on, of a stream telling of a request of each day, its line held, and
// one whose line is yet to come. The ledger digested day by day, the Log is
// read back half an hour past the midnight after the last day the worlds
// file lines on, that day read from its lines, its head yet to be built;
// and at half past one, read from its rows; every day before it from its
// rows, never its lines, the busy days' in more than one chunk, but a day
// of no lines, which has no head, from its lines, as none; each day's
// records those of its lines read raw. Today, the last day as it runs, is
// read from its lines.
func TestTheLogReadBackOverRowsIsTheLogReadRaw(t *testing.T) {
	var lastRaw, lastRows, cut, lineless int
	for _, w := range readBackWorlds {
		for seed := range uint64(readBackSeeds) {
			c := newDigestChain(t, fmt.Sprintf("%s %d", w.name, seed), seed, w.lines(seed))
			f, filed := c.follower(), filedDates(c.lines)
			last := filed[len(filed)-1]
			for _, day := range c.days() {
				after := yesterdayOf(day) == last
				if after {
					c.by(day.Start.Add(30 * time.Minute))
					c.sameLogBack(f, f, filed[0], toldBack(c.lines, filed))
					if !c.sameDay(f, f, last, false) {
						lastRaw++
					}
				}
				c.by(day.Start.Add(90 * time.Minute))
				if day.Date == last {
					c.sameDay(f, f, last, false)
				}
				if !after {
					continue
				}
				c.sameLogBack(f, f, filed[0], toldBack(c.lines, filed))
				first, _, _ := dayfile.Day(filed[0])
				for _, date := range dayfile.Span(first, dayfile.DayStart(day.Start, -1)) {
					lined := slices.Contains(filed, date)
					switch rows := c.sameDay(f, f, date, lined); {
					case !lined:
						lineless++
					case rows:
						read, _ := f.DayRows(date, RowsVersion)
						cut += len(read.Chunks) - 1
						if date == last {
							lastRows++
						}
					}
				}
			}
		}
	}
	if lastRaw == 0 || lastRows == 0 || cut == 0 || lineless == 0 {
		t.Errorf("the last day read back from its lines past midnight %d times and from its rows at half past one %d times, rows cut into another chunk %d times, "+
			"a day of no lines read back %d times; want some of each", lastRaw, lastRows, cut, lineless)
	}
}

// busySeed is the seed of the busy days' world a day back is read of, as
// each kind of day upsets it: its busiest day's rows run to two chunks.
const busySeed = 6

// A day back reads the day's rows where they stand and every chunk of them
// restores as many rows as its head counts it, else its lines, as Day gives
// them, never part of a day from its rows: of the busy days' busiest, its
// rows in more than one chunk, read at the chain's end, its head built, and
// upset as each kind of day says. Either way the day's records are those of
// its lines read raw, and the Log read back to it shows what it shows over
// the days read raw, as sameDayBack has it.
func TestADayBackReadsItsRowsOnlyWhereTheyStandWhole(t *testing.T) {
	tests := []struct {
		name  string
		upset func(c *digestChain, date string)
		read  func(l Ledger) Ledger
		rows  bool
	}{
		{name: "its rows standing", rows: true},
		{name: "a late line filed onto it after its head was built", upset: (*digestChain).fileLateOnto},
		{name: "a line of it filed beside it, under the day after, whose digest tells of it", upset: (*digestChain).fileBesideAfter},
		{name: "its head of another version", upset: func(c *digestChain, date string) { c.headOfOtherVersion(date) }},
		{name: "its head built in another time zone", upset: func(c *digestChain, date string) { c.headOfOtherZone(date) }},
		{name: "a chunk after its first damaged, its rows file as long", upset: (*digestChain).damageAChunk},
		{name: "its rows file gone, its head standing", upset: (*digestChain).removeRows},
		{name: "each chunk after its first restoring a row fewer than its head counts", read: func(l Ledger) Ledger { return rowCut{l} }},
		{name: "its first chunk restoring a row more than its head counts, the next one fewer", read: func(l Ledger) Ledger { return recounted{l} }},
	}
	built := newDigestChain(t, "busy days", busySeed, busyDays(busySeed))
	built.digestDayByDay(func(LedgerDay) {})
	date := built.busiest(built.follower())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := built.cloned(t)
			f := c.follower()
			if tt.upset != nil {
				tt.upset(c, date)
			}
			var l Ledger = f
			if tt.read != nil {
				l = tt.read(f)
			}
			c.sameDayBack(l, f, date, tt.rows)
		})
	}
}

// busiest is the date of the day f gives the most chunks of rows of, of
// those the chain's lines are filed on, failing the test where none runs to
// more than one.
func (c *digestChain) busiest(f *ledger.Follower) string {
	c.t.Helper()
	var busiest string
	most := 0
	for _, date := range filedDates(c.lines) {
		if rows, ok := f.DayRows(date, RowsVersion); ok && len(rows.Chunks) > most {
			busiest, most = date, len(rows.Chunks)
		}
	}
	if most < 2 {
		c.t.Fatalf("%s: no day's rows run to more than one chunk at %v", c.name, c.at)
	}
	return busiest
}

// fileLateOnto files a late line onto the local day with the given date, its
// head built: a request of its own, arriving as the day's first request
// did, a copy of it otherwise.
func (c *digestChain) fileLateOnto(date string) {
	l := c.firstRequestOn(date)
	l.Request = "late-" + date
	c.file(date, l)
}

// fileBesideAfter files a line that arrived on the local day with the given
// date under the day after, as a change of time zone files one: a request
// of its own, arriving as the day's first request did, a copy of it
// otherwise; then has the router's round summarise and digest the day after
// again, as its digest then tells of the line, so the day's head, built
// again, isn't alone.
func (c *digestChain) fileBesideAfter(date string) {
	l := c.firstRequestOn(date)
	l.Request = "beside-" + date
	start, _, _ := dayfile.Day(date)
	c.file(dayfile.DateOf(dayfile.DayStart(start, 1)), l)
	c.by(c.at)
}

// firstRequestOn is the first request with an id the chain's lines give
// that arrived on the local day with the given date, failing the test
// where none did.
func (c *digestChain) firstRequestOn(date string) ledger.Line {
	c.t.Helper()
	i := slices.IndexFunc(c.lines, func(l ledger.Line) bool { return l.Request != "" && isRequest(&l) && dayfile.DateOf(l.At) == date })
	if i < 0 {
		c.t.Fatalf("%s: no request arrived on %s", c.name, date)
	}
	return c.lines[i]
}

// rowsPath is where the ledger keeps the rows of the local day with the
// given date, as it names them.
func (c *digestChain) rowsPath(date string) string {
	return filepath.Join(ledger.Dir(c.state), "rows-"+date+".gz")
}

// damageAChunk damages the second chunk of the rows of the local day with
// the given date, as its head points at it, as damageChunk damages one.
func (c *digestChain) damageAChunk(date string) {
	c.t.Helper()
	rows, ok := c.follower().DayRows(date, RowsVersion)
	if !ok || len(rows.Chunks) < 2 {
		c.t.Fatalf("%s: the rows of %s don't stand in more than one chunk", c.name, date)
	}
	c.damageChunk(date, rows.Chunks[1])
}

// removeRows removes the rows file of the local day with the given date,
// its head left as it is.
func (c *digestChain) removeRows(date string) {
	if err := os.Remove(c.rowsPath(date)); err != nil {
		c.t.Fatal(err)
	}
}

// rowCut is a ledger whose rows read a row fewer, the last, of each chunk
// after a day's first than its head counts.
type rowCut struct {
	Ledger
}

func (r rowCut) RowChunk(date string, chunk ledger.RowsChunk) ([]byte, error) {
	rows, err := r.Ledger.RowChunk(date, chunk)
	if err != nil || chunk.Offset == 0 {
		return rows, err
	}
	return rows[:bytes.LastIndexByte(rows[:len(rows)-1], '\n')+1], nil
}

// recounted is a ledger whose day's rows count a row more in their first
// chunk, and one fewer in their second, than they hold: as many in all.
type recounted struct {
	Ledger
}

func (r recounted) DayRows(date string, body int) (ledger.DayRows, bool) {
	rows, ok := r.Ledger.DayRows(date, body)
	if ok && len(rows.Chunks) > 1 {
		rows.Chunks = slices.Clone(rows.Chunks)
		rows.Chunks[0].Count++
		rows.Chunks[1].Count--
	}
	return rows, ok
}

// A day of no lines, its file holding only a line that arrived on the day
// before, as a change of time zone files one, reads back from its rows,
// which stand, holding none: the same day as of its lines read raw, as Day
// gives none, its records none, not nil.
func TestADayOfNoLinesReadsBackFromItsRowsAsOfItsLines(t *testing.T) {
	before := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	lines := []ledger.Line{
		{At: before, Request: "r1", Session: "s", Kind: ledger.KindMessage, Model: "claude-opus-4-1", Account: "work", Status: 200},
		{At: before.Add(48 * time.Hour), Request: "r3", Session: "s", Kind: ledger.KindMessage, Model: "claude-opus-4-1", Account: "work", Status: 200},
	}
	c := newDigestChain(t, "a day of no lines", 0, lines)
	beside := lines[0]
	beside.At, beside.Request = dayfile.DayStart(before, 1).Add(-30*time.Minute), "r2"
	empty := dayfile.DateOf(dayfile.DayStart(before, 1))
	c.file(empty, beside)
	c.lines = append(c.lines, beside)
	c.digestDayByDay(func(LedgerDay) {})
	f := c.follower()
	if rows, ok := f.DayRows(empty, RowsVersion); !ok || rows.Count != 0 {
		t.Fatalf("the rows of %s are %+v, standing %t; want them standing, of no rows", empty, rows, ok)
	}
	if day := wholeDay(f, empty); day.records == nil {
		t.Errorf("%s reads back as no records, nil; want none, not nil, as RecordsOf gives of no lines", empty)
	}
	c.sameDayBack(f, f, empty, true)
}

// readsCounted is a ledger as a day read back reads it, counting its reads,
// as reads has them.
type readsCounted struct {
	Ledger
	reads
}

// reads count a day's reads: of the days' summaries, of its rows, of their
// chunks, and of its lines.
type reads struct {
	summaries, rows, chunks, raw int
}

func (r *readsCounted) DaysOn(dates []string) []ledger.Summary {
	r.summaries++
	return r.Ledger.DaysOn(dates)
}

func (r *readsCounted) DayRows(date string, body int) (ledger.DayRows, bool) {
	r.rows++
	return r.Ledger.DayRows(date, body)
}

func (r *readsCounted) RowChunk(date string, chunk ledger.RowsChunk) ([]byte, error) {
	r.chunks++
	return r.Ledger.RowChunk(date, chunk)
}

func (r *readsCounted) Day(date string) []ledger.Held {
	r.raw++
	return r.Ledger.Day(date)
}

// keptBy are those of records f keeps, as chose keeps their rows, in order.
func keptBy(records []lineRecord, f LogFilter) []lineRecord {
	var kept []lineRecord
	for i := range records {
		row := lineFacts(&records[i].rowRecord, nil)
		if f.chose(&row) {
			kept = append(kept, records[i])
		}
	}
	return kept
}

// among reports whether records are some of all, in all's order.
func among(records, all []lineRecord) bool {
	for i := range all {
		if len(records) > 0 && reflect.DeepEqual(records[0], all[i]) {
			records = records[1:]
		}
	}
	return len(records) == 0
}

// throughWorlds are the worlds a day is read back through filters over: the
// busy days', whose rows run to more than one chunk a day, their sessions
// beginning hours apart; spread worlds, their sessions on days of their own,
// a zone change's lines filed under days beside those they arrived on, as
// newSpread files them; and a session run again three days on, so the days
// between hold no lines.
var throughWorlds = []struct {
	name  string
	chain func(t *testing.T) *digestChain
}{
	{name: "busy days", chain: func(t *testing.T) *digestChain {
		return newDigestChain(t, fmt.Sprintf("busy days %d", busySeed), busySeed, busyDays(busySeed))
	}},
	{name: "spread world 8", chain: func(t *testing.T) *digestChain { return spreadChain(t, 8) }},
	{name: "spread world 12", chain: func(t *testing.T) *digestChain { return spreadChain(t, 12) }},
	{name: "three days apart", chain: func(t *testing.T) *digestChain { return newDigestChain(t, "three days apart 0", 0, threeDaysApart(0)) }},
}

// spreadChain is the chain of the spread world of the seed given, as
// newSpread writes it.
func spreadChain(t *testing.T, seed uint64) *digestChain {
	t.Helper()
	return newSpread(t, seed, pricedAs{name: "the built-in table", built: ledger.Pricing, read: ledger.Pricing}, make(map[string]bool)).digestChain
}

// The moments a day read back through a filter meets, each met by some of
// throughWorlds.
const (
	throughRuledOut = "a day its summaries and those beside it rule out, unread"
	throughSkipped  = "a day whose rows stand, read of some of their chunks, others skipped"
	throughRaw      = "a day whose rows don't stand, its lines read"
)

// throughFilters is how many filters each day is read back through at each
// moment, as drawnFilter draws them.
const throughFilters = 6

// A past day read back through a filter, as ReadDay reads it, gives every
// record of the day's lines read raw that the filter keeps, as chose keeps
// its row, in order, among others of them alone, in their order; and holds
// the filter with every kind as its through. Of throughWorlds, through
// throughFilters filters a day, as drawnFilter draws them, half an hour past
// the midnight after the last day lines arrived on, that day's head yet to
// be built, and at half past one, its head built: of days ruled out by their
// summaries and those beside them, unread; of days whose rows stand, read
// of the chunks whose meta may keep a line alone, some skipped; and of days
// read raw. Through the Log's opening filter, which keeps every line of its
// kinds, it's the day read whole, reading no summary.
func TestADayReadBackThroughAFilterGivesEveryRecordItKeeps(t *testing.T) {
	met := make(map[string]bool)
	for _, w := range throughWorlds {
		c := w.chain(t)
		f, filed := c.follower(), filedDates(c.lines)
		last := filed[len(filed)-1]
		r := rand.New(rand.NewPCG(c.seed, 0x7d))
		for _, day := range c.days() {
			after := yesterdayOf(day) == last
			if after {
				c.by(day.Start.Add(30 * time.Minute))
				c.readBackThrough(f, r, met)
			}
			c.by(day.Start.Add(90 * time.Minute))
			if after {
				c.readBackThrough(f, r, met)
				break
			}
		}
	}
	for _, moment := range []string{throughRuledOut, throughSkipped, throughRaw} {
		if !met[moment] {
			t.Errorf("no world meets %s", moment)
		}
	}
}

// readBackThrough reads each past day of the chain's lines back through f, at
// the chain's clock, through throughFilters filters r draws of the ids its
// lines give, as sameThrough has each; and through the Log's opening
// filter, failing the test where that's otherwise than the day read whole,
// or reads a summary. It notes in met the moments the reads meet.
func (c *digestChain) readBackThrough(f *ledger.Follower, r *rand.Rand, met map[string]bool) {
	c.t.Helper()
	first, _, _ := dayfile.Day(filedDates(c.lines)[0])
	ids := idsOf(c.lines)
	for _, date := range dayfile.Span(first, dayfile.DayStart(c.at, -1)) {
		raw := rawDay(f, date)
		for range throughFilters {
			c.sameThrough(f, date, drawnFilter(r, ids), raw, met)
		}
		counted := &readsCounted{Ledger: f}
		if got := ReadDay(counted, date, LogOpening()); !reflect.DeepEqual(got, wholeDay(f, date)) || counted.summaries > 0 {
			c.t.Errorf("%s at %v: %s read back through the Log's opening filter, reading %+v, as\n%+v\nwant it read whole, reading no summary",
				c.name, c.at, date, counted.reads, got.records)
		}
	}
}

// sameThrough fails the test where the day with the given date, read back
// through f as ReadDay reads it through the filter given, at the chain's
// clock, gives otherwise than raw, its lines read raw, of the records the
// filter keeps; gives a record raw doesn't, or out of raw's order; or holds
// other than the filter with every kind as its through. It notes in met the
// moments the read meets.
func (c *digestChain) sameThrough(f *ledger.Follower, date string, filter LogFilter, raw LogDay, met map[string]bool) {
	c.t.Helper()
	counted := &readsCounted{Ledger: f}
	got := ReadDay(counted, date, filter)
	if kept, want := keptBy(got.records, filter), keptBy(raw.records, filter); !reflect.DeepEqual(kept, want) {
		c.t.Errorf("%s at %v: %s read back through %+v, reading %+v, keeps\n%s", c.name, c.at, date, filter, counted.reads, recordsUnlike(kept, want))
	}
	if !among(got.records, raw.records) {
		c.t.Errorf("%s at %v: %s read back through %+v, reading %+v, gives %d records not all of its %d read raw, in their order",
			c.name, c.at, date, filter, counted.reads, len(got.records), len(raw.records))
	}
	if !reflect.DeepEqual(got.through, filter.through()) {
		c.t.Errorf("%s at %v: %s read back through %+v holds %+v as its through, want %+v", c.name, c.at, date, filter, got.through, filter.through())
	}
	rows, standing := f.DayRows(date, RowsVersion)
	switch {
	case filter.whole():
	case counted.rows == 0 && counted.raw == 0:
		met[throughRuledOut] = true
	case counted.raw > 0:
		met[throughRaw] = true
	case standing && counted.chunks > 0 && counted.chunks < len(rows.Chunks):
		met[throughSkipped] = true
	}
}

// A day read back through a filter its own summary rules out, but not that
// of a day beside it, as Day reads a day's lines of those days' files too,
// is read of its rows, of none of their chunks, as no meta meets the
// filter, nor of its lines: of a session's, the day after the last it ran
// on. The day after that, the summaries of it and the days beside it ruling
// it out, is never read. And as no summary counts a day's failures, the
// failed alone rule out no day of lines, whose rows are read of the chunks
// counting a failure alone: none, of a day none failed on.
func TestADayBesideOneAFilterKeepsALineOnIsReadOfItsRowsAlone(t *testing.T) {
	at := func(day, hour int) time.Time { return time.Date(2026, 10, 7+day, hour, 0, 0, 0, time.Local) }
	line := func(request, session string, at time.Time, status int) ledger.Line {
		return ledger.Line{
			At: at, Request: request, Kind: ledger.KindMessage, Session: session, Model: foldOpus, Account: "work", Reason: "sticky", Status: status,
			Attempts: 1, TotalMS: 1000, Class: ledger.ClassMain,
		}
	}
	c := newDigestChain(t, "a session beside another's", 0, []ledger.Line{
		line("a1", "a", at(0, 10), 200), line("b1", "b", at(0, 11), 500),
		line("b2", "b", at(1, 11), 200), line("b3", "b", at(2, 11), 200), line("b4", "b", at(3, 11), 200),
	})
	c.digestDayByDay(func(LedgerDay) {})
	f := c.follower()
	ofA, failed := LogFilter{Sessions: []string{"a"}}, LogFilter{Failed: true}
	tests := []struct {
		name  string
		f     LogFilter
		day   int
		reads reads
		kept  int
	}{
		{"a session's, of the day it ran on", ofA, 0, reads{summaries: 1, rows: 1, chunks: 1}, 1},
		{"a session's, of the day after the last it ran on", ofA, 1, reads{summaries: 1, rows: 1}, 0},
		{"a session's, of two days after the last it ran on", ofA, 2, reads{summaries: 1}, 0},
		{"the failed alone, of a day one failed on", failed, 0, reads{summaries: 1, rows: 1, chunks: 1}, 1},
		{"the failed alone, of a day none failed on", failed, 2, reads{summaries: 1, rows: 1}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			date := dayfile.DateOf(at(tt.day, 12))
			counted := &readsCounted{Ledger: f}
			got := ReadDay(counted, date, tt.f)
			if counted.reads != tt.reads {
				t.Errorf("%s read back through %+v reads %+v, want %+v", date, tt.f, counted.reads, tt.reads)
			}
			kept, want := keptBy(got.records, tt.f), keptBy(rawDay(f, date).records, tt.f)
			if len(kept) != tt.kept || !reflect.DeepEqual(kept, want) {
				t.Errorf("%s read back through %+v keeps\n%+v\nwant, of its lines read raw, %d\n%+v", date, tt.f, kept, tt.kept, want)
			}
		})
	}
}
