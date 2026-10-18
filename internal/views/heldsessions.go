package views

import (
	"fmt"
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
	// fed is when it was last fed, as fedAt has it: of the reads it was
	// given, the latest fed, as a feed before it, as the clock set back or
	// the zone changed has it, holds none of what it held. unread is when it
	// was last fed as the days a read of the ledger's tail reads again for
	// it have it, as Unread has them: of the reads it was given, as fedWith
	// has it.
	fed, unread fedAt
	// feeds is how many feeds it's had, kept as the clock set back drops
	// what it holds, so what was fed later is known by it.
	feeds int
	// listing is set while it holds every one of today's sessions, as the
	// List wants them, its first read reading them whole.
	listing  bool
	sessions map[string]*heldSession
	// last is what its last feed did.
	last feedCounts
}

// fedAt is when a store was fed, by the clock its feeds are given, and the
// day that fell on: none where it never was.
type fedAt struct {
	at  time.Time
	day LedgerDay
}

// fedOn is a store fed at now.
func fedOn(now time.Time) fedAt {
	return fedAt{at: now, day: Today(now)}
}

// heldSession is a session the store holds: its horizon; its story of the
// requests behind it, sealed; those at or past it, open, in order, as
// inOrder has it; its story of them all, the sealed story told on with the
// open; the flights of its requests that arrived from the day before its
// day on, by their dates, for a line given again to be known; how many of
// its requests arrived on each day, by date, for a day read again whole
// before then; and, of the session whose page is open alone, what its page
// tallies of the requests sealed, its points among it, which hold its
// accounts' answers of the last run of its days they haven't sealed, and of
// today's and yesterday's, as pointing has them; nil of every other.
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
// their ids, each with the sum of all it decodes of its line, in layers, as
// stacked has them, so a session that takes more in shares them with the
// session it took them from.
type heldFlights = *stacked[map[string][]knownLine]

// knownLine is the flight of a request the store holds, and the sum of all
// it decodes of its line, as heldRequest's summed has it, so a line of the
// same request given again is known from one that says otherwise of it.
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
	// whole as it opened, or as the store has fed it since, nil where none
	// is: the store takes it in place of the session it holds without what
	// its page tallies, as Paging has it.
	paged *heldSession
}

// Paging is w wanting the page h holds too, the session it's of tallied as
// its page tallies it, pinned as the stream s tells where w isn't pinned.
// Where the store holds the session but not its page's tally, it takes the
// session as h holds it: read whole as the page opened, after the ledger's
// tail was last read, as no read of the tail runs beside the page's, or as
// the store fed it since, as Held takes it; so the read of the tail it's
// fed gives it every line since, those it holds passed over.
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

// Retell reports whether g tells otherwise of any request it holds: of a
// session read whole, or of one whose story told otherwise of a request,
// as SessionGain has them, so what's told of the store is told again.
func (g SessionGains) Retell() bool {
	for _, gain := range g {
		if gain.whole || len(gain.retold) > 0 {
			return true
		}
	}
	return false
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
// doesn't hold, and those whose lines it can't place, held afresh of a read
// through l, and a second of those it can't hold on their states, as read
// has them, of every one of today's sessions too where the List is wanted
// and h doesn't hold them yet; each session's
// requests sealed behind its horizon; and the sessions nobody wants let go.
// With h, it gives what changed of each session, as SessionGains has it.
func (h HeldSessions) Fed(l LedgerSessions, t Taken, want SessionsWanted, now time.Time) (HeldSessions, SessionGains) {
	f := h.feeding(want, now)
	for _, date := range slices.Sorted(maps.Keys(t.days)) {
		f.take(t.days[date], true)
	}
	f.take(t.gained, false)
	f.answer(t)
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
		h = HeldSessions{prices: h.prices, feeds: h.feeds}
	}
	f := &feed{
		HeldSessions: h, want: want, running: make(map[string]bool, len(want.Running)), now: now,
		today: make(map[string]bool), whole: make(map[string]bool), before: make(map[string]*heldSession), gains: make(SessionGains),
	}
	for _, id := range want.Running {
		f.running[id] = true
	}
	f.fed, f.last, f.feeds = fedOn(now), feedCounts{}, h.feeds+1
	f.unread = f.fed
	f.sessions = cloneOrMake(h.sessions)
	if s, held := f.sessions[want.Page]; held && s.page == nil && want.paged != nil {
		f.change(want.Page, want.paged)
	}
	for id, s := range f.sessions {
		if s.day().Date < f.fed.day.Date {
			f.change(id, s.on(f.fed.day, f.prices))
			f.whole[id] = f.whole[id] || s.repricedOn(f.prices, f.fed.day)
		}
	}
	if s, held := f.sessions[want.Page]; held && joinedLate(f.now, want.Pins[want.Page], s.horizon) {
		f.whole[want.Page] = true
	}
	return f
}

