package views

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
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

// spreadRepriced is the date of the day the spread worlds' dated change of
// prices begins on: not their sessions' second day, as changingPrices has
// it, as s's page held open across the midnight before it then rests on no
// state, its first day's digest yet to stand.
const spreadRepriced = "2026-10-10"

// The hard moments the reads over digests meet, each met by some spread
// world.
const (
	onYesterdayLate  = "at 23:59, sessions resting on the day before's digest"
	yesterdayRaw     = "past midnight, the day before read raw, sessions resting on an older digest"
	onYesterdayEarly = "at 01:30, sessions resting on the day before's digest"
	idleThenActive   = "a run of days broken and resumed: a session idle for days, then active today, resting on its old state"
	pageOnState      = "a page open in the store on a session a digest holds a state of, resting on it and its chain of parts"
	givenAgain       = "a line of the day before given again once its digest stands, its session read whole"
	zoneEarly        = "a zone change's early line in a near day's file, its session resting on none"
	zoneLate         = "a zone change's late line in a day's file, arrived the day after, its session resting on none or an older state"
	pinnedAcross     = "a request in flight across midnight, its session's state at the day before's end"
	wantedAgain      = "a session the store held across the moments let go, then wanted again, resting on its state"
	fedAcrossOne     = "the store held across one in the morning, fed as the day before's digest comes to stand"
)

// spreadMoments are the hard moments the reads over digests meet: their
// own; those of a round trip, as a session resting on its state meets them;
// those of the pages; those of the find; those of the Log read back over
// rows, and through filters; those of a turn read over rows, and read back
// short; and those the upsets make.
var spreadMoments = slices.Concat([]string{
	onYesterdayLate, yesterdayRaw, onYesterdayEarly, idleThenActive, pageOnState, givenAgain, zoneEarly, zoneLate, pinnedAcross, wantedAgain,
	fedAcrossOne, settledAfter, flightAcross, agentAcross, idlessStored, idlessLater, turnStoppedShort,
}, pageMoments, findMoments, logMoments, filteredMoments, turnMoments(), upsetMoments())

