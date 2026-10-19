package views

import (
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// HeldPage is a session's page as it's held between looks, so a look, a
// stream event or a tick works out only what they change: what it's doing,
// its turns as the stream adds to them, and the clock's words. It holds its
// session as the store holds it, its requests sealed into its story and its
// page's tally, its points among it, the recent held open, as HeldSessions
// holds the page's session, read as it's held, resting on its page, and as
// the store holds it from the first feed after; and of its turn open, the
// lines of its requests, read as it opened, and those the ledger gained
// since. On rolls it on to a later day; Held takes its session as the store
// fed it; Build builds the page of what's held, what the router lists and
// what the stream told. What changes it returns afresh, so a copy holding it
// never sees it change, sharing what it held before, which nothing changes.
type HeldPage struct {
	id     string
	day    LedgerDay
	prices ledger.Table
	keep   time.Duration
	// record is the session as the page holds it, its page tallied, as read
	// at read, or as the store last fed it; joined how many feeds the
	// store had had as the read joined it, as Joined has it.
	record *heldSession
	read   time.Time
	joined int
	turn   *turnLines
}

// turnLines are the lines of the requests of a session's turn open, as its
// page holds them: the prompt the turn serves, by which it's known however
// its number changes, as one of a request sent before it comes later;
// whether its lines have been read; and the records of the lines, of those
// read and those the ledger gained since it opened, of its requests, as
// mine has them.
type turnLines struct {
	prompt string
	read   bool
	lines  []lineRecord
}

// HeldTurn is a read of the lines of the requests of a session's turn, as
// ReadTurn reads them: the prompt the turn serves, and the records of the
// lines, newest first.
type HeldTurn struct {
	prompt string
	lines  []lineRecord
}

// lineKey is a line as a held page knows it from another: its request's id,
// and when it arrived.
type lineKey struct {
	request string
	at      instant
}

// PageLive is what a session's page is built of beside its lines: the
// session as the router lists it, or while it doesn't answer, as its last
// listing had it, as the List has it then, nil where it doesn't; whether no
// router lists the sessions, as ListLive's Unlisted has it; what the request
// stream told; and now.
type PageLive struct {
	Routed   *status.Session
	Unlisted bool
	Stream   Stream
	Now      time.Time
}

// Kept is l with the listing of the session its own, so what changes the
// router's listing it was read from in place changes not l.
func (l PageLive) Kept() PageLive {
	if l.Routed != nil {
		routed := keptListing(*l.Routed)
		l.Routed = &routed
	}
	return l
}

// Same reports whether a page built of l, of the same lines, is the page
// built of o, but for the clock, as BuiltPage's Until has it, and what a
// request's answer has streamed, as SessionPage's Live has it: the router
// lists the session alike, as sameListing has it, or neither lists it; it's
// alike whether a router lists the sessions; and the stream tells the same
// rows, as Stream's SameRows has them. A look listing the session the same
// builds no page again.
func (l PageLive) Same(o PageLive) bool {
	return sameListing(l.Routed, o.Routed) && l.Unlisted == o.Unlisted && l.Stream.SameRows(o.Stream)
}

// HoldPage holds the page of the session with the given id, of what read
// holds of it, and of the accounts it ran on, at now: its requests priced by
// prices at the day now falls on, and keep the ledger's [ledger] keep, as
// ledgerKept reads it, every one of them held open, as a page built of
// whatever the stream tells, at whatever now, takes each in in its place:
// so one read resting on its page, its state holding those before its day's
// end sealed, is read again whole, as holdPage has it.
func HoldPage(id string, read SessionsRead, prices ledger.Table, keep time.Duration, now time.Time) HeldPage {
	return holdPage(id, read, prices, keep, unsealed, now)
}

// unsealed is a horizon before every moment, behind which no request is
// sealed.
var unsealed = instant{sec: math.MinInt64}

// ReadPage reads, through l, at now, the page of the session with the given
// id, as ReadSession reads it once its id is known whole, and holds it, as
// HoldPage does, but sealed behind the horizon the store gives it, pinned as
// the stream s tells, as a page the watch opens is: so the store, taking
// the session in as the page holds it, holds it as it would have read it.
func ReadPage(l LedgerSessions, id string, s Stream, prices ledger.Table, keep time.Duration, now time.Time) HeldPage {
	return holdPage(id, ReadSession(l, now, id), prices, keep, horizonAt(now, pinsOf(s)[id]), now)
}

// holdPage holds the page of the session with the given id, as HoldPage
// does, sealed behind the horizon given: on what read rests it on, as
// heldAsOf holds it, its points of the answers read took; or, where it can't
// be held so, as its state won't restore or the horizon is before its end,
// read again whole, as read's whole reads it.
func holdPage(id string, read SessionsRead, prices ledger.Table, keep time.Duration, horizon instant, now time.Time) HeldPage {
	day := Today(now)
	record, err := heldAsOf(id, read.sessions[id], read.answers, prices, day, horizon, true)
	if err != nil {
		whole := read.whole(id)
		record = readOf(whole.sessions[id].Lines, whole.answers.of(id, ""), prices, day, horizon, true)
	}
	return HeldPage{id: id, day: day, prices: prices, keep: ledgerKept(keep), record: record, read: now}
}

// Found is the store of h's session alone, as h holds it, read as it was
// held, for the watch's store to take in, as HeldSessions' With has it.
func (h HeldPage) Found() HeldSessions {
	read := fedOn(h.read)
	return HeldSessions{prices: h.prices, fed: read, unread: read, sessions: map[string]*heldSession{h.id: h.record}}
}

// Joined is h as its read joins store, which holds its session, as Found
// gives it, from then on, as HeldSessions' With takes it in: or holds it
// already, of an earlier read, missing what h read since its last feed.
func (h HeldPage) Joined(store HeldSessions) HeldPage {
	h.joined = store.feeds
	return h
}

// Held is h holding its session as store holds it, reporting whether that
// changed it: where store holds it tallied as its page tallies it, fed
// since h's read joined it, as Joined has it, as the store holds the session
// whose page it's fed wanting, as SessionsWanted's Paging has it, from its
// first feed after. Else it's h, unchanged: the store holds none of the
// page's that's newer.
func (h HeldPage) Held(store HeldSessions) (HeldPage, bool) {
	s, ok := store.sessions[h.id]
	if !ok || s.page == nil || s == h.record || store.feeds <= h.joined {
		return h, false
	}
	h.record = s
	return h, true
}

// On is h at the day with the given date, as a read of the ledger's tail on
// it finds it, reporting whether it rolled on to it, so a page built of it
// before no longer stands: a later day than h is held at rolls it on to
// it; an earlier one, of a ledger yet to turn its day, never rolls it back.
// What the read gained it holds as the store does, as Held takes it, its
// points among it, and its turn open's lines as TurnOf takes them.
func (h HeldPage) On(date string) (HeldPage, bool) {
	if date <= h.day.Date {
		return h, false
	}
	h.day = dayOf(date)
	return h, true
}

// TurnOf is h with the turn open was has open, as Opening and Turned have
// it, the lines of its requests among those given, read of the ledger
// since, taken into it, as mine has them, each decoded into its record only
// once mine has taken it and the turn holds none of its id, as heldID has
// it, as a read afresh gives again the lines it holds: so a page grown of a
// read of the ledger's tail off the event loop, of a copy of the page from
// before its turn opened, holds the turn open as the page holds it now.
func (h HeldPage) TurnOf(was HeldPage, read ...[]ledger.Held) HeldPage {
	h.turn = was.turn
	if h.turn == nil {
		return h
	}
	var held map[turnKey]int
	var mine []lineRecord
	for _, lines := range read {
		for i := range lines {
			l := &lines[i].Line
			if !h.mine(turnFactsOf(l), false) {
				continue
			}
			if held == nil {
				held = h.turn.held()
			}
			if !heldID(held, flightOf(l)) {
				mine = append(mine, lineRecordOf(l))
			}
		}
	}
	if len(mine) > 0 {
		h.turn = h.turn.with(mine)
	}
	return h
}

// turnFacts are what mine reads of a request: the session it's of, whether
// it's a session's request, as isRequest has it, the subagent that sent it,
// and the prompt it serves. A line gives them undecoded, as turnFactsOf has
// them, and its record the same, as its turnFacts has them.
type turnFacts struct {
	session, agentID, prompt string
	request                  bool
}

// turnFactsOf are the turn facts of the line l, read of it undecoded.
func turnFactsOf(l *ledger.Line) turnFacts {
	return turnFacts{session: l.Session, agentID: l.AgentID, prompt: l.Prompt, request: isRequest(l)}
}

// turnFacts are the turn facts of r, as its line gives them.
func (r *lineRecord) turnFacts() turnFacts {
	return turnFacts{session: r.session, agentID: r.agentID, prompt: r.prompt, request: r.isRequest()}
}

// mine reports whether the request with the facts f may be of h's turn
// open: of its session's requests, every subagent's, as the first request a
// subagent made, which names its turn, may land after those the page knows
// of, and may have arrived before the turn began, as early says it did;
// else, of one that arrived since, one naming the prompt. It's asked of a
// line's facts before the line is decoded, so a line it passes over never
// is. Which a turn's member is, Build tells, as member has it, of the
// agents it knows then.
func (h HeldPage) mine(f turnFacts, early bool) bool {
	switch {
	case f.session != h.id || !f.request:
		return false
	case f.agentID != "":
		return true
	}
	return !early && f.prompt == h.turn.prompt
}

// with is t with the records given among its own, afresh: each of a request
// with an id once, as its key knows it; one without, as often as the records
// given hold it beyond those t holds alike, as their sums know them, so a
// line both read and gained is held once, and one the ledger's files hold
// twice, twice.
func (t *turnLines) with(lines []lineRecord) *turnLines {
	grown := *t
	grown.lines = slices.Clip(t.lines)
	held := t.held()
	for i := range lines {
		rec := &lines[i]
		switch k := turnKeyOf(rec); {
		case heldID(held, rec.flight):
		case held[k] > 0:
			held[k]--
		default:
			grown.lines = append(grown.lines, *rec)
			if k.request != "" {
				held[k]++
			}
		}
	}
	return &grown
}

// held are the keys of t's lines, as turnKeyOf has them, each counted.
func (t *turnLines) held() map[turnKey]int {
	held := make(map[turnKey]int, len(t.lines))
	for i := range t.lines {
		held[turnKeyOf(&t.lines[i])]++
	}
	return held
}

// heldID reports whether held, the keys of a turn's lines, as held counts
// them, hold the line of the request that flew as f by its id, as idKey
// keys it: a line with passes over, and TurnOf never decodes. Never of one
// without an id, whose key holds its record's sum, as turnKeyOf has it.
func heldID(held map[turnKey]int, f flight) bool {
	k, ok := f.idKey()
	return ok && held[k] > 0
}

// turnKey is a line as a turn's lines know it from another: as its flight's
// key has it, and, of a request without an id, the sum of its record, as its
// summed has it.
type turnKey struct {
	lineKey
	sum uint64
}

// turnKeyOf is the turnKey of the record r: its flight's, as idKey has it,
// with, where it has no id, its sum.
func turnKeyOf(r *lineRecord) turnKey {
	k, ok := r.idKey()
	if !ok {
		k.sum = r.summed()
	}
	return k
}

// idKey is the turnKey of the line of the request that flew as f, as its
// key has it, reporting false where it has no id, whose turnKey holds its
// record's sum too, as turnKeyOf has it.
func (f flight) idKey() (turnKey, bool) {
	k, ok := f.key()
	return turnKey{lineKey: k}, ok
}

// Opening is h with its turn numbered n open, from 1 at its first, as live
// builds its page, the lines of its requests to be read, as ReadTurn reads
// them, those the ledger gains of them from now on held for it meanwhile:
// none open, where it has no such turn, as Closing has it.
func (h HeldPage) Opening(n int, live PageLive) HeldPage {
	h = h.Closing()
	if n < 1 {
		return h
	}
	if turns := h.worked(live).tally.turns; n <= len(turns) {
		h.turn = &turnLines{prompt: turns[n-1].prompt}
	}
	return h
}

// Closing is h with no turn open, holding the lines of none.
func (h HeldPage) Closing() HeldPage {
	h.turn = nil
	return h
}

// ReadTurn reads, through l, the lines of the requests of h's turn open, as
// live builds its page: the session's lines, the latest arrived first, back
// to arrivesWithin before the turn began, as the line of a request it first
// knew of from the stream may have arrived as long before it was sent;
// those that may be its requests among them, as mine has them. Further back,
// to the horizon its session is held open from, it reads the subagents'
// lines the session as held doesn't hold, as the store has yet to be fed
// them: a subagent's turn is the one its first request names, which may have
// arrived before the turn began, and be filed only since, untold by the
// stream. It reads the turn's own days alone, and the horizon's, as
// turnSpans has them, none between; and decodes a line into its record only
// once it's taken it. None where no turn is open.
func (h HeldPage) ReadTurn(l LedgerSession, live PageLive) HeldTurn {
	if h.turn == nil {
		return HeldTurn{}
	}
	read := HeldTurn{prompt: h.turn.prompt}
	w := h.worked(live)
	i, ok := w.tally.prompts[read.prompt]
	if l == nil || !ok {
		return read
	}
	from := instantOf(w.tally.turns[i].started.Add(-arrivesWithin))
	for held := range l.Session(h.id, w.turnSpans(w.tally.turns[i])...) {
		at := instantOf(held.At)
		early := at.compare(from) < 0
		switch {
		case early && at.compare(w.rec.horizon) < 0:
			return read
		case !h.mine(turnFactsOf(&held.Line), early), early && w.rec.linedAt(held.Request, held.At):
			continue
		}
		read.lines = append(read.lines, lineRecordOf(&held.Line))
	}
	return read
}

// turnSpans are the stretches of time ReadTurn reads the lines of the turn
// t of, at w's now: from arrivesWithin before it began to when its last
// request ended, never before it began, as a line's time taken may be
// hand-edited below none, or to now while one is pending, as a member's line
// arrives by when its request ends, and every one the store holds, sealed
// behind its horizon, is among those it's tallied of; and from the horizon
// on, as the store has yet to be fed the lines since. None, to read every
// day, where the horizon is before every moment.
func (w worked) turnSpans(t *turnTally) []ledger.Span {
	if w.rec.horizon == unsealed {
		return nil
	}
	turn := ledger.Span{From: t.started.Add(-arrivesWithin), To: w.live.Now}
	if !t.pending {
		turn.To = later(instantOf(t.last.end), instantOf(t.started)).time()
	}
	return []ledger.Span{turn, {From: w.rec.horizon.time(), To: w.live.Now}}
}

// Turned is h with the lines of its turn open read, as t read them, among
// those the ledger gained since it opened: h, where t is of another turn
// than h has open.
func (h HeldPage) Turned(t HeldTurn) HeldPage {
	if h.turn == nil || h.turn.prompt != t.prompt {
		return h
	}
	read := (&turnLines{prompt: t.prompt, read: true}).with(t.lines)
	h.turn = read.with(h.turn.lines)
	return h
}

// BuiltPage is a session's page as Build builds it: the page; its story;
// the number of its turn open, from 1 at its first, 0 where none is, which
// changes as a request sent before it comes later, and its requests, none
// while they're read, as RequestsLoading says; and until when it stands by
// the clock, as Until has it.
type BuiltPage struct {
	Page     SessionPage
	Story    Story
	Open     int
	Requests []LogLine
	// RequestsLoading is set while the lines of the requests of its turn
	// open are read, as ReadTurn reads them.
	RequestsLoading bool
	// ReadsWhole is set, and nothing else built, where its store's next feed
	// reads its session whole, as no page built before it of what it holds
	// stands: the stream tells of a request that joined late, behind its
	// horizon, as readsWhole has it.
	ReadsWhole bool
	// Until is the first moment a page built of the same would be built
	// otherwise, by the clock alone: the next minute, as its story steps on
	// a minute at a time, and its words count whole minutes; or sooner, as a
	// model of a session running stops running, as stops has it, as the
	// stream's sight of the session lapses, as saw has it, or, with no router
	// listing the sessions, its last request does, as activity ends it, or as
	// its cache runs out, which a move no longer costs, as moveCost has it. A
	// request's row in flight counts on as it's drawn, as RequestRow.Live has
	// it.
	Until time.Time
}

// Build is the page of the session h holds, as Page builds it, with its
// story, as Story tells it, and its turn open and its requests, as
// TurnRequests has them, all three of one working of what h holds and what
// live gives, as worked has it, and until when it stands, as BuiltPage's
// Until has it: reporting false where Page does, with until when that
// stands. Where its session is to be read whole, it builds none of it, as
// BuiltPage's ReadsWhole has it.
func (h HeldPage) Build(live PageLive) (BuiltPage, bool) {
	if h.readsWhole(live) {
		return BuiltPage{ReadsWhole: true}, true
	}
	w := h.worked(live)
	page, ok := w.page()
	if !ok {
		return BuiltPage{Until: w.until()}, false
	}
	open := w.open()
	requests, loading := w.turnRequests(open)
	return BuiltPage{Page: page, Story: w.story(page), Open: open, Requests: requests, RequestsLoading: loading, Until: w.until()}, true
}

// readsWhole reports whether the store's next feed reads h's session whole,
// as live gives the page: the stream telling of a request that joined late,
// as joinedLate has it, as the feed checks it.
func (h HeldPage) readsWhole(live PageLive) bool {
	return joinedLate(live.Now, pinsOf(live.Stream)[h.id], h.record.horizon)
}

// open is the number of the turn open of the page w is built of, as
// BuiltPage's Open has it.
func (w worked) open() int {
	if w.h.turn == nil {
		return 0
	}
	if i, ok := w.tally.prompts[w.h.turn.prompt]; ok {
		return i + 1
	}
	return 0
}

// until is when a page built of w stands otherwise by the clock alone, as
// BuiltPage's Until has it.
func (w worked) until() time.Time {
	now := w.live.Now
	until := now.Truncate(time.Minute).Add(time.Minute)
	sooner := func(at time.Time) {
		if at.After(now) && at.Before(until) {
			until = at
		}
	}
	for _, s := range w.by.running {
		for _, a := range s.Assignments {
			sooner(stops(a))
		}
	}
	for _, k := range w.by.h.sat[w.h.id] {
		sooner(lapsed(w.by.h.seats[k].last))
	}
	if s := w.rec.story; w.by.unlisted && s.begun {
		sooner(lapsed(s.last.arrived()))
	}
	if _, _, cold := w.rec.story.moving(); !cold.IsZero() {
		sooner(cold)
	}
	return until
}

// worked is what a page of the session h holds is built of, worked out
// once for the page, its story and its turn open's requests alike: h; its
// session as it holds it at live's now, rolled on to the day now falls on
// where it's held at an earlier one; live; the sessions as by lists them,
// the router listing the session where live says it does, the stream
// telling what live gives; the session's requests the stream told of; and
// its page's tally, of its requests sealed, with those it holds open and
// those the stream told of whose lines haven't come taken in, as sentOf has
// them.
type worked struct {
	h     HeldPage
	rec   *heldSession
	live  PageLive
	by    lister
	told  []Told
	tally *pageTally
}

// worked is what a page of the session h holds is built of, of what live
// gives.
func (h HeldPage) worked(live PageLive) worked {
	rec := h.record
	if day := Today(live.Now); rec.day().Date < day.Date {
		rec = rec.on(day, h.prices)
	}
	var running []status.Session
	if live.Routed != nil {
		running = []status.Session{*live.Routed}
	}
	w := worked{h: h, rec: rec, live: live, by: newLister(running, live.Unlisted, live.Stream, live.Now)}
	w.told = w.by.h.told[h.id]
	w.tally = rec.page.told(sentOf(rec.open, w.told, rec.lined))
	return w
}

// Page is the page of the session h holds, as SessionPageOf builds it, of
// what h holds of its lines and what live gives, reporting false where it
// has no request, and the router doesn't list it.
func (h HeldPage) Page(live PageLive) (SessionPage, bool) {
	return h.worked(live).page()
}

// page is the page as Page builds it, of w.
func (w worked) page() (SessionPage, bool) {
	h, rec, p, live, by := w.h, w.rec, w.tally, w.live, w.by
	routed, running := by.routed[h.id]
	if !rec.story.begun && !running {
		return SessionPage{}, false
	}
	priced := pricing{prices: h.prices, date: rec.day().Date}
	listed := by.listed(h.id, rec.story)
	page := SessionPage{
		Session: listed.Session, Dir: listed.Dir, Account: listed.Account, Model: listed.Model, Running: listed.Running, LastSeen: listed.LastSeen,
		Models: listed.Models, State: listed.State, MoveCost: listed.MoveCost, Ended: listed.Ended, Started: listed.Started,
		Resumed: p.resumed, Accounts: p.accountsSaid(priced), Moves: movesOf(rec.story), Activity: listed.Activity,
	}
	unplaced := running && by.h.unplaced(routed)
	page.Turns = turnsOf(p.turns, p.inNone, page.Runs(), unplaced, priced)
	page.Goes = by.goesOf(h.id, live.Stream, p)
	if running {
		page.Pin = routed.Pin
		if !live.Unlisted {
			page.Auto = autoOf(routed, page.Model)
		}
	}
	if !page.Running && rec.story.begun {
		page.KeptUntil, _ = dayfile.KeptUntil(Today(rec.story.first.arrived()).Date, h.keep)
	}
	page.Totals = p.totalsOf(len(page.Turns), priced)
	return page, true
}

// TurnRequests are the Log's lines of the requests of the session's turn
// numbered n, from 1 at its first, as a turn opened shows them, oldest
// first, of what h holds of the turn open and what the stream told, as live
// gives it: each its row as the Log's, by its line, as LogRows builds it,
// with the session's story, or before its line comes, by what the stream
// told; each with no mark, as a turn's rows have none; and its details, as
// Details gives them. None where it has no such turn, or it isn't the turn
// open, its lines read, as Opening and Turned have them.
func (h HeldPage) TurnRequests(n int, live PageLive) []LogLine {
	w := h.worked(live)
	if n != w.open() {
		return nil
	}
	requests, _ := w.turnRequests(n)
	return requests
}

// turnRequests are the requests of the turn open, numbered n, as
// TurnRequests has them, of w, reporting whether they're still being read:
// none where none is open.
func (w worked) turnRequests(n int) ([]LogLine, bool) {
	t := w.h.turn
	switch {
	case n == 0:
		return nil, false
	case !t.read:
		return nil, true
	}
	tally := w.membership()
	prompt := t.prompt
	var lines []*lineRecord
	for i := range t.lines {
		if rec := &t.lines[i]; rec.isRequest() && tally.member(rec.agentID, rec.prompt, prompt) {
			lines = append(lines, rec)
		}
	}
	slices.SortStableFunc(lines, func(a, b *lineRecord) int { return a.compare(b.flight) })
	var told []Told
	for _, r := range w.told {
		if !w.lined(r) && tally.member(r.AgentID, r.Prompt, prompt) {
			told = append(told, r)
		}
	}
	return w.rowsOf(lines, told), false
}

// membership is the tally the turn open's rows are counted in their turn
// by, as member has it: w's, but where the turn's lines hold one of a
// request the session as w holds it doesn't, read of the ledger past what
// the store has been fed, its tally of what it holds open with those lines
// among them, as a subagent's turn is the one its first request names,
// which may be such a line.
func (w worked) membership() *pageTally {
	var unfed []heldRequest
	for i := range w.h.turn.lines {
		if rec := &w.h.turn.lines[i]; rec.isRequest() && rec.at.compare(w.rec.horizon) >= 0 && !w.rec.linedAt(rec.request, rec.arrived()) {
			unfed = append(unfed, newHeldRequest(rec.decodedRequest))
		}
	}
	if len(unfed) == 0 {
		return w.tally
	}
	return w.rec.page.told(sentOf(withRequests(w.rec.open, unfed), w.told, w.lined))
}

// lined reports whether a line of the request t the stream told of is held
// by what the turn open's rows show: the session as w holds it, as its
// lined has it, or the turn's own lines, read of the ledger past what the
// store has been fed, as holds has it.
func (w worked) lined(t Told) bool {
	return w.rec.lined(t) || w.h.turn != nil && w.h.turn.holds(t)
}

// holds reports whether t holds the line of the request r the stream told
// of, as r's filedAs has it of each.
func (t *turnLines) holds(r Told) bool {
	for i := range t.lines {
		if r.filedAs(t.lines[i].flight) {
			return true
		}
	}
	return false
}

// rowsOf are the Log's lines of a turn's requests, as turnRequests has
// them: of the records of its lines, in order, as inOrder has it, and of
// told, the requests the stream told of whose lines haven't come, the first
// sent first, each after the lines that arrived by when it was sent.
func (w worked) rowsOf(lines []*lineRecord, told []Told) []LogLine {
	rows := make([]LogLine, 0, len(lines)+len(told))
	date := Today(w.live.Now).Date
	next := 0
	line := func() {
		rec := lines[next]
		row := LogLine{Row: lineRow(rec, w.rec.story), record: rec}
		row.Row.Worth = rec.worth(w.h.prices, date)
		rows, next = append(rows, row), next+1
	}
	for _, t := range told {
		for next < len(lines) && !lines[next].arrived().After(t.Sent) {
			line()
		}
		rows = append(rows, LogLine{Row: toldRow(t, w.live.Now)})
	}
	for next < len(lines) {
		line()
	}
	for i := range rows {
		rows[i].Row.Mark = NoMark
	}
	return rows
}

// Awaited are the dates of the days before the day live's now falls on that
// the line of a request of the session h holds the page of is filed on, as
// Told.FiledOn has it, the stream having told its end, its line yet to
// come, as its row's stage says, each once, the oldest first: a later day's
// tail of the ledger never reads it, as the Log's Requests await theirs.
// None is awaited of a request the stream told no sending of. The session's
// requests are gone through only where the stream told of one so ended, as
// it's asked at every look, and one is so only about midnight.
func (h HeldPage) Awaited(live PageLive) []string {
	today := Today(live.Now)
	awaits := func(t Told) (LedgerDay, bool) {
		if toldRow(t, live.Now).Stage != RowEnded {
			return LedgerDay{}, false
		}
		day, ok := t.FiledOn()
		return day, ok && day.Start.Before(today.Start)
	}
	told := live.Stream.hearing().told[h.id]
	if !slices.ContainsFunc(told, func(t Told) bool { _, ok := awaits(t); return ok }) {
		return nil
	}
	var dates []string
	for _, t := range told {
		if day, ok := awaits(t); ok && !h.record.lined(t) && !slices.Contains(dates, day.Date) {
			dates = append(dates, day.Date)
		}
	}
	return dates
}

// Unread are the dates of those of the days missed, the days the ledger's
// tail followed no further, oldest first, that a read of it reads again
// whole for the page h holds: the day h is held at and those after it, as
// it read or took in every line filed on a day before it.
func (h HeldPage) Unread(missed []string) []string {
	return slices.DeleteFunc(slices.Clone(missed), func(date string) bool { return date < h.day.Date })
}

// unplaced reports whether the router lists a request of the session s in
// flight that the request stream doesn't place: on a model's seat it hasn't
// told of, or with the stream unheard, as by the verbs, or from a router
// from before it.
func (h hearing) unplaced(s status.Session) bool {
	return slices.ContainsFunc(s.Assignments, func(a status.Assignment) bool {
		_, told := h.seats[Seat{Session: s.ID, Model: a.Model, Account: a.Account}]
		return a.InFlight != "" && (!h.live || !told)
	})
}

// goesOf are where the models of the session with the given id go now, as
// its page's row 7 has them: where the router lists it, as it lists them,
// the moves the stream s told of since, as unlisted has them, taken in, each
// with the reason and time of the move that took it there; else each model's
// latest account, in the order the models first came, as its requests as
// its turns take them in, sent, give them, as its tally p holds them, each
// with the reason of its latest line, of its requests, oldest first, the
// router didn't keep where it was. Either way, a model that came there by
// starting there gives why a request since kept it there, as keptLines has
// it.
func (by lister) goesOf(id string, s Stream, p *pageTally) []SessionModel {
	kept := p.kept
	if _, listed := by.routed[id]; listed {
		var goes []SessionModel
		for _, a := range by.goes[id] {
			goes = append(goes, SessionModel{Model: a.Model, Account: a.Account, Reason: kept[a.Model].why(a.Account, a.Reason), Since: cameAt(a)})
		}
		for _, m := range newestMoves(s.unlisted()) {
			if i := slices.IndexFunc(goes, func(g SessionModel) bool { return m.Session == id && g.Model == m.Model && g.Account == m.To }); i >= 0 {
				goes[i].Reason, goes[i].Since = m.Reason, m.At
			}
		}
		return goes
	}
	goes := slices.Clone(p.goes)
	for i, g := range goes {
		goes[i].Reason = kept[g.Model].why(g.Account, kept[g.Model].came)
	}
	return goes
}

// keptLines are what a session model's lines tell of why it goes where it
// does: the account its latest went to, the reason of its latest the router
// didn't keep where it was, which the router keeps as the reason its
// assignment came by, and its latest's own reason where the router kept it.
type keptLines struct {
	account, came, kept string
}

// why is why the model goes to account, having come there for the reason
// came: where it came by starting there, why its latest line there says a
// request since kept it there, as the router never says; else came.
func (k keptLines) why(account, came string) string {
	if k.kept != "" && k.account == account && (came == "" || status.MovedNothing(came)) {
		return k.kept
	}
	return came
}
