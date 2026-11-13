package views

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// The pools these tests' lines are routed in, by their ids: Acme, and the
// first pool, where a line naming none is.
const (
	acmePool  = "q7mt"
	firstPool = "b2n8"
)

// poolReads are the reads of a day these tests hold to its lines read raw:
// Acme's and the first pool's, once the move to pools is done; every
// pool's; and none's.
var poolReads = []struct {
	name string
	read PoolRead
}{
	{name: "Acme's", read: PoolRead{ID: acmePool, First: firstPool, PoolOf: placedNowhere}},
	{name: "the first pool's", read: PoolRead{ID: firstPool, First: firstPool, PoolOf: placedNowhere}},
	{name: "every pool's", read: PoolRead{}},
	{name: "no pool's", read: NoPool()},
}

// placedNowhere places no account in any pool, as of accounts removed before
// pools.
func placedNowhere(string) (string, bool) {
	return "", false
}

// pooledLine is a request's line, of the session with the given id, routed
// in the pool with the given id, "" for none, arriving at at.
func pooledLine(request, session, pool string, at time.Time) ledger.Line {
	return ledger.Line{
		At: at, Request: request, Kind: ledger.KindMessage, Session: session, Model: foldOpus, Account: "work", Reason: "sticky", Status: 200,
		Attempts: 1, TotalMS: 100, Class: ledger.ClassMain, Pool: pool,
	}
}

// rawIn is the LogDay of the local day with the given date read raw, through
// l, in the pool read p: of its lines in the pool, as Day gives them and p's
// Held keeps them.
func rawIn(l Ledger, date string, p PoolRead) LogDay {
	return RecordsOf(p.Held(l.Day(date)))
}

// pooledBusyDays are busyDays' lines, each routed in the pool of its
// session: s0 to s3 in Acme, s4 to s7 in the first pool, and the rest naming
// none, as before pools, so the first pool's too; but on s3's last day, the
// lines counting toward it alternate between Acme and none, as of a session
// resumed in another pool.
func pooledBusyDays(seed uint64) []ledger.Line {
	lines := busyDays(seed)
	last := ""
	for _, l := range lines {
		if l.Session == "s3" {
			last = max(last, dayfile.DateOf(l.At))
		}
	}
	resumed := 0
	for i := range lines {
		l := &lines[i]
		var k int
		if _, err := fmt.Sscanf(l.Session, "s%d", &k); err != nil {
			panic(err)
		}
		switch {
		case k < 4:
			l.Pool = acmePool
		case k < 8:
			l.Pool = firstPool
		}
		if l.Session == "s3" && dayfile.DateOf(l.At) == last && l.CountedSession() != "" {
			if resumed%2 == 1 {
				l.Pool = ""
			}
			resumed++
		}
	}
	return lines
}

// Each past day of busy days split between pools, read back in a pool as
// ReadDay reads it, its head built, gives the records of the day's lines in
// the pool read raw, as Held keeps them, in order, its rows read and its
// lines never: whole, Acme's its own alone, the first pool's those naming it
// and those naming none, every pool's every one, and none's none. Read back
// through a filter, it keeps every record of those the filter keeps, in
// order, among others of the pool alone.
func TestAPoolsDayBackIsItsLinesInThePoolReadRaw(t *testing.T) {
	t.Parallel()
	c := newDigestChain(t, fmt.Sprintf("busy days in pools %d", busySeed), busySeed, pooledBusyDays(busySeed))
	c.digestDayByDay(func(LedgerDay) {})
	f, ids := c.follower(), idsOf(c.lines)
	r := rand.New(rand.NewPCG(busySeed, 0x9c))
	for _, date := range filedDates(c.lines) {
		for _, p := range poolReads {
			raw := rawIn(f, date, p.read)
			l := readingRows(f)
			if got := ReadDay(l, date, LogFilter{}, p.read); !reflect.DeepEqual(got.records, raw.records) || !l.fromRows(date) {
				t.Errorf("%s, %s whole, from its rows alone %t:\n%s", date, p.name, l.fromRows(date), recordsUnlike(got.records, raw.records))
			}
			for range throughFilters {
				filter := drawnFilter(r, ids)
				got := ReadDay(f, date, filter, p.read)
				if kept, want := keptBy(got.records, filter), keptBy(raw.records, filter); !reflect.DeepEqual(kept, want) {
					t.Errorf("%s, %s through %+v keeps\n%s", date, p.name, filter, recordsUnlike(kept, want))
				}
				if !among(got.records, raw.records) {
					t.Errorf("%s, %s through %+v gives %d records not all of the %d of its lines in the pool, in their order", date, p.name, filter, len(got.records), len(raw.records))
				}
			}
		}
	}
}

