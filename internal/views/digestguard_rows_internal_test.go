package views

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"iter"
	"maps"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// The rows' part of the reads over digests: the Log read back at half past
// one, as the watch reads it back, each past day as ReadDay reads it,
// through the Follower resting on digests, which reads a day's rows where
// they stand, held to the same read over every day's lines, as Day gives
// them: as it's shown, its table, each row's details, each day's rule and
// totals, what each day's lines count and the days its rows await a line
// on. A day read at another moment is read back alone, its records held to
// its lines'. And a turn of a page opened then, its lines read as ReadTurn
// reads them through the Follower resting on digests and through one
// reading every line of its session raw. A read from rows is told by what
// the ledger was asked, as rowsNoted and turnReads note it: a day read from
// its rows alone, never its lines; a fallback to them meets no moment of
// rows. Where a head's upset, or a late line filed onto its day, makes it
// stop standing, its rows are planted with a row no line has, as plantRows
// has it, and a chunk spoiled in place holds one too, so a read of rows that
// ignored why they don't stand would show otherwise than one reading the
// day raw.

// The hard moments the Log read back over rows meets, each met by some
// spread world, judged from the reads compared: a day read from its rows
// alone, or from its lines alone though its head is built.
const (
	loggedFromRows      = "the Log reading a past day back from its rows, never its lines"
	loggedYesterdayRaw  = "past midnight, the Log reading the day before back from its lines, its head yet to be built"
	loggedYesterdayRows = "at 01:30, the Log reading the day before back from its rows"
	loggedInFlight      = "a request in flight across midnight from a day the Log reads back from its rows, its row placed there by when it was sent, a line awaited there"
	loggedSettledAfter  = "a move made on a day the Log reads back from its rows, settled by a request after midnight, its row's routing told live"
	loggedIdless        = "a day the Log reads back from its rows holding a line without an id"
	loggedIdlessTwice   = "a day the Log reads back from its rows holding a line without an id given twice"
	loggedIDTwice       = "a day the Log reads back from its rows holding a line with an id given twice"
	loggedLate          = "a late line filed onto a day after its head was built, the Log reading the day back from its lines alone"
	loggedRebuilt       = "a day a late line was filed onto, read back from its lines, read back from its rows once its head is built again"
	loggedOtherVersion  = "a day's head of another version, the Log reading the day back from its lines alone"
	loggedOtherZone     = "a day's head built in another time zone, its bounds not the day's, the Log reading the day back from its lines alone"
	loggedNotAlone      = "a zone change's line filed beside the day it arrived on, the Log reading that day back from its lines alone, its head built"
)

// logMoments are the hard moments the Log read back over rows meets, but
// those of the tables it's read with and those the upsets make.
var logMoments = []string{
	loggedFromRows, loggedYesterdayRaw, loggedYesterdayRows, loggedInFlight, loggedSettledAfter, loggedIdless, loggedIdlessTwice, loggedIDTwice,
	loggedLate, loggedRebuilt, loggedOtherVersion, loggedOtherZone, loggedNotAlone,
}

// loggedAs are the Log's moments of a day read back from its lines though
// its head is built, by the find's of the same reason, as unstood gives it.
var loggedAs = map[string]string{
	foundLate: loggedLate, foundOtherVersion: loggedOtherVersion, foundOtherZone: loggedOtherZone, foundNotAlone: loggedNotAlone,
}

// loggedPricedEither are the moments of a day the Log reads back from its
// rows, read with each of the tables given.
func loggedPricedEither(prices []pricedAs) []string {
	var moments []string
	for _, p := range prices {
		moments = append(moments, loggedPriced(p.name))
	}
	return moments
}

// loggedPriced is the moment of a day the Log reads back from its rows,
// holding a request the tables named price otherwise than the built-in one
// does, as no row holds a price.
func loggedPriced(prices string) string {
	return prices + ": a day the Log reads back from its rows holding a request priced otherwise than by the built-in table"
}

// sameLog fails the test where the Log read back at the moment m, at half
// past one, each past day as ReadDay reads it, shows otherwise than read
// back over each day's lines, as Day gives them, as logBack has each,
// through the Follower resting on digests, told what logStream tells,
// today's lines and the days' totals read once for both, as readOnce has
// them. It's read back to the day two before, so a move made then shows
// beside the request the next day that settled it, and a day whose head
// stops standing once the next day's digest tells of a line of it is read.
// Its store is fed alike, as the stores fed through each Follower are held
// to each other at each moment, so the two differ only by their days'
// reads. Each of the days daysBack gives the Log didn't read is read back
// alone, as sameDayBack has it. It fails the test too where a day's lines
// are read whose rows stand and hold no chunk spoiled, as stillSpoiled has
// it, or today's rows are; and notes the hard moments the reads meet. The
// Log is read at no other moment, as each read slows the run: what the
// others meet, a day's reads meet alone.
func (w *spread) sameLog(m moment) {
	w.t.Helper()
	var shown []any
	if m.now.Hour() == 1 {
		from, s, once := twoBack(m.now), logStream(m), readingOnce()
		shown = logBack(once.through(m.logged), wholeDay, from, s, w.prices.read, m.now)
		if want := logBack(once.through(w.based), rawDay, from, s, w.prices.read, m.now); !reflect.DeepEqual(shown, want) {
			w.t.Errorf("%s at %v: the Log read back to %s over rows shows otherwise than read raw\n%s", w.name, m.now, from, unlike(shown, want))
		}
	}
	for _, date := range w.daysBack(m.now) {
		if !m.logged.rows[date] && !m.logged.raw[date] {
			w.sameDayBack(m, date)
		}
	}
	for date := range m.logged.rows {
		if m.logged.raw[date] && !w.stillSpoiled(date) {
			w.t.Errorf("%s at %v: the lines of %s were read back, its rows standing, none of them upset", w.name, m.now, date)
		}
	}
	if today := Today(m.now).Date; m.logged.rows[today] {
		w.t.Errorf("%s at %v: today's rows, %s, were read, want none till it has ended", w.name, m.now, today)
	}
	w.noteLogged(m, shown)
}

