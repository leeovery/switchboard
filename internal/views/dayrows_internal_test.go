package views

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// rowsWorlds are the worlds a day's rows are built of: the chained digests',
// and a busy one, its days holding more rows than a chunk does.
var rowsWorlds = append(slices.Clip(chainWorlds), struct {
	name  string
	lines func(seed uint64) []ledger.Line
}{name: "busy days", lines: busyDays})

// busyDays are a dozen generated sessions' lines, as generatedSession makes
// them, each session and request of its own, side by side, in the order the
// ledger files them: so their days hold more rows than a chunk of them does.
func busyDays(seed uint64) []ledger.Line {
	var lines []ledger.Line
	for k := range uint64(12) {
		for _, l := range generatedSession(seed*12 + k) {
			l.Session = fmt.Sprintf("s%d", k)
			if l.Request != "" {
				l.Request = fmt.Sprintf("%d-%s", k, l.Request)
			}
			lines = append(lines, l)
		}
	}
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	return lines
}

// rowsRead returns the records of the rows f gives of the local day with the
// given date, as DayRows points at them, each chunk read as RowChunk reads
// it and its rows restored, in order, as restoredRows restores them; and
// each chunk's meta, as chunkMetaOf has it, beside the records of its rows.
// It reports false where DayRows gives none, failing the test where a chunk
// doesn't read, or holds other than as many rows as the head counts.
func rowsRead(t *testing.T, f *ledger.Follower, date string) ([]lineRecord, []metaRead, bool) {
	t.Helper()
	rows, ok := f.DayRows(date, RowsVersion)
	if !ok {
		return nil, nil, false
	}
	var records []lineRecord
	var metas []metaRead
	for i, c := range rows.Chunks {
		data, err := f.RowChunk(date, c)
		if err != nil {
			t.Fatalf("chunk %d of %s: %v", i, date, err)
		}
		from := len(records)
		if records, err = restoredRows(records, data); err != nil {
			t.Fatalf("chunk %d of %s: %v", i, date, err)
		}
		if n := len(records) - from; n != c.Count {
			t.Fatalf("chunk %d of %s holds %d rows, its head counts %d", i, date, n, c.Count)
		}
		meta, err := chunkMetaOf(c.Meta)
		if err != nil {
			t.Fatalf("chunk %d of %s: %v", i, date, err)
		}
		metas = append(metas, metaRead{meta: meta, records: records[from:]})
	}
	return records, metas, true
}

// metaRead is a chunk's meta as it's read, beside the records of its rows.
type metaRead struct {
	meta    chunkMeta
	records []lineRecord
}

// namesItsRows reports what's wrong with m, a chunk's meta, of the records
// of its rows: "" where it names the sessions they count toward, each with
// as many of them, and when the first and the last of them arrived.
func (m metaRead) namesItsRows() string {
	sessions := make(map[string]int)
	for i := range m.records {
		if id := m.records[i].countedSession; id != "" {
			sessions[id]++
		}
	}
	arrival := func(a, b lineRecord) int { return a.at.compare(b.at) }
	first, last := slices.MinFunc(m.records, arrival).at, slices.MaxFunc(m.records, arrival).at
	switch got := m.meta.lines.Sessions; {
	case !maps.Equal(got, sessions):
		return fmt.Sprintf("names sessions %v, its rows %v", got, sessions)
	case m.meta.first != first || m.meta.last != last:
		return fmt.Sprintf("spans %v to %v, its rows %v to %v", m.meta.first.time(), m.meta.last.time(), first.time(), last.time())
	}
	return ""
}

// A day's rows, as the views' Digests build them of a real ledger's lines
// day by day, read back through the ledger, each chunk restored, are the
// records of the day's lines read raw, as Day gives them, record for record,
// in order: and stand of every day the worlds file lines on, once its head
// is built, in chunks of rowsAChunk, the last of those left, the busy days
// in more than one. Each chunk's meta names the sessions its rows count
// toward, each with as many, and when the first and the last of them
// arrived.
func TestADaysRowsAreTheRecordsOfItsLinesReadRaw(t *testing.T) {
	var stood, cut int
	for _, w := range rowsWorlds {
		for seed := range uint64(chainSeeds) {
			c := newDigestChain(t, fmt.Sprintf("%s %d", w.name, seed), seed, w.lines(seed))
			c.digestDayByDay(func(LedgerDay) {})
			f := ledger.NewFollower(c.state, func() time.Time { return c.at }, ledger.CapsOf(nil, nil), DigestVersion, quiet)
			for _, date := range filedDates(c.lines) {
				records, metas, ok := rowsRead(t, f, date)
				if !ok {
					t.Errorf("%s: the rows of %s don't stand at %v, its head built", c.name, date, c.at)
					continue
				}
				stood++
				if want := RecordsOf(f.Day(date)).records; !reflect.DeepEqual(records, want) {
					t.Errorf("%s: the rows of %s restore as\n%+v\nwant, as its lines read raw give\n%+v", c.name, date, records, want)
				}
				for i, m := range metas {
					if full := i < len(metas)-1; full && len(m.records) != rowsAChunk || !full && (len(m.records) == 0 || len(m.records) > rowsAChunk) {
						t.Errorf("%s: chunk %d of %d of %s holds %d rows, want %d a chunk, the last those left", c.name, i, len(metas), date, len(m.records), rowsAChunk)
					}
					if problem := m.namesItsRows(); problem != "" {
						t.Errorf("%s: the meta of chunk %d of %s %s", c.name, i, date, problem)
					}
				}
				cut += len(metas) - 1
			}
		}
	}
	if stood == 0 || cut == 0 {
		t.Errorf("rows stood of %d days, cut into another chunk %d times; want some of each", stood, cut)
	}
}

