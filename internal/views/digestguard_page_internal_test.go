package views

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// The page's part of the reads over digests: each session's page opened
// afresh at each moment, as the watch opens one, its turn open read; s's
// page held open across the moments, as the watch holds it; and s's page as
// sessions <id> prints it. Each is read through the Follower resting on
// digests and held to the same read through the one reading raw, as it's
// shown, never as it's held: a page read raw holds the answers of days a
// page resting on its state never reads, and shows the same.

// pageSession is the id of the spread worlds' session whose page the stores
// hold open, the watch holds open across the moments, and sessions <id>
// prints: pagedRun's.
const pageSession = "s"

// The hard moments the pages read over digests meet, each met by some spread
// world, judged from the reads compared.
const (
	pagedOnYesterdayLate  = "at 23:59, a page opened resting on its state at the day before's end and the chain of parts behind it"
	pagedYesterdayRaw     = "past midnight, a page opened resting on an older state's chain of parts, the day before read raw"
	pagedOnYesterdayEarly = "at 01:30, a page opened resting on its state at the day before's end and the chain of parts behind it"
	pagedHeldAcross       = "a page opened at 23:59 on its chain of parts, held open to half past one, its store fed at each moment"
	pagedRepriced         = "a page held open across a midnight its table's prices change at, resting on its chain of parts"
	pagedTurnAcross       = "a page resting on a chain of parts two days of which a turn of it ran on"
	pagedAgentAcross      = "a page resting on a chain of parts a subagent of it was first seen on a day of, its requests going on a day after"
	pagedPinnedAcross     = "a page pinned by a request in flight across midnight, given its state at the day before's end, read whole"
	pagedRunResumed       = "a page resting on its state of a run of days broken, its session making requests again since"
	pagedIdless           = "a page resting on its chain of parts, the days it's read of holding an answer without an id"
	pagedIdlessOwn        = "a page resting on a chain of parts one of which holds a request of its own without an id"
	pagedFlightAgain      = "a page resting on a state a request of it was in flight at the end of the day of, a line of that request arriving after it, in its flight"
	pagedTurnOpened       = "a page resting on its chain of parts, a turn of it open, its requests read"
	pagedNamedIdle        = "a page resting on a chain of parts of a day naming its session, which made no request of its own on it"
	pagedZoneEarly        = "a page of a session a zone change's early line of is filed on a near day of, asked on its chain of parts, resting on none"
	pagedZoneLate         = "a page of a session a zone change's late line of is filed on a day of, arrived the day after, resting on none or an older state"
	pagedReadWhole        = "a page given its state and its chain of parts, refused, read whole"
	sessionsPrinted       = "sessions <id> read among sessions resting on states of several days, its page on its chain of parts"
)

// pageMoments are the hard moments the pages read over digests meet, but
// those of the tables they're read with and those the upsets make.
var pageMoments = []string{
	pagedOnYesterdayLate, pagedYesterdayRaw, pagedOnYesterdayEarly, pagedHeldAcross, pagedRepriced, pagedTurnAcross, pagedAgentAcross, pagedPinnedAcross,
	pagedRunResumed, pagedIdless, pagedIdlessOwn, pagedFlightAgain, pagedTurnOpened, pagedZoneEarly, pagedZoneLate, pagedReadWhole, sessionsPrinted,
}

// pagePricedEither are the moments of a page resting on its chain of parts,
// built with the built-in table, read with each of the tables given.
func pagePricedEither(prices []pricedAs) []string {
	var moments []string
	for _, p := range prices {
		moments = append(moments, pagePriced(p.name))
	}
	return moments
}

// pagePriced is the moment of a page resting on its chain of parts read with
// the tables named.
func pagePriced(prices string) string {
	return prices + ": a page resting on its chain of parts, built with the built-in table"
}

// shownPage is a session's page as the watch shows it, as Build builds it,
// and whether it built one: its story's times sent a list of them, as
// SentTimes' All gives them, as which of them a page holds sealed is its own.
type shownPage struct {
	BuiltPage
	Built bool
}

