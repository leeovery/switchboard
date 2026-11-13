package views

import (
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
// line, and keeps the pool's records of them alone, reading no summary
// through no filter: of a day of Acme's lines in the morning, a chunk and
// part of the next, and the first pool's in the afternoon, some naming it
// and some none, Acme's read restores the first two chunks, the first
// pool's the last two, every pool's all three, and none's none.
func TestAPoolsDayBackRestoresOnlyTheChunksThatMayHoldItsRows(t *testing.T) {
	at := func(hour int, i int) time.Time {
		return time.Date(2026, 10, 7, hour, 0, i, 0, time.Local)
	}
	line := func(request, session, pool string, at time.Time) ledger.Line {
		return ledger.Line{
			At: at, Request: request, Kind: ledger.KindMessage, Session: session, Model: foldOpus, Account: "work", Reason: "sticky", Status: 200,
			Attempts: 1, TotalMS: 100, Class: ledger.ClassMain, Pool: pool,
		}
	}
	var lines []ledger.Line
	for i := range 300 {
		lines = append(lines, line(fmt.Sprintf("a%d", i), "a", acmePool, at(9, i)))
	}
	for i := range 300 {
		lines = append(lines, line(fmt.Sprintf("b%d", i), "b", []string{firstPool, ""}[i%2], at(15, i)))
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
			t.Errorf("%s read of the day reads %+v, want %d chunks, no summary and no line", p.name, counted.reads, chunks[p.name])
		}
		if want := rawIn(f, date, p.read); len(got.records) != kept[p.name] || !reflect.DeepEqual(got.records, want.records) {
			t.Errorf("%s read of the day gives %d records, want %d:\n%s", p.name, len(got.records), kept[p.name], recordsUnlike(got.records, want.records))
		}
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
