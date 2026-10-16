package views

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// openFor is how long a session's requests the store holds stay open after
// they arrive, told again at each feed, for the lines yet to come of those
// that arrived before them: a request never runs an hour, so a line that
// arrived longer ago than that has come, or never will. pinnedFor is how long
// a request the stream tells of keeps its session's later requests open at
// most, as one lost may never have a line.
const (
	openFor   = time.Hour
	pinnedFor = 6 * time.Hour
)

// HeldSessions are the sessions the readers of the ledger want, held between
// reads of its tail, each one's story told on as its lines come, never told
// afresh: the one store of them every reader reads, fed once a read. A
// session's requests are sealed into its story once they're behind its
// horizon, no line of a request that arrived before it yet to come; those at
// or past it are held open, and told again on the sealed story at each feed,
// so a line that comes late tells as it would read at once. A line landing
// behind the horizon after all, a day read again that holds what the store
// can't place, the clock set back or the zone changed, has the session, or
// every one, read whole again.
//
// What changes it returns afresh, sharing with it what it doesn't change, so
// a holder never sees it change, and a feed off the event loop grows it as
// the watch draws the one it holds.
type HeldSessions struct {
	prices ledger.Table
	// day is the day it was last fed on, and fed when, by the clock its
	// feeds are given: a feed before either, as the clock set back or the
	// zone changed has it, holds none of what it held.
	day LedgerDay
	fed time.Time
	// listing is set while it holds every one of today's sessions, as the
	// List wants them, its first read reading them whole.
	listing  bool
	sessions map[string]*heldSession
	// last is what its last feed did.
	last feedCounts
}

// heldSession is a session the store holds: its horizon; its story of the
// requests behind it, sealed; those at or past it, open, in order, as
// inOrder has it; its story of them all, the sealed story told on with the
// open; the flights of its requests that arrived from the day before its
// day on, by their dates, for a line given again to be known; how many of
// its requests arrived on each day, by date, for a day read again whole
// before then; and, of the session whose page is open alone, what its page
// tallies of the requests sealed, nil of every other.
type heldSession struct {
	horizon instant
	sealed  *story
	open    []heldRequest
	story   *story
	flights map[string]heldFlights
	counts  map[string]int
	page    *pageTally
}

// heldFlights are the flights of a session's requests the store holds, by
// their ids, each with the sum of what its line says, in layers, as stacked
// has them, so a session that takes more in shares them with the session it
// took them from.
type heldFlights = *stacked[map[string][]knownLine]

// knownLine is the flight of a request the store holds, and the sum of what
// its line says of it, as sumOf has it, so a line of the same request given
// again is known from one that says otherwise of it.
type knownLine struct {
	flight
	sum uint64
}

// feedCounts are what a feed did: the requests it took in new of the
// sessions held before it; those it told again of them, sealing or folding
// the open on; and those it read whole.
type feedCounts struct {
	gained, folded, read int
}

// SessionsWanted are the sessions the readers of the store want held: where
// List is set, today's, those with a request today, and those running, as
// the router lists them, by their ids; the session whose page is open; and
// those of the Log's days. Pins are when each of a session's requests the
// stream tells of went upstream, by its id, in no order: each a pin, which
// holds the session's later requests open, as the line of one that arrived
// before them may yet come.
type SessionsWanted struct {
	List    bool
	Running []string
	Page    string
	Log     []string
	Pins    map[string][]time.Time
	// paged is the session of the page open as its page holds it, read
	// whole as it opened, nil where none is: the store takes it in place of
	// the session it holds without what its page tallies, as Paging has it.
	paged *heldSession
}

// Paging is w wanting the page h holds too, the session it's of tallied as
// its page tallies it, pinned as the stream s tells where w isn't pinned.
// Where the store holds the session but not its page's tally, it takes the
// session as h holds it, read whole as the page opened, after the ledger's
// tail was last read, as no read of the tail runs beside the page's: so the
// read of the tail it's fed gives it every line since, those it holds
// passed over.
func (w SessionsWanted) Paging(h HeldPage, s Stream) SessionsWanted {
	w.Page, w.paged = h.id, h.record
	if w.Pins == nil {
		w.Pins = pinsOf(s)
	}
	return w
}

