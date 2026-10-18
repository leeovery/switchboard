package views

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// spreadSeeds are the seeds of the spread worlds the reads over digests
// run: four that between them meet every hard moment, in whatever zone the
// tests run, as some moments, a move on a day settled after midnight among
// them, are met by few seeds, and which by the zone the worlds' days fall
// in.
var spreadSeeds = []uint64{8, 11, 12, 13}

// The hard moments the reads over digests meet, each met by some spread
// world.
const (
	onYesterdayLate  = "at 23:59, sessions resting on the day before's digest"
	yesterdayRaw     = "past midnight, the day before read raw, sessions resting on an older digest"
	onYesterdayEarly = "at 01:30, sessions resting on the day before's digest"
	idleThenActive   = "a run of days broken and resumed: a session idle for days, then active today, resting on its old state"
	pageOnState      = "a page open on a session a digest holds a state of, read whole"
	givenAgain       = "a line of the day before given again once its digest stands, its session read whole"
	zoneEarly        = "a zone change's early line in a near day's file, its session resting on none"
	zoneLate         = "a zone change's late line in a day's file, arrived the day after, its session resting on none or an older state"
	pinnedAcross     = "a request in flight across midnight, its session's state at the day before's end"
	wantedAgain      = "a session the store held across the moments let go, then wanted again, resting on its state"
	fedAcrossOne     = "the store held across one in the morning, fed as the day before's digest comes to stand"
)

// spreadMoments are the hard moments the reads over digests meet: their
// own; those of a round trip, as a session resting on its state meets them;
// and those the upsets make.
var spreadMoments = slices.Concat([]string{
	onYesterdayLate, yesterdayRaw, onYesterdayEarly, idleThenActive, pageOnState, givenAgain, zoneEarly, zoneLate, pinnedAcross, wantedAgain,
	fedAcrossOne, settledAfter, flightAcross, agentAcross, idlessStored, idlessLater,
}, upsetMoments())

// Every view read through the store over digests, of a real ledger digested
// at each moment as the router digests it, read through a Follower resting
// each session on the newest state a digest holds of it that stands, is what
// it is read through a Follower of the same files that reads every line, as
// of another version, for which none stands: the List, as the sessions verb
// prints it without the router; the routing card's costs; the Log's
// sessions; the store as the List's first read holds it, a session's page
// open, and what the page shows; and the store held across the moments, fed
// as the dashboard feeds it, the router's sessions and the page wanted at
// some moments and not at others; each store given the day before's lines
// again at half past one. At a minute before midnight, a minute after, a
// minute before one, the day before read raw, and half past one, its digest
// standing; the world's files upset between the router's rounds at some of
// them, as upsets has it; of spread worlds, built with the built-in table and
// read with another, as digestPrices has them, meeting each of
// spreadMoments.
func TestEveryViewOverDigestsIsTheViewReadRaw(t *testing.T) {
	met := make(map[string]bool)
	prices := digestPrices(t)
	for _, seed := range spreadSeeds {
		w := newSpread(t, seed, prices[seed%uint64(len(prices))], met)
		for i, at := range w.moments() {
			w.by(at)
			w.upset(at)
			w.check(i, at)
		}
	}
	for _, moment := range slices.Concat(spreadMoments, costedEither(prices)) {
		if !met[moment] {
			t.Errorf("no spread world meets %s", moment)
		}
	}
}

// costedEither are the moments of a session resting on a state that costs a
// move otherwise than as it was built, read with each of the tables given.
func costedEither(prices []pricedAs) []string {
	var moments []string
	for _, p := range prices {
		moments = append(moments, costedOtherwise(p.name))
	}
	return moments
}