// A day's rows are cut into chunks of rowsAChunk, the last of those left,
// none empty: none of no records, one of fewer than a chunk holds, or as
// many, two of one more, and two of twice as many. Restored in order, they
// are the records taken.
func TestADaysRowsAreCutIntoChunksNoneEmpty(t *testing.T) {
	tests := []struct {
		records, chunks int
	}{
		{records: 0, chunks: 0},
		{records: 1, chunks: 1},
		{records: rowsAChunk - 1, chunks: 1},
		{records: rowsAChunk, chunks: 1},
		{records: rowsAChunk + 1, chunks: 2},
		{records: 2 * rowsAChunk, chunks: 2},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d records", tt.records), func(t *testing.T) {
			lines := make([]ledger.Line, tt.records)
			at := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
			for i := range lines {
				lines[i] = ledger.Line{At: at.Add(time.Duration(i) * time.Second), Request: fmt.Sprintf("r%d", i), Kind: ledger.KindMessage,
					Session: "s", Model: foldOpus, Account: "work", Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 3000}
			}
			taken := recordsOfLines(lines)
			var d dayRows
			for i := range taken {
				d.take(&taken[i])
			}

			chunks, err := d.built()
			if err != nil {
				t.Fatal(err)
			}
			if len(chunks) != tt.chunks {
				t.Errorf("%d records are cut into %d chunks, want %d", tt.records, len(chunks), tt.chunks)
			}
			var restored []lineRecord
			for i, c := range chunks {
				if c.Count == 0 || len(c.Rows) == 0 {
					t.Errorf("chunk %d of %d holds %d rows, %q; want none empty", i, len(chunks), c.Count, c.Rows)
				}
				if restored, err = restoredRows(restored, c.Rows); err != nil {
					t.Fatal(err)
				}
			}
			if len(restored)+len(taken) > 0 && !reflect.DeepEqual(restored, taken) {
				t.Errorf("the chunks of %d records restore as %d records, want those taken, in order", tt.records, len(restored))
			}
		})
	}
}

// A day whose summary counts a line, but whose read gives none, as its one
// line arrived on the day after, has a head of no rows, as the views'
// Digests build it of a real ledger's lines: no rows file, and its rows read
// as none, standing.
func TestADayGivingNoLinesHasNoRows(t *testing.T) {
	const date = "2026-10-07"
	line := func(request string, at time.Time) ledger.Line {
		return ledger.Line{At: at, Request: request, Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: "work", Reason: "sticky",
			Status: 200, Attempts: 1, TotalMS: 3000}
	}
	after := time.Date(2026, 10, 8, 0, 30, 0, 0, time.Local)
	c := newDigestChain(t, "a day giving no lines", 0, []ledger.Line{line("r1", after)})
	c.file(date, line("r2", after.Add(time.Minute)))
	c.digestDayByDay(func(LedgerDay) {})

	if _, err := os.Stat(filepath.Join(ledger.Dir(c.state), "rows-"+date+".gz")); err == nil {
		t.Errorf("the ledger holds a rows file of %s, built of none; want none", date)
	}
	f := ledger.NewFollower(c.state, func() time.Time { return c.at }, ledger.CapsOf(nil, nil), DigestVersion, quiet)
	if rows, ok := f.DayRows(date, RowsVersion); !ok || len(rows.Chunks) > 0 || rows.Count > 0 {
		t.Errorf("DayRows(%s) = %+v, %t; want none, read", date, rows, ok)
	}
}

// The heads and rows files the views' Digests build of a real ledger's
// lines day by day, pass by pass, are those a ledger opened afresh builds
// of the same days, from none, byte for byte.
func TestADaysRowsBuiltDayByDayAreThoseBuiltAfresh(t *testing.T) {
	for _, w := range rowsWorlds {
		for seed := range uint64(chainSeeds) {
			c := newDigestChain(t, fmt.Sprintf("%s %d", w.name, seed), seed, w.lines(seed))
			c.digestDayByDay(func(LedgerDay) {})
			built := c.headsAndRows()
			for path := range built {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}

			l := ledger.Open(ledger.Dir(c.state), 400*24*time.Hour, func() time.Time { return c.at }, ledger.HistoryIn(readings.Files(readings.Dir(c.state), quiet)),
				ledger.CapsOf(nil, nil), quiet, ledger.Digesting(Digests{}))
			l.DigestEnded(t.Context(), c.at)

			if afresh := c.headsAndRows(); !reflect.DeepEqual(afresh, built) {
				t.Errorf("%s: built afresh, the heads and rows files are %v; want them byte for byte as built day by day, %v",
					c.name, slices.Sorted(maps.Keys(afresh)), slices.Sorted(maps.Keys(built)))
			}
		}
	}
}

// headsAndRows are the day heads and rows files the chain's ledger holds,
// each file's bytes by its path.
func (c *digestChain) headsAndRows() map[string]string {
	c.t.Helper()
	files := make(map[string]string)
	for _, pattern := range []string{"logday-*", "rows-*"} {
		paths, err := filepath.Glob(filepath.Join(ledger.Dir(c.state), pattern))
		if err != nil {
			c.t.Fatal(err)
		}
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				c.t.Fatal(err)
			}
			files[path] = string(data)
		}
	}
	return files
}