// readOnce are the days' lines, their summaries, their rows and the chunks
// of them, as the Log's reads back at a moment read them, each read once,
// through whichever ledger reads it first, as all read them alike and
// nothing's filed while they read: each ledger read through it, as through
// gives it, reads them so. Lines given are shared, so none is changed.
type readOnce struct {
	lines  map[string][]ledger.Held
	sums   map[string][]ledger.Summary
	rows   map[rowsAt]givenRows
	chunks map[chunkAt]givenChunk
}

// rowsAt is which rows DayRows is asked for: the date of their day, and the
// version of their body.
type rowsAt struct {
	date string
	body int
}

// givenRows are a day's rows as DayRows gave them, and whether they stood.
type givenRows struct {
	rows ledger.DayRows
	ok   bool
}

// chunkAt is which chunk of a day's rows RowChunk is asked for: the date of
// their day, and the offset of its member.
type chunkAt struct {
	date   string
	offset int64
}

// givenChunk is a chunk's rows as RowChunk gave them, and why it gave none.
type givenChunk struct {
	rows []byte
	err  error
}

// readingOnce is readOnce, nothing read yet.
func readingOnce() *readOnce {
	return &readOnce{
		lines: make(map[string][]ledger.Held), sums: make(map[string][]ledger.Summary), rows: make(map[rowsAt]givenRows), chunks: make(map[chunkAt]givenChunk),
	}
}

// through is l, but its reads of the days' lines, summaries, rows and chunks
// of them, as o reads them once.
func (o *readOnce) through(l Ledger) Ledger {
	return onceRead{Ledger: l, o: o}
}

// onceRead is a ledger read through o, as through gives it.
type onceRead struct {
	Ledger
	o *readOnce
}

func (r onceRead) Day(date string) []ledger.Held {
	return once(r.o.lines, date, func() []ledger.Held { return r.Ledger.Day(date) })
}

func (r onceRead) DaysOn(dates []string) []ledger.Summary {
	return once(r.o.sums, fmt.Sprint(dates), func() []ledger.Summary { return r.Ledger.DaysOn(dates) })
}

func (r onceRead) DayRows(date string, body int) (ledger.DayRows, bool) {
	given := once(r.o.rows, rowsAt{date: date, body: body}, func() givenRows {
		rows, ok := r.Ledger.DayRows(date, body)
		return givenRows{rows: rows, ok: ok}
	})
	return given.rows, given.ok
}

func (r onceRead) RowChunk(date string, chunk ledger.RowsChunk) ([]byte, error) {
	given := once(r.o.chunks, chunkAt{date: date, offset: chunk.Offset}, func() givenChunk {
		rows, err := r.Ledger.RowChunk(date, chunk)
		return givenChunk{rows: rows, err: err}
	})
	return given.rows, given.err
}

// once is what read gives, held in read under key the first time it's
// asked for, as held there after.
func once[K comparable, V any](held map[K]V, key K, read func() V) V {
	v, ok := held[key]
	if !ok {
		v = read()
		held[key] = v
	}
	return v
}

// daysBack are the dates of the days read back alone at now, as sameDayBack
// reads them, where the Log doesn't read them, in order: a minute after
// midnight, the day two before's, the latest whose head is built, its rows
// upset then, as rowsUpsetAt has it, and the day before's, its head yet to
// be built; a minute before one, the day two before's, its head or lines
// upset then, as headsUpsetAt has it, and the day whose head was, as
// headMade has it; and each a late line was filed onto, read from its lines
// for it, its rows yet to be read again, as relogged holds them.
func (w *spread) daysBack(now time.Time) []string {
	dates := slices.Collect(maps.Keys(w.relogged))
	switch {
	case rowsUpsetAt(now):
		dates = append(dates, twoBack(now), yesterdayOf(Today(now)))
	case headsUpsetAt(now):
		dates = append(dates, twoBack(now))
		if w.headMade != nil {
			dates = append(dates, w.headMade.date)
		}
	}
	slices.Sort(dates)
	return slices.Compact(dates)
}

// sameDayBack fails the test where the day with the given date, read back at
// the moment m as ReadDay reads it, through the Follower resting on digests,
// as m.logged notes the read, is otherwise than its lines read raw, as Day
// gives them.
func (w *spread) sameDayBack(m moment, date string) {
	w.t.Helper()
	if got, want := wholeDay(m.logged, date), rawDay(w.based, date); !reflect.DeepEqual(got, want) {
		w.t.Errorf("%s at %v: %s, read from its rows %t, reads back as\n%s", w.name, m.now, date, m.logged.fromRows(date), recordsUnlike(got.records, want.records))
	}
}

// recordsUnlike says the first of a day's records read back, got, that
// differs from want, those of its lines read raw, for a failure to show.
func recordsUnlike(got, want []lineRecord) string {
	for i := range min(len(got), len(want)) {
		if !reflect.DeepEqual(got[i], want[i]) {
			return fmt.Sprintf("record %d of %d\n%+v\nwant, as its lines read raw give, of %d\n%+v", i, len(got), got[i], len(want), want[i])
		}
	}
	return fmt.Sprintf("%d records, want %d, as its lines read raw give", len(got), len(want))
}