// ListWanted is what the List wants of the store: today's sessions and
// those running, as the router lists them, each pinned as the stream s
// tells, as pinsOf has it.
func ListWanted(running []status.Session, s Stream) SessionsWanted {
	return SessionsWanted{List: true, Running: runningIDs(running), Pins: pinsOf(s)}
}

// pinsOf are when each of the requests s tells of went upstream, or, of
// one yet to go, when s last told of it, by its session's id: in flight, or
// with its line to come, as s holds a request till its line is read.
func pinsOf(s Stream) map[string][]time.Time {
	pins := make(map[string][]time.Time)
	for _, t := range s.Requests {
		session := t.countedSession()
		if session == "" {
			continue
		}
		sent := t.Sent
		if sent.IsZero() {
			sent = t.Heard
		}
		pins[session] = append(pins[session], sent)
	}
	return pins
}

// SessionGains are what a feed changed of the sessions the store holds, by
// their ids, for a reader that tells of their requests to tell again only
// what changed: of a session read whole, or folded afresh, everything.
type SessionGains map[string]SessionGain

// SessionGain is what a feed changed of a session the store holds: whether
// it read it whole, every request of it telling afresh; the requests it
// sealed, in order; and the requests whose story told otherwise of them, as
// toldOf tells of a line, its first, the moves made and the requests that
// rewrote them, each as it was and as it is.
type SessionGain struct {
	whole  bool
	sealed []heldRequest
	retold []flight
}

// Holding is what the store holds, and what its last feed did, as the
// watch's guard holds them to what a reader shows: the date of the day it
// was last fed on; how many requests each session it holds keeps open, by
// its id; how many keys of requests it holds, for lines given again; and
// how many requests its last feed took in new of the sessions it held
// before it, how many it told again of them, sealing them or folding the
// open on, and how many it read whole.
type Holding struct {
	Day                  string
	Open                 map[string]int
	Keys                 int
	Gained, Folded, Read int
}

// HoldSessions returns a store holding no session, its requests priced by
// prices at the day each feed falls on.
func HoldSessions(prices ledger.Table) HeldSessions {
	return HeldSessions{prices: prices}
}

// Fed is h fed a read of the ledger's tail at now, t as Took decoded it,
// holding the sessions want wants: rolled on to the day now falls on, each
// session's requests on the day counted afresh and its moves priced at its
// prices; the late days t holds taken in, the oldest first, then today's
// gain, each line h holds already passed over; the sessions wanted that it
// doesn't hold, and those whose lines it can't place, read whole through l,
// in one read, of every one of today's sessions too where the List is wanted
// and h doesn't hold them yet; each session's requests sealed behind its
// horizon; and the sessions nobody wants let go. With h, it gives what
// changed of each session, as SessionGains has it.
func (h HeldSessions) Fed(l LedgerSessions, t Taken, want SessionsWanted, now time.Time) (HeldSessions, SessionGains) {
	f := h.feeding(want, now)
	for _, date := range slices.Sorted(maps.Keys(t.days)) {
		f.take(t.days[date], true)
	}
	f.take(t.gained, false)
	f.read(l)
	f.seal()
	f.letGo()
	return f.HeldSessions, f.gains
}

// feed is a feed of the store under way: the store as fed so far, its map of
// sessions its own; what's wanted, the sessions running among it, and the
// time it's fed at; the sessions it doesn't hold met in the read with a
// request today, by their ids; the sessions to read whole; those whose
// stories are to be told on, by their ids, with how they stood before; and
// what changed.
type feed struct {
	HeldSessions
	want    SessionsWanted
	running map[string]bool
	now     time.Time
	today   map[string]bool
	whole   map[string]bool
	before  map[string]*heldSession
	gains   SessionGains
}

// feeding is a feed of h at now, wanting want: of none of what h held where
// the clock was set back or the zone changed since, as turnedBack has it;
// each session h holds rolled on to the day now falls on, as on has it.
func (h HeldSessions) feeding(want SessionsWanted, now time.Time) *feed {
	if h.turnedBack(now) {
		h = HoldSessions(h.prices)
	}
	f := &feed{
		HeldSessions: h, want: want, running: make(map[string]bool, len(want.Running)), now: now,
		today: make(map[string]bool), whole: make(map[string]bool), before: make(map[string]*heldSession), gains: make(SessionGains),
	}
	for _, id := range want.Running {
		f.running[id] = true
	}
	f.day, f.fed, f.last = Today(now), now, feedCounts{}
	f.sessions = cloneOrMake(h.sessions)
	if s, held := f.sessions[want.Page]; held && s.page == nil && want.paged != nil {
		f.change(want.Page, want.paged)
	}
	for id, s := range f.sessions {
		if s.day().Date < f.day.Date {
			f.change(id, s.on(f.day, f.prices))
			f.whole[id] = f.whole[id] || s.page != nil && !s.page.pricedAlike(f.prices, s.day().Date, f.day.Date)
		}
	}
	if s, held := f.sessions[want.Page]; held && f.joinsLate(want.Page, s.horizon) {
		f.whole[want.Page] = true
	}
	return f
}