// spreadWorld is a world of five or six days as the seed given makes it, in
// the order the ledger files its lines, as each request ends: pagedRun's,
// its session s among others from the day before s began; generatedRun's
// session of the next seed, as k, two days on; and generatedSession's of the
// seed after, as idle, its first half's requests, by when they arrived, on
// their own days, the rest three days on, idle between. And two requests as
// a change of time zone files them: early, of k's, arrived half an hour
// before the day k began on ended, to be filed under the day after; and
// late, of idle's, arrived half an hour after the last day idle ran on
// ended, to be filed under that day.
func spreadWorld(seed uint64) (lines []ledger.Line, early, late ledger.Line) {
	lines, _ = pagedRun(seed)
	k := renamed(generatedRun(seed+1), "k")
	for i := range k {
		k[i] = movedOn(k[i], 2)
	}
	idle := renamed(generatedSession(seed+2), "idle")
	half := idle[arrivalOrder(idle)[len(idle)/2]].At
	for i := range idle {
		if !idle[i].At.Before(half) {
			idle[i] = movedOn(idle[i], 3)
		}
	}
	lines = slices.Concat(lines, k, idle)
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	early = k[arrivalOrder(k)[0]]
	early.Request, early.At = "zone-early", dayfile.DayStart(early.At.Local(), 1).Add(-30*time.Minute)
	late = idle[arrivalOrder(idle)[len(idle)-1]]
	late.Request, late.At = "zone-late", dayfile.DayStart(late.At.Local(), 1).Add(30*time.Minute)
	return lines, early, late
}

// renamed are lines, each of the session with the given id, afresh.
func renamed(lines []ledger.Line, id string) []ledger.Line {
	named := slices.Clone(lines)
	for i := range named {
		named[i].Session = id
	}
	return named
}

// movedOn is l arriving the given number of days later, at the same time of
// day.
func movedOn(l ledger.Line, days int) ledger.Line {
	l.At = l.At.In(time.Local).AddDate(0, 0, days)
	return l
}

// spread is a spread world written as a real ledger, digested at each
// moment as the router digests it, as digestChain has it, its zone lines,
// early and late, filed under the days beside those they arrived on; read
// through two Followers of its files, each held across the moments, as a
// dashboard holds one: based, of the version the digests are of, which rests
// each session on the newest state a digest holds of it that stands; and
// raw, of another, for which none stands, so it reads every line. It prices
// as prices reads, the digests' states built with the built-in table; holds
// the stores the dashboard would, fed through each Follower at each moment,
// as across has them; counts the moments it came to upset its files at, as
// upset has it; and notes the hard moments its reads meet among met.
type spread struct {
	*digestChain
	early, late ledger.Line
	based, raw  *ledger.Follower
	prices      pricedAs
	held        across
	upsets      int
	met         map[string]bool
}

// across are the stores a dashboard holds, fed through each Follower at each
// moment a read of the ledger's tail, as tailAt reads it: of today's lines,
// those since the read to mark; and each day from the one last fed on,
// day, to today's, read again whole, as the tail missed the end of each.
// restsOn is the date of the digest each session the store fed through the
// Follower resting on digests holds rested on as it was last held afresh,
// as restingOn gives it then, by its id: none of one read whole then; and
// ever the sessions it has held.
type across struct {
	based, raw HeldSessions
	mark       ledger.Mark
	day        string
	restsOn    map[string]string
	ever       map[string]bool
}

// newSpread is the spread world of the seed given, written in a state
// directory of the test's own, read with prices, its hard moments noted
// among met.
func newSpread(t *testing.T, seed uint64, prices pricedAs, met map[string]bool) *spread {
	t.Helper()
	lines, early, late := spreadWorld(seed)
	c := newDigestChain(t, fmt.Sprintf("spread world %d, %s", seed, prices.name), seed, lines)
	clock := func() time.Time { return c.at }
	w := &spread{
		digestChain: c, early: early, late: late, prices: prices, met: met,
		based: ledger.NewFollower(c.state, clock, ledger.CapsOf(nil, nil), DigestVersion, quiet),
		raw:   ledger.NewFollower(c.state, clock, ledger.CapsOf(nil, nil), DigestVersion+1, quiet),
		held:  across{based: HoldSessions(prices.read), raw: HoldSessions(prices.read), restsOn: make(map[string]string), ever: make(map[string]bool)},
	}
	w.fileLine(early)
	w.fileLine(late)
	return w
}

// filedUnder is the date of the local day the world files l under: the day
// after the one it arrived on, of early; the day before, of late; and the
// day it arrived on, of every other line.
func (w *spread) filedUnder(l ledger.Line) string {
	switch l.Request {
	case w.early.Request:
		return dayfile.DateOf(dayfile.DayStart(l.At, 1))
	case w.late.Request:
		return dayfile.DateOf(dayfile.DayStart(l.At, -1))
	}
	return dayfile.DateOf(l.At)
}

