package views

import (
	"iter"
	"maps"
	"slices"
	"strings"
	"time"
	"unique"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
)

// LogRead is a read of the ledger for the Log's Requests, grown as the
// ledger is read and never read again whole: the lines of the days the
// table spans, by day, each decoded once into the Log's line as it's taken
// in, priced once, at the prices of the day it's taken in on, placed once,
// and its row told once of its session's story, as the store holds it, and
// told again only as the store says that story changed. A row's story is
// never told from the table's lines alone, which would make a session that
// began the day before new today, and a move after a night's idle cost its
// context written again. What grows it makes it afresh, sharing with it what
// it doesn't change, so a frame holding it never sees it change.
type LogRead struct {
	prices ledger.Table
	// price, where it's set, prices a line's request in place of its
	// record's worth, as a test counts the lines priced.
	price func(r *lineRecord, date string) Worth
	// days are the days it holds lines of, the oldest first; seq is how many
	// lines it has taken in, the last's seq; and written the latest end of
	// its lines.
	days    []loggedDay
	seq     int
	written time.Time
	// grew tells one read from another the same, as a table built of one
	// takes in what the other gained: none of a read never grown; gen counts
	// its changes; and changes are its last few, the lines each told again.
	grew    *int
	gen     int
	changes []readChange
}

// loggedDay is a day of the Log's read: its lines, in the order taken in,
// as their seqs have them; their keys, by which a line given again is
// known, and a session's lines found; and the handles of their words, as
// interning holds them, in layers, as stacked has them.
type loggedDay struct {
	day   LedgerDay
	lines chunked[*loggedLine]
	keys  *stacked[loggedKeys]
	names *stacked[interning]
}

// loggedKeys are the keys of a day's lines, as loggedDay holds them: those
// with ids, by id, each with its flight, and its seq; those without, by
// their session and when they arrived, as idlessAt has them, each with the
// sum of its record, as its summed has it, and its seq; and the seqs of each
// session's requests, by its id.
type loggedKeys struct {
	byID      map[string][]loggedKey
	idless    map[idlessAt][]loggedKey
	bySession map[string][]int
}

// idlessAt is where a line without an id is known among a day's: the
// session it's of, and when it arrived, as two sessions' lines may arrive
// at one moment.
type idlessAt struct {
	session string
	at      instant
}

// loggedKey is a line as loggedKeys knows it.
type loggedKey struct {
	flight flight
	sum    uint64
	seq    int
}

// readChange is a change of a read's, the genth: the lines it told again,
// each as it is told now.
type readChange struct {
	gen    int
	retold []*loggedLine
}

// readChangesKept are how many of its changes a read keeps, for a table
// built of it to take in: one built further back is built afresh.
const readChangesKept = 8

// ReadLog is a read for the Log's Requests holding no line, its lines
// priced by prices, at the day each is taken in on.
func ReadLog(prices ledger.Table) LogRead {
	return LogRead{prices: prices}
}

// LogTaking is what the Log's read takes lines in by: the store fed with
// them, as HeldSessions holds it, which tells each row's story; what the
// feed changed of each session, as SessionGains has it, whose rows the read
// tells again; what the request stream told, which places each row as
// LogLinesOf has it; and now, its day's prices pricing each line.
type LogTaking struct {
	Held   HeldSessions
	Gains  SessionGains
	Stream Stream
	Now    time.Time
}

// Taken is r with the lines of day taken in, each of its record, as t says,
// and the rows of r's lines told again whose sessions' stories the store's
// feed changed, as t's Gains have them: of a session read whole, every one.
// The lines are a tail of today's, as Ledger's Today gives it, or, whole
// where they're every one of a day's, as Today gives them afresh, or Day
// gives a day's, as the tables reach back a day, or a day is read again
// whole; or either but the lines with ids r holds, as Unheld gives them,
// which leaves every line without one, so whole holds of it still. A line
// with an id that r holds already is passed over, and one given twice that
// r holds none of is taken in once; one with none is taken in
// as often as it's given, but for one whole, which is taken in only as often
// as it's given beyond those of its day, arriving at its moment, its record
// saying the same, that r holds already. So a read costs what the ledger
// gained, never what r holds.
func (r LogRead) Taken(day LogDay, whole bool, t LogTaking) LogRead {
	grown := r
	grown.days = slices.Clone(r.days)
	took := grown.take(day, whole, t)
	retold := grown.retell(t, r.seq)
	if !took && len(retold) == 0 {
		return r
	}
	return grown.changed(retold)
}