// Every view read through the store over digests, of a real ledger digested
// at each moment as the router digests it, read through a Follower resting
// each session on the newest state a digest holds of it that stands, is what
// it is read through a Follower of the same files that reads every line, as
// of another version, for which none stands: the List, as the sessions verb
// prints it without the router; the routing card's costs; the Log's
// sessions, its find, as sameFind has it, and its past days read back, as
// sameLog has it, and read back through filters, as sameFiltered has it;
// the store as the List's first read holds it, a session's page open, and
// what the page shows; and the store held across the moments, fed as the
// dashboard feeds it, the router's sessions and the page wanted at some
// moments and not at others; each store given the day before's lines again
// at half past one; each session's page, as checkPages has them; and a turn
// of a page opened, as checkTurns has it, and one read back short of a
// crowded day's start, as checkStopped has it. At a minute
// before midnight, a minute after, a minute before one, the day before read
// raw, and half past one, its digest and head standing; the world's files,
// its days' heads and its days' rows upset between the router's rounds at
// some of them, as upset has it; of spread worlds, built with the built-in
// table and read with another, as digestPrices has them, meeting each of
// spreadMoments.
func TestEveryViewOverDigestsIsTheViewReadRaw(t *testing.T) {
	met := make(map[string]bool)
	prices := digestPrices(t, changingPricesFrom(spreadRepriced))
	for _, seed := range spreadSeeds {
		w := newSpread(t, seed, prices[seed%uint64(len(prices))], met)
		for i, at := range w.moments() {
			w.by(at)
			w.upset(at)
			w.check(i, at)
		}
	}
	for _, moment := range slices.Concat(spreadMoments, costedEither(prices), pagePricedEither(prices), loggedPricedEither(prices)) {
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
// the order the ledger files its lines, as each request ends: againRun's,
// its session s among others from the day before s began, a request of s's
// going upstream again after the midnight after its first while in flight,
// and s's first request on its second day without an id, as idlessOnce has
// it, given twice, with another of that day's, as twiceGiven has them;
// generatedRun's session of the next seed, as k, two days on; and
// generatedSession's of the seed after, as idle, its first half's requests,
// by when they arrived, on their own days, the rest three days on, idle
// between. And two requests as a change of time zone files them: early, of
// k's, arrived half an hour before the day k began on ended, to be filed
// under the day after; and late, of idle's, arrived half an hour after the
// last day idle ran on ended, to be filed under that day. Of crowdedSeed,
// that day is crowded besides, as crowded has it.
func spreadWorld(seed uint64) (lines []ledger.Line, early, late ledger.Line) {
	lines = twiceGiven(idlessOnce(againRun(seed)))
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
	if seed == crowdedSeed {
		lines = crowded(lines, dayfile.DateOf(idle[arrivalOrder(idle)[len(idle)-1]].At))
	}
	early = k[arrivalOrder(k)[0]]
	early.Request, early.At = "zone-early", dayfile.DayStart(early.At.Local(), 1).Add(-30*time.Minute)
	late = idle[arrivalOrder(idle)[len(idle)-1]]
	late.Request, late.At = "zone-late", dayfile.DayStart(late.At.Local(), 1).Add(30*time.Minute)
	return lines, early, late
}

// idlessOnce is lines, s's first request on the day after its first's
// without an id, as from before requests had them.
func idlessOnce(lines []ledger.Line) []ledger.Line {
	order := arrivalOrder(lines)
	first := lines[order[slices.IndexFunc(order, func(i int) bool { return lines[i].Session == pageSession })]]
	second := dayfile.DayStart(first.At.Local(), 1)
	if i := slices.IndexFunc(order, func(i int) bool {
		return lines[i].Session == pageSession && isRequest(&lines[i]) && !lines[i].At.Before(second)
	}); i >= 0 {
		lines[order[i]].Request = ""
	}
	return lines
}

// twiceGiven is lines, two of s's requests given twice, each again right
// after itself, as a ledger's files may hold a line twice: its first without
// an id, and the first with one that arrived on the day it did.
func twiceGiven(lines []ledger.Line) []ledger.Line {
	idless := slices.IndexFunc(lines, func(l ledger.Line) bool { return l.Session == pageSession && l.Request == "" && isRequest(&l) })
	if idless < 0 {
		return lines
	}
	day := dayfile.DateOf(lines[idless].At)
	twice := []int{idless}
	if i := slices.IndexFunc(lines, func(l ledger.Line) bool {
		return l.Session == pageSession && l.Request != "" && isRequest(&l) && dayfile.DateOf(l.At) == day
	}); i >= 0 {
		twice = append(twice, i)
	}
	slices.Sort(twice)
	for _, i := range slices.Backward(twice) {
		lines = slices.Insert(lines, i+1, lines[i])
	}
	return lines
}

// crowdedSeed is the seed of the spread world whose last day idle runs on is
// crowded, as crowded has it: of spreadSeeds, one whose idle ends its run
// on that day early enough for the crowd to be done by the evening.
const crowdedSeed = 11

// crowdRun is how many requests of no session crowd a day, as crowded has
// them: with the three of them crowded gives twice, readBackRun less one, the
// lines in a row a session's read back from a day's end reads before it
// stops, each ended before what it wants, so the line filed before them is
// the last read; and more than three chunks of rows, of which a filter
// skips some. Each slows every read of the day's file, so they're no more.
const crowdRun = readBackRun - 4

// crowded is lines, in the order the ledger files them, the local day with
// the given date crowded after them: a turn of a session of its own,
// crowded, a minute after the last of that day's lines ended, a quota check
// of it filed just before, its first turn two days before, at noon; then
// crowdRun requests of no session, each its own, of no model or account,
// arriving twenty seconds apart from a minute after that, each taking a
// second, so they're filed after the day's others, as each ends, and done
// by the evening. A turn of crowded's first, opened that night, reads the
// day back to its second request, the check unread. Two of the crowd
// failed, each given twice across an edge of the day's chunks of rows, as
// givenAcross has them: one with an id, at the second chunk's end; one
// without, at the third's. One in the fourth chunk is given twice within
// it, on work and then on personal, as givenElsewhere has it. And the day
// ends with one more given twice, its copy ending days on, as givenLater
// has it.
func crowded(lines []ledger.Line, date string) []ledger.Line {
	var last time.Time
	for _, l := range lines {
		if end := ended(&l); dayfile.DateOf(l.At) == date && end.After(last) {
			last = end
		}
	}
	first := ledger.Line{
		At: dayfile.DayStart(last, -2).Add(12 * time.Hour).UTC(), Request: "crowded-1", Kind: ledger.KindMessage, Session: "crowded", Model: foldOpus,
		Account: "work", Reason: "new", Status: 200, Attempts: 1, TotalMS: 3000, Class: ledger.ClassMain, Prompt: "c1",
	}
	again := first
	again.At, again.Request, again.Reason, again.Prompt = last.Add(time.Minute).UTC(), "crowded-2", "sticky", "c2"
	check := ledger.Line{
		At: last.Add(30 * time.Second).UTC(), Request: "crowded-check", Kind: ledger.KindCheck, Session: "crowded", Model: foldOpus, Account: "work",
		Status: 200, TotalMS: 500,
	}
	lines = append(lines, first, check, again)
	for i := range crowdRun {
		lines = append(lines, ledger.Line{
			At: ended(&again).Add(time.Minute + time.Duration(i)*20*time.Second).UTC(), Request: fmt.Sprintf("crowd-%04d", i), Kind: ledger.KindMessage,
			Status: 200, TotalMS: 1000,
		})
	}
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	return givenLater(givenElsewhere(givenAcross(givenAcross(lines, date, 2, false), date, 3, true), date, 4, "work", "personal"), date)
}

// givenAcross is lines, in the order the ledger files them, the last row of
// the chunk numbered chunk, from 1, of the rows of the local day with the
// given date failed, without an id where idless says, and given twice,
// right after itself: so its copy is the next chunk's first row, as the
// day's rows are its lines in the order they're filed, where no other day's
// file holds a line of it.
func givenAcross(lines []ledger.Line, date string, chunk int, idless bool) []ledger.Line {
	i := onDay(lines, date)[chunk*rowsAChunk-1]
	lines[i].Status = 500
	if idless {
		lines[i].Request = ""
	}
	return slices.Insert(lines, i+1, lines[i])
}

// givenElsewhere is lines, in the order the ledger files them, the row in
// the middle of the chunk numbered chunk, from 1, of the rows of the local
// day with the given date on the account first, and given again right after
// itself on the account again, both in that chunk: as two copies of a
// request say otherwise of its account.
func givenElsewhere(lines []ledger.Line, date string, chunk int, first, again string) []ledger.Line {
	i := onDay(lines, date)[(chunk-1)*rowsAChunk+rowsAChunk/2]
	lines[i].Account = first
	copied := lines[i]
	copied.Account = again
	return slices.Insert(lines, i+1, copied)
}

// givenLater is lines, in the order the ledger files them, the local day
// with the given date ended with a request of no session of its own at
// half past eleven that night, taking a second, given twice, right after
// itself, its copy saying it took three days: as two copies of a request may
// say otherwise of when it ended. The copy is passed over as given again, so
// a read that counted it would tell the ledger last wrote days on.
func givenLater(lines []ledger.Line, date string) []ledger.Line {
	start, _, _ := dayfile.Day(date)
	late := ledger.Line{At: start.Add(23*time.Hour + 30*time.Minute).UTC(), Request: "crowd-late", Kind: ledger.KindMessage, Status: 200, TotalMS: 1000}
	copied := late
	copied.TotalMS = (72 * time.Hour).Milliseconds()
	i, _ := slices.BinarySearchFunc(lines, ended(&late), func(l ledger.Line, end time.Time) int { return ended(&l).Compare(end) })
	return slices.Insert(lines, i, late, copied)
}

// onDay are the indexes of lines of those that arrived on the local day
// with the given date, in order.
func onDay(lines []ledger.Line, date string) []int {
	var on []int
	for i := range lines {
		if dayfile.DateOf(lines[i].At) == date {
			on = append(on, i)
		}
	}
	return on
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
// early and late, filed under the days beside those they arrived on, as
// beside has the dates of those it files so, by their requests' ids; read
// through two Followers of its files, each held across the moments, as a
// dashboard holds one: based, of the version the digests are of, which rests
// each session on the newest state a digest holds of it that stands; and
// raw, of another, for which none stands, so it reads every line. It prices
// as prices reads, the digests' states built with the built-in table; holds
// the stores the dashboard would, fed through each Follower at each moment,
// as across has them, and s's page held open across the moments, as
// pageAcross has it, nil while none is; counts the moments it came to upset
// its files at, those it came to upset its pages' at, its days' heads' and
// its days' rows', as upset has them, holding the upset made at the moment
// for its reads to judge, and the one of a day's rows, nil where none was,
// the late lines filed then, as fileLate files them, and the head
// upset then, as headUpset has it; counts the sessions and rows planted in
// its days' heads and rows, as plant and spoilChunk have them; holds the
// member of each day's rows it spoiled, as spoilChunk has it, by the day's
// date; and the dates of the days read back from their lines alone for a
// late line, their rows yet to be read again; and notes the hard moments
// its reads meet among met.
type spread struct {
	*digestChain
	early, late ledger.Line
	beside      map[string]string
	based, raw  *ledger.Follower
	prices      pricedAs
	held        across
	paging      *pageAcross
	upsets      int
	pageUpsets  int
	headUpsets  int
	rowsUpsets  int
	planted     int
	made        *upsetMade
	rowsMade    *upsetMade
	filedLate   []ledger.Line
	headMade    *headUpset
	spoiled     map[string]ledger.Member
	relogged    map[string]bool
	met         map[string]bool
}

// across are the stores a dashboard holds, fed through each Follower at each
// moment a read of the ledger's tail, as tail reads it through the one
// reading raw, from the day of the first moment on. restsOn is the date of
// the digest each session the store fed through the Follower resting on
// digests holds rested on as it was last held afresh, as restingOn gives it
// then, by its id: none of one read whole then; and ever the sessions it
// has held.
type across struct {
	based, raw HeldSessions
	tail       ledgerTail
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
		based:  ledger.NewFollower(c.state, clock, ledger.CapsOf(nil, nil), DigestVersion, quiet),
		raw:    ledger.NewFollower(c.state, clock, ledger.CapsOf(nil, nil), DigestVersion+1, quiet),
		held:   across{based: HoldSessions(prices.read), raw: HoldSessions(prices.read), restsOn: make(map[string]string), ever: make(map[string]bool)},
		beside: make(map[string]string), spoiled: make(map[string]ledger.Member), relogged: make(map[string]bool),
	}
	w.fileBeside(dayfile.DateOf(dayfile.DayStart(early.At, 1)), early)
	w.fileBeside(dayfile.DateOf(dayfile.DayStart(late.At, -1)), late)
	w.held.tail = ledgerTail{l: w.raw, day: Today(w.moments()[0]).Date}
	return w
}

// filedUnder is the date of the local day the world files l under: the one
// it was filed beside its own under, as fileBeside files it, else the day it
// arrived on.
func (w *spread) filedUnder(l ledger.Line) string {
	if date, beside := w.beside[l.Request]; beside {
		return date
	}
	return dayfile.DateOf(l.At)
}

// fileLine files l under the day filedUnder gives, after the lines filed
// there before, and holds it among the world's lines.
func (w *spread) fileLine(l ledger.Line) {
	w.file(w.filedUnder(l), l)
	w.lines = append(w.lines, l)
}

// fileBeside files l under the local day with the given date, beside the
// one it arrived on, as a change of time zone files a line, as fileLine
// files it.
func (w *spread) fileBeside(date string, l ledger.Line) {
	w.beside[l.Request] = date
	w.fileLine(l)
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
	w.same(m, "the List, as the sessions verb prints it", func(l Ledger) any {
		return ListSessions(SessionSources{Unlisted: true, Ledger: l, Prices: w.prices.read, Now: now})
	})
	listed := readingThrough(w.based)
	dashboards := func(l Ledger) SessionList {
		return ListSessions(SessionSources{Running: m.running, Stream: m.stream, Ledger: l, Prices: w.prices.read, Now: now})
	}
	w.alike(now, "the List, as the dashboard's first read lists it", dashboards(listed), dashboards(m.raw))
	m.based.got.add(listed.got)
	w.same(m, "the routing card's costs", func(l Ledger) any {
		return ListSessions(SessionSources{Running: m.running, Ledger: l, Prices: w.prices.read, Now: now}).Sessions
	})
	w.same(m, "the Log's sessions", func(l Ledger) any { return w.logSessions(l, m) })
	w.sameFind(m)
	w.sameLog(m)
	w.sameFiltered(m)

	paged := ListWanted(m.running, m.stream)
	paged.Page = pageSession
	based, raw := w.fresh(m.based, paged, now), w.fresh(m.raw, paged, now)
	w.sameStores(m, "the store, as the List's first read holds it, s's page open", based, raw, m.based.got.dates(), paged.Page)
	w.givenAgain(m, "the store, as the List's first read holds it", based, raw, paged)
	w.feedHeld(m)
	pages := w.checkPages(m)
	w.checkTurns(m, pages)
	w.checkStopped(m)
	w.noteRested(m, raw)
	w.note(m, listed.got.dates())
	w.judgeUpset(m, pages)
	if len(m.raw.got.states) > 0 {
		w.t.Errorf("%s at %v: the Follower reading raw gave %v resting on a digest's state, want none", w.name, now, m.raw.got.dates())
	}
}

// moment is what the reads at a moment share: it's the i-th, now; the
// world's sessions as the router lists them then, as running has them; the
// stream, as pinning has it; the two Followers, as the views read sessions
// through them at the moment, each read's sessions noted, as reading notes
// them; the Follower resting on digests as the find reads it, as found
// notes its reads, and as the Log reads its days back, as logged notes
// them; today's lines, as a Follower's Day reads them; and at half past
// one, the day before's lines to be given a store again, as the dashboard's
// read of the ledger's tail reads a day again whole, none at another
// moment.
type moment struct {
	i          int
	now        time.Time
	running    []status.Session
	stream     Stream
	based, raw reading
	found      *sidesRead
	logged     *rowsNoted
	today      []ledger.Held
	again      *Taken
}

// momentAt is the i-th moment, now, as moment has it, its lines read
// through the Follower reading raw, each day's once.
func (w *spread) momentAt(i int, now time.Time) moment {
	day := Today(now)
	m := moment{
		i: i, now: now, running: w.running(now), stream: w.pinning(now), today: w.raw.Day(day.Date),
		based: readingThrough(w.based), raw: readingThrough(w.raw), found: readingSides(w.based), logged: readingRows(w.based),
	}
	if now.Hour() == 1 {
		yesterday := yesterdayOf(day)
		again := Took(nil, map[string][]ledger.Held{yesterday: w.raw.Day(yesterday)})
		m.again = &again
	}
	return m
}

// feedHeld feeds the stores held across the moments at the moment m, as the
// dashboard feeds the one it holds: a read of the ledger's tail, as across's
// tail reads it, wanting what heldWants wants; and the day before's lines
// again, as givenAgain gives them. It holds the two to each other, each
// session's flights set aside as across's restsOn notes them, of the dates
// of the states the sessions rest on at m; and notes a session let go and
// wanted again, as the feed's reads gave it, and a store held since before
// one in the morning whose feed's reads rest a session on the day before's
// digest once it stands.
func (w *spread) feedHeld(m moment) {
	w.t.Helper()
	want, held := w.heldWants(m), &w.held
	t := held.tail.at(m.now)
	fed := readingThrough(w.based)
	based, gains := held.based.Fed(fed, t, want, m.now)
	raw, _ := held.raw.Fed(m.raw, t, want, m.now)
	m.based.got.add(fed.got)
	rested := fed.got.dates()
	yesterday := yesterdayOf(Today(m.now))
	for id, gain := range gains {
		if !gain.whole {
			continue
		}
		if _, was := held.based.sessions[id]; !was && held.ever[id] && rested[id] != "" {
			w.met[wantedAgain] = true
		}
		held.restsOn[id], held.ever[id] = rested[id], true
	}
	if m.now.Hour() == 1 && len(held.based.sessions) > 0 && slices.Contains(slices.Collect(maps.Values(rested)), yesterday) {
		w.met[fedAcrossOne] = true
	}
	w.sameStores(m, "the store held across the moments", based, raw, held.restsOn, want.Page)
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
		want.Page = pageSession
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
	based, basedGains := based.Fed(m.based, *m.again, want, m.now)
	raw, rawGains := raw.Fed(m.raw, *m.again, want, m.now)
	w.sameStores(m, what+", given the day before's lines again", based, raw, nil, want.Page)
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
	read, today := ReadLog(w.prices.read), RecordsOf(m.today)
	held, gains := HoldSessions(w.prices.read).Fed(l, Took(m.today, nil), SessionsWanted{Log: read.Wanted(Stream{}, today)}, m.now)
	read = read.Taken(today, true, LogTaking{Held: held, Gains: gains, Now: m.now})
	lines := make(map[string]int)
	for _, h := range m.today {
		lines[h.Session]++
	}
	return LogSessions(LogSessionSources{Read: read, Held: held, Lines: lines, Unlisted: true, Now: m.now})
}

// same fails the test where what a view gives at the moment m, read through
// the Follower resting on digests, isn't what it gives read through the one
// reading raw.
func (w *spread) same(m moment, view string, read func(l Ledger) any) {
	w.t.Helper()
	w.alike(m.now, view, read(m.based), read(m.raw))
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
// doesn't; a page of it shown otherwise, as pageShown shows it at m; or a
// List of either otherwise than the other's, as the dashboard lists it live
// at the moment m, told what m's stream tells.
func (w *spread) sameStores(m moment, what string, got, want HeldSessions, through map[string]string, page string) {
	w.t.Helper()
	now := m.now
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
		w.alike(now, what+", "+page+"'s page", pageShown(got, page, m), pageShown(want, page, m))
	}
	live := ListLive{Unlisted: true, Stream: m.stream, Now: now}
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
// it as h holds it, tallied, shows it at the moment m, no router listing the
// sessions, the stream telling what m's tells of it, as shownPageOf has it:
// none where h holds it untallied, or not at all.
func pageShown(h HeldSessions, id string, m moment) shownPage {
	s := h.sessions[id]
	if s == nil || s.page == nil {
		return shownPage{}
	}
	page := HeldPage{id: id, day: Today(m.now), prices: h.prices, keep: ledgerKept(0), record: s, read: m.now}
	return shownPageOf(page, PageLive{Unlisted: true, Stream: m.stream.Of(id), Now: m.now})
}

// restingOn is the state each of the world's sessions rests on, by its id,
// as the Follower resting on digests gives them asked for none whole: none
// of one it reads whole.
func (w *spread) restingOn() map[string]*ledger.SessionBase {
	bases := make(map[string]*ledger.SessionBase)
	for id, from := range w.based.SessionsOf(sessionsIn(w.lines), nil, nil) {
		if from.Base != nil {
			bases[id] = from.Base
		}
	}
	return bases
}

// reading is a Follower as the views read sessions through it, each read's
// sessions noted in got. It hands each read's take on as the view gave it,
// none where it gave none, unlike datesTaken, which hands every read one of
// its own to note the days it reads: so the ledger decodes each read's lines
// as it would the view's.
type reading struct {
	Ledger
	got *basesGot
}

// readingThrough is f read through as reading has it, no read noted yet.
func readingThrough(f *ledger.Follower) reading {
	return reading{Ledger: f, got: &basesGot{
		states: make(map[string]*ledger.SessionBase), pages: make(map[string]ledger.SessionFrom), none: make(map[string]bool), paged: make(map[string]bool),
	}}
}

func (r reading) Sessions(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return r.got.noted(r.Ledger.Sessions(ids, r.got.asking(rest), take))
}

func (r reading) SessionsOf(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return r.got.noted(r.Ledger.SessionsOf(ids, r.got.asking(rest), take))
}

// basesGot are what reads of sessions were asked and gave: the state each
// session they gave rests on, by its id, the newest any read gave it; the
// state each they gave resting on its page rests on, with its chain of parts,
// the newest any gave; the sessions some read gave resting on none; and
// those some read was asked to rest on their pages, as RestOnPage asks.
type basesGot struct {
	states map[string]*ledger.SessionBase
	pages  map[string]ledger.SessionFrom
	none   map[string]bool
	paged  map[string]bool
}

// asking is what a read asked to rest each session as rest says asks, as a
// read takes nil to rest each on its state, each asked to rest on its page
// noted.
func (g *basesGot) asking(rest func(string) ledger.Rest) func(string) ledger.Rest {
	return func(id string) ledger.Rest {
		on := ledger.RestOnState
		if rest != nil {
			on = rest(id)
		}
		if on == ledger.RestOnPage {
			g.paged[id] = true
		}
		return on
	}
}

// noted is sessions, a read's, noted among what reads gave: those given a
// chain of parts as resting on their pages.
func (g *basesGot) noted(sessions map[string]ledger.SessionFrom) map[string]ledger.SessionFrom {
	for id, from := range sessions {
		if from.Base == nil {
			g.none[id] = true
			continue
		}
		if was := g.states[id]; was == nil || from.Base.Date > was.Date {
			g.states[id] = from.Base
		}
		if was := g.pages[id].Base; len(from.Parts) > 0 && (was == nil || from.Base.Date > was.Date) {
			g.pages[id] = ledger.SessionFrom{Base: from.Base, Parts: from.Parts}
		}
	}
	return sessions
}

// add notes what o's reads were asked and gave among g's.
func (g *basesGot) add(o *basesGot) {
	for id, base := range o.states {
		g.noted(map[string]ledger.SessionFrom{id: {Base: base}})
	}
	g.noted(o.pages)
	maps.Copy(g.none, o.none)
	maps.Copy(g.paged, o.paged)
}

// restedPage is the state the reads rested the page of the session with the
// given id on, with its chain of parts, the newest any gave, reporting false
// where none gave one, or one read it whole, as where it's refused.
func (g *basesGot) restedPage(id string) (ledger.SessionFrom, bool) {
	from := g.pages[id]
	return from, from.Base != nil && !g.none[id]
}

// refusedPage reports whether a read gave the page of the session with the
// given id resting on a state and its chain of parts, and one read it whole
// after, as where it can't be held on them.
func (g *basesGot) refusedPage(id string) bool {
	return g.pages[id].Base != nil && g.none[id]
}

// dates are the dates of the days whose digests hold the states the reads
// gave, by their sessions' ids.
func (g *basesGot) dates() map[string]string {
	dates := make(map[string]string, len(g.states))
	for id, base := range g.states {
		dates[id] = base.Date
	}
	return dates
}

// noteRested notes the hard moments of a round trip, as restoring's note
// notes them, that each session a read compared at the moment m rested on a
// state meets: its state as built, restored with the built-in table onto the
// day after its own, the trip's day D; the day m falls on its day T; and the
// session read whole there as whole, a store read raw, holds it; its
// requests those arrived by m.
func (w *spread) noteRested(m moment, whole HeldSessions) {
	w.t.Helper()
	for id, base := range m.based.got.states {
		held := whole.sessions[id]
		if held == nil {
			continue
		}
		start, _, _ := dayfile.Day(base.Date)
		built, err := restoredFrom(ledger.SessionFrom{Base: base}, ledger.Pricing, Today(dayfile.DayStart(start, 1)), pageNone)
		if err != nil {
			w.t.Fatalf("%s at %v: session %s rests on a state that won't restore: %v", w.name, m.now, id, err)
		}
		lines := slices.DeleteFunc(slices.Clone(w.lines), func(l ledger.Line) bool { return l.Session != id || l.At.After(m.now) })
		r := &restoring{t: w.t, name: w.name, seed: w.seed, prices: w.prices, lines: lines, requests: heldRequestsOf(lines), met: w.met}
		r.note(trip{r: r, from: dayState{day: Today(start), held: built}, on: Today(m.now), whole: held})
	}
}

// note notes the hard moments the reads compared at the moment m meet, as
// what they gave has them: the states the sessions rest on, and those read
// resting on none; the stream pinning one. Those of the clock, of the List
// itself, as listed gives the dates of the states its read rested the
// sessions on.
func (w *spread) note(m moment, listed map[string]string) {
	now, stream, bases, none := m.now, m.stream, m.based.got.dates(), m.based.got.none
	ids := sessionsIn(w.lines)
	today := Today(now)
	yesterday := yesterdayOf(today)
	onYesterday := slices.ContainsFunc(ids, func(id string) bool { return listed[id] == yesterday })
	onOlder := slices.ContainsFunc(ids, func(id string) bool { return listed[id] != "" && listed[id] < yesterday })
	for _, moment := range []struct {
		name string
		met  bool
	}{
		{name: onYesterdayLate, met: now.Hour() == 23 && onYesterday},
		{name: yesterdayRaw, met: now.Hour() == 0 && onOlder && !onYesterday},
		{name: onYesterdayEarly, met: now.Hour() == 1 && onYesterday},
		{name: idleThenActive, met: bases["idle"] != "" && bases["idle"] < yesterday && w.arrivedOn("idle", today, now)},
		{name: pageOnState, met: m.based.got.paged[pageSession] && m.based.got.pages[pageSession].Base != nil},
		{name: zoneEarly, met: !w.early.At.After(now) && none[w.early.Session] && bases[w.early.Session] == "" && w.digestHolds(dayfile.DateOf(w.early.At), w.early.Session)},
		{name: zoneLate, met: !w.late.At.After(now) && (none[w.late.Session] || bases[w.late.Session] != "") && bases[w.late.Session] < w.filedUnder(w.late) && w.digested(w.filedUnder(w.late))},
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
	lateOnDigested     = "a late line onto the day before, after its digest, of a session resting on its state of it, its page opened then falling back behind the day"
	lateCascading      = "a late line onto D-2, after D-1's and D's digests, of a session resting on a state of one of them, its page opened then falling back behind D-2"
	lateNeverRan       = "a late line of a session onto a day before its state's, a day it never ran on"
	digestGone         = "a digest deleted, a session resting on the day after's state and on none of the day's lines, its page opened then falling back behind the day"
	digestOtherVersion = "the day before's digest of the version before the views'"
	dayPruned          = "the first day's lines pruned under a state resting on them"
)

// upsetting is an upset of a spread world's files between the router's
// rounds, by the hard moment it makes: upset makes it at now, giving what
// judges whether the reads compared then meet it, nil where the world didn't
// let it, as where no session rests on what it needs; and the id of the
// session whose page is opened then for it to judge, "" of none.
type upsetting struct {
	moment string
	upset  func(w *spread, now time.Time) (upsetJudged, string)
}

// upsetJudged judges whether the reads compared at the moment m meet the
// moment of an upset made then, of what they gave, the pages opened among
// them, by their sessions' ids.
type upsetJudged func(m moment, pages map[string]openedPage) bool

// upsetMade is an upset made at a moment, its own moment to be judged met by
// the reads compared then, as judged judges it, page the id of the session
// whose page is opened then for it, "" of none.
type upsetMade struct {
	moment string
	judged upsetJudged
	page   string
}

// madeMeets is an upset that meets its moment as it's made, as upset reports
// whether the world let it.
func madeMeets(upset func(w *spread, now time.Time) bool) func(w *spread, now time.Time) (upsetJudged, string) {
	return func(w *spread, now time.Time) (upsetJudged, string) {
		if !upset(w, now) {
			return nil, ""
		}
		return func(moment, map[string]openedPage) bool { return true }, ""
	}
}

// upsets are the upsets a spread world's files take, each in turn.
var upsets = []upsetting{
	{moment: lateOnDigested, upset: lateOnto(1)},
	{moment: lateCascading, upset: lateOnto(3)},
	{moment: lateNeverRan, upset: madeMeets((*spread).lateNeverRan)},
	{moment: digestGone, upset: (*spread).digestGone},
	{moment: digestOtherVersion, upset: madeMeets((*spread).digestOtherVersion)},
	{moment: dayPruned, upset: madeMeets((*spread).dayPruned)},
}

// upsetMoments are the hard moments the upsets make, the pages' and the
// rows' among them.
func upsetMoments() []string {
	var moments []string
	for _, u := range slices.Concat(upsets, pageUpsets, rowsUpsets) {
		moments = append(moments, u.moment)
	}
	return moments
}

// upset has the next of upsets, in turn from the one the world's seed
// gives, upset its files at now, at each moment the day before's digest
// stands, a minute before midnight and half past one, as yesterdayStands
// has them; and a minute after midnight, before it stands, the pages opened
// then, as pagesOpenAt has them, the next of pageUpsets, as madeUnmet has
// it, and beside it, the next of rowsUpsets alike, each upset slowing every
// read after it: holding the upset made to judge, as judgeUpset judges it,
// where the world lets it make it; and a minute before one, the head of a
// day upset, as upsetHead has it. The router's round at the next moment
// digests again what it upset.
func (w *spread) upset(now time.Time) {
	w.made, w.rowsMade, w.filedLate, w.headMade = nil, nil, nil, nil
	switch {
	case headsUpsetAt(now):
		w.upsetHead(now)
	case yesterdayStands(now):
		w.made = upsets[(w.upsets+int(w.seed))%len(upsets)].made(w, now)
		w.upsets++
	case pagesOpenAt(now):
		w.rowsMade = w.madeUnmet(rowsUpsets, &w.rowsUpsets, now)
		w.made = w.madeUnmet(pageUpsets, &w.pageUpsets, now)
	}
}

// madeUnmet is the next of us the world lets make at now, of those whose
// moments no world has met yet, as each has its one moment a day, in turn
// from the one the turns n counts and the world's seed give, n counting
// each turn taken: none where it lets make none.
func (w *spread) madeUnmet(us []upsetting, n *int, now time.Time) *upsetMade {
	for range us {
		u := us[(*n+int(w.seed))%len(us)]
		*n++
		if w.met[u.moment] {
			continue
		}
		if made := u.made(w, now); made != nil {
			return made
		}
	}
	return nil
}

// made is the upset u makes at now, held to judge: none where the world
// doesn't let it make it.
func (u upsetting) made(w *spread, now time.Time) *upsetMade {
	judged, page := u.upset(w, now)
	if judged == nil {
		return nil
	}
	return &upsetMade{moment: u.moment, judged: judged, page: page}
}

// yesterdayStands reports whether the day before the one now falls on has its
// digest standing, of the moments the reads are held at: a minute before
// midnight and half past one, but not a minute after midnight, nor a minute
// before one.
func yesterdayStands(now time.Time) bool {
	return now.Hour() == 23 || now.Hour() == 1
}

// judgeUpset notes the moment of each upset made at the moment m, of the
// world's files and of a day's rows, where one was, met where the reads
// compared then meet it, as it judges them, pages the pages opened among
// them.
func (w *spread) judgeUpset(m moment, pages map[string]openedPage) {
	for _, u := range []*upsetMade{w.made, w.rowsMade} {
		if u != nil && u.judged(m, pages) {
			w.met[u.moment] = true
		}
	}
}

// lateOnto is the upset filing a late line onto the day back days before
// the one now falls on, of a session resting on a state resting on that day,
// a later day's where back is more than one, s's where it does, as
// pageFirst orders them, so the line stales each state of the session's
// chain from that day on: a copy of one of the session's requests that
// arrived on it, as another. Judged met as fellBehind judges the session's
// page opened then.
func lateOnto(back int) func(w *spread, now time.Time) (upsetJudged, string) {
	return func(w *spread, now time.Time) (upsetJudged, string) {
		date := dayfile.DateOf(dayfile.DayStart(now, -back))
		states := w.restingOn()
		for _, id := range pageFirst(slices.Collect(maps.Keys(states))) {
			base := states[id]
			if _, rests := base.RestsOn[date]; !rests || back > 1 && base.Date == date {
				continue
			}
			if i := slices.IndexFunc(w.lines, func(l ledger.Line) bool { return l.Session == id && w.filedUnder(l) == date }); i >= 0 {
				w.fileLate(w.another(w.lines[i]))
				return fellBehind(id, date), id
			}
		}
		return nil, ""
	}
}

// pageFirst are the given ids, s's first, then the rest in order.
func pageFirst(ids []string) []string {
	slices.Sort(ids)
	if i := slices.Index(ids, pageSession); i > 0 {
		ids = slices.Insert(slices.Delete(ids, i, i+1), 0, pageSession)
	}
	return ids
}

// fellBehind judges an upset met where the page of the session with the
// given id, opened then, rests on the state of a day before the one with the
// given date, and its chain of parts, or is read whole: so it reads that
// day's lines raw, as the upset leaves its states of it, and of each after,
// standing for none.
func fellBehind(id, date string) upsetJudged {
	return func(_ moment, pages map[string]openedPage) bool {
		o, opened := pages[id]
		if !opened {
			return false
		}
		from, rested := o.got.restedPage(id)
		return o.got.none[id] || rested && from.Base.Date < date
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
		w.fileLate(l)
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

// digestGone deletes the digest of a day, the latest before the one now falls
// on where the ledger holds the digests of it and the day after and a session
// rests across it, as acrossGap finds one; and files a late line of that
// session onto it, arrived at noon the day after, which the session's state
// of the day after never took in. Judged met as fellBehind judges the
// session's page opened then.
func (w *spread) digestGone(now time.Time) (upsetJudged, string) {
	w.t.Helper()
	states := w.restingOn()
	for back := 2; ; back++ {
		gap, after := dayfile.DayStart(now, -back), dayfile.DayStart(now, -back+1)
		if !w.digestsHeld(gap, after) {
			return nil, ""
		}
		id, across := acrossGap(states, dayfile.DateOf(gap), dayfile.DateOf(after))
		if !across {
			continue
		}
		if err := os.Remove(w.digestPath(dayfile.DateOf(gap))); err != nil {
			w.t.Fatal(err)
		}
		l := w.another(w.lines[slices.IndexFunc(w.lines, func(l ledger.Line) bool { return l.Session == id })])
		l.At = after.Add(12 * time.Hour).UTC()
		w.fileBeside(dayfile.DateOf(gap), l)
		return fellBehind(id, dayfile.DateOf(gap)), id
	}
}

// digestsHeld reports whether the ledger holds the digest of each local day
// beginning at one of starts.
func (w *spread) digestsHeld(starts ...time.Time) bool {
	return !slices.ContainsFunc(starts, func(start time.Time) bool {
		_, err := os.Stat(w.digestPath(dayfile.DateOf(start)))
		return err != nil
	})
}

// acrossGap is the id of the first of the sessions resting on states, as
// pageFirst orders their ids, to rest across the local day with the date
// gap: on the state of the day after, after, and on none of the gap's lines;
// reporting false where none does.
func acrossGap(states map[string]*ledger.SessionBase, gap, after string) (string, bool) {
	ids := pageFirst(slices.Collect(maps.Keys(states)))
	i := slices.IndexFunc(ids, func(id string) bool {
		_, rests := states[id].RestsOn[gap]
		return states[id].Date == after && !rests
	})
	if i < 0 {
		return "", false
	}
	return ids[i], true
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
	if err := os.Remove(w.dayPath(date)); err != nil {
		w.t.Fatal(err)
	}
	w.lines = slices.DeleteFunc(w.lines, func(l ledger.Line) bool { return w.filedUnder(l) == date })
	return true
}