// fileLine files l under the day filedUnder gives, after the lines filed
// there before, and holds it among the world's lines.
func (w *spread) fileLine(l ledger.Line) {
	w.file(w.filedUnder(l), l)
	w.lines = append(w.lines, l)
}

// moments are the moments the reads are held at, in order: of each of the
// chain's days, as days has them, through the one after the last line
// arrived on, a minute before it began, a minute into it, a minute before
// one in the morning, the day before's digest yet to stand, and half past
// one, once it stands.
func (w *spread) moments() []time.Time {
	last := slices.MaxFunc(w.lines, func(a, b ledger.Line) int { return a.At.Compare(b.At) }).At
	var moments []time.Time
	for _, day := range w.days() {
		if day.Start.Before(dayfile.DayStart(last, 2)) {
			moments = append(moments, day.Start.Add(-time.Minute), day.Start.Add(time.Minute), day.Start.Add(59*time.Minute), day.Start.Add(90*time.Minute))
		}
	}
	return moments
}

// by sets the world's clock to at, and has the router's round summarise,
// and its digester digest, the days that have ended by then, as they would
// have.
func (w *spread) by(at time.Time) {
	w.at = at
	w.ledger.SummariseEnded(at)
	w.ledger.DigestEnded(w.t.Context(), at)
}

// check holds each view read at the i-th moment, now, through the Follower
// resting on digests to the view read through the one reading raw, as the
// store's test names them, noting the hard moments met. The flights of the
// requests that arrived on the day of the state each session rests on, or
// before, are set aside, as a session restored from it holds none, but where
// the day before's lines are given a store again, which reads such a session
// whole.
func (w *spread) check(i int, now time.Time) {
	w.t.Helper()
	m := w.momentAt(i, now)
	w.note(now, m.stream, m.bases)
	w.same(now, "the List", func(l Ledger) any {
		return ListSessions(SessionSources{Unlisted: true, Ledger: l, Prices: w.prices.read, Now: now})
	})
	w.same(now, "the routing card's costs", func(l Ledger) any {
		return ListSessions(SessionSources{Running: m.running, Ledger: l, Prices: w.prices.read, Now: now}).Sessions
	})
	w.same(now, "the Log's sessions", func(l Ledger) any { return w.logSessions(l, m) })

	paged := ListWanted(m.running, m.stream)
	paged.Page = "s"
	based, raw := w.fresh(w.based, paged, now), w.fresh(w.raw, paged, now)
	w.sameStores(now, "the store, as the List's first read holds it, s's page open", based, raw, m.bases, paged.Page)
	w.noteRested(m, raw)
	w.givenAgain(m, "the store, as the List's first read holds it", based, raw, paged)
	w.feedHeld(m)
}

// moment is what the reads at a moment share: it's the i-th, now; the
// world's sessions as the router lists them then, as running has them; the
// stream, as pinning has it; the state each session rests on, as restingOn
// has them, and its date; today's lines, as a Follower's Day reads them; and
// at half past one, the day before's lines to be given a store again, as
// the dashboard's read of the ledger's tail reads a day again whole, none
// at another moment.
type moment struct {
	i       int
	now     time.Time
	running []status.Session
	stream  Stream
	states  map[string]*ledger.SessionBase
	bases   map[string]string
	today   []ledger.Held
	again   *Taken
}

// momentAt is the i-th moment, now, as moment has it, its lines read
// through the Follower reading raw, each day's once.
func (w *spread) momentAt(i int, now time.Time) moment {
	day := Today(now)
	states := w.restingOn()
	m := moment{i: i, now: now, running: w.running(now), stream: w.pinning(now), states: states, bases: datesOf(states), today: w.raw.Day(day.Date)}
	if now.Hour() == 1 {
		yesterday := yesterdayOf(day)
		again := Took(nil, map[string][]ledger.Held{yesterday: w.raw.Day(yesterday)})
		m.again = &again
	}
	return m
}