// shownPageOf is the page h holds as it's shown, live as live gives it, as
// shownPage has it.
func shownPageOf(h HeldPage, live PageLive) shownPage {
	built, ok := h.Build(live)
	built.Story.Sent = SentAt(slices.Collect(built.Story.Sent.All())...)
	return shownPage{BuiltPage: built, Built: ok}
}

// checkPages holds each page read at the moment m through the Follower
// resting on digests to the same read through the one reading raw, as
// shownPageOf shows each, noting the hard moments each meets: the pages
// opened afresh then, as openPage opens each, of those pagesAt gives; s's
// page held open across the moments, as holdPage holds it; and s's page as
// sessions <id> prints it, as checkPrinted has it. It gives the pages
// opened, by their sessions' ids.
func (w *spread) checkPages(m moment) map[string]openedPage {
	w.t.Helper()
	pages := make(map[string]openedPage)
	for _, id := range w.pagesAt(m) {
		o := w.openPage(m, id)
		shown := shownPageOf(o.based, o.live)
		w.alike(m.now, id+"'s page, opened", shown, shownPageOf(o.raw, o.live))
		w.notePage(m, o, shown)
		pages[id] = o
	}
	w.holdPage(m, pages[pageSession])
	w.checkPrinted(m)
	return pages
}

// pagesAt are the ids of the sessions whose pages are opened at the moment
// m, of those of the world's with a request that arrived by then: none at a
// minute before one, as pagesOpenAt has it; s's at the others; where the day
// before's digest stands, as yesterdayStands has it, one other's, each in
// turn, from the one the world's seed gives, and the one agentAcrossOf
// gives; and the one whose page judges the upset made then, as upsetMade's
// page names it.
func (w *spread) pagesAt(m moment) []string {
	if !pagesOpenAt(m.now) {
		return nil
	}
	begun := sessionsIn(slices.DeleteFunc(slices.Clone(w.lines), func(l ledger.Line) bool { return l.At.After(m.now) }))
	others := slices.DeleteFunc(slices.Clone(begun), func(id string) bool { return id == pageSession })
	pages := slices.DeleteFunc(begun, func(id string) bool { return id != pageSession })
	if len(others) > 0 && yesterdayStands(m.now) {
		pages = append(pages, others[(m.i+int(w.seed))%len(others)])
		if id := w.agentAcrossOf(m.now); id != "" && !slices.Contains(pages, id) {
			pages = append(pages, id)
		}
	}
	if u := w.made; u != nil && u.page != "" && !slices.Contains(pages, u.page) {
		pages = append(pages, u.page)
	}
	return pages
}

// agentAcrossOf is the id of the first of the world's sessions but s, by
// id, of which agentToday reports at now: one whose page, resting on its
// state of the latest day before now's it made a request on, takes its
// subagent's request in on what an older part holds of it, as agentAcross
// judges it; "" of none, and once some world has met that moment, as each
// page opened slows the run. s's own runs of days are too short to hold
// one.
func (w *spread) agentAcrossOf(now time.Time) string {
	if w.met[pagedAgentAcross] {
		return ""
	}
	today := Today(now).Date
	for _, id := range sessionsIn(w.lines) {
		if id != pageSession && w.agentToday(id, today, now) {
			return id
		}
	}
	return ""
}

// agentToday reports whether a subagent of the session with the given id
// made a request on the day with the date today, arrived by now, having
// been first seen before the latest earlier day the session made one on.
func (w *spread) agentToday(id, today string, now time.Time) bool {
	last, first, todays := "", make(map[string]string), make(map[string]bool)
	for _, l := range w.arrivedBy(id, now) {
		if !isRequest(&l) {
			continue
		}
		date := w.filedUnder(l)
		if date < today {
			last = max(last, date)
		}
		if l.AgentID == "" {
			continue
		}
		if f, seen := first[l.AgentID]; !seen || date < f {
			first[l.AgentID] = date
		}
		todays[l.AgentID] = todays[l.AgentID] || date == today
	}
	for agent, date := range first {
		if todays[agent] && date < last {
			return true
		}
	}
	return false
}