// logStream is what the stream tells the Log at the moment m, live, as the
// watch's is: what m's tells, a request in flight since half an hour before
// the day began, as pinning has it, where it tells of one, so the Log places
// its row on the day before; and one of the same session sent ten minutes
// after it, ended half a minute after midnight, its line yet to come, so the
// Log's rows await it on the day before.
func logStream(m moment) Stream {
	s := Stream{Live: true, Requests: maps.Clone(m.stream.Requests)}
	for _, pinned := range m.stream.Requests {
		landed := pinned
		landed.Request, landed.Sent, landed.Status = "landed", pinned.Sent.Add(10*time.Minute), 200
		landed.Ended = Today(m.now).Start.Add(30 * time.Second)
		landed.Heard = landed.Ended
		s.Requests[landed.Request] = landed
	}
	return s
}

// noteLogged notes the hard moments the days read back at the moment m meet,
// as m.logged noted their reads, shown what the Log showed then, as logBack
// has it, none where it wasn't read: the days read from their rows alone, a
// day a late line was filed onto read from its lines at an earlier moment
// among them, and, of those the Log read, as far back as the day two
// before, those holding what the world and the Log's stream give the
// moments of, as noteShown has them; and the days read from their lines,
// the day before's past midnight, its head yet to be built, and those whose
// heads are built, by why they don't stand, as unstood has it, where the
// find read their lines too, so it's the head that doesn't stand, not the
// rows an upset of them spoiled, a late line's day held to be read from its
// rows again, as relogged holds it.
func (w *spread) noteLogged(m moment, shown []any) {
	yesterday := yesterdayOf(Today(m.now))
	for date := range m.logged.rows {
		if !m.logged.fromRows(date) {
			continue
		}
		w.met[loggedFromRows] = true
		w.met[loggedRebuilt] = w.met[loggedRebuilt] || w.relogged[date]
		delete(w.relogged, date)
		if shown != nil && date >= twoBack(m.now) {
			w.noteShown(m, date, shown)
		}
	}
	for date := range m.logged.raw {
		if m.logged.rows[date] {
			continue
		}
		if why := w.unstood(date); len(why) == 1 && loggedAs[why[0]] != "" && m.found.raw[date] {
			w.met[loggedAs[why[0]]] = true
			w.relogged[date] = w.relogged[date] || why[0] == foundLate
		}
		w.met[loggedYesterdayRaw] = w.met[loggedYesterdayRaw] || date == yesterday && m.now.Hour() == 0 && !w.headBuilt(date)
	}
}

// noteShown notes the hard moments the Log read back at the moment m, at
// half past one, as shown has what it showed, meets of the day with the
// given date it read from its rows alone: the day before's; a request in
// flight across midnight from it, a line awaited on it, as the Log's stream
// tells; a move made on it settled after midnight, as settledAcross has
// it; and what the world's lines on it hold, as givenOn and repricedOn have
// them.
func (w *spread) noteShown(m moment, date string, shown []any) {
	idless, idlessTwice, idTwice := w.givenOn(date, m.now)
	awaited := shown[len(shown)-1].([]string)
	for _, moment := range []struct {
		name string
		met  bool
	}{
		{name: loggedYesterdayRows, met: date == yesterdayOf(Today(m.now))},
		{name: loggedInFlight, met: slices.Contains(awaited, date) && placedAcross(m, shown, date)},
		{name: loggedSettledAfter, met: settledAcross(shown, date)},
		{name: loggedIdless, met: idless},
		{name: loggedIdlessTwice, met: idlessTwice},
		{name: loggedIDTwice, met: idTwice},
		{name: loggedPriced(w.prices.name), met: w.repricedOn(date, m.now)},
	} {
		w.met[moment.name] = w.met[moment.name] || moment.met
	}
}

// placedAcross reports whether the Log, as shown has what it showed, places
// a row of each request in flight since before midnight that m's stream
// tells of, sent on the day with the given date, on that day, as it was
// sent, and of one at least.
func placedAcross(m moment, shown []any, date string) bool {
	placed := 0
	for _, t := range m.stream.Requests {
		if Today(t.Sent).Date != date {
			continue
		}
		if !slices.ContainsFunc(rowsShown(shown), func(r RequestRow) bool { return r.Request == t.Request && Today(r.At).Date == date }) {
			return false
		}
		placed++
	}
	return placed > 0
}

// rowsShown are the rows of the lines the Log shows, as shown has them.
func rowsShown(shown []any) []RequestRow {
	var rows []RequestRow
	for _, s := range shown {
		if l, ok := s.(LogLine); ok && l.IsRow() {
			rows = append(rows, l.Row)
		}
	}
	return rows
}

// settledAcross reports whether a row the Log shows, as logBack has it, of a
// request that arrived on the day with the given date tells its session's
// move, not settled by itself, as it didn't write the context again, but
// after midnight, as settledLater has it: so the move is told live, as no
// row holds its story.
func settledAcross(shown []any, date string) bool {
	rows := rowsShown(shown)
	slices.SortStableFunc(rows, func(a, b RequestRow) int { return a.At.Compare(b.At) })
	for i, moved := range rows {
		if Today(moved.At).Date == date && moved.Routing.From != "" && !moved.Rewrote && settledLater(moved, rows[i+1:]) {
			return true
		}
	}
	return false
}

// settledLater reports whether the move moved made was settled on a later
// day than it was made on, as later, the rows after it, oldest first, tell:
// none of its session and model followed it that day, and the first of
// theirs after it that settles a move, writing the context again or making
// one of its own, is among them.
func settledLater(moved RequestRow, later []RequestRow) bool {
	for _, r := range later {
		switch {
		case r.Session != moved.Session || r.Model != moved.Model:
		case Today(r.At).Date == Today(moved.At).Date:
			return false
		case r.Rewrote || r.Routing.From != "":
			return true
		}
	}
	return false
}