// joinsLate reports whether the stream tells of a request of the session
// with the given id, a pin of it, that may have arrived behind the horizon
// given, as its page takes its requests in by when they were sent, the
// stream's among its lines: one heard first after the horizon passed it,
// as the stream joined late, whose page is tallied again whole. One pinned
// longer than pinnedFor ago may be lost, its line never coming, and is
// taken in after the requests sealed.
func (f *feed) joinsLate(id string, horizon instant) bool {
	return slices.ContainsFunc(f.want.Pins[id], func(pin time.Time) bool {
		return f.now.Sub(pin) <= pinnedFor && instantOf(pin.Add(-arrivesWithin)).compare(horizon) < 0
	})
}

// turnedBack reports whether the clock was set back, or the zone changed,
// since h was last fed, now: now is before then, or the day then falls on
// starts otherwise than it did.
func (h HeldSessions) turnedBack(now time.Time) bool {
	return !h.fed.IsZero() && (now.Before(h.fed) || !Today(h.fed).Start.Equal(h.day.Start))
}

// change has the session with the given id held as s, its story to be told
// on, as it stood before the feed kept for what changed.
func (f *feed) change(id string, s *heldSession) {
	if _, ok := f.before[id]; !ok {
		f.before[id] = f.sessions[id]
	}
	f.sessions[id] = s
}

// take takes in the requests of a read's lines, by their sessions' ids,
// whole where they're a day's read again whole: those of a session the store
// holds, the new among them added, as gained has them, or, where it can't
// place them all, the session read whole; and of one it doesn't, whether
// one is today's. A read whole, which reads every line filed, holds every
// one of them it read.
func (f *feed) take(batch takenLines, whole bool) {
	for _, id := range slices.Sorted(maps.Keys(batch)) {
		requests := batch[id]
		s, held := f.sessions[id]
		switch {
		case !held:
			f.today[id] = f.today[id] || slices.ContainsFunc(requests, func(r heldRequest) bool { return f.day.Holds(r.arrived()) })
			continue
		case f.whole[id]:
			continue
		}
		added, placed := s.gained(requests, whole)
		switch {
		case !placed:
			f.whole[id] = true
		case len(added) > 0:
			f.change(id, s.with(pricedAt(added, f.prices, f.day.Date)))
			f.last.gained += len(added)
		}
	}
}

// read reads whole through l, in one read, the sessions wanted that the
// store doesn't hold, and those it can't place a line of, each held afresh,
// as hold holds it: every one of today's too, where the List is wanted and
// the store doesn't hold them yet.
func (f *feed) read(l LedgerSessions) {
	ids := f.unheld()
	listing := f.want.List && !f.listing
	if len(ids) == 0 && !listing {
		return
	}
	var read map[string][]ledger.Line
	if listing {
		read, f.listing = l.Sessions(ids, nil), true
	} else {
		read = l.SessionsOf(ids, nil)
	}
	for _, id := range ids {
		f.hold(id, read[id])
	}
	for id, lines := range read {
		if _, held := f.sessions[id]; !held {
			f.hold(id, lines)
		}
	}
}

// unheld are the ids of the sessions to read whole, in order: those wanted
// that the store doesn't hold, the List's today's among those met in the
// read among them; and those it can't place a line of.
func (f *feed) unheld() []string {
	var ids []string
	add := func(id string) {
		s, held := f.sessions[id]
		if id != "" && (!held || f.whole[id] || id == f.want.Page && s.page == nil) {
			ids = append(ids, id)
		}
	}
	for id := range f.whole {
		add(id)
	}
	for _, id := range slices.Concat(f.want.Running, f.want.Log, []string{f.want.Page}) {
		add(id)
	}
	for id, today := range f.today {
		if f.want.List && today {
			add(id)
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(ids)))
}