// pagesOpenAt reports whether pages are opened afresh at now, of the moments
// the reads are held at: at each but a minute before one, whose reads of the
// day before, read raw, a minute after midnight's read alike.
func pagesOpenAt(now time.Time) bool {
	return now.Hour() != 0 || now.Minute() < 30
}

// openedPage is a session's page opened afresh at a moment through each
// Follower, as openPage opens it, by its session's id: what its reads
// through the Follower resting on digests were asked and gave, got; and what
// it's built of beside its lines, live.
type openedPage struct {
	id         string
	got        *basesGot
	live       PageLive
	based, raw HeldPage
}

// openPage is the page of the session with the given id opened afresh at the
// moment m through each Follower, as the watch opens one: read as ReadPage
// reads it, pinned as m's stream tells, the router listing the world's
// sessions as m has them; then, of s's at a minute before midnight on one
// day in two, where it's held open from, its turn open, as turnToOpen
// chooses it, read, as turnRead has it, as reading a turn's lines reads the
// session's files back, raw, either way.
func (w *spread) openPage(m moment, id string) openedPage {
	read := readingThrough(w.based)
	o := openedPage{id: id, got: read.got, live: PageLive{Routed: Routed(m.running, id), Stream: m.stream.Of(id), Now: m.now}}
	o.based, o.raw = ReadPage(read, id, m.stream, w.prices.read, 0, m.now), ReadPage(m.raw, id, m.stream, w.prices.read, 0, m.now)
	if id == pageSession && m.now.Hour() == 23 && Today(m.now).Start.YearDay()%2 == 0 {
		n := o.turnToOpen()
		o.based, o.raw = turnRead(o.based, n, o.live, w.based), turnRead(o.raw, n, o.live, w.raw)
	}
	return o
}

// turnToOpen is the number of the turn of o's page to open, as its page read
// raw shows them: the newest begun before the end of the day of the state
// its read resting on digests rested it on, the last that state's fold took
// in; else the newest; none where it shows none.
func (o openedPage) turnToOpen() int {
	page, ok := o.raw.Page(o.live)
	if !ok || len(page.Turns) == 0 {
		return 0
	}
	if from := o.got.pages[o.id]; from.Base != nil {
		_, end, _ := dayfile.Day(from.Base.Date)
		if i := slices.IndexFunc(page.Turns, func(t Turn) bool { return t.Started.Before(end) }); i >= 0 {
			return page.Turns[i].Turn
		}
	}
	return page.Turns[0].Turn
}

// turnRead is h with its turn numbered n open and its lines read through l,
// live as live gives it, as the watch opens a turn: Opening it, its lines
// read as ReadTurn reads them, then Turned.
func turnRead(h HeldPage, n int, live PageLive, l LedgerSession) HeldPage {
	h = h.Opening(n, live)
	return h.Turned(h.ReadTurn(l, live))
}

// notePage notes the hard moments the page o, opened at the moment m, meets,
// as its reads through the Follower resting on digests were asked and gave
// it, and as it shows, as shown has it.
func (w *spread) notePage(m moment, o openedPage, shown shownPage) {
	id, now, got := o.id, m.now, o.got
	yesterday := yesterdayOf(Today(now))
	from, rested := got.restedPage(id)
	var base string
	var chain []string
	if rested {
		base, chain = from.Base.Date, partDates(from)
	}
	given := baseDateOf(got.pages[id])
	for _, moment := range []struct {
		name string
		met  bool
	}{
		{name: pagedOnYesterdayLate, met: now.Hour() == 23 && rested && base == yesterday},
		{name: pagedYesterdayRaw, met: now.Hour() == 0 && rested && base < yesterday && w.filedBy(id, yesterday, now)},
		{name: pagedOnYesterdayEarly, met: now.Hour() == 1 && rested && base == yesterday},
		{name: pagedTurnAcross, met: rested && w.turnAcross(id, chain, now)},
		{name: pagedAgentAcross, met: rested && w.agentAcross(id, chain, base, now)},
		{name: pagedPinnedAcross, met: now.Hour() == 1 && got.refusedPage(id) && given == yesterday && pinnedBy(m.stream, id)},
		{name: pagedRunResumed, met: rested && w.resumed(id, base, now)},
		{name: pagedIdless, met: rested && w.idlessOn(slices.Concat(chain, datesAfter(base, now)), now)},
		{name: pagedIdlessOwn, met: rested && w.idlessOf(id, chain)},
		{name: pagedFlightAgain, met: rested && w.flightAgain(id, chain, base, now)},
		{name: pagedTurnOpened, met: rested && shown.Open != 0 && len(shown.Requests) > 0},
		{name: pagedNamedIdle, met: rested && w.namedIdle(id, chain)},
		{name: pagedZoneEarly, met: id == w.early.Session && !w.early.At.After(now) && got.paged[id] && got.none[id] && given == "" &&
			w.digestHolds(dayfile.DateOf(w.early.At), id)},
		{name: pagedZoneLate, met: id == w.late.Session && !w.late.At.After(now) && got.paged[id] && (got.none[id] || given != "") &&
			given < w.filedUnder(w.late) && w.digested(w.filedUnder(w.late))},
		{name: pagedReadWhole, met: got.refusedPage(id)},
		{name: pagePriced(w.prices.name), met: rested},
	} {
		w.met[moment.name] = w.met[moment.name] || moment.met
	}
}