// TakenBack is r with day taken in whole, every one of a day's lines'
// records, as ReadDay gives the day before those the Log's tables span, at
// now: the sessions of its requests held doesn't hold read whole through l
// into a store of their own, as Fed reads them, wanting them alone, each
// pinned as the stream s tells, at r's prices; each line placed as s tells
// and its row told of the stories held and that store hold, as Taken takes
// a day. It gives the store beside it, for the watch's to take in: nil
// where held holds every one of the day's sessions.
func (r LogRead) TakenBack(l LedgerSessions, day LogDay, held HeldSessions, s Stream, now time.Time) (LogRead, *HeldSessions) {
	var found *HeldSessions
	telling := held
	if ids := held.Unheld(ReadLog(r.prices).Wanted(day)); len(ids) > 0 {
		want := StreamWanted(s)
		want.Log = ids
		fresh, _ := HoldSessions(r.prices).Fed(l, Taken{}, want, now)
		found, telling = &fresh, held.With(fresh)
	}
	return r.Taken(day, true, LogTaking{Held: telling, Stream: s, Now: now}), found
}

// From is r holding none of the days before the day given, as the Log no
// longer shows them.
func (r LogRead) From(day LedgerDay) LogRead {
	i := slices.IndexFunc(r.days, func(d loggedDay) bool { return d.day.Date >= day.Date })
	if i == 0 || len(r.days) == 0 {
		return r
	}
	if i < 0 {
		i = len(r.days)
	}
	r.days = slices.Clone(r.days[i:])
	return r.changed(nil)
}

// Same reports whether r and o are the same read: o grown into r, or r into
// o, by nothing.
func (r LogRead) Same(o LogRead) bool {
	return r.Gen() == o.Gen()
}

// LogGen is which read a LogRead is, as Same knows it, holding none of its
// lines: two reads are the same where their gens are.
type LogGen struct {
	grew *int
	gen  int
}

// Gen is which read r is, as LogGen has it.
func (r LogRead) Gen() LogGen {
	return LogGen{grew: r.grew, gen: r.gen}
}