// hold holds the session with the given id afresh, of lines, every line of
// it read whole, as readOf has it, sealed behind its horizon, as horizonOf
// has it, its page tallied where it's the page's.
func (f *feed) hold(id string, lines []ledger.Line) {
	s := readOf(lines, f.prices, f.day, f.horizonOf(id), id == f.want.Page)
	f.sessions[id] = s
	delete(f.before, id)
	f.gains[id] = SessionGain{whole: true}
	f.last.read += s.held()
}

// readOf is a session held afresh of lines, every line of it read whole,
// each request once, as distinct has it, each priced by prices at the day
// d, sealed behind the horizon given, its page tallied where paged says.
func readOf(lines []ledger.Line, prices ledger.Table, d LedgerDay, horizon instant, paged bool) *heldSession {
	var read []heldRequest
	for i := range lines {
		if isRequest(&lines[i]) {
			read = append(read, heldRequestOf(&lines[i]))
		}
	}
	return heldSessionOf(pricedAt(distinct(read), prices, d.Date), prices, d, horizon, paged)
}

// held is how many requests s holds.
func (s *heldSession) held() int {
	n := 0
	for _, c := range s.counts {
		n += c
	}
	return n
}

// horizonOf is the horizon of the session with the given id at the feed's
// time, as horizonAt has it of its pins.
func (f *feed) horizonOf(id string) instant {
	return horizonAt(f.now, f.want.Pins[id])
}

// horizonAt is the horizon of a session at now, before a moment it last
// had: openFor before then, or earlier, where the earliest of its pins
// within pinnedFor before then is, a moment earlier than it, as its request
// may have arrived arrivesWithin before. A pin earlier than pinnedFor may be
// of a request lost, whose line never comes.
func horizonAt(now time.Time, pins []time.Time) instant {
	horizon := now.Add(-openFor)
	for _, pin := range pins {
		if now.Sub(pin) <= pinnedFor && pin.Add(-arrivesWithin).Before(horizon) {
			horizon = pin.Add(-arrivesWithin)
		}
	}
	return instantOf(horizon)
}

// seal seals each session's requests behind its horizon, as it moves on at
// the feed's time, and tells on afresh the story of each changed, noting
// what changed of each.
func (f *feed) seal() {
	for id, s := range f.sessions {
		if f.gains[id].whole {
			continue
		}
		horizon := later(s.horizon, f.horizonOf(id))
		before, changed := f.before[id]
		if !changed && (len(s.open) == 0 || s.open[0].at.compare(horizon) >= 0) {
			continue
		}
		sealed := s.sealedAt(horizon)
		f.sessions[id], f.last.folded = sealed, f.last.folded+len(s.open)
		gain := SessionGain{sealed: s.open[:len(s.open)-len(sealed.open)]}
		if changed {
			gain.retold = retold(before.story, sealed.story)
		}
		f.gains[id] = gain
	}
}

// letGo lets go of the sessions nobody wants: of the List's, those with no
// request today that don't run, and every one where it's no longer wanted;
// and of what a page tallies of a session it's no longer open on.
func (f *feed) letGo() {
	for id, s := range f.sessions {
		list := f.want.List && (s.story.requests > 0 || f.running[id])
		switch {
		case !list && id != f.want.Page && !slices.Contains(f.want.Log, id):
			delete(f.sessions, id)
			delete(f.gains, id)
		case id != f.want.Page && s.page != nil:
			unpaged := *s
			unpaged.page = nil
			f.sessions[id] = &unpaged
		}
	}
	f.listing = f.listing && f.want.List
}

// cloneOrMake is a copy of m, or, where it's nil, a map of its own.
func cloneOrMake[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return make(map[K]V)
	}
	return maps.Clone(m)
}

// later is the later of two instants.
func later(a, b instant) instant {
	if a.compare(b) >= 0 {
		return a
	}
	return b
}

// heldSessionOf is a session held of its requests, in order, as inOrder has
// it, each priced, its requests on the day d counted and its moves priced by
// prices, sealed behind the horizon given, its page tallied where paged
// says.
func heldSessionOf(requests []heldRequest, prices ledger.Table, d LedgerDay, horizon instant, paged bool) *heldSession {
	s := &heldSession{horizon: horizon, sealed: newStory(prices, d), flights: make(map[string]heldFlights), counts: make(map[string]int)}
	if paged {
		s.page = newPageTally()
	}
	s.open = requests
	s.taking(requests)
	return s.sealedAt(horizon)
}