// feedHeld feeds the stores held across the moments at the moment m, as the
// dashboard feeds the one it holds: a read of the ledger's tail, as tailAt
// reads it, wanting what heldWants wants; and the day before's lines again,
// as givenAgain gives them. It holds the two to each other, each session's
// flights set aside as across's restsOn notes them, of the dates of the
// states the sessions rest on at m; and notes a session let go and wanted
// again, and a store held since before one in the morning holding a session
// that rests on the day before's digest once it stands.
func (w *spread) feedHeld(m moment) {
	w.t.Helper()
	want, held := w.heldWants(m), &w.held
	yesterday := yesterdayOf(Today(m.now))
	if m.now.Hour() == 1 && slices.ContainsFunc(slices.Collect(maps.Keys(held.based.sessions)), func(id string) bool { return m.bases[id] == yesterday }) {
		w.met[fedAcrossOne] = true
	}
	t := w.tailAt(m.now)
	based, gains := held.based.Fed(w.based, t, want, m.now)
	raw, _ := held.raw.Fed(w.raw, t, want, m.now)
	for id, gain := range gains {
		if !gain.whole {
			continue
		}
		if _, was := held.based.sessions[id]; !was && held.ever[id] && m.bases[id] != "" {
			w.met[wantedAgain] = true
		}
		held.restsOn[id], held.ever[id] = m.bases[id], true
	}
	w.sameStores(m.now, "the store held across the moments", based, raw, held.restsOn, want.Page)
	held.based, held.raw = w.givenAgain(m, "the store held across the moments", based, raw, want)
}

// heldWants is what the dashboard's store wants at the moment m, its List
// and its page open: today's sessions, pinned as m's stream tells; those the
// router lists, but at one moment in three, as where the router lists none,
// so those with no request today are let go, to be wanted again; and s's
// page, open at two moments in four, the seed shifting which, so it opens
// and closes across midnight and across one in the morning.
func (w *spread) heldWants(m moment) SessionsWanted {
	shift, running := m.i+int(w.seed), m.running
	if shift%3 == 2 {
		running = nil
	}
	want := ListWanted(running, m.stream)
	if shift%4 < 2 {
		want.Page = "s"
	}
	return want
}

// givenAgain is based and raw, the stores what names, held through each
// Follower at the moment m, wanting want, fed the day before's lines again,
// where m gives them, held to each other, every flight alike: a session
// resting on the day's state, which holds none of the day's flights, can't
// place them, so it's read whole. Where m gives none, it's the two as given.
func (w *spread) givenAgain(m moment, what string, based, raw HeldSessions, want SessionsWanted) (HeldSessions, HeldSessions) {
	w.t.Helper()
	if m.again == nil {
		return based, raw
	}
	based, basedGains := based.Fed(w.based, *m.again, want, m.now)
	raw, rawGains := raw.Fed(w.raw, *m.again, want, m.now)
	w.sameStores(m.now, what+", given the day before's lines again", based, raw, nil, want.Page)
	for id, gain := range basedGains {
		if id != want.Page && gain.whole && !rawGains[id].whole {
			w.met[givenAgain] = true
		}
	}
	return based, raw
}

// running are the world's sessions as the router lists them at now, each
// running Claude Opus 5.5 on work.
func (w *spread) running(now time.Time) []status.Session {
	var running []status.Session
	for _, id := range sessionsIn(w.lines) {
		running = append(running, status.Session{ID: id, Assignments: []status.Assignment{{Model: foldOpus, Account: "work", Reason: "sticky", LastSeen: now}}})
	}
	return running
}

// pinning is the stream at now, telling of a request in flight since half
// an hour before the day now falls on began, of the session whose request
// last arrived before then, as one running across midnight pins its
// session: of none where no request did.
func (w *spread) pinning(now time.Time) Stream {
	sent := dayfile.DayStart(now, 0).Add(-30 * time.Minute)
	var last *ledger.Line
	for i := range w.lines {
		if l := &w.lines[i]; isRequest(l) && l.At.Before(sent) && (last == nil || l.At.After(last.At)) {
			last = l
		}
	}
	if last == nil {
		return Stream{}
	}
	pinned := Told{Request: "pinned", Session: last.Session, Model: last.Model, Account: last.Account, Class: ledger.ClassMain, Sent: sent, Heard: sent}
	return Stream{Requests: map[string]Told{pinned.Request: pinned}}
}

