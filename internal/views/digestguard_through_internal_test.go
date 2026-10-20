package views

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// The filters' part of the reads over digests: the Log read back at each
// moment through filters, as the watch reads it, each past day read through
// the filter the page shows, as ThroughOn has it, by ReadDay through the
// Follower resting on digests, held to the same read with every day read
// whole from its lines, as Day gives them, as each shows through the
// filter, as shownThrough has it: its lines, rules, the oldest day's line,
// each row's details, today's count and the days its rows await a line on;
// and when the ledger last wrote. Each filter is read three ways: its past
// days read through it; read through a filter wider than it, which serves
// it; and read through one crossing it, then read again where they don't
// serve it through the join of the two, as the watch refilters them. The
// Log's Events read only the days held whole, and once every day is read
// again whole, all of them. What a read through a filter read is told by
// what the ledger was asked, as daysAsked notes it, never by what it gave:
// a day ruled out is asked for neither its rows nor its lines; a chunk
// skipped is never read.

// The hard moments the Log read back through filters meets, each met by
// some spread world, judged from the reads compared.
const (
	filteredSkipped       = "a past day read through a filter from its rows, standing in more than one chunk, some read and others skipped by their metas"
	filteredRuledOut      = "a past day of lines read through a filter its summaries and those beside it rule out, neither its rows nor its lines read"
	filteredToldHidden    = "a past day read whole through a filter hiding the line of a request the stream tells of, filed there, the line held"
	filteredLate          = "a late line filed onto a day the Log holds through a filter, the day read again whole and taken in through it"
	filteredYesterdayRaw  = "at 00:01, the day before read through a filter from its lines, its head yet to be built"
	filteredYesterdayLate = "at 00:59, the day before read through a filter from its lines, its head yet to be built"
	filteredFailedAlone   = "the failed alone, no past day of lines ruled out by its summaries, a chunk of a day's rows counting no failure skipped by its meta"
	filteredIDAcross      = "a line with an id given twice across an edge of a day's chunks, read through a filter keeping it, both chunks read"
	filteredIdlessAcross  = "a line without an id given twice across an edge of a day's chunks, read through a filter keeping it, both chunks read"
)

// filteredMoments are the hard moments the Log read back through filters
// meets.
var filteredMoments = []string{
	filteredSkipped, filteredRuledOut, filteredToldHidden, filteredLate, filteredYesterdayRaw, filteredYesterdayLate, filteredFailedAlone, filteredIDAcross,
	filteredIdlessAcross,
}

// throughKind is a kind of filter the Log is read through: the filter
// ticking what the kind ticks of a world's ids, as r draws them, ticking no
// kind, which drawn draws.
type throughKind func(r *rand.Rand, ids filterIDs) LogFilter

// The kinds of filter the Log is read through.
var (
	ofASession throughKind = func(r *rand.Rand, ids filterIDs) LogFilter {
		return LogFilter{Sessions: oneOf(r, ids.sessions)}
	}
	ofAnAccount throughKind = func(r *rand.Rand, ids filterIDs) LogFilter {
		return LogFilter{Accounts: oneOf(r, ids.accounts)}
	}
	ofAModel throughKind = func(r *rand.Rand, ids filterIDs) LogFilter {
		return LogFilter{Models: oneOf(r, ids.models)}
	}
	ofAnAccountsModel throughKind = func(r *rand.Rand, ids filterIDs) LogFilter {
		return LogFilter{Accounts: oneOf(r, ids.accounts), Models: oneOf(r, ids.models)}
	}
	failedAlone throughKind = func(*rand.Rand, filterIDs) LogFilter {
		return LogFilter{Failed: true}
	}
	failedOfASession throughKind = func(r *rand.Rand, ids filterIDs) LogFilter {
		return LogFilter{Sessions: oneOf(r, ids.sessions), Failed: true}
	}
)

// throughPairs are the pairs of kinds of filter the Log is read through, a
// pair a spread world, by its seed: each of a pair ticks what the other
// doesn't, so neither serves the other, whatever ids they tick, and a read
// through either is read again through their join for the other. The
// failed alone are crowdedSeed's, as its crowded day's failures alone are
// read of its chunks through them.
var throughPairs = [][2]throughKind{
	{ofASession, ofAnAccount},
	{ofAnAccountsModel, failedOfASession},
	{ofAModel, failedAlone},
}