// givenOn reports, of the lines filed under the local day with the given
// date that arrived by now, whether one has no id; whether one without an
// id is given twice, its every field alike; and whether two of them give
// the same id.
func (w *spread) givenOn(date string, now time.Time) (idless, idlessTwice, idTwice bool) {
	var on []ledger.Line
	for _, l := range w.lines {
		if w.filedUnder(l) == date && !l.At.After(now) {
			on = append(on, l)
		}
	}
	for i, l := range on {
		idless = idless || l.Request == ""
		for _, o := range on[i+1:] {
			idlessTwice = idlessTwice || l.Request == "" && reflect.DeepEqual(l, o)
			idTwice = idTwice || l.Request != "" && l.Request == o.Request
		}
	}
	return idless, idlessTwice, idTwice
}

// repricedOn reports whether a request filed under the local day with the
// given date, arrived by now, is worth otherwise at that day's prices, as
// the world reads them, than at the built-in table's.
func (w *spread) repricedOn(date string, now time.Time) bool {
	return slices.ContainsFunc(w.lines, func(l ledger.Line) bool {
		if w.filedUnder(l) != date || l.At.After(now) {
			return false
		}
		r := lineRecordOf(&l)
		return !reflect.DeepEqual(r.worth(w.prices.read, date), r.worth(ledger.Pricing, date))
	})
}

// The hard moments the upsets of a day's rows make, each made by an upset of
// a spread world's files: its head standing, the find reads its side, and
// the Log reads the day back from its lines alone.
const (
	rowsDamaged = "a chunk of a day's rows damaged, its file as long, the Log reading the whole day back from its lines, the find its side"
	rowsGone    = "a day's rows file gone, its head standing, the Log reading the day back from its lines, the find its side"
	rowsLonger  = "a day's head beside a rows file longer than it records, every chunk it points at whole, the Log reading the day back from its lines, the find its side"
)

// rowsUpsets are the upsets of a day's rows a spread world's files take,
// each in turn, as upset has them.
var rowsUpsets = []upsetting{
	{moment: rowsDamaged, upset: rowsSpoiled((*spread).spoilChunk, (*rowsNoted).wholeRaw)},
	{moment: rowsGone, upset: rowsSpoiled((*spread).loseRows, (*rowsNoted).rowsAside)},
	{moment: rowsLonger, upset: rowsSpoiled((*spread).lengthenRows, (*rowsNoted).rowsAside)},
}

// rowsUpsetAt reports whether a day's rows are upset at now, of the moments
// the reads are held at: a minute after midnight, when the day two before's
// head is the latest built, and no other head is upset.
func rowsUpsetAt(now time.Time) bool {
	return now.Hour() == 0 && now.Minute() < 30
}

// rowsSpoiled is the upset spoiling, as spoil spoils them, the rows of the
// day two before the one now falls on, where they stand, as the Follower
// resting on digests reads them: judged met where the find read the day's
// side then, as its head stands, and the Log read the day back as read
// says.
func rowsSpoiled(spoil func(w *spread, date string, rows ledger.DayRows), read func(r *rowsNoted, date string) bool) func(w *spread, now time.Time) (upsetJudged, string) {
	return func(w *spread, now time.Time) (upsetJudged, string) {
		date := twoBack(now)
		rows, ok := w.based.DayRows(date, RowsVersion)
		if !ok || len(rows.Chunks) == 0 {
			return nil, ""
		}
		spoil(w, date, rows)
		return func(m moment, _ map[string]openedPage) bool { return m.found.sides[date] && read(m.logged, date) }, ""
	}
}

// wholeRaw reports whether the day with the given date was read from its
// lines, its rows standing: so a chunk of them that didn't read had the
// whole day read from its lines.
func (r *rowsNoted) wholeRaw(date string) bool {
	return r.rows[date] && r.raw[date]
}

// rowsAside reports whether the day with the given date was read from its
// lines, its rows not standing.
func (r *rowsNoted) rowsAside(date string) bool {
	return !r.rows[date] && r.raw[date]
}

// spoilChunk has the first chunk of rows, of the local day with the given
// date, hold other rows than its head points at, its gzip member whole and
// as long, as many rows in it: its first a row no line has, as plantedRecord
// has it. So it doesn't read, by its CRC, and the day's rows file is as long
// as its head records, so its day isn't built again; and a read of it that
// passed over its CRC would show that row. It holds where the member lies,
// and its CRC, as spoiled, for stillSpoiled.
func (w *spread) spoilChunk(date string, rows ledger.DayRows) {
	w.t.Helper()
	file := w.rowsFile(date)
	c := rows.Chunks[0].Member
	records := w.chunkRecords(file, c)
	w.planted++
	records[0] = plantedRecord(records[0], w.planted)
	member, ok := memberOfLength(storedRowsOf(w.t, records), int(c.Length))
	if !ok {
		w.t.Fatalf("%s: the first chunk of the rows of %s, planted, won't make a gzip member of its %d bytes", w.name, date, c.Length)
	}
	copy(file[c.Offset:], member)
	w.writeRowsFile(date, file)
	w.spoiled[date] = ledger.Member{Offset: c.Offset, Length: c.Length, CRC: crc32.ChecksumIEEE(member)}
}