// A day back in a pool restores only the chunks of its rows whose meta names
// a pool the read keeps, as a filter's read restores those that may keep a
// line, and keeps the pool's records of them alone, summarising no day
// through no filter: of a day of Acme's lines in the morning, a chunk and
// part of the next, and the first pool's in the afternoon, some naming it
// and some none, Acme's read restores the first two chunks, the first
// pool's the last two, every pool's all three, and none's none.
func TestAPoolsDayBackRestoresOnlyTheChunksThatMayHoldItsRows(t *testing.T) {
	at := func(hour int, i int) time.Time {
		return time.Date(2026, 10, 7, hour, 0, i, 0, time.Local)
	}
	var lines []ledger.Line
	for i := range 300 {
		lines = append(lines, pooledLine(fmt.Sprintf("a%d", i), "a", acmePool, at(9, i)))
	}
	for i := range 300 {
		lines = append(lines, pooledLine(fmt.Sprintf("b%d", i), "b", []string{firstPool, ""}[i%2], at(15, i)))
	}
	c := newDigestChain(t, "a morning in Acme, an afternoon in the first pool", 0, lines)
	c.digestDayByDay(func(LedgerDay) {})
	f, date := c.follower(), dayfile.DateOf(at(9, 0))
	if rows, _ := f.DayRows(date, RowsVersion); len(rows.Chunks) != 3 {
		t.Fatalf("the day's rows are in %d chunks, want 3", len(rows.Chunks))
	}
	chunks := map[string]int{"Acme's": 2, "the first pool's": 2, "every pool's": 3, "no pool's": 0}
	kept := map[string]int{"Acme's": 300, "the first pool's": 300, "every pool's": 600, "no pool's": 0}
	for _, p := range poolReads {
		counted := &readsCounted{Ledger: f}
		got := ReadDay(counted, date, LogFilter{}, p.read)
		if counted.chunks != chunks[p.name] || counted.summaries > 0 || counted.raw > 0 {
			t.Errorf("%s read of the day reads %+v, want %d chunks, no day summarised and no line", p.name, counted.reads, chunks[p.name])
		}
		if want := rawIn(f, date, p.read); len(got.records) != kept[p.name] || !reflect.DeepEqual(got.records, want.records) {
			t.Errorf("%s read of the day gives %d records, want %d:\n%s", p.name, len(got.records), kept[p.name], recordsUnlike(got.records, want.records))
		}
	}
}

// A sparse pool's day back through no filter passes over a day the
// summaries the ledger holds rule out, of the day and the days beside it
// whose files may hold its lines, none telling of the pool's work, reading
// neither its head nor its lines, and summarising none: of Acme's lines on
// the first day and the fifth alone, and the first pool's every day, Acme's
// read passes over the third, and reads the second and the fourth, beside a
// day of its work, the first pool's reading each, every read as the day's
// lines in the pool read raw give it.
func TestASparsePoolsDayBackPassesOverTheDaysItsSummariesRuleOut(t *testing.T) {
	at := func(d, hour int) time.Time {
		return time.Date(2026, 10, 5+d, hour, 0, 0, 0, time.Local)
	}
	var lines []ledger.Line
	for d := range 5 {
		lines = append(lines, pooledLine(fmt.Sprintf("b%d", d), "b", firstPool, at(d, 15)))
		if d == 0 || d == 4 {
			lines = append(lines, pooledLine(fmt.Sprintf("a%d", d), "a", acmePool, at(d, 9)))
		}
	}
	c := newDigestChain(t, "a sparse pool", 0, lines)
	c.digestDayByDay(func(LedgerDay) {})
	f := c.follower()
	for d := 1; d < 4; d++ {
		date := dayfile.DateOf(at(d, 0))
		for _, p := range poolReads[:2] {
			counted := &readsCounted{Ledger: f}
			got := ReadDay(counted, date, LogFilter{}, p.read)
			read, passed := counted.rows+counted.chunks+counted.raw > 0, p.read.ID == acmePool && d == 2
			if read == passed || counted.summaries > 0 {
				t.Errorf("%s, %s read reads %+v, want the day passed over unread %t, and no day summarised", date, p.name, counted.reads, passed)
			}
			if want := rawIn(f, date, p.read); !reflect.DeepEqual(got.records, want.records) {
				t.Errorf("%s, %s read gives\n%s", date, p.name, recordsUnlike(got.records, want.records))
			}
		}
	}
}