// oneOf is a choice ticking one of the ids given, as r draws it.
func oneOf(r *rand.Rand, ids []string) []string {
	return []string{ids[r.IntN(len(ids))]}
}

// drawn is a filter of the kind given as r draws it of the ids given, of
// any kinds, as drawnFilter draws them.
func (k throughKind) drawn(r *rand.Rand, ids filterIDs) LogFilter {
	f := k(r, ids)
	f.Kinds = slices.DeleteFunc(RowKinds(), func(RowKind) bool { return r.IntN(2) == 0 })
	return f
}

// widerThan is a filter r draws that serves f, as widened draws one, but
// that f doesn't serve: drawn again till it is one. f keeps fewer than
// every line, so some is.
func widerThan(r *rand.Rand, f LogFilter, ids filterIDs) LogFilter {
	for {
		if w := widened(r, f, ids); !f.through().serves(w) {
			return w
		}
	}
}

// sameFiltered fails the test where the Log read back at the moment m
// through the filters of the world's pair, as throughPairs has it, each
// drawn of its ids then, shows through either otherwise than with every day
// read whole from its lines, as Day gives them, as shownThrough has it, or
// tells otherwise when the ledger last wrote, as sameWritten has it: read
// back to the day three before, as threeBack has it, told what
// throughStream tells, each day's lines, summaries and rows read once for
// every read, as readOnce has them. One of the pair, in turn, is shown, the
// other crossing it. The one shown is read three ways: its past days
// through it, its store holding none as it begins; through a filter wider
// than it; and through the one crossing it, then read again where they
// don't serve it through the join of the two, as refilteredAll reads them;
// and where a late line was filed at m, read as before it was, then taken
// in, as sameLate has it. Of its past days read through it, the Events are
// held as sameEvents holds them. The read with every day whole, and the one
// shown through it, begin with stores holding none, each feeding its own;
// the rest begin beside the former's, as the dashboard's store holds what a
// read before fed it. It notes the hard moments the reads through each of
// the pair meet.
func (w *spread) sameFiltered(m moment) {
	w.t.Helper()
	r := rand.New(rand.NewPCG(w.seed, uint64(m.i)))
	ids, turn := idsOf(w.lines), (m.i+m.i/4)%2
	pair := throughPairs[w.seed%uint64(len(throughPairs))]
	f, crossing := pair[turn].drawn(r, ids), pair[1-turn].drawn(r, ids)
	b := backThrough{w: w, l: readingOnce().through(w.based), now: m.now, from: threeBack(m.now), s: w.throughStream(m)}
	var held HeldSessions
	b.whole, held = readHeldBack(readRaw{b.l}, wholly, b.from, b.s, w.prices.read, m.now)
	read, across := b.read(HoldSessions(w.prices.read), f), b.read(held, crossing)
	w.noteThrough(m, b, f, read)
	w.noteThrough(m, b, crossing, across)
	want := b.shown(b.whole, f)
	b.alike("its past days read through it", f, read, want)
	b.alike("its past days read through it", crossing, across, b.shown(b.whole, crossing))
	b.alike("its past days read through a wider one", f, b.read(held, widerThan(r, f, ids)), want)
	refiltered, _ := refilteredAll(across.asked, across.read, across.held, f, b.s, m.now)
	b.alike(fmt.Sprintf("its past days read through %+v, then again through their join", crossing), f, filteredRead{read: refiltered, asked: across.asked}, want)
	w.sameLate(b, held, f, want)
	b.sameEvents(read, held)
}

// wholly is the filter every day is read whole through, whatever its date.
func wholly(string) LogFilter {
	return LogFilter{}
}

// backThrough is the Log read back through filters at a moment of the
// world w's, as sameFiltered reads it: through l; at now; back to the day
// with the date from; told what s tells, as throughStream has it; and the
// read with every day whole from its lines, whole.
type backThrough struct {
	w     *spread
	l     Ledger
	now   time.Time
	from  string
	s     Stream
	whole LogRead
}

// filteredRead is a read of the Log back through a filter: what it holds,
// the store its rows are told by, and what it asked of the ledger, as
// daysAsked notes it.
type filteredRead struct {
	read  LogRead
	held  HeldSessions
	asked *daysAsked
}