// repricedOn reports whether what s's page tallies, priced at its day's
// prices, is worth otherwise on the day given, as prices have it, a later
// one than its own: its page is tallied again whole as it rolls on to it.
func (s *heldSession) repricedOn(prices ledger.Table, day LedgerDay) bool {
	return s.page != nil && s.day().Date < day.Date && !s.page.pricedAlike(prices, s.day().Date, day.Date)
}

// joinedLate reports whether the stream tells, at now, of a request of a
// session, a pin of it among its pins, that may have arrived behind the
// horizon given, as its page takes its requests in by when they were sent,
// the stream's among its lines: one heard first after the horizon passed
// it, as the stream joined late, whose page is tallied again whole. One
// pinned longer than pinnedFor ago may be lost, its line never coming, and
// is taken in after the requests sealed.
func joinedLate(now time.Time, pins []time.Time, horizon instant) bool {
	return slices.ContainsFunc(pins, func(pin time.Time) bool {
		return now.Sub(pin) <= pinnedFor && instantOf(pin.Add(-arrivesWithin)).compare(horizon) < 0
	})
}

// turnedBack reports whether the clock was set back, or the zone changed,
// since h was last fed, now: now is before then, or the day then falls on
// starts otherwise than it did.
func (h HeldSessions) turnedBack(now time.Time) bool {
	return !h.fed.at.IsZero() && (now.Before(h.fed.at) || !Today(h.fed.at).Start.Equal(h.fed.day.Start))
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
			f.today[id] = f.today[id] || slices.ContainsFunc(requests, func(r heldRequest) bool { return f.fed.day.Holds(r.arrived()) })
			continue
		case f.whole[id]:
			continue
		}
		added, placed := s.gained(requests, whole)
		switch {
		case !placed:
			f.whole[id] = true
		case len(added) > 0:
			f.change(id, s.with(pricedAt(added, f.prices, f.fed.day.Date)))
			f.last.gained += len(added)
		}
	}
}

// answer takes the lines a read of the ledger's tail read into the points
// of the session whose page is open, where the store holds it tallied, and
// reads it whole where they can't be placed: every account's answers, as
// pointing's took has them, of the late days, the oldest first, each read
// whole, then today's gain.
func (f *feed) answer(t Taken) {
	id := f.want.Page
	s, held := f.sessions[id]
	if !held || s.page == nil || f.whole[id] {
		return
	}
	points, ok := s.page.points, true
	for _, date := range slices.Sorted(maps.Keys(t.read.days)) {
		if points, ok = points.took(date, answersIn(t.read.days[date], id), true, f.fed.day); !ok {
			break
		}
	}
	if ok {
		points, ok = points.took(f.fed.day.Date, answersIn(t.read.gained, id), false, f.fed.day)
	}
	switch {
	case !ok:
		f.whole[id] = true
	case points != s.page.points:
		paged := *s
		paged.page = s.page.pointed(points)
		f.change(id, &paged)
	}
}

// read holds afresh, of one read through l, the sessions wanted that the
// store doesn't hold, and those it can't place a line of, as hold holds
// each: every one of today's too, where the List is wanted and the store
// doesn't hold them yet. Each rests on the newest state a digest holds of it
// that stands, as the ledger finds it, but those readsWhole reports true of,
// read whole; and one that can't be held on its state, as heldOf can't hold
// it, is read again, whole. Where the session whose page is open is among
// them, every account's answers on the days read are taken for its points,
// as answering has them.
func (f *feed) read(l LedgerSessions) {
	ids := f.unheld()
	listing := f.want.List && !f.listing
	if len(ids) == 0 && !listing {
		return
	}
	answers, take := f.answering(ids)
	sessions := l.SessionsOf
	if listing {
		sessions, f.listing = l.Sessions, true
	}
	if unrestored := f.holdFound(ids, sessions(ids, f.readsWhole, take), answers); len(unrestored) > 0 {
		f.holdFound(unrestored, l.SessionsOf(unrestored, eachWhole, nil), answers)
	}
}

// readsWhole reports whether a feed reads the session with the given id
// whole, resting on no state a digest holds: the page's, as its tally and
// its points take in every one of its days' answers; and one the store can't
// place a line of, as gained has it, which the same state would leave as
// unplaced, feed after feed, as a session restored onto the day after its
// state's holds none of that day's flights.
func (f *feed) readsWhole(id string) bool {
	return id == f.want.Page || f.whole[id]
}

