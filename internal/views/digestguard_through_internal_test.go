package views

import (
	"fmt"
	"iter"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
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
	filteredBeside        = "a past day read through a filter its own summary rules out but one beside it doesn't, read of its rows, none of their chunks"
	filteredToldHidden    = "a past day read whole through a filter hiding the line of a request the stream tells of, filed there, the line held"
	filteredToldElsewhere = "a past day read whole through a filter of an account, the stream telling of a request filed there on another account than its line's"
	filteredToldAcross    = "a past day read whole through a filter hiding the line of a request the stream tells of as sent half a minute into the next, on another account, the next read whole too"
	filteredLate          = "a late line filed onto a day the Log holds through a filter, the day read again whole and taken in through it"
	filteredLateTold      = "a late line of a request the stream tells of, filed onto a day the Log holds through a filter keeping none like it"
	filteredYesterdayRaw  = "at 00:01, the day before read through a filter from its lines, its head yet to be built"
	filteredYesterdayLate = "at 00:59, the day before read through a filter from its lines, its head yet to be built"
	filteredRowsAside     = "a day before yesterday read through a filter from its lines, its head or its rows not standing"
	filteredOtherZone     = "a past day read through a filter from its lines, its head built in another time zone"
	filteredDamaged       = "a past day read through a filter from its lines, its rows standing, a chunk of them the filter chose won't read"
	filteredFacetsApart   = "a chunk read through a filter of an account and a model, its meta counting each, none of its rows of both"
	filteredFailedAlone   = "the failed alone, no past day of lines ruled out by its summaries, a chunk of a day's rows counting no failure skipped by its meta"
	filteredIDAcross      = "a line with an id given twice across an edge of a day's chunks, read through a filter keeping it, both chunks read"
	filteredIdlessAcross  = "a line without an id given twice across an edge of a day's chunks, read through a filter keeping it, both chunks read"
	filteredCopyElsewhere = "a line given twice in a chunk, on one account and then another, read through a filter of the second's alone"
)