// fresh is a store as a reader's first read of it holds it, fed through l at
// now, as want wants.
func (w *spread) fresh(l LedgerSessions, want SessionsWanted, now time.Time) HeldSessions {
	held, _ := HoldSessions(w.prices.read).Fed(l, Taken{}, want, now)
	return held
}

// logSessions are the Log's panel's sessions at the moment m, of its today's
// lines, the Log's store fed them through l as the Log's read feeds it,
// wanting the sessions of its lines.
func (w *spread) logSessions(l Ledger, m moment) []LogSession {
	read := ReadLog(w.prices.read)
	held, gains := HoldSessions(w.prices.read).Fed(l, Took(m.today, nil), SessionsWanted{Log: read.Wanted(m.today)}, m.now)
	read = read.Taken(m.today, true, LogTaking{Held: held, Gains: gains, Now: m.now})
	lines := make(map[string]int)
	for _, h := range m.today {
		lines[h.Session]++
	}
	return LogSessions(LogSessionSources{Read: read, Held: held, Lines: lines, Unlisted: true, Now: m.now})
}

// tailAt is a read of the ledger's tail at now, as the dashboard reads it,
// through the Follower reading raw: today's lines since the last read, as
// Today gives them; and the lines of each day from the one the stores were
// last fed on until today's, read again whole, as Day reads them, as the
// tail missed the end of each.
func (w *spread) tailAt(now time.Time) Taken {
	gained, mark, _ := w.raw.Today(w.held.mark)
	days := make(map[string][]ledger.Held)
	today := Today(now).Date
	for date := w.held.day; date != "" && date < today; date = nextDate(date) {
		days[date] = w.raw.Day(date)
	}
	w.held.mark, w.held.day = mark, today
	return Took(gained, days)
}

// same fails the test where what a view gives, read through the Follower
// resting on digests, isn't what it gives read through the one reading raw.
func (w *spread) same(now time.Time, view string, read func(l Ledger) any) {
	w.t.Helper()
	w.alike(now, view, read(w.based), read(w.raw))
}

// alike fails the test where got, what a view gives over digests, isn't
// want, what it gives read raw.
func (w *spread) alike(now time.Time, view string, got, want any) {
	w.t.Helper()
	if !reflect.DeepEqual(got, want) {
		w.t.Errorf("%s at %v: %s, over digests, is\n%+v\nwant, as read raw\n%+v", w.name, now, view, got, want)
	}
}

// sameStores fails the test where got, a store fed through the Follower
// resting on digests, holds otherwise than want, the store fed through the
// one reading raw: a session one holds and the other doesn't; one each holds
// otherwise, as sameHeld has them, the flights of the requests that arrived
// on the day with the date through gives of the session, or before, set
// aside, as a session restored from its state at that day's end holds none,
// and the page of the session with the given id, as its points hold the
// answers of the days a read reads, which differ while what it shows
// doesn't; a page of it shown otherwise, as pageShown shows it; or a List of
// either otherwise than the other's.
func (w *spread) sameStores(now time.Time, what string, got, want HeldSessions, through map[string]string, page string) {
	w.t.Helper()
	either := cloneOrMake(got.sessions)
	maps.Copy(either, want.sessions)
	for _, id := range slices.Sorted(maps.Keys(either)) {
		g, h := got.sessions[id], want.sessions[id]
		switch {
		case g == nil || h == nil:
			w.t.Errorf("%s at %v: %s holds session %s %t, want %t, as read raw", w.name, now, what, id, g != nil, h != nil)
		case !sameHeld(unpaged(g, id == page), unpaged(h, id == page), through[id]):
			q := queryOf(w.lines)
			w.t.Errorf("%s at %v: %s holds session %s\n%s\nflights %v\nwant, as read raw\n%s\nflights %v", w.name, now, what, id,
				heldSaid(g, q), flightsSaid(g, through[id]), heldSaid(h, q), flightsSaid(h, through[id]))
		}
	}
	if page != "" {
		w.alike(now, what+", "+page+"'s page", pageShown(got, page, now), pageShown(want, page, now))
	}
	live := ListLive{Unlisted: true, Now: now}
	if g, h := got.List(live), want.List(live); !reflect.DeepEqual(g, h) {
		w.t.Errorf("%s at %v: %s lists\n%+v\nwant, as read raw\n%+v", w.name, now, what, g, h)
	}
}