// day is the day s counts its requests on.
func (s *heldSession) day() LedgerDay {
	return s.sealed.day
}

// since is the date of the first day s holds the flights of its requests
// on: the day before its own, so a line given again of a request that
// arrived before midnight, read again whole as the day turns, is known.
func (s *heldSession) since() string {
	return dayfile.DateOf(dayfile.DayStart(s.day().Start, -1))
}

// gained are the requests among requests, the session's of a read, that s
// doesn't hold, in order, reporting false where it can't place them all,
// whole where they're a day's read again whole: one with no id, which can't
// be known from another; one of a request it holds whose line says
// otherwise of it, which a read whole may hold in its place, as distinct
// keeps the first of a request's lines it reads; one that arrived behind the
// horizon, which would change what's sealed; or the requests of a day before
// those whose flights it holds other than as many as it holds of that day,
// or not read whole.
func (s *heldSession) gained(requests []heldRequest, whole bool) ([]heldRequest, bool) {
	since := s.since()
	counted := make(map[string]int)
	var added []heldRequest
	for i := range requests {
		r := &requests[i]
		date := dateOf(r)
		sum, held := s.heldSum(r, date)
		switch {
		case r.request == "":
			return nil, false
		case date < since:
			counted[date]++
		case held && sum == r.sum:
		case held, r.at.compare(s.horizon) < 0:
			return nil, false
		default:
			added = append(added, *r)
		}
	}
	for date, n := range counted {
		if !whole || s.counts[date] != n {
			return nil, false
		}
	}
	return added, true
}

// dateOf is the date of the local day r arrived on, as the ledger files its
// line.
func dateOf(r *heldRequest) string {
	return dayfile.DateOf(r.arrived())
}

// heldSum is the sum of the line s holds of the request r, as sumOf has it,
// reporting false where it holds none: of the same id, arrived at the same
// moment, on the day with the given date, one of those it holds the flights
// of.
func (s *heldSession) heldSum(r *heldRequest, date string) (uint64, bool) {
	f, held := heldFlight(s.flights[date], r.request, func(f flight) bool { return f.at == r.at })
	return f.sum, held
}

// heldFlight is the flight held has of the request with the given id that
// is as is says, reporting false where it has none.
func heldFlight(held heldFlights, id string, is func(flight) bool) (knownLine, bool) {
	for ; held != nil; held = held.under {
		if i := slices.IndexFunc(held.top[id], func(f knownLine) bool { return is(f.flight) }); i >= 0 {
			return held.top[id][i], true
		}
	}
	return knownLine{}, false
}

// lined reports whether s holds the line of the request t the stream told
// of, as Told.FiledAs has it.
func (s *heldSession) lined(t Told) bool {
	return s.linedAt(t.Request, t.Sent)
}

// linedAt reports whether s holds the line of the request with the given
// id that arrived, or went upstream, at at, as sameRequest has it.
func (s *heldSession) linedAt(id string, at time.Time) bool {
	for _, flights := range s.flights {
		if _, held := heldFlight(flights, id, func(f flight) bool { return f.is(id, at) }); held {
			return true
		}
	}
	return false
}

// with is s with the requests added, each priced, held open, in order,
// afresh, its story to be told on.
func (s *heldSession) with(added []heldRequest) *heldSession {
	grown := *s
	grown.open = withRequests(s.open, added)
	grown.flights, grown.counts = maps.Clone(s.flights), maps.Clone(s.counts)
	grown.taking(added)
	return &grown
}

// taking notes the requests taken in of s, whose maps must be its own: the
// flights of those that arrived since the day before its own, and how many
// arrived each day.
func (s *heldSession) taking(requests []heldRequest) {
	since := s.since()
	byDate := make(map[string]map[string][]knownLine)
	for i := range requests {
		r := &requests[i]
		date := dateOf(r)
		s.counts[date]++
		if date < since {
			continue
		}
		if byDate[date] == nil {
			byDate[date] = make(map[string][]knownLine)
		}
		byDate[date][r.request] = append(byDate[date][r.request], knownLine{flight: r.flight, sum: r.sum})
	}
	for date, flights := range byDate {
		s.flights[date] = s.flights[date].with(flights, len(flights), mergedFlights)
	}
}