// read is the Log read back through b's ledger, each past day through the
// filter f shows, as ThroughOn has it of the day, beside the store held, as
// readBeside reads it, what it asks of the ledger noted.
func (b backThrough) read(held HeldSessions, f LogFilter) filteredRead {
	asked := askingDays(b.l)
	return b.readOf(asked, asked, held, f)
}

// readOf is the Log read back as read reads it, but through l, which asks
// of the ledger through asked, which notes it.
func (b backThrough) readOf(l Ledger, asked *daysAsked, held HeldSessions, f LogFilter) filteredRead {
	read, held := readBeside(l, held, func(date string) LogFilter { return f.ThroughOn(b.s, date) }, b.from, b.s, b.now)
	return filteredRead{read: read, held: held, asked: asked}
}

// shown is what the Log shows of read through the filter f, as
// shownThrough has it.
func (b backThrough) shown(read LogRead, f LogFilter) []any {
	return shownThrough(b.l, read, f, b.from, b.s, b.w.prices.read, b.now)
}

// alike fails the test where fr's read, read back as how says, doesn't
// serve f, as Serves has it, shows through f otherwise than want, what the
// read with every day whole shows through it, or tells otherwise when the
// ledger last wrote, as sameWritten has it.
func (b backThrough) alike(how string, f LogFilter, fr filteredRead, want []any) {
	w := b.w
	w.t.Helper()
	said := fmt.Sprintf("%s at %v: the Log through %+v, %s, back to %s", w.name, b.now, f, how, b.from)
	if !fr.read.Serves(f) {
		w.t.Errorf("%s, serves it not", said)
		return
	}
	if got := b.shown(fr.read, f); !reflect.DeepEqual(got, want) {
		w.t.Errorf("%s, shows otherwise than with every day whole\n%s", said, unlike(got, want))
	}
	b.sameWritten(said, fr)
}

// sameWritten fails the test where fr's read, as said, tells the ledger
// last wrote otherwise than the read with every day whole tells it, as
// WrittenTo has them: but for earlier where each line of the days from b's
// on that arrived by now and ended after it arrived on a day fr asked the
// ledger for none or some of, never all, as readAll has it, as no file
// stores when a line passed over unread ended.
func (b backThrough) sameWritten(said string, fr filteredRead) {
	b.w.t.Helper()
	wrote, want := fr.read.WrittenTo(), b.whole.WrittenTo()
	if wrote.Equal(want) || wrote.Before(want) && b.passedOver(fr.asked, wrote) {
		return
	}
	b.w.t.Errorf("%s, tells the ledger last wrote at %v, want %v, as with every day whole", said, wrote, want)
}

// passedOver reports whether each line of the days from b's on that arrived
// by b's now and ended after the time given, as endedBy has it, arrived on
// a day asked noted none or some of read, never all, as readAll has it.
func (b backThrough) passedOver(asked *daysAsked, after time.Time) bool {
	return !slices.ContainsFunc(b.w.lines, func(l ledger.Line) bool {
		date := Today(l.At).Date
		return date >= b.from && !l.At.After(b.now) && endedBy(&l).After(after) && asked.readAll(date)
	})
}

// sameEvents fails the test where the Log's Events built of fr's read, its
// past days read through a filter, beside fr's store, show otherwise than
// of the read with every day whole from the first day they read of it, as
// its wholeFrom has it, the days before unread, beside the same store; or,
// each day it holds through a filter read again whole, as refilteredAll
// reads them, beside the store held, which holds each session of the read
// with every day whole, show otherwise than of that read, or tell
// otherwise when the ledger last wrote, as sameWritten has it. Each shows
// the events of eventsBy, as eventsShown has them.
func (b backThrough) sameEvents(fr filteredRead, held HeldSessions) {
	w := b.w
	w.t.Helper()
	happened := ReadEvents(w.eventsBy(b.from, b.now), dayOf(b.from).Start, b.now)
	shown := func(from string, read LogRead, held HeldSessions) []any {
		return eventsShown(LogEventSources{
			From: dayOf(from).Start, Events: happened, Held: held, Read: read, Oldest: OldestDay{Day: dayOf(b.from), Keep: 30 * 24 * time.Hour}, Now: b.now,
		})
	}
	said := fmt.Sprintf("%s at %v: the Log's Events, back to %s", w.name, b.now, b.from)
	first := max(b.from, fr.read.wholeFrom().Date)
	without, _ := readBeside(readRaw{b.l}, fr.held, wholly, first, b.s, b.now)
	if got, want := shown(first, fr.read, fr.held), shown(first, without, fr.held); !reflect.DeepEqual(got, want) {
		w.t.Errorf("%s, of a read holding a day before %s through a filter, show otherwise than of the read whole from it\n%s", said, first, unlike(got, want))
	}
	refiltered, _ := refilteredAll(fr.asked, fr.read, held, LogFilter{}, b.s, b.now)
	if got, want := shown(b.from, refiltered, held), shown(b.from, b.whole, held); !reflect.DeepEqual(got, want) {
		w.t.Errorf("%s, each day held through a filter read again whole, show otherwise than of the read with every day whole\n%s", said, unlike(got, want))
	}
	b.sameWritten(said+", each day held through a filter read again whole", filteredRead{read: refiltered, asked: fr.asked})
}