// unpaged is s, its page set aside where aside says, afresh.
func unpaged(s *heldSession, aside bool) *heldSession {
	if !aside {
		return s
	}
	without := *s
	without.page = nil
	return &without
}

// flightsSaid are how many requests' flights s holds of each day after the
// one with the given date, by its date.
func flightsSaid(s *heldSession, through string) map[string]int {
	said := make(map[string]int)
	for date, flights := range flightsAfter(s.flights, through) {
		for layer := flights; layer != nil; layer = layer.under {
			for _, f := range layer.top {
				said[date] += len(f)
			}
		}
	}
	return said
}

// pageShown is the page of the session with the given id as a page holding
// it as h holds it, tallied, shows it at now: none where h holds it
// untallied, or not at all.
func pageShown(h HeldSessions, id string, now time.Time) SessionPage {
	s := h.sessions[id]
	if s == nil || s.page == nil {
		return SessionPage{}
	}
	page, _ := HeldPage{id: id, day: Today(now), prices: h.prices, keep: ledgerKept(0), record: s, read: now}.Page(PageLive{Unlisted: true, Now: now})
	return page
}

// restingOn is the state each of the world's sessions rests on, by its id,
// as the Follower resting on digests gives them asked for none whole: none
// of one it reads whole.
func (w *spread) restingOn() map[string]*ledger.SessionBase {
	bases := make(map[string]*ledger.SessionBase)
	for id, from := range w.based.SessionsOf(sessionsIn(w.lines), func(string) bool { return false }, nil) {
		if from.Base != nil {
			bases[id] = from.Base
		}
	}
	return bases
}

// datesOf are the dates of the days whose digests hold the states given, by
// their sessions' ids.
func datesOf(states map[string]*ledger.SessionBase) map[string]string {
	dates := make(map[string]string, len(states))
	for id, base := range states {
		dates[id] = base.Date
	}
	return dates
}

// noteRested notes the hard moments of a round trip, as restoring's note
// notes them, that each session resting on a state at the moment m meets:
// its state as built, restored with the built-in table onto the day after
// its own, the trip's day D; the day m falls on its day T; and the session
// read whole there as whole, a store read raw, holds it; its requests those
// arrived by m.
func (w *spread) noteRested(m moment, whole HeldSessions) {
	w.t.Helper()
	for id, base := range m.states {
		held := whole.sessions[id]
		if held == nil {
			continue
		}
		start, _, _ := dayfile.Day(base.Date)
		built, err := restoredFrom(base, ledger.Pricing, Today(dayfile.DayStart(start, 1)))
		if err != nil {
			w.t.Fatalf("%s at %v: session %s rests on a state that won't restore: %v", w.name, m.now, id, err)
		}
		lines := slices.DeleteFunc(slices.Clone(w.lines), func(l ledger.Line) bool { return l.Session != id || l.At.After(m.now) })
		r := &restoring{t: w.t, name: w.name, seed: w.seed, prices: w.prices, lines: lines, requests: heldRequestsOf(lines), met: w.met}
		r.note(trip{r: r, from: dayState{day: Today(start), held: built}, on: Today(m.now), whole: held})
	}
}

// note notes the hard moments the reads at now meet, the world's sessions
// resting on the states of the days bases gives, as restingOn has them, the
// stream given pinning one.
func (w *spread) note(now time.Time, stream Stream, bases map[string]string) {
	ids := sessionsIn(w.lines)
	today := Today(now)
	yesterday := yesterdayOf(today)
	onYesterday := slices.ContainsFunc(ids, func(id string) bool { return bases[id] == yesterday })
	onOlder := slices.ContainsFunc(ids, func(id string) bool { return bases[id] != "" && bases[id] < yesterday })
	for _, moment := range []struct {
		name string
		met  bool
	}{
		{name: onYesterdayLate, met: now.Hour() == 23 && onYesterday},
		{name: yesterdayRaw, met: now.Hour() == 0 && onOlder && !onYesterday},
		{name: onYesterdayEarly, met: now.Hour() == 1 && onYesterday},
		{name: idleThenActive, met: bases["idle"] != "" && bases["idle"] < yesterday && w.arrivedOn("idle", today, now)},
		{name: pageOnState, met: bases["s"] != ""},
		{name: zoneEarly, met: !w.early.At.After(now) && bases[w.early.Session] == "" && w.digestHolds(dayfile.DateOf(w.early.At), w.early.Session)},
		{name: zoneLate, met: !w.late.At.After(now) && bases[w.late.Session] < w.filedUnder(w.late) && w.digested(w.filedUnder(w.late))},
		{name: pinnedAcross, met: now.Hour() == 1 && slices.ContainsFunc(slices.Collect(maps.Values(stream.Requests)), func(t Told) bool {
			return bases[t.Session] == yesterday
		})},
	} {
		w.met[moment.name] = w.met[moment.name] || moment.met
	}
}

