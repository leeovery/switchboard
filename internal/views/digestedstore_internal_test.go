package views

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// spreadSeeds is how many seeds of the spread worlds the reads over digests
// run.
const spreadSeeds = 4

// The hard moments the reads over digests meet, each met by some spread
// world.
const (
	onYesterdayLate  = "at 23:59, sessions resting on the day before's digest"
	yesterdayRaw     = "past midnight, the day before read raw, sessions resting on an older digest"
	onYesterdayEarly = "at 01:30, sessions resting on the day before's digest"
	idleThenActive   = "a session idle for days, then active today, resting on its old state"
	pageOnState      = "a page open on a session a digest holds a state of, read whole"
	givenAgain       = "a line of the day before given again once its digest stands, its session read whole"
	zoneEarly        = "a zone change's early line in a near day's file, its session resting on none"
	pinnedAcross     = "a request in flight across midnight, its session's state at the day before's end"
	wantedAgain      = "a session the store held across the moments let go, then wanted again, resting on its state"
)

// spreadMoments are the hard moments the reads over digests meet.
var spreadMoments = []string{onYesterdayLate, yesterdayRaw, onYesterdayEarly, idleThenActive, pageOnState, givenAgain, zoneEarly, pinnedAcross, wantedAgain}

// The store, and every view read through it, of a real ledger digested at
// each moment as the router digests it, read through a Follower resting
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
// standing; of spread worlds, meeting each of spreadMoments.
func TestTheStoreOverDigestsIsTheStoreReadRaw(t *testing.T) {
	met := make(map[string]bool)
	for seed := range uint64(spreadSeeds) {
		w := newSpread(t, seed, met)
		for i, at := range w.moments() {
			w.by(at)
			w.check(i, at)
		}
	}
	for _, moment := range spreadMoments {
		if !met[moment] {
			t.Errorf("no spread world meets %s", moment)
		}
	}
}

// spreadWorld is a world of five or six days as the seed given makes it, in
// the order the ledger files its lines, as each request ends: pagedRun's,
// its session s among others from the day before s began; generatedRun's
// session of the next seed, as k, two days on; and generatedSession's of the
// seed after, as idle, its first half's requests, by when they arrived, on
// their own days, the rest three days on, idle between. And zone, a request
// of k's that arrived half an hour before the day k began on ended, to be
// filed under the day after, as after a change of time zone.
func spreadWorld(seed uint64) (lines []ledger.Line, zone ledger.Line) {
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
	zone = k[arrivalOrder(k)[0]]
	zone.Request, zone.At = "zone", dayfile.DayStart(zone.At.Local(), 1).Add(-30*time.Minute)
	return lines, zone
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
// moment as the router digests it, as digestChain has it, its zone line
// filed under the day after the one it arrived on; read through two
// Followers of its files, each held across the moments, as a dashboard
// holds one: based, of the version the digests are of, which rests each
// session on the newest state a digest holds of it that stands; and raw, of
// another, for which none stands, so it reads every line. It prices as
// changingPrices does; holds the stores the dashboard would, fed through
// each Follower at each moment, as across has them; and notes the hard
// moments its reads meet among met.
type spread struct {
	*digestChain
	zone       ledger.Line
	based, raw *ledger.Follower
	prices     ledger.Table
	held       across
	met        map[string]bool
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
// directory of the test's own, its hard moments noted among met.
func newSpread(t *testing.T, seed uint64, met map[string]bool) *spread {
	t.Helper()
	lines, zone := spreadWorld(seed)
	c := newDigestChain(t, fmt.Sprintf("spread world %d", seed), seed, lines)
	c.file(dayfile.DateOf(dayfile.DayStart(zone.At, 1)), zone)
	c.lines = append(c.lines, zone)
	clock := func() time.Time { return c.at }
	prices := changingPrices()
	return &spread{
		digestChain: c, zone: zone, prices: prices, met: met,
		based: ledger.NewFollower(c.state, clock, ledger.CapsOf(nil, nil), DigestVersion, quiet),
		raw:   ledger.NewFollower(c.state, clock, ledger.CapsOf(nil, nil), DigestVersion+1, quiet),
		held:  across{based: HoldSessions(prices), raw: HoldSessions(prices), restsOn: make(map[string]string), ever: make(map[string]bool)},
	}
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
		return ListSessions(SessionSources{Unlisted: true, Ledger: l, Prices: w.prices, Now: now})
	})
	w.same(now, "the routing card's costs", func(l Ledger) any {
		return ListSessions(SessionSources{Running: m.running, Ledger: l, Prices: w.prices, Now: now}).Sessions
	})
	w.same(now, "the Log's sessions", func(l Ledger) any { return w.logSessions(l, m) })

	paged := ListWanted(m.running, m.stream)
	paged.Page = "s"
	based, raw := w.fresh(w.based, paged, now), w.fresh(w.raw, paged, now)
	w.sameStores(now, "the store, as the List's first read holds it, s's page open", based, raw, m.bases, paged.Page)
	w.givenAgain(m, "the store, as the List's first read holds it", based, raw, paged)
	w.feedHeld(m)
}

// moment is what the reads at a moment share: it's the i-th, now; the
// world's sessions as the router lists them then, as running has them; the
// stream, as pinning has it; the date of the state each session rests on,
// as restingOn has them; today's lines, as a Follower's Day reads them; and
// at half past one, the day before's lines to be given a store again, as
// the dashboard's read of the ledger's tail reads a day again whole, none
// at another moment.
type moment struct {
	i       int
	now     time.Time
	running []status.Session
	stream  Stream
	bases   map[string]string
	today   []ledger.Held
	again   *Taken
}