// eventsBy are the router's files of events as they stand at now: of each
// day from the one with the date from, half a minute into it, where a
// request arrived before then, the account the last that did went out on
// reaching its cap, and its session moved from there to another account of
// the world's, a second later. So a cap's card counts the sessions of the
// day before's last hour.
func (w *spread) eventsBy(from string, now time.Time) eventFiles {
	run, accounts := dayOf(from).Start, idsOf(w.lines).accounts
	var filed eventFiles
	for day := run; ; day = dayfile.DayStart(day, 1) {
		at := day.Add(30 * time.Second)
		if at.After(now) {
			return filed
		}
		var last *ledger.Line
		for i := range w.lines {
			if l := &w.lines[i]; isRequest(l) && l.At.Before(at) && (last == nil || l.At.After(last.At)) {
				last = l
			}
		}
		if last == nil {
			continue
		}
		to := accounts[(slices.Index(accounts, last.Account)+1)%len(accounts)]
		for _, e := range []status.Event{
			{At: at, Kind: status.EventCap, Account: last.Account},
			{At: at.Add(time.Second), Kind: status.EventMoved, Session: last.Session, From: last.Account, To: to},
		} {
			e.ID = len(filed) + 1
			filed = append(filed, events.Line{Event: e, Run: run})
		}
	}
}

// threeBack is the date of the day three before the one now falls on: as
// far back as the Log is read through filters, so a day before is read
// through one while the stream tells of a line on each of the two days
// before, as throughStream has it, and a late line filed three days back,
// as lateOnto files one, is taken in.
func threeBack(now time.Time) string {
	return dayfile.DateOf(dayfile.DayStart(now, -3))
}

// throughStream is what the stream tells the Log read through filters at
// the moment m: on alternate days, by the world's seed, nothing, so the day
// before is read through the filter as any other; else what the Log's
// stream tells, as logStream has it, which reads the day before whole, and
// of the newest request with an id that arrived on the day two before,
// ended, its line held, and one sent as that day ended, its line yet to
// come, as toldBack tells of them: so that day is read whole too, whatever
// the filter.
func (w *spread) throughStream(m moment) Stream {
	if (m.i/4+int(w.seed))%2 == 0 {
		return Stream{}
	}
	s, told := logStream(m), toldBack(w.lines, []string{twoBack(m.now)})
	maps.Copy(told.Requests, s.Requests)
	s.Requests = told.Requests
	return s
}

// sameLate fails the test where the Log read back through the filter f, as
// b reads it beside the store held, the late lines filed at its moment
// aside, as unfiled has it, so it holds the days they're filed onto as it
// did before they were, then taken in as the watch reads a late day again
// whole, of each day one arrived on that the Log spans, as Day gives its
// lines, shows otherwise than want, what the read with every day whole
// shows through f. It notes the moment met where the read held one of those
// days through a filter. None where no late line was filed then.
func (w *spread) sameLate(b backThrough, held HeldSessions, f LogFilter, want []any) {
	w.t.Helper()
	var dates []string
	for _, l := range w.filedLate {
		if date := Today(l.At).Date; date >= b.from {
			dates = append(dates, date)
		}
	}
	if len(dates) == 0 {
		return
	}
	asked := askingDays(b.l)
	before := b.readOf(unfiled{Ledger: asked, lines: w.filedLate}, asked, held, f)
	taking := LogTaking{Held: before.held, Stream: b.s, Now: b.now}
	read := before.read
	for _, date := range slices.Compact(slices.Sorted(slices.Values(dates))) {
		if i, held := read.dayAt(date); held && !read.days[i].through.whole() {
			w.met[filteredLate] = true
		}
		read = read.Taken(read.RecordsToTake(asked.Day(date)), true, taking)
	}
	b.alike(fmt.Sprintf("read before the late lines on %v were filed, then taken in", dates), f, filteredRead{read: read, asked: asked}, want)
}