// filteredMoments are the hard moments the Log read back through filters
// meets: §5's B moments of the read, but the filter's changes, the find
// and today whole becoming yesterday whole, which the watch's tests hold;
// and 23:59 and prices changed, at which every spread world reads.
var filteredMoments = []string{
	filteredSkipped, filteredRuledOut, filteredBeside, filteredToldHidden, filteredToldElsewhere, filteredToldAcross, filteredLate, filteredLateTold,
	filteredYesterdayRaw, filteredYesterdayLate, filteredRowsAside, filteredOtherZone, filteredDamaged, filteredFacetsApart, filteredFailedAlone,
	filteredIDAcross, filteredIdlessAcross, filteredCopyElsewhere,
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
// pair a spread world, in spreadSeeds' order: each of a pair ticks what the
// other doesn't, so neither serves the other, as crossingOf draws them, and
// a read through either is read again through their join for the other.
// Two sessions join ticking both, so a day is read again through a join
// that keeps fewer than every line. An account and the failed alone are
// crowdedSeed's, as its crowded day's failures alone are read of its
// chunks through the latter, and a line of it given again on another
// account through the former.
var throughPairs = [][2]throughKind{
	{ofASession, ofASession},
	{ofAnAccount, failedAlone},
	{ofASession, ofAModel},
	{ofAnAccountsModel, failedOfASession},
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

// crossingOf is a filter of the kind given, as r draws it of the world's
// ids, that crosses f: f doesn't serve it, nor it f, as serves has it.
// It's drawn again till it is one, failing the test where a hundred draws
// give none.
func (b backThrough) crossingOf(r *rand.Rand, k throughKind, f LogFilter) LogFilter {
	b.w.t.Helper()
	ids := idsOf(b.w.lines)
	for range 100 {
		if c := k.drawn(r, ids); !f.through().serves(c) && !c.through().serves(f) {
			return c
		}
	}
	b.w.t.Fatalf("%s at %v: of a hundred filters drawn, none crosses %+v", b.w.name, b.now, f)
	return LogFilter{}
}

// widerThan is a filter r draws that serves f, as widened draws one, but
// that f doesn't serve: drawn again till it is one, failing the test where
// a hundred draws give none, as of f keeping fewer than every line they
// never should.
func (b backThrough) widerThan(r *rand.Rand, f LogFilter, ids filterIDs) LogFilter {
	b.w.t.Helper()
	for range 100 {
		if wider := widened(r, f, ids); !f.through().serves(wider) {
			return wider
		}
	}
	b.w.t.Fatalf("%s at %v: of a hundred filters drawn serving %+v, it serves each", b.w.name, b.now, f)
	return LogFilter{}
}

// sameFiltered fails the test where the Log read back at the moment m
// through the filters of the world's pair, as throughPairs has it, shows
// through either otherwise than with every day read whole from its lines,
// as Day gives them, as shownThrough has it, or tells otherwise when the
// ledger last wrote, as sameWritten has it: read back to the day three
// before, as threeBack has it, told what throughStream tells, each day's
// lines, summaries and rows read once for every read, as readOnce has
// them. One of the pair, in turn, is shown, drawn of the ids that ids
// gives; the other crosses it, as crossingOf draws it. The one shown is
// read three ways: its past days through it, its store holding none as it
// begins; through a filter wider than it; and through the one crossing it,
// which then serves it not, as
// unserved holds it, then read again where they don't serve it through the
// join of the two, as refiltered reads them, holding each day read again;
// and where a late line was filed at m, read as before it was, then taken
// in, as sameLate has it. Of its past days read through it, the Events are
// held as sameEvents holds them, and the days a request told as sent just
// after a midnight may be filed on, as toldHeld holds them. The first read
// begins with a store holding none; the read with every day whole beside the
// store it fed, and the rest beside the whole read's, as the dashboard's
// store holds what a read before fed it. It notes the hard moments the reads
// through each of the pair meet.
func (w *spread) sameFiltered(m moment) {
	w.t.Helper()
	r := rand.New(rand.NewPCG(w.seed, uint64(m.i)))
	b := backThrough{w: w, l: readingOnce().through(w.based), now: m.now, from: threeBack(m.now), s: w.throughStream(m)}
	ids, turn := b.ids(), (m.i+m.i/4)%2
	pair := throughPairs[slices.Index(spreadSeeds, w.seed)]
	f := pair[turn].drawn(r, ids)
	crossing := b.crossingOf(r, pair[1-turn], f)
	read := b.read(HoldSessions(w.prices.read), f)
	var held HeldSessions
	b.whole, held = readBeside(readRaw{b.l}, read.held, wholly, b.from, b.s, m.now)
	b.toldHeld(read, f)
	across := b.read(held, crossing)
	w.noteThrough(m, b, f, read)
	w.noteThrough(m, b, crossing, across)
	want := b.shown(b.whole, f)
	b.alike("its past days read through it", f, read, want)
	b.alike("its past days read through it", crossing, across, b.shown(b.whole, crossing))
	b.alike("its past days read through a wider one", f, b.read(held, b.widerThan(r, f, ids)), want)
	b.unserved(across, f, crossing)
	refiltered := b.refiltered(across, f, crossing)
	b.alike(fmt.Sprintf("its past days read through %+v, then again through their join", crossing), f, filteredRead{read: refiltered, asked: across.asked}, want)
	b.sameNewSince(across.read, refiltered, f, crossing)
	w.sameLate(b, held, f, want)
	b.sameEvents(read, f, held)
}

// unserved fails the test where fr's read, its past days read through the
// filter crossing, holding a day through it, as filteredOn has it, serves f,
// as Serves has it, or has the Requests shown f built through other than
// crossing, which it served as they were last built, as Through has it.
func (b backThrough) unserved(fr filteredRead, f, crossing LogFilter) {
	w := b.w
	w.t.Helper()
	if len(b.filteredOn(crossing)) == 0 {
		return
	}
	said := fmt.Sprintf("%s at %v: the Log through %+v, its past days read through %+v", w.name, b.now, f, crossing)
	if fr.read.Serves(f) {
		w.t.Errorf("%s, serves it", said)
	}
	if through := fr.read.Through(f, crossing); !through.Equal(crossing) {
		w.t.Errorf("%s, has its Requests built through %+v", said, through)
	}
}

// refiltered is fr's read, its past days read through the filter crossing,
// with each day that doesn't serve f read again, as refilteredAll reads
// them, failing the test where, after each, the day is held through a
// filter that doesn't serve both, as serves has it, as their join does, or
// the read doesn't serve the filter the Requests shown f are built through,
// as Through has it, they last built through crossing.
func (b backThrough) refiltered(fr filteredRead, f, crossing LogFilter) LogRead {
	w := b.w
	w.t.Helper()
	read, held := fr.read, fr.held
	for _, day := range read.Unserved(f) {
		var fresh *HeldSessions
		read, fresh, _ = read.Refiltered(fr.asked, ReadDay(fr.asked, day.Date, read.RefilterThrough(day.Date, f, b.s)), held, b.s, b.now)
		if fresh != nil {
			held = held.With(*fresh)
		}
		said := fmt.Sprintf("%s at %v: the Log through %+v, its past days read through %+v, %s read again", w.name, b.now, f, crossing, day.Date)
		if i, _ := read.dayAt(day.Date); !read.days[i].through.serves(f) || !read.days[i].through.serves(crossing) {
			w.t.Errorf("%s, is held through %+v, which doesn't serve both", said, read.days[i].through)
		}
		if shown := read.Through(f, crossing); !read.Serves(shown) {
			w.t.Errorf("%s, serves not %+v, which its Requests are built through", said, shown)
		}
	}
	return read
}

// sameNewSince fails the test where the Requests built of across, the read
// with its past days read through the filter crossing, paused as they show
// it, then built of refiltered, its days read again where they don't serve
// f, through f, count rows come since they paused, as NewSince counts them,
// other than those of the read with every day whole, paused and shown
// alike: the days read again add none, as no line was taken in since.
func (b backThrough) sameNewSince(across, refiltered LogRead, f, crossing LogFilter) {
	b.w.t.Helper()
	start, _, _ := dayfile.Day(b.from)
	table := func(read LogRead, f LogFilter) LogTable {
		return LogRows(LogSources{From: start, Read: read, Stream: b.s, Filter: f, Now: b.now})
	}
	got := table(refiltered, f).NewSince(table(across, crossing).Mark())
	if want := table(b.whole, f).NewSince(table(b.whole, crossing).Mark()); got != want {
		b.w.t.Errorf("%s at %v: the Log paused through %+v, its past days read through it, then again for %+v, counts %d rows come since; want %d, as with every day whole",
			b.w.name, b.now, crossing, f, got, want)
	}
}

// filteredOn are the dates of the past days the Log read back through the
// filter f, as b reads it, reads through a filter, as ThroughOn has it,
// not whole, the newest first.
func (b backThrough) filteredOn(f LogFilter) []string {
	var dates []string
	for day := dayfile.DayStart(b.now, -1); dayfile.DateOf(day) >= b.from; day = dayfile.DayStart(day, -1) {
		if date := dayfile.DateOf(day); !f.ThroughOn(b.s, date).whole() {
			dates = append(dates, date)
		}
	}
	return dates
}

// ids are the ids the filter shown is drawn of: of the first chunk of a
// past day's rows that b reads back to, spoiled, as spoilChunk spoils one,
// those its meta counts, each choice of one or more, so a filter of any
// kind but the failed alone chooses the chunk; else, where b's stream tells
// of the request acrossMidnight gives, those of the row it tells of, so a
// filter of its account chooses the row and rules out every past day; else
// the world's, as idsOf has them.
func (b backThrough) ids() filterIDs {
	w := b.w
	for _, date := range slices.Sorted(maps.Keys(w.spoiled)) {
		if date < b.from || !w.stillSpoiled(date) {
			continue
		}
		rows, ok := b.l.DayRows(date, RowsVersion)
		if !ok || len(rows.Chunks) == 0 {
			continue
		}
		meta, err := chunkMetaOf(rows.Chunks[0].Meta)
		if err != nil {
			w.t.Fatalf("%s at %v: the meta of the first chunk of %s's rows won't read: %v", w.name, b.now, date, err)
		}
		c := &meta.lines
		ids := filterIDs{accounts: slices.Sorted(maps.Keys(c.Accounts)), models: slices.Sorted(maps.Keys(c.Models)), sessions: slices.Sorted(maps.Keys(c.Sessions))}
		if len(ids.accounts) > 0 && len(ids.models) > 0 && len(ids.sessions) > 0 {
			return ids
		}
	}
	if _, t, ok := b.toldAcrossMidnight(); ok {
		return filterIDs{accounts: []string{t.Account}, models: []string{t.Model}, sessions: []string{t.Session}}
	}
	return idsOf(w.lines)
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
// last wrote otherwise than the latest end of the records it was handed,
// as handedEnd has it: every one counted, kept or dropped, and none it
// passed over unread, whose end no file stores.
func (b backThrough) sameWritten(said string, fr filteredRead) {
	b.w.t.Helper()
	if wrote, want := fr.read.WrittenTo(), fr.asked.handedEnd(); !wrote.Equal(want) {
		b.w.t.Errorf("%s, tells the ledger last wrote at %v, want %v, as the records it was handed end", said, wrote, want)
	}
}

// sameEvents fails the test where the Log's Events built of fr's read, its
// past days read through the filter f, beside fr's store, show otherwise
// than of the read with every day whole from the day after the newest it
// reads through a filter, as filteredOn has them, the days before unread,
// beside the same store; or,
// each day it holds through a filter read again whole, as refilteredAll
// reads them, beside the store held, which holds each session of the read
// with every day whole, show otherwise than of that read, or tell
// otherwise when the ledger last wrote, as sameWritten has it. Each shows
// the events of eventsBy, as eventsShown has them.
func (b backThrough) sameEvents(fr filteredRead, f LogFilter, held HeldSessions) {
	w := b.w
	w.t.Helper()
	happened := ReadEvents(w.eventsBy(b.from, b.now), dayOf(b.from).Start, b.now)
	shown := func(from string, read LogRead, held HeldSessions) []any {
		return eventsShown(LogEventSources{
			From: dayOf(from).Start, Events: happened, Held: held, Read: read, Oldest: OldestDay{Day: dayOf(b.from), Keep: 30 * 24 * time.Hour}, Now: b.now,
		})
	}
	said := fmt.Sprintf("%s at %v: the Log's Events, back to %s", w.name, b.now, b.from)
	first := b.from
	if filtered := b.filteredOn(f); len(filtered) > 0 {
		first = nextDate(filtered[0])
	}
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
// far back as the Log is read through filters, so the day two before,
// whose head and rows the upsets spoil, is read through one while the
// stream tells of a line on the day before and on this one, as
// throughStream has it, and a late line filed three days back, as lateOnto
// files one, is taken in.
func threeBack(now time.Time) string {
	return dayfile.DateOf(dayfile.DayStart(now, -3))
}

// throughStream is what the stream tells the Log read through filters at
// the moment m. On alternate days, by the world's seed, it tells only of a
// request sent half a minute after a past day's midnight, its line filed
// the day before, as acrossMidnight has it, where there's one: so the day
// before now's is read through the filter as any other, unless it's one of
// those two. Else it tells what the Log's stream tells, as logStream has
// it, which reads the day before whole, and of the newest request with an
// id that arrived on the day three before, ended, its line held, and one
// sent as that day ended, its line yet to come, as toldBack tells of them:
// so that day is read whole too, whatever the filter. It tells of each on
// the next of the world's accounts after the one toldBack tells, as
// nextAccount has it, as of a request moved after its line was filed.
func (w *spread) throughStream(m moment) Stream {
	if (m.i/4+int(w.seed))%2 == 0 {
		if _, t, ok := w.acrossMidnight(m.now); ok {
			return Stream{Requests: map[string]Told{t.Request: t}}
		}
		return Stream{}
	}
	s, told := logStream(m), toldBack(w.lines, []string{threeBack(m.now)})
	for id, t := range told.Requests {
		t.Account = w.nextAccount(t.Account)
		told.Requests[id] = t
	}
	maps.Copy(told.Requests, s.Requests)
	s.Requests = told.Requests
	return s
}

// acrossMidnight is the line of a request with an id that arrived in the
// ten minutes before a midnight and was in flight half a minute after it,
// as again's is, and the stream's telling of it as sent then, ended, as
// toldOf tells it, on spareAccount: as one sent within arrivesWithin after
// midnight,
// whose line may be filed on either day, as linedOn has them. It's the
// first such that arrived from the day three before now's on, sent before
// now's day, reporting false where none is.
func (w *spread) acrossMidnight(now time.Time) (ledger.Line, Told, bool) {
	from, today := threeBack(now), Today(now)
	for _, l := range w.lines {
		midnight := dayfile.DayStart(l.At, 1)
		sent := midnight.Add(30 * time.Second)
		if l.Request == "" || !isRequest(&l) || midnight.Sub(l.At) > 10*time.Minute || !ended(&l).After(sent) || Today(l.At).Date < from ||
			!sent.Before(today.Start) {
			continue
		}
		t := toldOf(&l)
		t.Sent, t.Account = sent, spareAccount
		return l, t, true
	}
	return ledger.Line{}, Told{}, false
}

// toldHeld fails the test, where b's stream tells of the request
// acrossMidnight gives, where fr's read, its past days read through f,
// holds through a filter either day its line may be filed on, of those it
// spans: the day before the one it was sent on, as it was sent within
// arrivesWithin after midnight, and that day; or where the read with every
// day whole holds its line, yet shows the row the stream tells of it as
// though it held none, as ShowsTold has it.
func (b backThrough) toldHeld(fr filteredRead, f LogFilter) {
	w := b.w
	w.t.Helper()
	_, t, ok := b.toldAcrossMidnight()
	if !ok {
		return
	}
	said := fmt.Sprintf("%s at %v: %s, told as sent at %v", w.name, b.now, t.Request, t.Sent)
	for _, day := range []LedgerDay{Today(t.Sent.Add(-arrivesWithin)), Today(t.Sent)} {
		if i, held := fr.read.dayAt(day.Date); held && !fr.read.days[i].through.whole() {
			w.t.Errorf("%s, the Log through %+v holds %s through %+v", said, f, day.Date, fr.read.days[i].through)
		}
	}
	held := false
	for l := range b.whole.lines() {
		held = held || t.filedAs(l.flight)
	}
	if held && b.whole.ShowsTold(t, dayOf(b.from)) {
		w.t.Errorf("%s, the Log read with every day whole holds its line, yet shows the row the stream tells of it", said)
	}
}

// spareAccount is the account the stream tells of the request
// acrossMidnight gives on, as of one moved to an account added since: one
// no line of a spread world's names, so a filter of it rules out every
// day by its summaries.
const spareAccount = "spare"

// toldAcrossMidnight is the request acrossMidnight gives at b's moment, its
// line and the stream's telling of it, reporting false where b's stream
// doesn't tell of it so.
func (b backThrough) toldAcrossMidnight() (ledger.Line, Told, bool) {
	l, t, ok := b.w.acrossMidnight(b.now)
	told, tells := b.s.Requests[t.Request]
	return l, t, ok && tells && told.Sent.Equal(t.Sent)
}

// nextAccount is the next of the world's accounts after the one given, as
// idsOf orders them, the first after the last: the one given where the
// world has none of it.
func (w *spread) nextAccount(account string) string {
	accounts := idsOf(w.lines).accounts
	if i := slices.Index(accounts, account); i >= 0 {
		return accounts[(i+1)%len(accounts)]
	}
	return account
}

// toldAcross reports whether, of the request acrossMidnight gives, where
// b's stream tells of it, the line, filed the day before it was sent, is
// held among that day's lines, f hiding it, that day and the next both
// read whole, neither read so through f were the stream to tell of the
// rest alone, as ThroughOn has it; and the former, read through f so, as
// ReadDay reads it, would pass the line over, so it's the day read whole
// that keeps it, f choosing the row the stream tells of, which would show
// were the line passed over.
func (b backThrough) toldAcross(read LogRead, f LogFilter) bool {
	l, t, ok := b.toldAcrossMidnight()
	if !ok {
		return false
	}
	rest := b.s
	rest.Requests = maps.Clone(b.s.Requests)
	delete(rest.Requests, t.Request)
	filed := Today(l.At).Date
	for _, date := range []string{filed, Today(t.Sent).Date} {
		if i, held := read.dayAt(date); !held || !read.days[i].through.whole() || f.ThroughOn(rest, date).whole() {
			return false
		}
	}
	if slices.ContainsFunc(ReadDay(b.l, filed, f.ThroughOn(rest, filed)).records, func(r lineRecord) bool { return t.filedAs(r.flight) }) {
		return false
	}
	i, _ := read.dayAt(filed)
	told := rowChoice{account: t.Account, model: t.Model, session: t.Session}
	for _, line := range read.days[i].lines.all() {
		if t.filedAs(line.flight) && t.Account != line.row.Account && !f.chose(&line.row) && f.chosen(told) {
			return true
		}
	}
	return false
}

// sameLate fails the test where the Log read back through the filter f, as
// b reads it beside the store held, the late lines filed at its moment
// aside, as unfiled has it, so it holds the days they're filed onto as it
// did before they were, then taken in as the watch reads a late day again
// whole, of each day one arrived on that the Log spans, as Day gives its
// lines, shows otherwise than want, what the read with every day whole
// shows through f. Where one of them is of a request with an id, on a day
// the read held through a filter keeping none like it, the stream tells of
// it as they're taken in, as toldLate has it, so the day keeps it, and the
// read is held to the read with every day whole as the stream tells of it
// too. It notes the moments met where the read held one of those days
// through a filter, and where the stream told of a line such a day keeps
// none like. None where no late line was filed then.
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
	if told, ok := w.toldLate(before.read, b.s); ok {
		w.met[filteredLateTold] = true
		b.s = told
		b.whole, _ = readBeside(readRaw{b.l}, held, wholly, b.from, b.s, b.now)
		want = b.shown(b.whole, f)
	}
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

// toldLate is s telling besides of the request of the first late line filed
// at the moment, with an id, on a day read holds through a filter that
// keeps none like it, as the stream tells of one ended, as toldOf tells it:
// reporting false, s as it is, where none is.
func (w *spread) toldLate(read LogRead, s Stream) (Stream, bool) {
	for _, l := range w.filedLate {
		i, held := read.dayAt(Today(l.At).Date)
		if rec := lineRecordOf(&l); l.Request == "" || !held || read.days[i].through.whole() || read.days[i].through.keeps(&rec) {
			continue
		}
		told := s
		told.Requests = maps.Clone(s.Requests)
		if told.Requests == nil {
			told.Requests = make(map[string]Told)
		}
		told.Requests[l.Request] = toldOf(&l)
		return told, true
	}
	return s, false
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
		if d.day.Date >= today.Date {
			continue
		}
		past = append(past, d.day.Date)
		if !d.through.whole() {
			w.noteDayThrough(m, b, f, fr, d.day.Date)
		}
	}
	if f.Failed && len(f.Sessions) == 0 {
		read := !slices.ContainsFunc(w.filedDates(), func(date string) bool { return date >= b.from && date < today.Date && fr.asked.unread(date) })
		w.met[filteredFailedAlone] = w.met[filteredFailedAlone] || read && slices.ContainsFunc(past, fr.asked.skipsAny)
	}
	w.met[filteredToldHidden] = w.met[filteredToldHidden] || toldHidden(fr.read, f, b.s, today)
	w.met[filteredToldElsewhere] = w.met[filteredToldElsewhere] || toldElsewhere(fr.read, f, b.s, today)
	w.met[filteredToldAcross] = w.met[filteredToldAcross] || b.toldAcross(fr.read, f)
}

// noteDayThrough notes the hard moments the past day with the given date,
// held through the filter f at the moment m, as fr asked the ledger of it,
// meets: some of its chunks read and others skipped; none of it read, a day
// of lines; its own summary ruling it out, as b reads it, but not one
// beside it, so it's read of its rows, none of their chunks; read from its
// lines at 00:01 or 00:59, the day before, its head yet to be built; read
// from its lines, another day, its head or rows not standing, its head
// built in another time zone among them, or a chunk of its rows won't
// read; a chunk read whose meta counts f's account and its model, though
// none of its rows is of both, as keptApart has it; and a line given twice
// across an edge of its chunks read, as givenAcross has it.
func (w *spread) noteDayThrough(m moment, b backThrough, f LogFilter, fr filteredRead, date string) {
	a := fr.asked
	yesterday := date == yesterdayOf(Today(m.now))
	unbuilt := yesterday && m.now.Hour() == 0 && a.ofLines(date) && !w.headBuilt(date)
	beside := a.rows[date] != nil && !a.ofLines(date) && len(a.chunks[date]) == 0 && !slices.ContainsFunc(b.l.DaysOn([]string{date}), f.through().mayKeep)
	id, idless := a.givenAcross(date, f)
	for _, moment := range []struct {
		name string
		met  bool
	}{
		{name: filteredSkipped, met: a.skipsSome(date)},
		{name: filteredRuledOut, met: a.unread(date) && w.filedOn(date)},
		{name: filteredBeside, met: beside},
		{name: filteredYesterdayRaw, met: unbuilt && m.now.Minute() < 30},
		{name: filteredYesterdayLate, met: unbuilt && m.now.Minute() >= 30},
		{name: filteredRowsAside, met: !yesterday && a.rowsAside(date)},
		{name: filteredOtherZone, met: a.rowsAside(date) && w.headMade != nil && *w.headMade == headUpset{date: date, moment: foundOtherZone}},
		{name: filteredDamaged, met: a.damaged(date)},
		{name: filteredFacetsApart, met: a.keptApart(date, f)},
		{name: filteredIDAcross, met: id},
		{name: filteredIdlessAcross, met: idless},
		{name: filteredCopyElsewhere, met: a.keptSecond(date, f)},
	} {
		w.met[moment.name] = w.met[moment.name] || moment.met
	}
}

// toldHidden reports whether, of the lines told on days read holds whole,
// as toldOnWhole gives them, one is one whose row the filter f, keeping
// fewer than every line, doesn't choose.
func toldHidden(read LogRead, f LogFilter, s Stream, today LedgerDay) bool {
	for l := range toldOnWhole(read, s, today) {
		if !f.whole() && !f.chose(&l.row) {
			return true
		}
	}
	return false
}

// toldElsewhere reports whether, of the lines told on days read holds
// whole, as toldOnWhole gives them, one is told of on another account than
// its own, the filter f ticking an account.
func toldElsewhere(read LogRead, f LogFilter, s Stream, today LedgerDay) bool {
	for l, t := range toldOnWhole(read, s, today) {
		if len(f.Accounts) > 0 && t.Account != l.row.Account {
			return true
		}
	}
	return false
}

// toldOnWhole are the lines read holds of the days before today it holds
// whole, s telling of a request whose line may be filed there, as
// ThroughOn has it, of the requests s tells of, each with what it tells.
func toldOnWhole(read LogRead, s Stream, today LedgerDay) iter.Seq2[*loggedLine, Told] {
	return func(yield func(*loggedLine, Told) bool) {
		for _, d := range read.days {
			if d.day.Date >= today.Date || !d.through.whole() || !tellsOfLineOn(s, d.day.Date) {
				continue
			}
			for _, l := range d.lines.all() {
				if t, told := s.Requests[l.flight.request]; told && t.filedAs(l.flight) && !yield(l, t) {
					return
				}
			}
		}
	}
}

// daysAsked is a ledger as the Log's read through a filter reads it,
// noting, by date, the days whose rows DayRows was asked for, with the rows
// it gave, none where they don't stand; what RowChunk gave of each chunk of
// a day's rows, by the chunk's offset; and the lines Day gave of each day
// whose lines it read.
type daysAsked struct {
	Ledger
	rows   map[string]*ledger.DayRows
	chunks map[string]map[int64][]byte
	raw    map[string][]ledger.Held
}

// askingDays is l read as daysAsked notes it, nothing asked yet.
func askingDays(l Ledger) *daysAsked {
	return &daysAsked{
		Ledger: l, rows: make(map[string]*ledger.DayRows), chunks: make(map[string]map[int64][]byte), raw: make(map[string][]ledger.Held),
	}
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
	lines := a.Ledger.Day(date)
	a.raw[date] = lines
	return lines
}

// ofLines reports whether the lines of the day with the given date were
// read.
func (a *daysAsked) ofLines(date string) bool {
	_, read := a.raw[date]
	return read
}

// unread reports whether neither the rows of the day with the given date
// nor its lines were asked for.
func (a *daysAsked) unread(date string) bool {
	_, asked := a.rows[date]
	return !asked && !a.ofLines(date)
}

// rowsAside reports whether the day with the given date was read from its
// lines, its rows asked for and not standing.
func (a *daysAsked) rowsAside(date string) bool {
	rows, asked := a.rows[date]
	return asked && rows == nil && a.ofLines(date)
}

// damaged reports whether the day with the given date was read from its
// lines though its rows stood, as where a chunk of them read won't restore.
func (a *daysAsked) damaged(date string) bool {
	return a.rows[date] != nil && a.ofLines(date)
}

// fromRows are the records of the chunks of the rows of the day with the
// given date that were read, by their offsets, each restored, as
// restoredRows restores it, reporting false where the day was read from its
// lines, its rows not standing, or a chunk won't restore, so ReadDay read
// its lines instead.
func (a *daysAsked) fromRows(date string) (map[int64][]lineRecord, bool) {
	if a.rows[date] == nil || a.ofLines(date) {
		return nil, false
	}
	read := make(map[int64][]lineRecord)
	for offset, data := range a.chunks[date] {
		records, err := restoredRows(nil, data)
		if err != nil {
			return nil, false
		}
		read[offset] = records
	}
	return read, true
}

// handedEnd is when the ledger last wrote of what a read through a was
// handed, as WrittenTo has it: the latest end of the records of each day
// it read, those of the day's lines where it read them, else those of the
// chunks of its rows it read, as fromRows has them, in order, but a copy of
// one with an id given before it, which countedOnce passes over as given
// again; zero where it was handed none. A copy without an id is counted as
// it's given, as each ends as its first does in every world.
func (a *daysAsked) handedEnd() time.Time {
	var last time.Time
	later := func(records []lineRecord) {
		given := make(map[lineKey]bool)
		for i := range records {
			if k, id := records[i].key(); id {
				if given[k] {
					continue
				}
				given[k] = true
			}
			last = score.Later(last, records[i].ended())
		}
	}
	for _, lines := range a.raw {
		later(RecordsOf(lines).records)
	}
	for date := range a.chunks {
		chunks, _ := a.fromRows(date)
		var records []lineRecord
		for _, offset := range slices.Sorted(maps.Keys(chunks)) {
			records = append(records, chunks[offset]...)
		}
		later(records)
	}
	return last
}

// skipsAny reports whether the day with the given date was read from its
// rows alone, which stood, some chunk of them never read.
func (a *daysAsked) skipsAny(date string) bool {
	rows := a.rows[date]
	return rows != nil && !a.ofLines(date) && len(a.chunks[date]) < len(rows.Chunks)
}

// skipsSome reports whether the day with the given date was read from its
// rows alone, which stood in more than one chunk, some of them read and
// others never.
func (a *daysAsked) skipsSome(date string) bool {
	rows := a.rows[date]
	return a.skipsAny(date) && len(rows.Chunks) > 1 && len(a.chunks[date]) > 0
}

// keptApart reports whether, of the day with the given date read from its
// rows alone, a chunk was read through the filter f, which ticks both an
// account and a model, its meta counting a row of each, none of its rows
// one f keeps, as keeps has it.
func (a *daysAsked) keptApart(date string, f LogFilter) bool {
	read, ok := a.fromRows(date)
	through := f.through()
	return ok && len(f.Accounts) > 0 && len(f.Models) > 0 && slices.ContainsFunc(slices.Collect(maps.Values(read)), func(records []lineRecord) bool {
		return !slices.ContainsFunc(records, func(r lineRecord) bool { return through.keeps(&r) })
	})
}

// keptSecond reports whether, of the day with the given date read from its
// rows alone, a chunk read holds a line with an id given twice in a row,
// its copies on two accounts, the filter f keeping the second's and not the
// first's, as keeps has it.
func (a *daysAsked) keptSecond(date string, f LogFilter) bool {
	read, _ := a.fromRows(date)
	through := f.through()
	for _, records := range read {
		for i := 1; i < len(records); i++ {
			first, again := &records[i-1], &records[i]
			if first.request != "" && first.request == again.request && first.choice().account != again.choice().account && !through.keeps(first) &&
				through.keeps(again) {
				return true
			}
		}
	}
	return false
}

// givenAcross reports, of the day with the given date read from its rows
// alone, which stood, whether two chunks of them in a row were read, the
// last row of the first the first of the second, a line given twice, which
// the filter f keeps, as keeps has it: one with an id; and one without.
func (a *daysAsked) givenAcross(date string, f LogFilter) (id, idless bool) {
	read, ok := a.fromRows(date)
	if !ok {
		return false, false
	}
	chunks := a.rows[date].Chunks
	for i := 1; i < len(chunks); i++ {
		before, after := read[chunks[i-1].Offset], read[chunks[i].Offset]
		if len(before) == 0 || len(after) == 0 {
			continue
		}
		last, first := before[len(before)-1], after[0]
		if !reflect.DeepEqual(last, first) || !f.through().keeps(&last) {
			continue
		}
		id, idless = id || last.request != "", idless || last.request == ""
	}
	return id, idless
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
