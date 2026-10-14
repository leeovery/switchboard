package views

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// HeldPage is a session's page as its lines build it, held between looks,
// so a look, a stream event or a tick works out only what they change: what
// it's doing, its turns as the stream adds to them, and the clock's words.
// It holds its requests, each priced once; its story; every account's
// requests on its days, each answered once, for the points; and the accounts
// it ran on, with what they took. Take takes in the lines a day gained since,
// each once; Page builds the page of what's held, what the router lists and
// what the stream told. What changes it returns afresh, so a copy holding it
// never sees it change.
type HeldPage struct {
	id     string
	day    LedgerDay
	prices ledger.Table
	keep   time.Duration
	// held are the lines taken, of whichever session, by keyOf: none where
	// they have no id.
	held     map[lineKey]bool
	answers  map[string][]answered
	requests []request
	story    *story
	accounts []SessionAccount
}

// lineKey is a line as a held page knows it from another: its request's id,
// and when it arrived.
type lineKey struct {
	request string
	at      int64
}

// keyOf is the line l's key, reporting false where it has no id, as a line
// from before the router gave requests ids, which is taken as often as it's
// given.
func keyOf(l *ledger.Line) (lineKey, bool) {
	return lineKey{request: l.Request, at: l.At.UnixNano()}, l.Request != ""
}

// PageLive is what a session's page is built of beside its lines: the
// session as the router lists it, nil where it doesn't; whether no router
// lists the sessions; the session as the router's last listing had it, nil
// where it didn't, which counts only while no router lists them, and then
// for its own pin alone, as the pin stands at the router all the same; what
// the request stream told; and now.
type PageLive struct {
	Routed     *status.Session
	Unlisted   bool
	LastRouted *status.Session
	Stream     Stream
	Now        time.Time
}

// HoldPage holds the page of the session with the given id, of what read
// holds of it, and of the accounts it ran on, at now: its requests priced by
// prices at the day now falls on, and keep the ledger's [ledger] keep, as
// ledgerKept reads it.
func HoldPage(id string, read SessionsRead, prices ledger.Table, keep time.Duration, now time.Time) HeldPage {
	h := HeldPage{id: id, day: Today(now), prices: prices, keep: ledgerKept(keep), held: make(map[lineKey]bool), answers: answersOf(read.requests, id)}
	for _, lines := range read.requests {
		h.hold(lines)
	}
	h.hold(read.lines[id])
	h.requests = requestsOf(read.lines[id], prices, h.day.Date)
	return h.built()
}

// hold notes each of lines as taken.
func (h HeldPage) hold(lines []ledger.Line) {
	for i := range lines {
		if k, ok := keyOf(&lines[i]); ok {
			h.held[k] = true
		}
	}
}

// built is h with what its requests and answers make worked out: its story,
// and the accounts it ran on, with the points they took.
func (h HeldPage) built() HeldPage {
	h.story = storyOf(h.requests, h.prices, h.day)
	h.accounts = accountsOf(h.requests)
	points := pointsOf(h.answers, h.requests)
	for i := range h.accounts {
		h.accounts[i].Points = points[h.accounts[i].Account]
	}
	return h
}

// Take is h with the lines given taken in, those a day, of the given date,
// gained since it was held, as the ledger's Today gives them: each request
// of the session's, priced once, and each of an account's, every session's,
// for the points, each line once, whether it was held already or not. A
// date after the day h is held at rolls it on to that day, its requests
// priced afresh, as Page would price them every look until it's held there;
// an earlier one, of a ledger yet to turn its day, never rolls it back. It's
// h itself where none is new and the date is no later than its day.
func (h HeldPage) Take(date string, lines []ledger.Held) HeldPage {
	var gained []ledger.Line
	for _, l := range lines {
		if k, ok := keyOf(&l.Line); !ok || !h.held[k] {
			gained = append(gained, l.Line)
		}
	}
	rolls := date > h.day.Date
	if len(gained) == 0 && !rolls {
		return h
	}
	if rolls {
		h = h.on(dayOf(date))
	}
	h.held, h.answers = maps.Clone(h.held), maps.Clone(h.answers)
	h.hold(gained)
	var mine []ledger.Line
	for i := range gained {
		l := &gained[i]
		if l.Counted() && l.Account != "" {
			h.answers[date] = append(slices.Clip(h.answers[date]), answeredOf(l, h.id))
		}
		if l.Session == h.id {
			mine = append(mine, *l)
		}
	}
	h.requests = inArrival(h.requests, requestsOf(mine, h.prices, h.day.Date))
	return h.built()
}

// inArrival returns held, a session's requests oldest first, with those
// given added among them, in the order they arrived, each after any that
// arrived with it, held afresh.
func inArrival(held, added []request) []request {
	all := slices.Concat(held, added)
	slices.SortStableFunc(all, func(a, b request) int { return a.At.Compare(b.At) })
	return all
}

// at is h at now: as it's held, where now falls on the day it's held at,
// else built afresh, its requests priced at the day now falls on, as a page
// is priced at today's prices.
func (h HeldPage) at(now time.Time) HeldPage {
	day := Today(now)
	if day.Date == h.day.Date {
		return h
	}
	return h.on(day).built()
}

// on is h held at the day given, its requests priced afresh at it, but for
// what they make, which it leaves to built.
func (h HeldPage) on(day LedgerDay) HeldPage {
	lines := make([]ledger.Line, len(h.requests))
	for i, r := range h.requests {
		lines[i] = *r.Line
	}
	h.day, h.requests = day, requestsOf(lines, h.prices, day.Date)
	return h
}