// answering is, where the session whose page is open is among the sessions
// with the given ids, every account's answers on the days a read of them
// reads, by date, as answeredOf has each, and what takes them in, each line
// read handed it: none where it isn't.
func (f *feed) answering(ids []string) (map[string][]answered, func(string, ledger.Line)) {
	if !slices.Contains(ids, f.want.Page) {
		return nil, nil
	}
	answers := make(map[string][]answered)
	return answers, func(date string, line ledger.Line) {
		if isAnswer(&line) {
			answers[date] = append(answers[date], answeredOf(&line, f.want.Page))
		}
	}
}

// holdFound holds afresh, as hold holds each, the sessions with the given
// ids, each as read gives it, resting on nothing and of none of its lines
// where it gives none, and those others read gives that the store doesn't
// hold, of today's; giving the ids of those that can't be held on their
// states, as heldOf can't hold them, held none of, in order: none where read
// reads each whole.
func (f *feed) holdFound(ids []string, read map[string]ledger.SessionFrom, answers map[string][]answered) []string {
	found := make(map[string]ledger.SessionFrom, len(read))
	for id, from := range read {
		if _, held := f.sessions[id]; !held {
			found[id] = from
		}
	}
	for _, id := range ids {
		found[id] = read[id]
	}
	var unrestored []string
	for id, from := range found {
		if f.hold(id, from, answers) != nil {
			unrestored = append(unrestored, id)
		}
	}
	slices.Sort(unrestored)
	return unrestored
}

// unheld are the ids of the sessions to read, in order: those wanted that
// the store doesn't hold, the List's today's among those met in the read
// among them; and those it can't place a line of.
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

// hold holds the session with the given id afresh, as heldOf has it of
// from, failing where heldOf does, holding nothing.
func (f *feed) hold(id string, from ledger.SessionFrom, answers map[string][]answered) error {
	s, err := f.heldOf(id, from, answers)
	if err != nil {
		return err
	}
	f.sessions[id] = s
	delete(f.before, id)
	f.gains[id] = SessionGain{whole: true}
	f.last.read += s.held()
	return nil
}

// heldOf is the session with the given id held afresh, of from, as a read
// of it found it, on the feed's day, sealed behind its horizon, as horizonOf
// has it: as heldAsOf holds it, but the page's, read whole, as readOf reads
// it, its page tallied, of answers. It fails where heldAsOf does, as from's
// state won't restore, or holds sealed what the horizon holds open, for the
// session to be read whole.
func (f *feed) heldOf(id string, from ledger.SessionFrom, answers map[string][]answered) (*heldSession, error) {
	if id == f.want.Page {
		return readOf(from.Lines, answers, f.prices, f.fed.day, f.horizonOf(id), true), nil
	}
	return heldAsOf(from, f.prices, f.fed.day, f.horizonOf(id))
}

// readOf is a session held afresh of lines, every line of it read whole,
// each request once, as distinct has it, each priced by prices at the day
// d, sealed behind the horizon given, its page tallied, its points of
// answers, every account's on the days read, by date, where paged says.
func readOf(lines []ledger.Line, answers map[string][]answered, prices ledger.Table, d LedgerDay, horizon instant, paged bool) *heldSession {
	return heldSessionOf(pricedAt(distinct(requestsIn(lines)), prices, d.Date), answers, prices, d, horizon, paged)
}

