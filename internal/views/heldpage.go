package views

import (
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
// never sees it change, sharing what it took before, which nothing changes.
type HeldPage struct {
	id     string
	day    LedgerDay
	prices ledger.Table
	keep   time.Duration
	// held are the keys of the lines taken, as heldKeys has them; answers
	// every account's requests taken, answered, by the date of the day whose
	// files hold them; and over what the session's points are taken over.
	held     heldKeys
	answers  map[string]answersTaken
	over     pointsOver
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

// HoldPage holds the page of the session with the given id, of what read
// holds of it, and of the accounts it ran on, at now: its requests priced by
// prices at the day now falls on, and keep the ledger's [ledger] keep, as
// ledgerKept reads it.
func HoldPage(id string, read SessionsRead, prices ledger.Table, keep time.Duration, now time.Time) HeldPage {
	h := HeldPage{id: id, day: Today(now), prices: prices, keep: ledgerKept(keep)}
	answers := answersOf(read.requests, id)
	for date, lines := range read.requests {
		h.held = holding(h.held, lines)
		h.answers = answering(h.answers, date, answers[date])
	}
	h.held = holding(h.held, read.lines[id])
	h.requests = requestsOf(read.lines[id], prices, h.day.Date)
	h.over.days = daysOf(answers)
	return h.built()
}

// built is h with what its requests and answers make worked out: its story,
// and the accounts it ran on, with the points they took, as pointed has
// them.
func (h HeldPage) built() HeldPage {
	h.story = storyOf(h.requests, h.prices, h.day)
	h.accounts = accountsOf(h.requests)
	h.over = pointsOverOf(h.requests, h.over.days)
	return h.pointed()
}

// pointed is h with the points the session took of the windows of the
// accounts it ran on worked out afresh, as pointsOf has them, of the answers
// it holds of the days they're taken over, its accounts given them afresh.
func (h HeldPage) pointed() HeldPage {
	answers := make(map[string][]answered, len(h.over.days))
	for _, date := range h.over.days {
		answers[date] = takenAll(h.answers[date])
	}
	points := pointsOf(answers, h.over)
	h.accounts = slices.Clone(h.accounts)
	for i := range h.accounts {
		h.accounts[i].Points = points[h.accounts[i].Account]
	}
	return h
}

// Take is h with the lines given taken in, those a day, of the given date,
// gained since it was held, as the ledger's Today gives them: each request
// of the session's, priced once, and each of an account's, every session's,
// for the points, each line once, whether it was held already or not. Only
// what they change is worked out afresh: the whole page, where they're of
// the session; else its points, where one is of an account it ran on, on a
// day it made requests on, as its points are shared out among them; else
// none of it. A date after the day h is held at rolls it on to that day,
// its requests priced afresh, as Page would price them every look until
// it's held there; an earlier one, of a ledger yet to turn its day, never
// rolls it back. It's h itself where none is new and the date is no later
// than its day.
func (h HeldPage) Take(date string, lines []ledger.Held) HeldPage {
	var gained []ledger.Line
	for _, l := range lines {
		if k, ok := keyOf(&l.Line); !ok || !holds(h.held, k) {
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
	h.held = holding(h.held, gained)
	var mine []ledger.Line
	var answers []answered
	counts := false
	for i := range gained {
		l := &gained[i]
		if l.Counted() && l.Account != "" {
			a := answeredOf(l, h.id)
			answers, counts = append(answers, a), counts || h.over.counts(date, a)
		}
		if l.Session == h.id {
			mine = append(mine, *l)
		}
	}
	h.answers = answering(h.answers, date, answers)
	switch {
	case len(mine) > 0 || rolls:
		h.requests = inArrival(h.requests, requestsOf(mine, h.prices, h.day.Date))
		h.over = h.over.withDay(date, answers)
		return h.built()
	case counts:
		return h.pointed()
	}
	return h
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

// BuiltPage is a session's page as Build builds it: the page; its story;
// and the requests of its turn open, none where none is.
type BuiltPage struct {
	Page     SessionPage
	Story    Story
	Requests []LogLine
}

// Build is the page of the session h holds, as Page builds it, with its
// story, as Story tells it, and the requests of its turn numbered open, as
// TurnRequests has them, none where open is 0, all three of one working of
// what h holds and what live gives, as worked has it: reporting false where
// Page does.
func (h HeldPage) Build(open int, live PageLive) (BuiltPage, bool) {
	w := h.worked(live)
	page, ok := w.page()
	if !ok {
		return BuiltPage{}, false
	}
	return BuiltPage{Page: page, Story: w.story(page), Requests: w.turnRequests(open)}, true
}

// worked is what a page of the session h holds is built of, worked out
// once for the page, its story and its turn open's requests alike: h at
// live's now, as at has it; live; the sessions as by lists them, the router
// listing the session where live says it does, the stream telling what live
// gives; the session's requests the stream told of; its requests as its
// turns take them in, as sentOf has them; and its turns' tallies, as
// talliesOf has them, with whether a request in none is in flight.
type worked struct {
	h       HeldPage
	live    PageLive
	by      lister
	told    []Told
	sent    []sent
	tallies []*turnTally
	inNone  bool
}

// worked is what a page of the session h holds is built of, of what live
// gives.
func (h HeldPage) worked(live PageLive) worked {
	h = h.at(live.Now)
	var running []status.Session
	if live.Routed != nil {
		running = []status.Session{*live.Routed}
	}
	w := worked{h: h, live: live, by: newLister(running, live.Unlisted, live.Stream, live.Now)}
	w.told = w.by.h.told[h.id]
	w.sent = sentOf(h.requests, w.told)
	w.tallies, w.inNone = talliesOf(w.sent)
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
	h, live, by := w.h, w.live, w.by
	routed, running := by.routed[h.id]
	if len(h.requests) == 0 && !running {
		return SessionPage{}, false
	}
	listed := by.listed(h.id, h.story)
	page := SessionPage{
		Session: listed.Session, Dir: listed.Dir, Account: listed.Account, Model: listed.Model, Running: listed.Running, LastSeen: listed.LastSeen,
		Models: listed.Models, State: listed.State, MoveCost: listed.MoveCost, Ended: listed.Ended, Started: listed.Started,
		Resumed: resumedAt(w.sent), Accounts: slices.Clone(h.accounts), Moves: movesOf(h.story), Activity: listed.Activity,
	}
	unplaced := running && by.h.unplaced(routed)
	page.Turns = turnsOf(w.tallies, w.inNone, page.Runs(), unplaced)
	page.Goes = by.goesOf(h.id, live.Stream, h.requests, w.sent)
	if running {
		page.Pin = routed.Pin
		if !live.Unlisted {
			page.Auto = autoOf(routed, page.Model)
		}
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
	return h.worked(live).turnRequests(n)
}

// turnRequests are the requests of the turn numbered n, as TurnRequests has
// them, of w.
func (w worked) turnRequests(n int) []LogLine {
	if n < 1 || n > len(w.tallies) {
		return nil
	}
	lines := make([]LogLine, len(w.tallies[n-1].members))
	for i, r := range w.tallies[n-1].members {
		if r.line != nil {
			lines[i] = LogLine{Row: lineRow(r.line.Line, w.h.story), line: r.line.Line}
			lines[i].Row.Worth = r.line.worth
		} else {
			lines[i] = LogLine{Row: toldRow(*r.told, w.live.Now)}
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
// the moves the stream s told of since, as unlisted has them, taken in, each
// with the reason and time of the move that took it there; else each model's
// latest account, in the order the models first came, as its requests as
// its turns take them in, sent, give them, each with the reason of its
// latest line, of its requests, oldest first, the router didn't keep where
// it was. Either way, a model that came there by starting there gives why a
// request since kept it there, as keptLines has it.
func (by lister) goesOf(id string, s Stream, requests []request, sent []sent) []SessionModel {
	kept := keptOf(requests)
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
	var goes []SessionModel
	for _, r := range sent {
		if r.account == "" {
			continue
		}
		i := slices.IndexFunc(goes, func(g SessionModel) bool { return g.Model == r.model })
		if i < 0 {
			i, goes = len(goes), append(goes, SessionModel{Model: r.model})
		}
		goes[i].Account = r.account
	}
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

// keptOf are what the lines of requests, a session's, oldest first, tell of
// each of its models, as keptLines has it, by model.
func keptOf(requests []request) map[string]keptLines {
	by := make(map[string]keptLines)
	for _, r := range requests {
		k := keptLines{account: r.Account, came: by[r.Model].came}
		if status.Kept(r.Reason) {
			k.kept = r.Reason
		} else {
			k.came = r.Reason
		}
		by[r.Model] = k
	}
	return by
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