// datesAfter are the dates of the days after the one with the given date
// through the one now falls on, oldest first.
func datesAfter(date string, now time.Time) []string {
	var dates []string
	for d, today := nextDate(date), Today(now).Date; d <= today; d = nextDate(d) {
		dates = append(dates, d)
	}
	return dates
}

// pinnedBy reports whether the stream s tells of a request of the session
// with the given id.
func pinnedBy(s Stream, id string) bool {
	return slices.ContainsFunc(slices.Collect(maps.Values(s.Requests)), func(t Told) bool { return t.Session == id })
}

// arrivedBy are those of the world's lines of the session with the given id
// that arrived by now, in the order they arrived.
func (w *spread) arrivedBy(id string, now time.Time) []ledger.Line {
	var lines []ledger.Line
	for _, i := range arrivalOrder(w.lines) {
		if l := w.lines[i]; l.Session == id && !l.At.After(now) {
			lines = append(lines, l)
		}
	}
	return lines
}

// filedBy reports whether a line of the session with the given id that
// arrived by now is filed under the day with the given date.
func (w *spread) filedBy(id, date string, now time.Time) bool {
	return slices.ContainsFunc(w.arrivedBy(id, now), func(l ledger.Line) bool { return w.filedUnder(l) == date })
}

// turnAcross reports whether a turn of the session with the given id ran on
// two of the days with the given dates: a prompt a request of its own
// conversation's, or a side request's, that arrived by now serves, as its
// turns are told apart, filed under two of them.
func (w *spread) turnAcross(id string, dates []string, now time.Time) bool {
	days := make(map[string]map[string]bool)
	for _, l := range w.arrivedBy(id, now) {
		date := w.filedUnder(l)
		if !isRequest(&l) || l.Prompt == "" || l.AgentID != "" || !slices.Contains(dates, date) {
			continue
		}
		if days[l.Prompt] == nil {
			days[l.Prompt] = make(map[string]bool)
		}
		days[l.Prompt][date] = true
	}
	return slices.ContainsFunc(slices.Collect(maps.Values(days)), func(on map[string]bool) bool { return len(on) > 1 })
}

// agentAcross reports whether a subagent of the session with the given id
// was first seen on a day of the chain with the given dates before the one
// with the date base, the chain's newest, a request of it filed after base
// arriving by now: so the page takes that request in on what an older part
// holds of the subagent.
func (w *spread) agentAcross(id string, chain []string, base string, now time.Time) bool {
	first, after := make(map[string]string), make(map[string]bool)
	for _, l := range w.arrivedBy(id, now) {
		if !isRequest(&l) || l.AgentID == "" {
			continue
		}
		date := w.filedUnder(l)
		if seen, ok := first[l.AgentID]; !ok || date < seen {
			first[l.AgentID] = date
		}
		after[l.AgentID] = after[l.AgentID] || date > base
	}
	for agent, date := range first {
		if after[agent] && date < base && slices.Contains(chain, date) {
			return true
		}
	}
	return false
}