// Wanted are the ids of the sessions the Log wants the store to hold, in
// order, each once: those of the requests r holds lines of, and of those
// among the days' lines given.
func (r LogRead) Wanted(days ...LogDay) []string {
	var ids []string
	for _, d := range r.days {
		for layer := d.keys; layer != nil; layer = layer.under {
			ids = slices.AppendSeq(ids, maps.Keys(layer.top.bySession))
		}
	}
	for _, given := range days {
		for i := range given.records {
			if rec := &given.records[i]; rec.isRequest() {
				ids = append(ids, rec.session)
			}
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// Unheld is the LogDay of those of lines r may take in, each its record, in
// their order: every one but those r holds already, as holdsFlight knows
// them, which Taken would pass over, so a day read again whole decodes only
// what it gained. Those it keeps are found first, so the records take only
// the room they need, as a day read again whole needs little.
func (r LogRead) Unheld(lines []ledger.Held) LogDay {
	var unheld []int
	for i := range lines {
		if !r.holdsFlight(flightOf(&lines[i].Line)) {
			unheld = append(unheld, i)
		}
	}
	day := LogDay{records: make([]lineRecord, len(unheld))}
	for j, i := range unheld {
		day.records[j] = lineRecordOf(&lines[i].Line)
	}
	return day
}

// holdsFlight reports whether r holds the line of the request that flew as
// f, as Taken knows one with an id, by its flight's key, among the lines of
// the day it arrived on, as idHeld tells it: never of one without, which
// Taken knows by its record's sum.
func (r LogRead) holdsFlight(f flight) bool {
	k, ok := f.idKey()
	if !ok {
		return false
	}
	i, found := r.dayAt(Today(f.arrived()).Date)
	return found && idHeld(k, idsHeld(r.days[i].keys, k.lineKey))
}

// Len is how many lines r holds: it changes only as r grows.
func (r LogRead) Len() int {
	n := 0
	for _, d := range r.days {
		n += d.lines.len()
	}
	return n
}

// WrittenTo is when the ledger last wrote of what r holds: the latest end of
// its lines, every kind's, each its arrival and its total_ms after; zero
// where it holds none. The Log not live is to it.
func (r LogRead) WrittenTo() time.Time {
	return r.written
}

// changed is r with a change of its own noted, the lines given told again
// by it.
func (r LogRead) changed(retold []*loggedLine) LogRead {
	if r.grew == nil {
		r.grew = new(int)
	}
	r.gen++
	r.changes = append(slices.Clone(r.changes[max(len(r.changes)-readChangesKept+1, 0):]), readChange{gen: r.gen, retold: retold})
	return r
}

// take takes in the lines of day r doesn't hold, as Taken has it, each
// priced at t's now, placed as t's stream tells, and told of its session's
// story as t's store holds it, reporting whether it took any. r's days are
// its own, but none of them.
func (r *LogRead) take(day LogDay, whole bool, t LogTaking) bool {
	today := Today(t.Now)
	read := countedOnceOf(len(day.records))
	added := make(map[string]*takenDay)
	for i := range day.records {
		rec := &day.records[i]
		at := r.dayOf(Today(rec.arrived()))
		if k := turnKeyOf(rec); read.again(k, loggedHeld(r.days[at].keys, rec.session, k, whole)) {
			continue
		}
		r.seq++
		date := r.days[at].day.Date
		if added[date] == nil {
			added[date] = &takenDay{names: make(interning)}
		}
		a := added[date]
		logged := loggedLineOf(rec, r.seq, placeOf(rec, t.Stream), r.worthOf(rec, today.Date), r.storyOf(rec, t.Held), a.names)
		a.lines = append(a.lines, logged)
		a.keys.add(logged)
		r.written = score.Later(r.written, rec.ended())
	}
	for i := range r.days {
		if a := added[r.days[i].day.Date]; a != nil {
			r.days[i].took(a)
		}
	}
	return len(added) > 0
}

// takenDay is what a read took in of a day's lines: the Log's lines, in the
// order taken in, their keys, and the handles of their words.
type takenDay struct {
	lines []*loggedLine
	keys  loggedKeys
	names interning
}

// took has d, its read's own, hold what a read took in of its lines, a,
// whose handles of words it holds already it lets go.
func (d *loggedDay) took(a *takenDay) {
	d.lines = d.lines.appended(a.lines)
	d.keys = d.keys.with(a.keys, a.keys.size(), mergedLoggedKeys)
	maps.DeleteFunc(a.names, func(h unique.Handle[string], _ struct{}) bool { return heldName(d.names, h) })
	d.names = d.names.with(a.names, len(a.names), mergedNames)
}

// loggedHeld is how many lines alike held, the keys of a day's lines held
// before a read, whole where it's every one of the day's, hold of the line
// of the session given with the key k, as turnKeyOf has it, as countedOnce
// counts them: of one with an id, one of its id that arrived at its moment;
// of one without, those of its session that arrived at its moment, their
// records saying what its sum says, but none in a read that isn't whole, a
// tail of lines filed since the last, each new. The record's sum holds its
// session, so countedOnce, knowing a line without an id by its arrival and
// sum alone, counts it as the keys held know it.
func loggedHeld(held *stacked[loggedKeys], session string, k turnKey, whole bool) int {
	switch {
	case k.request != "":
		return idsHeld(held, k.lineKey)
	case !whole:
		return 0
	}
	return idlessCount(held, idlessAt{session: session, at: k.at}, k.sum)
}

// idsHeld is how many lines with the key k the keys held hold, as Taken
// takes them in: one at most.
func idsHeld(s *stacked[loggedKeys], k lineKey) int {
	if holdsKey(s, k) {
		return 1
	}
	return 0
}

// holdsKey reports whether the keys held hold a line with the key k.
func holdsKey(s *stacked[loggedKeys], k lineKey) bool {
	for layer := s; layer != nil; layer = layer.under {
		if slices.ContainsFunc(layer.top.byID[k.request], func(o loggedKey) bool { return o.flight.at == k.at }) {
			return true
		}
	}
	return false
}

// idlessCount is how many lines without ids of the session that arrived at
// the moment at gives, saying what sum says, the keys held hold.
func idlessCount(s *stacked[loggedKeys], at idlessAt, sum uint64) int {
	n := 0
	for layer := s; layer != nil; layer = layer.under {
		for _, o := range layer.top.idless[at] {
			if o.sum == sum {
				n++
			}
		}
	}
	return n
}

// add notes the line l among the keys, which must be k's own.
func (k *loggedKeys) add(l *loggedLine) {
	key := loggedKey{flight: l.flight, sum: l.sum, seq: l.seq}
	if l.flight.request != "" {
		k.byID = withKey(k.byID, l.flight.request, key)
	} else {
		k.idless = withKey(k.idless, idlessAt{session: l.row.Session, at: l.flight.at}, key)
	}
	if l.isRequest() {
		k.bySession = withKey(k.bySession, l.row.Session, l.seq)
	}
}

// withKey is m with v added under k, made where it's nil.
func withKey[K comparable, V any](m map[K][]V, k K, v V) map[K][]V {
	if m == nil {
		m = make(map[K][]V)
	}
	m[k] = append(m[k], v)
	return m
}

// size is how many lines the keys hold.
func (k loggedKeys) size() int {
	n := 0
	for _, keys := range k.byID {
		n += len(keys)
	}
	for _, keys := range k.idless {
		n += len(keys)
	}
	return n
}

// mergedLoggedKeys are the keys under and over, afresh.
func mergedLoggedKeys(under, over loggedKeys) loggedKeys {
	return loggedKeys{byID: mergedLists(under.byID, over.byID), idless: mergedLists(under.idless, over.idless), bySession: mergedLists(under.bySession, over.bySession)}
}

// mergedLists are the lists under and over, by their keys, afresh, each
// key's under's then over's.
func mergedLists[K comparable, V any](under, over map[K][]V) map[K][]V {
	merged := make(map[K][]V, len(under)+len(over))
	maps.Copy(merged, under)
	for k, v := range over {
		merged[k] = append(slices.Clip(merged[k]), v...)
	}
	return merged
}

// dayOf is the place among r's days of the day given, made where r holds
// none of it, in order.
func (r *LogRead) dayOf(day LedgerDay) int {
	i, found := r.dayAt(day.Date)
	if !found {
		r.days = slices.Insert(r.days, i, loggedDay{day: day})
	}
	return i
}

// storyOf is the story of the session of the request of the record rec, as
// held holds it: nil where rec is of no session's request.
func (r *LogRead) storyOf(rec *lineRecord, held HeldSessions) *story {
	if !rec.isRequest() {
		return nil
	}
	return held.storyOf(rec.session)
}

// placeOf is where the row of the record r sits within its day, as placedRow
// has it: where it was sent, where s told that, as sentOf has it, so a row
// stays where it sat in flight as its line fills it in; else where it
// arrived.
func placeOf(r *lineRecord, s Stream) time.Time {
	if sent, told := s.sentOf(r.flight); told {
		return sent
	}
	return r.arrived()
}

// retell tells again the rows of r's lines, those taken in before the seq
// given, whose sessions' stories t's feed changed, as Taken has it,
// returning them as told now. r's days are its own, but none of them.
func (r *LogRead) retell(t LogTaking, before int) []*loggedLine {
	var retold []*loggedLine
	for id, gain := range t.Gains {
		s := t.Held.storyOf(id)
		for i := range r.days {
			d := &r.days[i]
			for _, seq := range d.retelling(id, gain) {
				if seq > before {
					continue
				}
				at := d.lines.search(func(l *loggedLine) bool { return l.seq >= seq })
				told := d.lines.at(at).toldBy(s)
				d.lines = d.lines.set(at, told)
				retold = append(retold, told)
			}
		}
	}
	return retold
}

// retelling are the seqs of the lines of the day d of the session with the
// given id that gain tells of afresh: every one, of a session read whole;
// else those of its requests whose story it told otherwise, each once,
// known by their flights: by id, or without one, among the session's lines
// that arrived at the moment.
func (d *loggedDay) retelling(id string, gain SessionGain) []int {
	var seqs []int
	for layer := d.keys; layer != nil; layer = layer.under {
		if gain.whole {
			seqs = append(seqs, layer.top.bySession[id]...)
			continue
		}
		for _, f := range gain.retold {
			for _, k := range slices.Concat(layer.top.byID[f.request], layer.top.idless[idlessAt{session: id, at: f.at}]) {
				if k.flight == f {
					seqs = append(seqs, k.seq)
				}
			}
		}
	}
	slices.Sort(seqs)
	return slices.Compact(seqs)
}

// worthOf is what the request of the record rec was worth at the prices of
// the local day with the given date, as its worth has it.
func (r LogRead) worthOf(rec *lineRecord, date string) Worth {
	if r.price != nil {
		return r.price(rec, date)
	}
	return rec.worth(r.prices, date)
}

// holdsLineOf reports whether r holds the line of the request t among the
// table's, as t.FiledAs knows it: on the day it was sent on, or the day its
// line is filed on, as FiledOn has it, which may be the day before.
func (r LogRead) holdsLineOf(t Told) bool {
	filed, ok := t.FiledOn()
	return r.holdsLineOn(t, toldOn(t)) || ok && r.holdsLineOn(t, filed)
}

// holdsLineOn reports whether r holds the line of the request t among the
// lines of the day given.
func (r LogRead) holdsLineOn(t Told, day LedgerDay) bool {
	i, found := r.dayAt(day.Date)
	if !found {
		return false
	}
	for layer := r.days[i].keys; layer != nil; layer = layer.under {
		if slices.ContainsFunc(layer.top.byID[t.Request], func(k loggedKey) bool { return k.flight.is(t.Request, t.Sent) }) {
			return true
		}
	}
	return false
}

// HoldsTold reports whether r holds, on a day before the one given, the
// line of a request s tells of that was sent on that day or after, as one
// sent just after midnight is filed on the day before: let go of, its row
// would be told again, as ShowsTold has it, awaiting the line on the day
// let go of.
func (r LogRead) HoldsTold(s Stream, day LedgerDay) bool {
	for t := range s.known() {
		if filed, ok := t.FiledOn(); ok && filed.Date < day.Date && toldOn(t).Date >= day.Date && r.holdsLineOn(t, filed) {
			return true
		}
	}
	return false
}

// dayAt is the place among r's days of the day with the given date,
// reporting false where r holds none of it, the place it would take.
func (r LogRead) dayAt(date string) (int, bool) {
	return slices.BinarySearchFunc(r.days, date, func(d loggedDay, date string) int { return strings.Compare(d.day.Date, date) })
}

// lines are r's lines, the oldest day's first, each day's in the order
// taken in.
func (r LogRead) lines() iter.Seq[*loggedLine] {
	return func(yield func(*loggedLine) bool) {
		for _, d := range r.days {
			for _, l := range d.lines.all() {
				if !yield(l) {
					return
				}
			}
		}
	}
}

// firstsOn are when each session's first line filed on the day d arrived,
// by the session's whole id, of the lines r holds: each that counts toward
// a session, as ledger.Line.CountedSession has it, so as the Log counts a
// session's lines.
func (r LogRead) firstsOn(d LedgerDay) map[string]time.Time {
	firsts := make(map[string]time.Time)
	i, found := r.dayAt(d.Date)
	if !found {
		return firsts
	}
	for _, l := range r.days[i].lines.all() {
		id := l.row.CountedSession
		if id == "" {
			continue
		}
		if at, ok := firsts[id]; !ok || l.row.At.Before(at) {
			firsts[id] = l.row.At.UTC()
		}
	}
	return firsts
}