// mergedFlights are the flights under and over, afresh.
func mergedFlights(under, over map[string][]knownLine) map[string][]knownLine {
	merged := maps.Clone(under)
	for id, flights := range over {
		merged[id] = append(slices.Clip(merged[id]), flights...)
	}
	return merged
}

// sealedAt is s with its open requests behind the horizon given sealed into
// its story, and its page's tally where it's tallied, those at or past it
// held open, and its story told on of them, afresh.
func (s *heldSession) sealedAt(horizon instant) *heldSession {
	i, _ := slices.BinarySearchFunc(s.open, horizon, func(r heldRequest, h instant) int { return r.at.compare(h) })
	sealed := *s
	sealed.horizon = horizon
	if i > 0 {
		sealed.sealed = s.sealed.told(s.open[:i])
	}
	if i > 0 && s.page != nil {
		sealed.page = s.page.took(s.open[:i])
	}
	sealed.open = slices.Clone(s.open[i:])
	sealed.story = sealed.sealed.told(sealed.open)
	return &sealed
}

// on is s rolled on to the day given, a later one than its own, afresh: its
// sealed story told on it, as story's on has it; its open requests priced
// at its prices, as they're told again on it; and the flights of those that
// arrived before the day before it let go, as how many arrived each day
// holds what a day read again whole needs.
func (s *heldSession) on(day LedgerDay, prices ledger.Table) *heldSession {
	rolled := *s
	rolled.sealed = s.sealed.on(day)
	rolled.open = pricedAt(s.open, prices, day.Date)
	since := rolled.since()
	rolled.flights = maps.Clone(s.flights)
	maps.DeleteFunc(rolled.flights, func(date string, _ heldFlights) bool { return date < since })
	rolled.story = rolled.sealed.told(rolled.open)
	return &rolled
}

// retold are the requests the story as it was, was, tells otherwise of than
// the story as it is, is, as toldOf tells of a line: its first, as it was
// and as it is, where that changed; and of each move made that changed, or
// that only one of them holds, the request that made it, and those that
// rewrote it in each.
func retold(was, is *story) []flight {
	var flights []flight
	if was.begun != is.begun || was.first != is.first {
		flights = append(flights, firstOf(was)...)
		flights = append(flights, firstOf(is)...)
	}
	made := make(map[flight]move, len(was.moves))
	for _, m := range was.moves {
		made[m.by] = m
	}
	for _, m := range is.moves {
		if before, ok := made[m.by]; !ok || !before.same(m) {
			flights = append(flights, m.flights()...)
			flights = append(flights, before.flights()...)
		}
		delete(made, m.by)
	}
	for _, m := range made {
		flights = append(flights, m.flights()...)
	}
	return flights
}

// firstOf is the flight of s's first request, none where it has none.
func firstOf(s *story) []flight {
	if !s.begun {
		return nil
	}
	return []flight{s.first}
}

// flights are the flights of the requests whose rows tell of m: the one
// that made it, and the one that rewrote the session's context as it
// settled, where one did. None of a move of no request, the zero move.
func (m move) flights() []flight {
	var flights []flight
	if m.by != (flight{}) {
		flights = append(flights, m.by)
	}
	if m.rewrote != nil {
		flights = append(flights, m.rewrote.flight)
	}
	return flights
}

// same reports whether m tells what o does, as its request's row tells it,
// and its rewriting request's.
func (m move) same(o move) bool {
	sameCost := m.cost == o.cost || m.cost != nil && o.cost != nil && *m.cost == *o.cost
	sameRewrote := (m.rewrote == nil) == (o.rewrote == nil) && (m.rewrote == nil || m.rewrote.flight == o.rewrote.flight)
	return m.by == o.by && slices.Equal(m.tried, o.tried) && m.at.Equal(o.at) && m.model == o.model && m.from == o.from && m.to == o.to &&
		m.reason == o.reason && m.settled == o.settled && m.written == o.written && sameCost && sameRewrote
}