// noteThrough notes the hard moments the Log read back through the filter
// f at the moment m, as b reads it, meets, as fr asked the ledger: of each
// past day, as noteDayThrough has them; of the failed alone, every past day
// of lines read, its rows or its lines, and some day's chunk skipped; and a
// day read whole, the stream telling of a request filed there whose line f
// hides, as toldHidden has it.
func (w *spread) noteThrough(m moment, b backThrough, f LogFilter, fr filteredRead) {
	today := Today(m.now)
	var past []string
	for _, d := range fr.read.days {
		if d.day.Date < today.Date {
			past = append(past, d.day.Date)
			w.noteDayThrough(m, f, fr, d.day.Date)
		}
	}
	if f.Failed && len(f.Sessions) == 0 {
		read := !slices.ContainsFunc(w.filedDates(), func(date string) bool { return date >= b.from && date < today.Date && fr.asked.unread(date) })
		w.met[filteredFailedAlone] = w.met[filteredFailedAlone] || read && slices.ContainsFunc(past, fr.asked.skipsAny)
	}
	w.met[filteredToldHidden] = w.met[filteredToldHidden] || toldHidden(fr.read, f, b.s, today)
}

// noteDayThrough notes the hard moments the past day with the given date,
// read through the filter f at the moment m, as fr asked the ledger of it,
// meets: some of its chunks read and others skipped; none of it read, a day
// of lines; read from its lines at 00:01 or 00:59, the day before, its head
// yet to be built; and a line given twice across an edge of its chunks
// read, as givenAcross has it.
func (w *spread) noteDayThrough(m moment, f LogFilter, fr filteredRead, date string) {
	a := fr.asked
	yesterday := date == yesterdayOf(Today(m.now)) && m.now.Hour() == 0 && a.raw[date] && !w.headBuilt(date)
	id, idless := a.givenAcross(date, f)
	for _, moment := range []struct {
		name string
		met  bool
	}{
		{name: filteredSkipped, met: a.skipsSome(date)},
		{name: filteredRuledOut, met: a.unread(date) && w.filedOn(date)},
		{name: filteredYesterdayRaw, met: yesterday && m.now.Minute() < 30},
		{name: filteredYesterdayLate, met: yesterday && m.now.Minute() >= 30},
		{name: filteredIDAcross, met: id},
		{name: filteredIdlessAcross, met: idless},
	} {
		w.met[moment.name] = w.met[moment.name] || moment.met
	}
}

// toldHidden reports whether read holds a day before today whole, s telling
// of a request whose line may be filed there, as ThroughOn has it, the line
// of one it tells of among its lines, whose row the filter f, keeping fewer
// than every line, doesn't choose.
func toldHidden(read LogRead, f LogFilter, s Stream, today LedgerDay) bool {
	if f.whole() {
		return false
	}
	for _, d := range read.days {
		if d.day.Date >= today.Date || !d.through.whole() || !tellsOfLineOn(s, d.day.Date) {
			continue
		}
		for _, l := range d.lines.all() {
			if _, told := s.sentOf(l.flight); told && !f.chose(&l.row) {
				return true
			}
		}
	}
	return false
}

// daysAsked is a ledger as the Log's read through a filter reads it,
// noting, by date, the days whose rows DayRows was asked for, with the rows
// it gave, none where they don't stand; what RowChunk gave of each chunk of
// a day's rows, by the chunk's offset; and the days whose lines Day read.
type daysAsked struct {
	Ledger
	rows   map[string]*ledger.DayRows
	chunks map[string]map[int64][]byte
	raw    map[string]bool
}