// heldAsOf is the session from gives, as the store would hold it on the day
// d, sealed behind the horizon given, had it read every line of it whole:
// resting on no base, its lines read whole, as readOf has them; else its
// base restored onto d, as restoredFrom has it, and its requests since told
// on, each once, as distinct has them, priced at d, as with has it, sealed
// behind the horizon given. It fails where its base won't decode, or restore
// onto d, or the horizon given is before its base's own, the end of its
// base's day, as the base holds sealed what a whole read would hold open:
// its caller reads it whole then, as it gives no session held otherwise than
// a whole read holds it.
func heldAsOf(from ledger.SessionFrom, prices ledger.Table, d LedgerDay, horizon instant) (*heldSession, error) {
	if from.Base == nil {
		return readOf(from.Lines, nil, prices, d, horizon, false), nil
	}
	s, err := restoredFrom(from.Base, prices, d)
	if err != nil {
		return nil, err
	}
	if horizon.compare(s.horizon) < 0 {
		return nil, fmt.Errorf("hold a session of the digest of %s behind %v: before the end of its day", from.Base.Date, horizon.time())
	}
	if since := requestsIn(from.Lines); len(since) > 0 {
		s = s.with(pricedAt(distinct(since), prices, d.Date))
	}
	return s.sealedAt(horizon), nil
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
// prices, sealed behind the horizon given, its page tallied, its points of
// answers, as pointingOf has them, where paged says.
func heldSessionOf(requests []heldRequest, answers map[string][]answered, prices ledger.Table, d LedgerDay, horizon instant, paged bool) *heldSession {
	s := &heldSession{horizon: horizon, sealed: newStory(prices, d), flights: make(map[string]heldFlights), counts: make(map[string]int)}
	if paged {
		s.page = newPageTally()
		s.page.points = pointingOf(answers, d)
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
// be known from another; one of a request it holds whose line decodes
// otherwise of it, as summed has it, which a read whole may hold in its
// place, as distinct keeps the first of a request's lines it reads; one that
// arrived behind the horizon, which would change what's sealed; or the
// requests of a day before those whose flights it holds other than as many
// as it holds of that day, or not read whole.
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

// heldSum is the sum s holds of the request r, as heldRequest's summed has
// it, reporting false where it holds none: of the same id, arrived at the
// same moment, on the day with the given date, one of those it holds the
// flights of.
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
// its story, and its page's tally where it's tallied, its points among it,
// those at or past it held open, and its story told on of them, afresh.
func (s *heldSession) sealedAt(horizon instant) *heldSession {
	i, _ := slices.BinarySearchFunc(s.open, horizon, func(r heldRequest, h instant) int { return r.at.compare(h) })
	sealed := *s
	sealed.horizon = horizon
	if i > 0 {
		sealed.sealed = s.sealed.told(s.open[:i])
	}
	if s.page != nil {
		page := s.page
		if i > 0 {
			page = page.took(s.open[:i])
		}
		sealed.page = page.pointedAt(horizon, s.day())
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
// changed. It's priced as h is, whatever found is priced by; as last fed
// as the later fed of the two, so a feed after the clock was set back
// behind either knows it was; and as last fed, for the days a read of the
// ledger's tail reads again for it, as fedWith has it, so those, as Unread
// has them, are those any session it holds may have missed lines of; its
// last feed's counts found's where it was never fed; holding every one of
// today's sessions where either does; and counting the more feeds of the
// two.
func (h HeldSessions) With(found HeldSessions) HeldSessions {
	if h.fed.at.IsZero() {
		h.last = found.last
	}
	if h.fedWith(found) {
		h.unread = found.unread
	}
	if found.fed.at.After(h.fed.at) {
		h.fed = found.fed
	}
	sessions := cloneOrMake(h.sessions)
	for id, s := range found.sessions {
		if _, held := sessions[id]; !held {
			sessions[id] = s
		}
	}
	h.sessions, h.listing, h.feeds = sessions, h.listing || found.listing, max(h.feeds, found.feeds)
	return h
}

// fedWith reports whether h given found's sessions is as last fed as found
// is, for the days a read of the ledger's tail reads again for it: where
// found was fed, and h never was; else, of the two that hold a session, the
// earlier fed, as only a session held may have missed lines since; of two
// that hold none, the later fed, as nothing held reads again what was
// missed before then.
func (h HeldSessions) fedWith(found HeldSessions) bool {
	if found.unread.at.IsZero() || h.unread.at.IsZero() {
		return h.unread.at.IsZero()
	}
	holds, finds := len(h.sessions) > 0, len(found.sessions) > 0
	switch {
	case holds != finds:
		return finds
	case holds:
		return found.unread.at.Before(h.unread.at)
	}
	return h.unread.at.Before(found.unread.at)
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
	return slices.DeleteFunc(slices.Clone(missed), func(date string) bool { return date < h.unread.day.Date })
}

// Len is how many sessions h holds.
func (h HeldSessions) Len() int {
	return len(h.sessions)
}

// Holding is what h holds, and what its last feed did, as Holding has them.
func (h HeldSessions) Holding() Holding {
	held := Holding{Day: h.fed.day.Date, Open: make(map[string]int, len(h.sessions)), Gained: h.last.gained, Folded: h.last.folded, Read: h.last.read}
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
	return newStory(h.prices, h.fed.day)
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

// StreamWanted is what the stream s wants of the store, whatever reader
// reads it: each session's requests it tells of pinned, as pinsOf has it.
func StreamWanted(s Stream) SessionsWanted {
	return SessionsWanted{Pins: pinsOf(s)}
}

// Unheld are those of the ids given of sessions h doesn't hold, in their
// order.
func (h HeldSessions) Unheld(ids []string) []string {
	return slices.DeleteFunc(slices.Clone(ids), func(id string) bool { _, held := h.sessions[id]; return held })
}