// stillSpoiled reports whether the rows file of the local day with the given
// date holds the member spoilChunk spoiled where it put it, forgetting it
// once it doesn't, as the day's rows are built again.
func (w *spread) stillSpoiled(date string) bool {
	m, ok := w.spoiled[date]
	if !ok {
		return false
	}
	file, err := os.ReadFile(w.rowsPath(date))
	if err == nil && int64(len(file)) >= m.Offset+m.Length && crc32.ChecksumIEEE(file[m.Offset:m.Offset+m.Length]) == m.CRC {
		return true
	}
	delete(w.spoiled, date)
	return false
}

// loseRows removes the rows file of the local day with the given date, its
// head left as it is.
func (w *spread) loseRows(date string, _ ledger.DayRows) {
	w.removeRows(date)
}

// lengthenRows has the rows file of the local day with the given date hold
// its first chunk's member again after the rest, its head as it was. Every
// chunk its head points at still reads, by its CRC, and a read of them
// would show what the day's lines do: what's held is that the head's
// record of the file's size alone has the rows not stand.
func (w *spread) lengthenRows(date string, rows ledger.DayRows) {
	file := w.rowsFile(date)
	m := rows.Chunks[0].Member
	w.writeRowsFile(date, slices.Concat(file, file[m.Offset:m.Offset+m.Length]))
}

// plantRows has the rows of the head the ledger holds of the local day with
// the given date hold a row no line of the day has, as plantedChunk has it,
// in a chunk of its own after the rest, its member CRC and all, and the
// count of the day's lines and rows one more, the head's grounds left as
// they are: so a read of rows that read them where the head doesn't stand
// would show it. It plants none where the ledger holds no head of the day,
// or none of rows.
func (w *spread) plantRows(date string) {
	w.t.Helper()
	plain, ok := w.headIn(date)
	if !ok {
		return
	}
	var head struct {
		Lines    int                `json:"lines"`
		RowsSize int64              `json:"rows_size"`
		Rows     int                `json:"rows"`
		Chunks   []ledger.RowsChunk `json:"chunks"`
	}
	if err := json.Unmarshal(plain, &head); err != nil {
		w.t.Fatalf("%s: the head at %s doesn't read: %v", w.name, w.headPath(date), err)
	}
	if _, err := os.Stat(w.rowsPath(date)); len(head.Chunks) == 0 || errors.Is(err, fs.ErrNotExist) {
		return
	}
	w.planted++
	planted := w.plantedChunk(date, head.Chunks[0].Member)
	at, size := w.rowsAppended(date, planted)
	field := func(key string, n int64) []byte { return fmt.Appendf(nil, `"%s":%d,`, key, n) }
	plain = w.replacedIn(date, plain,
		[2][]byte{field("lines", int64(head.Lines)), field("lines", int64(head.Lines+1))},
		[2][]byte{field("rows_size", head.RowsSize), field("rows_size", size)},
		[2][]byte{field("rows", int64(head.Rows)), field("rows", int64(head.Rows+1))})
	end := bytes.LastIndex(plain, []byte("]}"))
	if end < 0 || len(bytes.TrimSpace(plain[end+2:])) > 0 {
		w.t.Fatalf("%s: the head at %s doesn't end with its chunks:\n%s", w.name, w.headPath(date), plain)
	}
	chunk := fmt.Appendf(nil, `,{"offset":%d,"length":%d,"crc":%d,"count":1,"meta":%s}`, at.Offset, at.Length, at.CRC, planted.Meta)
	w.writeHead(date, slices.Concat(plain[:end], chunk, plain[end:]))
}

// plantedChunk is a chunk of rows built as the views' Digests build one, of
// a row no line of the world has: the first row of the chunk of the rows of
// the local day with the given date that first points at, its request
// given an id of its own, as plantedRecord has it.
func (w *spread) plantedChunk(date string, first ledger.Member) ledger.BuiltChunk {
	w.t.Helper()
	var c rowsChunk
	planted := plantedRecord(w.chunkRecords(w.rowsFile(date), first)[0], w.planted)
	if err := c.take(&planted); err != nil {
		w.t.Fatal(err)
	}
	built, err := c.built()
	if err != nil {
		w.t.Fatal(err)
	}
	return ledger.BuiltChunk{Rows: built.rows, Count: built.count, Meta: built.meta}
}

// plantedRecord is a record no line of the world has: r's, but for its
// request's id, the n-th planted's, and all it holds beside what tells its
// request apart, its session, its turn and when it went, left out.
func plantedRecord(r lineRecord, n int) lineRecord {
	return lineRecord{
		request: fmt.Sprintf("upset-row-%d", n), at: r.at, end: r.end, model: r.model, account: r.account, class: r.class, agentID: r.agentID, prompt: r.prompt,
		kind: r.kind, session: r.session, countedSession: r.countedSession, status: r.status,
	}
}

// storedRowsOf are records' stored rows, a JSON line each, in order, as a
// chunk holds them.
func storedRowsOf(t *testing.T, records []lineRecord) []byte {
	var c rowsChunk
	for i := range records {
		if err := c.take(&records[i]); err != nil {
			t.Fatal(err)
		}
	}
	return c.rows
}

// chunkRecords are the records of the rows of the gzip member m points at
// in file, a day's rows file, read regardless of its CRC.
func (w *spread) chunkRecords(file []byte, m ledger.Member) []lineRecord {
	w.t.Helper()
	records, err := restoredRows(nil, gunzipped(w.t, file[m.Offset:m.Offset+m.Length]))
	if err != nil || len(records) == 0 {
		w.t.Fatalf("%s: the chunk at %d of a rows file restores %d rows: %v", w.name, m.Offset, len(records), err)
	}
	return records
}