// A day's head built of lines naming no pool, as before pools and in a
// config without them, names no pool: neither its chunks' metas nor its
// side's sessions; and is read as of lines naming none, the first pool's
// read keeping each of its chunks and its sessions, read from its side,
// and Acme's none.
func TestADayOfLinesNamingNoPoolHasAHeadNamingNone(t *testing.T) {
	at := func(i int) time.Time {
		return time.Date(2026, 10, 7, 9, 0, i, 0, time.Local)
	}
	var lines []ledger.Line
	for i := range 300 {
		lines = append(lines, pooledLine(fmt.Sprintf("n%d", i), []string{"m", "n"}[i%2], "", at(i)))
	}
	c := newDigestChain(t, "a day naming no pool", 0, lines)
	c.digestDayByDay(func(LedgerDay) {})
	f, date := c.follower(), dayfile.DateOf(at(0))
	rows, ok := f.DayRows(date, RowsVersion)
	side, sided := f.DaySide(date, RowsVersion)
	if !ok || !sided || len(rows.Chunks) != 2 {
		t.Fatalf("the day's head stands %t, its side %t, its rows in %d chunks, want it standing, its rows in 2", ok, sided, len(rows.Chunks))
	}
	for i, chunk := range rows.Chunks {
		if bytes.Contains(chunk.Meta, []byte(`"pl"`)) {
			t.Errorf("chunk %d's meta is %s, want it naming no pool", i, chunk.Meta)
		}
	}
	if bytes.Contains(side, []byte(`"pools"`)) {
		t.Errorf("the day's side is %s, want it naming no pool", side)
	}
	kept := map[string]int{"Acme's": 0, "the first pool's": len(rows.Chunks)}
	for _, p := range poolReads[:2] {
		chunks, _ := chunksThrough(f, date, LogFilter{}, p.read)
		l := readingSides(f)
		got, want := DaySessionsOn(l, date, p.read), DaySessions(rawIn(f, date, p.read))
		if len(chunks) != kept[p.name] {
			t.Errorf("%s read keeps %d of the day's %d chunks, want %d", p.name, len(chunks), len(rows.Chunks), kept[p.name])
		}
		if !reflect.DeepEqual(got, want) || l.raw[date] {
			t.Errorf("%s sessions are %+v, its lines read %t; want %+v, as its lines in the pool read raw give, from its side", p.name, got, l.raw[date], want)
		}
	}
}

// A session whose lines name none and the first pool, as before and after
// the move to pools, is the first pool's whole, as each of its lines is:
// read from the day's side, its lines unread, in the first pool's read,
// all six of its lines; and in Acme's, none of it, from the side too.
func TestASessionNamingNoneAndTheFirstPoolIsTheFirstPoolsFromItsSide(t *testing.T) {
	at := func(i int) time.Time {
		return time.Date(2026, 10, 7, 9, i, 0, 0, time.Local)
	}
	var lines []ledger.Line
	for i := range 6 {
		lines = append(lines, pooledLine(fmt.Sprintf("m%d", i), "m", []string{"", firstPool}[i%2], at(i)))
	}
	c := newDigestChain(t, "a session naming none and the first pool", 0, lines)
	c.digestDayByDay(func(LedgerDay) {})
	f, date := c.follower(), dayfile.DateOf(at(0))
	for _, p := range poolReads[:2] {
		l := readingSides(f)
		got, want := DaySessionsOn(l, date, p.read), DaySessions(rawIn(f, date, p.read))
		if !reflect.DeepEqual(got, want) || !l.sides[date] || l.raw[date] {
			t.Errorf("%s sessions are %+v, its side read %t and its lines %t; want %+v, as its lines in the pool read raw give, from its side alone",
				p.name, got, l.sides[date], l.raw[date], want)
		}
	}
	if got := DaySessionsOn(f, date, poolReads[1].read); len(got) != 1 || got[0].Lines != 6 {
		t.Errorf("the first pool's sessions are %+v, want m's six lines", got)
	}
}

// The sessions of each day of busy days split between pools, read in a
// pool as DaySessionsOn reads them, are those of the day's lines in the pool
// read raw: from the day's side, its lines unread, where each session's
// lines are all in the pool read or none; and of its lines where one's are
// in the pool and another, as s3's on its last day are in Acme and the
// first pool, as the side counts none of its lines by pool.
func TestAPoolsSessionsOfADayAreThoseOfItsLinesInThePool(t *testing.T) {
	t.Parallel()
	c := newDigestChain(t, fmt.Sprintf("busy days in pools %d", busySeed), busySeed, pooledBusyDays(busySeed))
	c.digestDayByDay(func(LedgerDay) {})
	f := c.follower()
	resumedOn := ""
	for _, l := range c.lines {
		if l.Session == "s3" {
			resumedOn = max(resumedOn, dayfile.DateOf(l.At))
		}
	}
	var raw []string
	for _, date := range filedDates(c.lines) {
		for _, p := range poolReads {
			l := readingSides(f)
			got, want := DaySessionsOn(l, date, p.read), DaySessions(rawIn(f, date, p.read))
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s, %s sessions are\n%+v\nwant, as its lines in the pool read raw give\n%+v", date, p.name, got, want)
			}
			if l.raw[date] {
				raw = append(raw, date+" "+p.name)
			}
		}
	}
	if want := []string{resumedOn + " Acme's", resumedOn + " the first pool's"}; !slices.Equal(raw, want) {
		t.Errorf("the days whose lines were read for their sessions are %q, want %q, where s3 ran in two pools, for those two pools", raw, want)
	}
}