// askingDays is l read as daysAsked notes it, nothing asked yet.
func askingDays(l Ledger) *daysAsked {
	return &daysAsked{Ledger: l, rows: make(map[string]*ledger.DayRows), chunks: make(map[string]map[int64][]byte), raw: make(map[string]bool)}
}

func (a *daysAsked) DayRows(date string, body int) (ledger.DayRows, bool) {
	rows, ok := a.Ledger.DayRows(date, body)
	a.rows[date] = nil
	if ok {
		a.rows[date] = &rows
	}
	return rows, ok
}

func (a *daysAsked) RowChunk(date string, chunk ledger.RowsChunk) ([]byte, error) {
	data, err := a.Ledger.RowChunk(date, chunk)
	if a.chunks[date] == nil {
		a.chunks[date] = make(map[int64][]byte)
	}
	a.chunks[date][chunk.Offset] = data
	return data, err
}

func (a *daysAsked) Day(date string) []ledger.Held {
	a.raw[date] = true
	return a.Ledger.Day(date)
}

// unread reports whether neither the rows of the day with the given date
// nor its lines were asked for.
func (a *daysAsked) unread(date string) bool {
	_, asked := a.rows[date]
	return !asked && !a.raw[date]
}

// readAll reports whether every line of the day with the given date was
// read: its lines, or each chunk of its rows, which stood.
func (a *daysAsked) readAll(date string) bool {
	rows := a.rows[date]
	return a.raw[date] || rows != nil && len(a.chunks[date]) == len(rows.Chunks)
}

// skipsAny reports whether the day with the given date was read from its
// rows alone, which stood, some chunk of them never read.
func (a *daysAsked) skipsAny(date string) bool {
	rows := a.rows[date]
	return rows != nil && !a.raw[date] && len(a.chunks[date]) < len(rows.Chunks)
}

// skipsSome reports whether the day with the given date was read from its
// rows alone, which stood in more than one chunk, some of them read and
// others never.
func (a *daysAsked) skipsSome(date string) bool {
	rows := a.rows[date]
	return a.skipsAny(date) && len(rows.Chunks) > 1 && len(a.chunks[date]) > 0
}

// givenAcross reports, of the day with the given date read from its rows
// alone, which stood, whether two chunks of them in a row were read, the
// last row of the first the first of the second, a line given twice, which
// the filter f keeps, as keeps has it: one with an id; and one without.
func (a *daysAsked) givenAcross(date string, f LogFilter) (id, idless bool) {
	rows := a.rows[date]
	if rows == nil || a.raw[date] {
		return false, false
	}
	read := a.chunks[date]
	for i := 1; i < len(rows.Chunks); i++ {
		before, after := read[rows.Chunks[i-1].Offset], read[rows.Chunks[i].Offset]
		if before == nil || after == nil {
			continue
		}
		last, first, ok := edgeRecords(before, after)
		if !ok || !reflect.DeepEqual(last, first) || !f.through().keeps(&last) {
			continue
		}
		id, idless = id || last.request != "", idless || last.request == ""
	}
	return id, idless
}

// edgeRecords are the last record the rows of one chunk restore and the
// first the next's do, reporting false where either restores none.
func edgeRecords(before, after []byte) (last, first lineRecord, ok bool) {
	b, errB := restoredRows(nil, before)
	n, errN := restoredRows(nil, after)
	if errB != nil || errN != nil || len(b) == 0 || len(n) == 0 {
		return lineRecord{}, lineRecord{}, false
	}
	return b[len(b)-1], n[0], true
}

// readRaw reads through l as a Follower of a ledger holding no day's rows
// does: every day's lines raw, as Day gives them, so a day read whole
// through it is of its lines.
type readRaw struct {
	Ledger
}

func (readRaw) DayRows(string, int) (ledger.DayRows, bool) {
	return ledger.DayRows{}, false
}

// unfiled reads through l as it read before the lines given were filed:
// each day's lines as Day gives them, theirs aside, known by their flights.
// A day a line is filed onto late has rows that don't stand, so it's read
// of its lines.
type unfiled struct {
	Ledger
	lines []ledger.Line
}

func (u unfiled) Day(date string) []ledger.Held {
	return slices.DeleteFunc(slices.Clone(u.Ledger.Day(date)), func(h ledger.Held) bool {
		return slices.ContainsFunc(u.lines, func(l ledger.Line) bool { return flightOf(&l) == flightOf(&h.Line) })
	})
}