// rowsAppended appends chunk, its rows as one gzip member, to the rows file
// of the local day with the given date, giving where it lies there, and how
// long the file is then.
func (w *spread) rowsAppended(date string, chunk ledger.BuiltChunk) (ledger.Member, int64) {
	file := w.rowsFile(date)
	member := gzipped(w.t, chunk.Rows)
	at := ledger.Member{Offset: int64(len(file)), Length: int64(len(member)), CRC: crc32.ChecksumIEEE(member)}
	file = append(file, member...)
	w.writeRowsFile(date, file)
	return at, int64(len(file))
}

// rowsFile is the rows file the ledger holds of the local day with the given
// date.
func (w *spread) rowsFile(date string) []byte {
	w.t.Helper()
	file, err := os.ReadFile(w.rowsPath(date))
	if err != nil {
		w.t.Fatal(err)
	}
	return file
}

// writeRowsFile writes file as the rows file of the local day with the given
// date.
func (w *spread) writeRowsFile(date string, file []byte) {
	w.t.Helper()
	if err := os.WriteFile(w.rowsPath(date), file, 0o600); err != nil {
		w.t.Fatal(err)
	}
}

// memberOfLength is content as one gzip member exactly length bytes long,
// its header's extra field padding it out, at whichever level of
// compression first lets it: reporting false where none does, as content
// deflates to more than length at each.
func memberOfLength(content []byte, length int) ([]byte, bool) {
	for level := gzip.BestSpeed; level <= gzip.BestCompression; level++ {
		member := memberAt(content, level, nil)
		// An extra field takes two bytes of its length beside those it holds.
		switch pad := length - len(member); {
		case pad == 0:
			return member, true
		case pad >= 2:
			return memberAt(content, level, make([]byte, pad-2)), true
		}
	}
	return nil, false
}

// memberAt is content as one gzip member, deflated at level, its header
// holding extra where it's not nil.
func memberAt(content []byte, level int, extra []byte) []byte {
	var member bytes.Buffer
	w, _ := gzip.NewWriterLevel(&member, level)
	w.Extra = extra
	// A bytes.Buffer's writes never fail, so neither can the member's.
	_, _ = w.Write(content)
	_ = w.Close()
	return member.Bytes()
}

// turnShape is a hard moment of a turn of a page opened, read over rows,
// judged from the reads compared: a chunk of the rows of a day of the turn's
// own read, before today. A turn is of its shape where is reports so, of
// the turn t of the page o, as it's read raw at the moment m; and, where
// pends is set, once a request of it the stream tells of is in flight, as
// pendingOf has it.
type turnShape struct {
	moment string
	is     func(w *spread, m moment, o openedPage, t *turnTally) bool
	pends  bool
}

// turnShapes are the shapes of the turns read over rows, each its hard
// moment's.
var turnShapes = []turnShape{
	{
		moment: "a turn begun on a day before today and ended on the next, a day of its own before today read from its rows",
		is: func(_ *spread, m moment, _ openedPage, t *turnTally) bool {
			first := Today(t.started).Date
			return !t.pending && !t.last.end.IsZero() && first < Today(t.last.end).Date && first < Today(m.now).Date
		},
	},
	{
		moment: "a turn a subagent of was first seen on a day of, running on into a later one, a day of its own before today read from its rows",
		is: func(w *spread, m moment, o openedPage, t *turnTally) bool {
			return Today(t.started).Date < Today(m.now).Date && w.agentOn(o.id, t.prompt, m.now)
		},
	},
	{
		moment: "a turn still pending, begun on a day before today, a day of its own before today read from its rows",
		is: func(_ *spread, m moment, _ openedPage, t *turnTally) bool {
			return !t.pending && Today(t.started).Date < Today(m.now).Date
		},
		pends: true,
	},
	{
		moment: "a turn of days back, a day or more between it and today, its session's lines since its horizon read too, a day of its own read from its rows",
		is: func(w *spread, m moment, o openedPage, t *turnTally) bool {
			return !t.pending && !t.last.end.IsZero() && Today(t.last.end).Date < yesterdayOf(Today(m.now)) && w.sinceHorizon(o.id, m.now)
		},
	},
	{
		moment: "a turn whose requests arrived on two days, its page resting on the later's state or after, so its read reads that day's lines, a day of its own read from its rows",
		is: func(w *spread, m moment, o openedPage, t *turnTally) bool {
			from, rested := o.got.restedPage(o.id)
			last := w.lastArrived(o.id, t.prompt, m.now)
			return !t.pending && rested && Today(t.started).Date < last && last <= from.Base.Date
		},
	},
}

// turnMoments are the hard moments a turn of a page opened, read over rows,
// meets, as turnShapes has them.
func turnMoments() []string {
	moments := make([]string, len(turnShapes))
	for i, s := range turnShapes {
		moments[i] = s.moment
	}
	return moments
}

// agentOn reports whether a subagent of the session with the given id,
// first seen in the turn serving the given prompt, made a request on a day
// after the one it was first seen on, arrived by now.
func (w *spread) agentOn(id, prompt string, now time.Time) bool {
	first := make(map[string]ledger.Line)
	for _, l := range w.arrivedBy(id, now) {
		if l.AgentID == "" || !isRequest(&l) {
			continue
		}
		f, seen := first[l.AgentID]
		if !seen {
			first[l.AgentID] = l
			continue
		}
		if f.Prompt == prompt && w.filedUnder(l) > w.filedUnder(f) {
			return true
		}
	}
	return false
}

// lastArrived is the date of the local day the last request of the session
// with the given id, in the turn serving the given prompt, arrived on, of
// those arrived by now: "" where none did.
func (w *spread) lastArrived(id, prompt string, now time.Time) string {
	last := ""
	for _, l := range w.arrivedBy(id, now) {
		if l.Prompt == prompt && isRequest(&l) {
			last = max(last, Today(l.At).Date)
		}
	}
	return last
}