// resumed reports whether the session with the given id made its first
// request after the day with the given date, arriving by now, on a day two
// or more after it, none on the day between: a run of its days broken, then
// begun again.
func (w *spread) resumed(id, date string, now time.Time) bool {
	next := ""
	for _, l := range w.arrivedBy(id, now) {
		if on := w.filedUnder(l); isRequest(&l) && on > date && (next == "" || on < next) {
			next = on
		}
	}
	return next > nextDate(date)
}

// idlessOn reports whether an answer without an id, as from before requests
// had them, that arrived by now, is filed under a day with one of the given
// dates.
func (w *spread) idlessOn(dates []string, now time.Time) bool {
	return slices.ContainsFunc(w.lines, func(l ledger.Line) bool {
		return l.Request == "" && isAnswer(&l) && !l.At.After(now) && slices.Contains(dates, w.filedUnder(l))
	})
}

// idlessOf reports whether a request of the session with the given id
// without an id, as from before requests had them, is filed under a day with
// one of the given dates.
func (w *spread) idlessOf(id string, dates []string) bool {
	return slices.ContainsFunc(w.lines, func(l ledger.Line) bool {
		return l.Session == id && l.Request == "" && isRequest(&l) && slices.Contains(dates, w.filedUnder(l))
	})
}

// flightAgain reports whether a request of the session with the given id,
// filed under a day of the chain with the given dates, was in flight at the
// end of the day with the date base, a line of it of the same id arriving
// after then, by now, while it was: so the page counts the two as one
// request on what its state holds in flight.
func (w *spread) flightAgain(id string, chain []string, base string, now time.Time) bool {
	_, end, _ := dayfile.Day(base)
	arrived := w.arrivedBy(id, now)
	return slices.ContainsFunc(arrived, func(a ledger.Line) bool {
		if a.Request == "" || !slices.Contains(chain, w.filedUnder(a)) || !inFlightAt(&a, end) {
			return false
		}
		return slices.ContainsFunc(arrived, func(b ledger.Line) bool {
			return b.Request == a.Request && !b.At.Before(end) && inFlightAt(&a, b.At)
		})
	})
}

// namedIdle reports whether a day with one of the given dates names the
// session with the given id, as a line of it filed there counts toward it,
// but holds no request of its own.
func (w *spread) namedIdle(id string, dates []string) bool {
	return slices.ContainsFunc(dates, func(date string) bool {
		named, requested := false, false
		for i := range w.lines {
			if l := &w.lines[i]; l.CountedSession() == id && w.filedUnder(*l) == date {
				named, requested = true, requested || isRequest(l)
			}
		}
		return named && !requested
	})
}

// ranBy reports whether the session with the given id made a request of the
// model given that arrived by then.
func (w *spread) ranBy(id, model string, then time.Time) bool {
	return slices.ContainsFunc(w.arrivedBy(id, then), func(l ledger.Line) bool { return isRequest(&l) && l.Model == model })
}

// pageAcross is s's page as the watch holds it open across the moments,
// through each Follower, as heldOpen has each: opened at a minute before
// midnight, as openPage opens it, its session read whole or resting on its
// chain of parts as its reads through the Follower resting on digests, got,
// gave it; then at each moment through half past one rolled on and fed a
// read of the ledger's tail, as tail reads it.
type pageAcross struct {
	opened     time.Time
	got        *basesGot
	tail       ledgerTail
	based, raw heldOpen
}

// heldOpen is a page as the watch holds it open: its store, as the watch's
// holds the page's session while the page alone is open, and the page.
type heldOpen struct {
	store HeldSessions
	page  HeldPage
}