// arrivedOn reports whether a request of the session with the given id
// arrived on the day given by now.
func (w *spread) arrivedOn(id string, day LedgerDay, now time.Time) bool {
	return slices.ContainsFunc(w.lines, func(l ledger.Line) bool { return l.Session == id && day.Holds(l.At) && !l.At.After(now) })
}

// digested reports whether the views' Digests were handed the local day
// with the given date, as recordedDigests records them.
func (w *spread) digested(date string) bool {
	_, handed := w.digests.handed[date]
	return handed
}

// digestHolds reports whether the views' Digests gave a state of the
// session with the given id of the local day with the given date, as
// recordedDigests records them.
func (w *spread) digestHolds(date, id string) bool {
	return w.digests.states[date][id] != nil
}

// The hard moments the upsets make, each made by an upset of a spread
// world's files.
const (
	lateOnDigested     = "a late line onto the day before, after its digest, of a session resting on its state of it"
	lateCascading      = "a late line onto D-2, after D-1's and D's digests, of a session resting on a state of one of them"
	lateNeverRan       = "a late line of a session onto a day before its state's, a day it never ran on"
	digestGone         = "a digest deleted mid-chain, the days either side digested"
	digestOtherVersion = "the day before's digest of the version before the views'"
	dayPruned          = "the first day's lines pruned under a state resting on them"
)

// upsetting is an upset of a spread world's files between the router's
// rounds, by the hard moment it makes: upset makes it at now, reporting
// whether the world let it, as where no session rests on what it needs.
type upsetting struct {
	moment string
	upset  func(w *spread, now time.Time) bool
}

// upsets are the upsets a spread world's files take, each in turn.
var upsets = []upsetting{
	{moment: lateOnDigested, upset: lateOnto(1)},
	{moment: lateCascading, upset: lateOnto(3)},
	{moment: lateNeverRan, upset: (*spread).lateNeverRan},
	{moment: digestGone, upset: (*spread).digestGone},
	{moment: digestOtherVersion, upset: (*spread).digestOtherVersion},
	{moment: dayPruned, upset: (*spread).dayPruned},
}

// upsetMoments are the hard moments the upsets make.
func upsetMoments() []string {
	moments := make([]string, len(upsets))
	for i, u := range upsets {
		moments[i] = u.moment
	}
	return moments
}

// upset has the next of upsets, in turn from the one the world's seed
// gives, upset its files at now, at each moment the day before's digest
// stands, a minute before midnight and half past one, noting the moment it
// makes where the world lets it make it. The router's round at the next
// moment digests again what it upset.
func (w *spread) upset(now time.Time) {
	if now.Hour() != 23 && now.Hour() != 1 {
		return
	}
	u := upsets[(w.upsets+int(w.seed))%len(upsets)]
	w.upsets++
	if u.upset(w, now) {
		w.met[u.moment] = true
	}
}

// lateOnto is the upset filing a late line onto the day back days before
// the one now falls on, of a session resting on a state resting on that day,
// a later day's where back is more than one, so the line stales each state
// of the session's chain from that day on: a copy of one of the session's
// requests that arrived on it, as another.
func lateOnto(back int) func(w *spread, now time.Time) bool {
	return func(w *spread, now time.Time) bool {
		date := dayfile.DateOf(dayfile.DayStart(now, -back))
		states := w.restingOn()
		for _, id := range slices.Sorted(maps.Keys(states)) {
			base := states[id]
			if _, rests := base.RestsOn[date]; !rests || back > 1 && base.Date == date {
				continue
			}
			if i := slices.IndexFunc(w.lines, func(l ledger.Line) bool { return l.Session == id && w.filedUnder(l) == date }); i >= 0 {
				w.fileLine(w.another(w.lines[i]))
				return true
			}
		}
		return false
	}
}