// sinceHorizon reports whether a line of the session with the given id
// arrived within the hour before now, the horizon a page opened then is
// held open from.
func (w *spread) sinceHorizon(id string, now time.Time) bool {
	return slices.ContainsFunc(w.arrivedBy(id, now), func(l ledger.Line) bool { return l.At.After(now.Add(-time.Hour)) })
}

// daysFromRows are the dates of the days a turn's read, as r notes it, read
// from their rows: chunks of them read, and none of the spans its session's
// lines were read within meeting the day.
func daysFromRows(r *turnReads) []string {
	var dates []string
	for date := range r.chunks {
		if !r.rawMeets(date) {
			dates = append(dates, date)
		}
	}
	return dates
}

// everyLine reads through l as a Follower of a ledger holding no day's rows
// does, but a session's every line, whatever spans it's asked for: so a turn
// read through it is held to all its session's lines, not those of the
// spans its read works out, as rawOnly reads them.
type everyLine struct {
	noRows
	l LedgerSession
}

func (e everyLine) Session(id string, _ ...ledger.Span) iter.Seq[ledger.Held] {
	return e.l.Session(id)
}

// checkTurns holds a turn of a page opened at the moment m, read over rows,
// as turnRead reads its lines through the Follower resting on digests, to
// the same read through the one reading raw, as sameTurn has it, of each of
// turnShapes whose moment no world has met yet, as each turn read slows the
// run: the newest of its shape of the first page, by its session's id,
// holding one, as turnOf finds it.
func (w *spread) checkTurns(m moment, pages map[string]openedPage) {
	w.t.Helper()
	for _, shape := range turnShapes {
		if w.met[shape.moment] {
			continue
		}
		for _, id := range slices.Sorted(maps.Keys(pages)) {
			if o, n, t, ok := w.turnOf(m, pages[id], shape); ok {
				w.met[shape.moment] = w.sameTurn(m, o, n, t)
				break
			}
		}
	}
}

// turnStoppedShort is the hard moment of a turn of a page opened, its
// session's lines since the horizon read of a day's file back from its end,
// stopping short of its start after a run of lines in a row each ended
// before the day's stop, as unreadWithin counts the lines left unread.
const turnStoppedShort = "a turn of a page opened, its session's lines since the horizon read of a day's file back from its end, stopping short of its start"

// readBackRun is how many lines in a row, each ended before what's wanted, a
// session's lines read back from the end of a day's file stop after, as the
// Follower reads them.
const readBackRun = 1000

// checkStopped holds the first turn of the page of each session with a
// request filed on a crowded day, by the moment m, as crowdedSessions has
// them, opened then, as checkTurns holds a turn: read as turnRead reads its
// lines through the Follower resting on digests, and through the one
// reading raw, given every line of the session, as everyLine has it. The
// former's read of the session's lines, as each of the turn's spans asks
// for them, leaves unread, as unreadWithin counts them, those the stop rule
// leaves behind, as leftBehind counts them, no more and no fewer. It notes
// its moment met where some were left.
func (w *spread) checkStopped(m moment) {
	w.t.Helper()
	for _, id := range w.crowdedSessions(m.now) {
		o := w.openPage(m, id)
		noted := readingTurn(w.based)
		based, raw := turnRead(o.based, 1, o.live, noted), turnRead(o.raw, 1, o.live, everyLine{l: w.raw})
		w.alike(m.now, id+"'s page, its first turn open", shownPageOf(based, o.live), shownPageOf(raw, o.live))
		for _, spans := range noted.spans {
			unread := w.unreadWithin(id, spans)
			if left := w.leftBehind(m.now, id, spans); unread != left {
				w.t.Errorf("%s at %v: %s's lines read within %+v left %d unread, want %d, as the stop rule leaves them", w.name, m.now, id, spans, unread, left)
			}
			w.met[turnStoppedShort] = w.met[turnStoppedShort] || unread > 0
		}
	}
}