// momentAt is the i-th moment, now, as moment has it, its lines read
// through the Follower reading raw, each day's once.
func (w *spread) momentAt(i int, now time.Time) moment {
	day := Today(now)
	m := moment{i: i, now: now, running: w.running(now), stream: w.pinning(now), bases: w.restingOn(), today: w.raw.Day(day.Date)}
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
// again.
func (w *spread) feedHeld(m moment) {
	w.t.Helper()
	want, held := w.heldWants(m), &w.held
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
	held, _ := HoldSessions(w.prices).Fed(l, Taken{}, want, now)
	return held
}

// logSessions are the Log's panel's sessions at the moment m, of its today's
// lines, the Log's store fed them through l as the Log's read feeds it,
// wanting the sessions of its lines.
func (w *spread) logSessions(l Ledger, m moment) []LogSession {
	read := ReadLog(w.prices)
	held, gains := HoldSessions(w.prices).Fed(l, Took(m.today, nil), SessionsWanted{Log: read.Wanted(m.today)}, m.now)
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

// restingOn is the date of the day whose digest holds the state each of the
// world's sessions rests on, by its id, as the Follower resting on digests
// gives them asked for none whole: none of one it reads whole.
func (w *spread) restingOn() map[string]string {
	bases := make(map[string]string)
	for id, from := range w.based.SessionsOf(sessionsIn(w.lines), func(string) bool { return false }, nil) {
		if from.Base != nil {
			bases[id] = from.Base.Date
		}
	}
	return bases
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
		{name: zoneEarly, met: !w.zone.At.After(now) && bases[w.zone.Session] == "" && w.digestHolds(dayfile.DateOf(w.zone.At), w.zone.Session)},
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

// digestHolds reports whether the views' Digests gave a state of the
// session with the given id of the local day with the given date, as
// recordedDigests records them.
func (w *spread) digestHolds(date, id string) bool {
	return w.digests.states[date][id] != nil
}

// datesTaken is a ledger as the one it holds reads it that notes the date of
// each day whose lines a read of its sessions hands on, as a Follower hands
// on each line it reads, of whichever session.
type datesTaken struct {
	Ledger
	dates map[string]bool
}

func (c datesTaken) Sessions(ids []string, whole func(string) bool, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return c.Ledger.Sessions(ids, whole, c.taking(take))
}

func (c datesTaken) SessionsOf(ids []string, whole func(string) bool, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return c.Ledger.SessionsOf(ids, whole, c.taking(take))
}

// taking is take, where it's given, each line handed it noted by its day's
// date first.
func (c datesTaken) taking(take func(string, ledger.Line)) func(string, ledger.Line) {
	return func(date string, l ledger.Line) {
		c.dates[date] = true
		if take != nil {
			take(date, l)
		}
	}
}

// The List at half past one, once the day before's digest stands, over nine
// days whose sessions each ran on every one, and made a request today,
// reads today's file alone, every session resting on its state at the day
// before's end: where the same List read raw reads every day's. A day is
// told read by the lines its read hands on, and the day after today's holds
// none.
func TestAListOverDigestsReadsTheNearDaysFilesAlone(t *testing.T) {
	ids := []string{"a", "b", "c"}
	first, today := Today(foldDays[0]), Today(dayfile.DayStart(foldDays[0], 9))
	var lines []ledger.Line
	for i, id := range ids {
		session := renamed(generatedSession(uint64(i)), id)
		early := session[0]
		early.Request, early.At = "today-"+id, today.Start.Add(time.Duration(30+i)*time.Minute)
		lines = append(lines, dayAfterDay(session, 9)...)
		lines = append(lines, early)
	}
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	c := newDigestChain(t, "nine days", 0, lines)
	now := today.Start.Add(90 * time.Minute)
	c.at = now
	c.ledger.SummariseEnded(now)
	c.ledger.DigestEnded(t.Context(), now)
	clock := func() time.Time { return now }
	based := ledger.NewFollower(c.state, clock, ledger.CapsOf(nil, nil), DigestVersion, quiet)
	yesterday := yesterdayOf(today)
	for id, from := range based.SessionsOf(ids, func(string) bool { return false }, nil) {
		if from.Base == nil || from.Base.Date != yesterday {
			t.Fatalf("session %s rests on %+v, want its state at the end of %s", id, from.Base, yesterday)
		}
	}
	running := make([]status.Session, len(ids))
	for i, id := range ids {
		running[i] = status.Session{ID: id, Assignments: []status.Assignment{{Model: foldOpus, Account: "work", LastSeen: now}}}
	}
	tests := []struct {
		name    string
		version int
		read    []string
	}{
		{name: "over digests", version: DigestVersion, read: []string{today.Date}},
		{name: "read raw", version: DigestVersion + 1, read: dayfile.Span(first.Start, today.Start)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := datesTaken{Ledger: ledger.NewFollower(c.state, clock, ledger.CapsOf(nil, nil), tt.version, quiet), dates: make(map[string]bool)}

			list := ListSessions(SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now})

			if len(list.Sessions) != len(ids) {
				t.Errorf("the List lists %d sessions, want %d", len(list.Sessions), len(ids))
			}
			if dates := slices.Sorted(maps.Keys(l.dates)); !slices.Equal(dates, tt.read) {
				t.Errorf("the List read the lines of %v, want those of %v", dates, tt.read)
			}
		})
	}
}