// holdPage holds s's page open across the moments: at a minute before
// midnight, o, the page opened then, afresh, as heldFrom holds it; at each
// moment after, while one is held, the page fed as fed feeds it, held to the
// same through the Follower reading raw, as shownPageOf shows each, noting the
// hard moments it meets.
func (w *spread) holdPage(m moment, o openedPage) {
	w.t.Helper()
	if m.now.Hour() == 23 {
		w.paging = w.heldFrom(m, o)
		return
	}
	p := w.paging
	if p == nil {
		return
	}
	t := p.tail.at(m.now)
	p.based, p.raw = p.based.fed(w.based, t, m), p.raw.fed(m.raw, t, m)
	live := PageLive{Routed: Routed(m.running, pageSession), Stream: m.stream.Of(pageSession), Now: m.now}
	shown := shownPageOf(p.raw.page, live)
	w.alike(m.now, fmt.Sprintf("%s's page, held open since %v", pageSession, p.opened), shownPageOf(p.based.page, live), shown)
	_, rested := p.got.restedPage(pageSession)
	repriced := !w.prices.read.SamePrices(foldOpus, Today(p.opened).Date, Today(m.now).Date)
	w.met[pagedHeldAcross] = w.met[pagedHeldAcross] || m.now.Hour() == 1 && rested
	w.met[pagedRepriced] = w.met[pagedRepriced] || rested && shown.Built && repriced && w.ranBy(pageSession, foldOpus, p.opened)
}

// heldFrom is the page o, opened at the moment m, held open through each
// Follower, as heldOpen holds each, its reads of the ledger's tail from m on:
// none where none was opened, or it shows none, as the watch keeps none
// open.
func (w *spread) heldFrom(m moment, o openedPage) *pageAcross {
	if o.got == nil || !shownPageOf(o.raw, o.live).Built {
		return nil
	}
	_, mark, _ := w.raw.Today(ledger.Mark{})
	return &pageAcross{opened: m.now, got: o.got, tail: ledgerTail{l: w.raw, mark: mark, day: Today(m.now).Date}, based: w.heldOpen(o.based), raw: w.heldOpen(o.raw)}
}

// heldOpen is page held open, its session taken into a store of its own, as
// the watch takes a page's read into its store as it opens.
func (w *spread) heldOpen(page HeldPage) heldOpen {
	store := HoldSessions(w.prices.read).With(page.Found())
	return heldOpen{store: store, page: page.Joined(store)}
}

// fed is h as a read of the ledger's tail, t, at the moment m grows it, as
// the watch grows a page open alone: its store fed t through l, wanting what
// m's stream pins and the page, as Paging has it; its page rolled on to the
// day m falls on, its turn open taking in the lines t read, holding its
// session as the store does.
func (h heldOpen) fed(l LedgerSessions, t Taken, m moment) heldOpen {
	store, _ := h.store.Fed(l, t, StreamWanted(m.stream).Paging(h.page, m.stream), m.now)
	page, _ := h.page.On(Today(m.now).Date)
	page, _ = page.TurnOf(page, tailLines(t)...).Held(store)
	return heldOpen{store: store, page: page}
}

// tailLines are the lines a read of the ledger's tail, t, read: those of each
// day it read again whole, the oldest first, then today's.
func tailLines(t Taken) [][]ledger.Held {
	var lines [][]ledger.Held
	for _, date := range slices.Sorted(maps.Keys(t.read.days)) {
		lines = append(lines, t.read.days[date])
	}
	return append(lines, t.read.gained)
}

// printedPage is a session's page as sessions <id> prints it, and whether it
// prints one.
type printedPage struct {
	Page    SessionPage
	Printed bool
}

// checkPrinted holds s's page as sessions <id> prints it at the moment m,
// where it's half past one on one day in two, the day before's digest
// standing, read through the Follower resting on digests, to the same read
// through the one reading raw, as printed has it: on one of those days in
// two as with no router, on the other as the router lists the world's
// sessions, read among them; noting the moment of its read among sessions
// resting on states of several days.
func (w *spread) checkPrinted(m moment) {
	w.t.Helper()
	day := Today(m.now).Start.YearDay()
	if m.now.Hour() != 1 || day%2 != 0 {
		return
	}
	listed := day%4 == 0
	read := readingThrough(w.based)
	w.alike(m.now, "s's page, as sessions <id> prints it", w.printed(read, m, listed), w.printed(m.raw, m, listed))
	from, rested := read.got.restedPage(pageSession)
	several := rested && slices.ContainsFunc(slices.Collect(maps.Values(read.got.states)), func(o *ledger.SessionBase) bool {
		return o.Date != from.Base.Date
	})
	w.met[sessionsPrinted] = w.met[sessionsPrinted] || several
}