// Page is the page of the session h holds, as SessionPageOf builds it, of
// what h holds of its lines and what live gives, reporting false where it
// has no request, and the router doesn't list it.
func (h HeldPage) Page(live PageLive) (SessionPage, bool) {
	h = h.at(live.Now)
	if len(h.requests) == 0 && live.Routed == nil {
		return SessionPage{}, false
	}
	var running []status.Session
	if live.Routed != nil {
		running = []status.Session{*live.Routed}
	}
	by := newLister(running, live.Unlisted, live.Stream, live.Now)
	listed := by.listed(h.id, h.story)
	told := by.h.told[h.id]
	page := SessionPage{
		Session: listed.Session, Dir: listed.Dir, Account: listed.Account, Model: listed.Model, Running: listed.Running, LastSeen: listed.LastSeen,
		Models: listed.Models, State: listed.State, MoveCost: listed.MoveCost, Ended: listed.Ended, Started: listed.Started,
		Resumed: resumedAt(sentOf(h.requests, told)), Accounts: slices.Clone(h.accounts), Moves: movesOf(h.story), Activity: listed.Activity,
	}
	unplaced := live.Routed != nil && by.h.unplaced(*live.Routed)
	page.Turns = turnsOf(h.requests, told, page.Runs(), unplaced)
	page.Goes = by.goesOf(h.id, live.Stream, h.requests, told)
	switch {
	case live.Routed != nil:
		page.Pin, page.Auto = live.Routed.Pin, autoOf(*live.Routed, page.Model)
	case live.Unlisted && live.LastRouted != nil:
		page.Pin = live.LastRouted.Pin
	}
	if !page.Running && h.story.first != nil {
		page.KeptUntil, _ = dayfile.KeptUntil(Today(h.story.first.At).Date, h.keep)
	}
	page.Totals = totalsOf(h.requests, len(page.Turns))
	return page, true
}

// TurnRequests are the Log's lines of the requests of the session's turn
// numbered n, from 1 at its first, as a turn opened shows them, oldest
// first, of what h holds and what the stream told, as live gives it: each
// its row as the Log's, by its line, as LogRows builds it, with the
// session's story, or before its line comes, by what the stream told; each
// with no mark, as a turn's rows have none; and its details, as Details
// gives them. None where it has no such turn.
func (h HeldPage) TurnRequests(n int, live PageLive) []LogLine {
	h = h.at(live.Now)
	tallies, _ := talliesOf(h.requests, live.Stream.hearing().told[h.id])
	if n < 1 || n > len(tallies) {
		return nil
	}
	lines := make([]LogLine, len(tallies[n-1].members))
	for i, r := range tallies[n-1].members {
		if r.line != nil {
			lines[i] = LogLine{Row: lineRow(r.line.Line, h.story), line: r.line.Line}
			lines[i].Row.Worth = r.line.worth
		} else {
			lines[i] = LogLine{Row: toldRow(*r.told, live.Now)}
		}
		lines[i].Row.Mark = NoMark
	}
	return lines
}

// Awaited are the dates of the days before the day live's now falls on that
// the line of a request of the session h holds the page of is filed on, as
// Told.FiledOn has it, the stream having told its end, its line yet to
// come, as its row's stage says, each once, the oldest first: a later day's
// tail of the ledger never reads it, as the Log's Requests await theirs.
// None is awaited of a request the stream told no sending of.
func (h HeldPage) Awaited(live PageLive) []string {
	today := Today(live.Now)
	var dates []string
	for _, s := range sentOf(h.requests, live.Stream.hearing().told[h.id]) {
		if s.told == nil || toldRow(*s.told, live.Now).Stage != RowEnded {
			continue
		}
		if day, ok := s.told.FiledOn(); ok && day.Start.Before(today.Start) && !slices.Contains(dates, day.Date) {
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
		_, told := h.seats[seatOf{session: s.ID, model: a.Model, account: a.Account}]
		return a.InFlight != "" && (!h.live || !told)
	})
}

// goesOf are where the models of the session with the given id go now, as
// its page's row 7 has them: where the router lists it, as it lists them,
// the moves the stream s told of since taken in, each with the reason and
// time of the move that took it there; else each model's latest account, in
// the order the models first came, as its requests, oldest first, and those
// the stream told of, told, give them, each with the reason of its latest
// line.
func (by lister) goesOf(id string, s Stream, requests []request, told []Told) []SessionModel {
	if _, listed := by.routed[id]; listed {
		var goes []SessionModel
		for _, a := range by.goes[id] {
			goes = append(goes, SessionModel{Model: a.Model, Account: a.Account, Reason: a.Reason, Since: cameAt(a)})
		}
		for _, m := range newestMoves(s.Moves) {
			if i := slices.IndexFunc(goes, func(g SessionModel) bool { return m.Session == id && g.Model == m.Model && g.Account == m.To }); i >= 0 {
				goes[i].Reason, goes[i].Since = m.Reason, m.At
			}
		}
		return goes
	}
	var goes []SessionModel
	for _, r := range sentOf(requests, told) {
		if r.account == "" {
			continue
		}
		i := slices.IndexFunc(goes, func(g SessionModel) bool { return g.Model == r.model })
		if i < 0 {
			i, goes = len(goes), append(goes, SessionModel{Model: r.model})
		}
		goes[i].Account = r.account
		if r.line != nil {
			goes[i].Reason = r.line.Reason
		}
	}
	return goes
}