// another is l as another request of its session, arrived when it did.
func (w *spread) another(l ledger.Line) ledger.Line {
	l.Request = fmt.Sprintf("upset-%d", w.upsets)
	return l
}

// lateNeverRan files a late line of a session resting on a state onto a day
// digested before its state's that it never ran on, so its state rests on no
// line of it there: a copy of one of its requests, arrived at noon that day.
func (w *spread) lateNeverRan(time.Time) bool {
	states := w.restingOn()
	for _, id := range slices.Sorted(maps.Keys(states)) {
		date, never := w.neverRanBefore(states[id])
		i := slices.IndexFunc(w.lines, func(l ledger.Line) bool { return l.Session == id })
		if !never || i < 0 {
			continue
		}
		l := w.another(w.lines[i])
		start, _, _ := dayfile.Day(date)
		l.At = start.Add(12 * time.Hour).UTC()
		w.fileLine(l)
		return true
	}
	return false
}

// neverRanBefore is the date of the first day digested before the one whose
// digest holds base, and still holding lines, that base rests on none of,
// reporting false where there's none.
func (w *spread) neverRanBefore(base *ledger.SessionBase) (string, bool) {
	for _, date := range slices.Sorted(maps.Keys(w.digests.handed)) {
		if _, ran := base.RestsOn[date]; !ran && date < base.Date && w.filedOn(date) {
			return date, true
		}
	}
	return "", false
}

// digestGone deletes the digest of the day two before the one now falls on,
// mid-chain, where the ledger holds the digests of the days either side.
func (w *spread) digestGone(now time.Time) bool {
	w.t.Helper()
	for back := 1; back <= 3; back++ {
		if _, err := os.Stat(w.digestPath(dayfile.DateOf(dayfile.DayStart(now, -back)))); err != nil {
			return false
		}
	}
	if err := os.Remove(w.digestPath(dayfile.DateOf(dayfile.DayStart(now, -2)))); err != nil {
		w.t.Fatal(err)
	}
	return true
}

// digestOtherVersion has the digest of the day before the one now falls on
// hold its states as of the version before the views', as an upgrade leaves
// one: its file as the ledger wrote it, but for the body's version its head
// names. Not a later one, as the Follower reading raw reads the next.
func (w *spread) digestOtherVersion(now time.Time) bool {
	w.t.Helper()
	path := w.digestPath(dayfile.DateOf(dayfile.DayStart(now, -1)))
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	if err != nil {
		w.t.Fatal(err)
	}
	was, is := fmt.Appendf(nil, `"body":%d,`, DigestVersion), fmt.Appendf(nil, `"body":%d,`, DigestVersion-1)
	plain := gunzipped(w.t, data)
	if !bytes.Contains(plain, was) {
		w.t.Fatalf("%s: the digest at %s names no body of version %d:\n%s", w.name, path, DigestVersion, plain)
	}
	if err := os.WriteFile(path, gzipped(w.t, bytes.Replace(plain, was, is, 1)), 0o600); err != nil {
		w.t.Fatal(err)
	}
	return true
}

// dayPruned prunes the lines of the first day the ledger holds, as the
// router's round prunes a day past keeping, where a session rests on a state
// resting on that day: they're gone from the world's lines too.
func (w *spread) dayPruned(time.Time) bool {
	w.t.Helper()
	first := slices.MinFunc(w.lines, func(a, b ledger.Line) int { return strings.Compare(w.filedUnder(a), w.filedUnder(b)) })
	date := w.filedUnder(first)
	if !slices.ContainsFunc(slices.Collect(maps.Values(w.restingOn())), func(b *ledger.SessionBase) bool { _, rests := b.RestsOn[date]; return rests }) {
		return false
	}
	if err := os.Remove(filepath.Join(ledger.Dir(w.state), "requests-"+date+".jsonl")); err != nil {
		w.t.Fatal(err)
	}
	w.lines = slices.DeleteFunc(w.lines, func(l ledger.Line) bool { return w.filedUnder(l) == date })
	return true
}