// printed is s's page as sessions <id> prints it at the moment m, read
// through l as the verb reads it: with no router listing the sessions, or,
// where listed says, as the router lists the world's sessions as m has them.
func (w *spread) printed(l Ledger, m moment, listed bool) printedPage {
	var running []status.Session
	if listed {
		running = m.running
	}
	page, ok := SessionPageOf(pageSession, PageSources{
		Routed: Routed(running, pageSession), Unlisted: !listed, Read: ReadSessions(l, running, m.now, pageSession), Prices: w.prices.read, Now: m.now,
	})
	return printedPage{Page: page, Printed: ok}
}

// The hard moments the upsets of a page's parts and points make, each made
// by an upset of a spread world's files.
const (
	partsGone     = "a day's parts file deleted, a page resting on the state of that day falling back on an older state's chain"
	memberDamaged = "a page's member of a day's parts file damaged, the file's size unchanged, the page falling back on an older state's chain"
	takesSplit    = "a model first reading a window only some models share in the run of a page's state, after it, the page given the state, read whole"
)

// pageUpsets are the upsets of s's page's parts and points a spread world's
// files take, each in turn.
var pageUpsets = []upsetting{
	{moment: partsGone, upset: partsSpoiled(removeParts)},
	{moment: memberDamaged, upset: partsSpoiled(damageMember)},
	{moment: takesSplit, upset: (*spread).takesSplit},
	{moment: pagedNamedIdle, upset: (*spread).countedIdle},
}

// judgedLater judges an upset's moment met by none of the reads at the moment
// it's made: those at the moments after judge it, as notePage does.
func judgedLater(moment, map[string]openedPage) bool { return false }

// partsSpoiled is the upset spoiling, as spoil spoils it, s's member of the
// parts file of the day of the state s's page rests on, as the Follower
// resting on digests rests it, where that state's chain of parts holds an
// older one for the page to fall back on: judged met where the page opened
// then is given an older state's chain, held on it or, as its points can't
// take in a day of its own before yesterday on it, read whole, the List's
// reads resting s on the day's state, or a later one, as a part spoiled
// spoils no state.
func partsSpoiled(spoil func(w *spread, path string, member ledger.Member)) func(w *spread, now time.Time) (upsetJudged, string) {
	return func(w *spread, _ time.Time) (upsetJudged, string) {
		from := w.based.SessionsOf([]string{pageSession}, onPage, nil)[pageSession]
		if len(from.Parts) < 2 {
			return nil, ""
		}
		date := from.Base.Date
		spoil(w, w.partsPath(date), from.Base.Part.Member)
		return func(m moment, pages map[string]openedPage) bool {
			o, opened := pages[pageSession]
			if !opened {
				return false
			}
			fell, listed := baseDateOf(o.got.pages[pageSession]), m.based.got.states[pageSession]
			return fell != "" && fell < date && listed != nil && listed.Date >= date
		}, pageSession
	}
}

// partsPath is where the ledger keeps the parts file of the local day with
// the given date, as it names one.
func (c *digestChain) partsPath(date string) string {
	return filepath.Join(ledger.Dir(c.state), "parts-"+date+".gz")
}

// removeParts deletes the parts file at path, as one lost.
func removeParts(w *spread, path string, _ ledger.Member) {
	if err := os.Remove(path); err != nil {
		w.t.Fatal(err)
	}
}

// damageMember damages the member the parts file at path holds where member
// lies, a byte amid it turned, the file's size unchanged, as a disk might.
func damageMember(w *spread, path string, member ledger.Member) {
	data, err := os.ReadFile(path)
	if err != nil {
		w.t.Fatal(err)
	}
	data[member.Offset+member.Length/2] ^= 0xff
	if err := os.WriteFile(path, data, 0o600); err != nil {
		w.t.Fatal(err)
	}
}