// crowdedSessions are the ids of the sessions with a request that arrived
// by now filed on a crowded day, in order: one of more lines than
// readBackRun, of those whose files a page's horizon an hour before now may
// read, as dayfile.DatesBounds has them, so a read of its lines back from
// the day's end may stop short.
func (w *spread) crowdedSessions(now time.Time) []string {
	first, last := dayfile.DatesBounds(now.Add(-time.Hour), now)
	filed := make(map[string]int)
	for _, l := range w.lines {
		filed[w.filedUnder(l)]++
	}
	var ids []string
	for _, l := range w.lines {
		if date := w.filedUnder(l); isRequest(&l) && !l.At.After(now) && first <= date && date <= last && filed[date] > readBackRun {
			ids = append(ids, l.Session)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// unreadWithin is how many of the lines of the session with the given id
// the Follower resting on digests gives as it reads every day, filed on a
// day the spans given take in, as ledger.Spanning has them, it doesn't give
// as it reads them within the spans: those it read none of, reading a day's
// file back from its end, as a run of lines in a row ended before the day's
// stop stopped it; a line given twice counted as often as it's left.
func (w *spread) unreadWithin(id string, spans []ledger.Span) int {
	within := make(map[string]int)
	for h := range w.based.Session(id, spans...) {
		within[string(h.JSON)]++
	}
	spanned, unread := ledger.Spanning(spans), 0
	for h := range w.based.Session(id) {
		switch key := string(h.JSON); {
		case within[key] > 0:
			within[key]--
		case spanned(w.filedUnder(h.Line)):
			unread++
		}
	}
	return unread
}

// leftBehind is how many lines of the session with the given id that
// arrived by now the stop rule leaves unread of a read of them within the
// spans given: of each day the spans take in whose plain file holds a
// request of it, as a summary names its sessions, that file's lines before
// the one where, read from its end, readBackRun lines in a row have ended
// before the day's stop, as stopOn gives it, each ended as it arrived, or
// that and the time it took where that's later.
func (w *spread) leftBehind(now time.Time, id string, spans []ledger.Span) int {
	left := 0
	for _, date := range w.filedDates() {
		stop, spanned := stopOn(spans, date)
		if !spanned {
			continue
		}
		lines := w.filedLines(date)
		if !slices.ContainsFunc(lines, func(l ledger.Line) bool { return l.Session == id && isRequest(&l) }) {
			continue
		}
		run := 0
		for i, l := range slices.Backward(lines) {
			run++
			if !endedBy(&l).Before(stop) {
				run = 0
			}
			if run == readBackRun {
				left += len(slices.DeleteFunc(slices.Clone(lines[:i]), func(l ledger.Line) bool { return l.Session != id || l.At.After(now) }))
				break
			}
		}
	}
	return left
}

// endedBy is when l's request ended, as the ledger's rule has it: when it
// arrived, or that and the time it took where that's later.
func endedBy(l *ledger.Line) time.Time {
	return l.At.Add(time.Duration(max(l.TotalMS, 0)) * time.Millisecond)
}

// stopOn is the stop of the local day with the given date within the spans
// given: the earliest From of those whose dates, as dayfile.DatesBounds
// bounds them, take it in; reporting false of a day none takes in.
func stopOn(spans []ledger.Span, date string) (stop time.Time, spanned bool) {
	for _, s := range spans {
		if first, last := dayfile.DatesBounds(s.From, s.To); first <= date && date <= last && (!spanned || s.From.Before(stop)) {
			stop, spanned = s.From, true
		}
	}
	return stop, spanned
}

// filedLines are the lines of the plain file of the local day with the
// given date, in the order filed: none where it's gone.
func (w *spread) filedLines(date string) []ledger.Line {
	w.t.Helper()
	data, err := os.ReadFile(w.dayPath(date))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		w.t.Fatal(err)
	}
	var lines []ledger.Line
	for row := range bytes.Lines(data) {
		var l ledger.Line
		if err := json.Unmarshal(row, &l); err != nil {
			w.t.Fatalf("%s: a line filed on %s won't read: %v", w.name, date, err)
		}
		lines = append(lines, l)
	}
	return lines
}

// turnOf is the newest turn of the page o, opened at the moment m, of the
// shape given, as it's read raw: the page it's opened on, o, or where the
// shape pends, o opened again as pendingOf has it; its number, from 1 at the
// first; and its tally, as read raw. It reports false where the page holds
// none of the shape.
func (w *spread) turnOf(m moment, o openedPage, shape turnShape) (openedPage, int, *turnTally, bool) {
	turns := o.raw.worked(o.live).tally.turns
	i := len(turns) - 1
	for i >= 0 && !shape.is(w, m, o, turns[i]) {
		i--
	}
	if i < 0 {
		return openedPage{}, 0, nil, false
	}
	if !shape.pends {
		return o, i + 1, turns[i], true
	}
	o = w.pendingOf(m, o, turns[i].prompt)
	tally := o.raw.worked(o.live).tally
	n, ok := tally.prompts[turns[i].prompt]
	return o, n + 1, tally.turns[n], ok && tally.turns[n].pending
}

// pendingOf is the page o opened again at the moment m, as openPage opens
// one, but the stream, live, telling of a request of its own conversation,
// in the turn serving the given prompt, sent ten minutes before, in flight:
// so that turn is pending.
func (w *spread) pendingOf(m moment, o openedPage, prompt string) openedPage {
	told := Told{
		Request: "pending", Session: o.id, Model: foldOpus, Account: "work", Prompt: prompt, Class: ledger.ClassMain, Sent: m.now.Add(-10 * time.Minute),
	}
	told.Heard = told.Sent
	s := Stream{Live: true, Requests: map[string]Told{told.Request: told}}
	read := readingThrough(w.based)
	return openedPage{
		id: o.id, got: read.got, live: PageLive{Routed: Routed(m.running, o.id), Stream: s.Of(o.id), Now: m.now},
		based: ReadPage(read, o.id, s, w.prices.read, 0, m.now), raw: ReadPage(m.raw, o.id, s, w.prices.read, 0, m.now),
	}
}

// sameTurn fails the test where the page o, opened at the moment m, its turn
// numbered n, t as read raw, open, shows otherwise over rows than read raw,
// as turnRead reads its lines through the Follower resting on digests and
// through the one reading raw, given no rows, every line of the session, as
// everyLine has it. It reports whether a day of the turn's own, before
// today, was read from its rows, as daysFromRows has it: from the day
// arrivesWithin before it began through the one its last request ended on,
// or today while it's pending.
func (w *spread) sameTurn(m moment, o openedPage, n int, t *turnTally) bool {
	w.t.Helper()
	noted := readingTurn(w.based)
	based, raw := turnRead(o.based, n, o.live, noted), turnRead(o.raw, n, o.live, everyLine{l: w.raw})
	w.alike(m.now, fmt.Sprintf("%s's page, its turn %d open", o.id, n), shownPageOf(based, o.live), shownPageOf(raw, o.live))
	from, through, today := Today(t.started.Add(-arrivesWithin)).Date, Today(m.now).Date, Today(m.now).Date
	if !t.pending {
		through = Today(t.last.end).Date
	}
	return slices.ContainsFunc(daysFromRows(noted), func(date string) bool { return from <= date && date <= through && date < today })
}