// With is h with the sessions found, held of reads of their own, as the
// List's first read holds them, that h doesn't hold: none it holds is
// changed. It's as last fed as the earlier fed of the two, so the days a
// read of the ledger's tail reads again for it, as Unread has them, are
// those any session it holds may have missed lines of. Where h holds no
// session, its feeds having let go of every one, or never fed, it's found
// itself, holding every one of today's sessions where either does.
func (h HeldSessions) With(found HeldSessions) HeldSessions {
	if len(h.sessions) == 0 {
		found.listing = found.listing || h.listing
		return found
	}
	sessions := cloneOrMake(h.sessions)
	for id, s := range found.sessions {
		if _, held := sessions[id]; !held {
			sessions[id] = s
		}
	}
	h.sessions, h.listing = sessions, h.listing || found.listing
	if !found.fed.IsZero() && found.fed.Before(h.fed) {
		h.fed, h.day = found.fed, found.day
	}
	return h
}

// Without is h holding none of the sessions held holds, else as it's held:
// what held.With(h) takes of h.
func (h HeldSessions) Without(held HeldSessions) HeldSessions {
	h.sessions = maps.Clone(h.sessions)
	maps.DeleteFunc(h.sessions, func(id string, _ *heldSession) bool { _, ok := held.sessions[id]; return ok })
	return h
}

// at is h at now: as it's held, where every session it holds counts its
// requests on the day now falls on, else with those held at an earlier day
// rolled on to it, as on has it, as a look before a feed rolls them sees.
func (h HeldSessions) at(now time.Time) HeldSessions {
	day := Today(now)
	var rolled map[string]*heldSession
	for id, s := range h.sessions {
		if s.day().Date >= day.Date {
			continue
		}
		if rolled == nil {
			rolled = maps.Clone(h.sessions)
		}
		rolled[id] = s.on(day, h.prices)
	}
	if rolled != nil {
		h.sessions = rolled
	}
	return h
}

// List is the List of what h holds and live gives, as ListSessions builds it
// of the same lines: the sessions the router lists, and those with requests
// today, at the day live's now falls on.
func (h HeldSessions) List(live ListLive) SessionList {
	h = h.at(live.Now)
	stories := make(map[string]*story, len(h.sessions))
	for id, s := range h.sessions {
		stories[id] = s.story
	}
	return listOf(stories, newLister(live.Running, live.Unlisted, live.Stream, live.Now), h.prices)
}

// Awaited are the dates of the days before the day live's now falls on that
// the lines of requests the stream told ended are filed on, as Told.FiledOn
// has it, their lines yet to come, of whichever session, each once, the
// oldest first: a later day's tail of the ledger never reads them.
func (h HeldSessions) Awaited(live ListLive) []string {
	today := Today(live.Now)
	var dates []string
	for id, told := range live.Stream.hearing().told {
		s := h.sessions[id]
		for _, t := range told {
			if s != nil && s.lined(t) || toldRow(t, live.Now).Stage != RowEnded {
				continue
			}
			if day, ok := t.FiledOn(); ok && day.Start.Before(today.Start) {
				dates = append(dates, day.Date)
			}
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(dates)))
}

// Unread are the dates of those of the days missed, the days the ledger's
// tail followed no further, oldest first, that a read of it reads again
// whole for the store: the day it was last fed on and those after it, as
// it took in every line filed on a day before it.
func (h HeldSessions) Unread(missed []string) []string {
	return slices.DeleteFunc(slices.Clone(missed), func(date string) bool { return date < h.day.Date })
}

// Holding is what h holds, and what its last feed did, as Holding has them.
func (h HeldSessions) Holding() Holding {
	held := Holding{Day: h.day.Date, Open: make(map[string]int, len(h.sessions)), Gained: h.last.gained, Folded: h.last.folded, Read: h.last.read}
	for id, s := range h.sessions {
		held.Open[id] = len(s.open)
		for _, flights := range s.flights {
			for layer := flights; layer != nil; layer = layer.under {
				for _, f := range layer.top {
					held.Keys += len(f)
				}
			}
		}
	}
	return held
}

// storyOf is the story h holds of the session with the given id: one of no
// requests, on the day h was last fed on, where it holds none.
func (h HeldSessions) storyOf(id string) *story {
	if s, ok := h.sessions[id]; ok {
		return s.story
	}
	return newStory(h.prices, h.day)
}

// namingOf is what the lines h holds of the session with the given id name
// it by, as its story has it, whatever day it's held at: none where it holds
// none of them.
func (h HeldSessions) namingOf(id string) naming {
	if s, ok := h.sessions[id]; ok {
		return s.story.naming
	}
	return naming{}
}