// takesSplit files a request of s's of Claude Opus 5.5, arrived a minute
// before now, its answer reading Claude Sonnet's week on the account a
// request of s's of Claude Sonnet's filed on the day of the state s's page
// rests on read it on, as the Follower resting on digests rests it, where s
// made a request on each day from that one through the day before: so, the
// run of its days going on to today, Claude Opus first reads in it a window
// only some models share, which the page's points can't take in on that
// state. Judged met where the page opened then is given that state, and read
// whole, the stream pinning none of s's requests, which would refuse it too.
// That the day's digest builder reads s's page whole again, and the next
// day's rests on it again, is held by
// TestAPageItsPointsCantTakeADayInOnIsFoldedWholeThenRestedOn.
func (w *spread) takesSplit(now time.Time) (upsetJudged, string) {
	from := w.based.SessionsOf([]string{pageSession}, onPage, nil)[pageSession]
	base := baseDateOf(from)
	i := slices.IndexFunc(w.lines, func(l ledger.Line) bool {
		return l.Session == pageSession && l.Model == foldSonnet && l.Limits[sonnetWeek+"-reset"] != "" && w.filedUnder(l) == base
	})
	if base == "" || i < 0 || !w.ranEach(pageSession, base, yesterdayOf(Today(now)), now) {
		return nil, ""
	}
	l := w.lines[i]
	l.Request, l.Model, l.At, l.TotalMS, l.FirstMS = "split-"+now.Format("2006-01-02T15:04"), foldOpus, now.Add(-time.Minute).UTC(), 3000, nil
	l.Limits = map[string]string{sonnetWeek + "-utilization": l.Limits[sonnetWeek+"-utilization"], sonnetWeek + "-reset": l.Limits[sonnetWeek+"-reset"]}
	w.fileLine(l)
	return func(m moment, pages map[string]openedPage) bool {
		o, opened := pages[pageSession]
		return opened && o.got.refusedPage(pageSession) && baseDateOf(o.got.pages[pageSession]) == base && !pinnedBy(m.stream, pageSession)
	}, pageSession
}

// countedIdle files a count of s's tokens, arrived a minute before now, on
// today, where no line of s's is filed, and no day of the world's names s
// without a request of its own yet, as namedIdle has it: so the day names
// s, though s makes no request of its own on it, and the state of s the
// day's digest holds, its points holding none of the day's answers, is the
// one the page rests on from the day after. Its moment is judged as the
// pages opened then rest on that state, as notePage judges them, none at
// the moment it's made.
func (w *spread) countedIdle(now time.Time) (upsetJudged, string) {
	today := Today(now).Date
	i := slices.IndexFunc(w.lines, func(l ledger.Line) bool { return l.Session == pageSession && isRequest(&l) })
	if i < 0 || slices.ContainsFunc(w.lines, func(l ledger.Line) bool { return l.Session == pageSession && w.filedUnder(l) == today }) ||
		w.namedIdle(pageSession, w.filedDates()) {
		return nil, ""
	}
	l := w.lines[i]
	l.Request, l.Kind, l.At, l.TotalMS, l.FirstMS, l.Limits = "count-"+now.Format("2006-01-02T15:04"), ledger.KindCount, now.Add(-time.Minute).UTC(), 300, nil, nil
	w.fileLine(l)
	return judgedLater, ""
}

// filedDates are the dates of the days the world's lines are filed under,
// as filedUnder has them, each once, oldest first.
func (w *spread) filedDates() []string {
	dates := make([]string, len(w.lines))
	for i, l := range w.lines {
		dates[i] = w.filedUnder(l)
	}
	return slices.Compact(slices.Sorted(slices.Values(dates)))
}

// ranEach reports whether the session with the given id made a request on
// each of the days from the one with the date from through the one with the
// date through, that arrived by now.
func (w *spread) ranEach(id, from, through string, now time.Time) bool {
	for date := from; date <= through; date = nextDate(date) {
		if !slices.ContainsFunc(w.arrivedBy(id, now), func(l ledger.Line) bool { return isRequest(&l) && w.filedUnder(l) == date }) {
			return false
		}
	}
	return true
}

// sonnetWeek is the name Claude Sonnet's week, a window only some models
// share, has among an answer's limits.
const sonnetWeek = "7d_sonnet"
