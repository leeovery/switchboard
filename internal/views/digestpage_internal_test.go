package views

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// pageSeeds is how many seeds of each of pageWorlds a page's chain folds.
const pageSeeds = 60

// nanosEnd is the last moment nanoseconds since the epoch hold.
var nanosEnd = time.Unix(0, math.MaxInt64)

// pageWorlds are the generated worlds whose session s has its page folded
// day by day: a run of feeds, as generatedRun makes it; a page's, as
// pagedRun makes it, other sessions' answers among it, reading their
// accounts' windows; one whose request goes upstream again after midnight,
// still in flight, as againRun makes it; and a page's run moved to end past
// the last moment nanoseconds since the epoch hold, and to begin before the
// first.
var pageWorlds = []struct {
	name  string
	lines func(seed uint64) []ledger.Line
}{
	{name: "generated", lines: generatedRun},
	{name: "paged", lines: func(seed uint64) []ledger.Line { lines, _ := pagedRun(seed); return lines }},
	{name: "a request going upstream again after midnight", lines: againRun},
	{name: "across the end of the nanoseconds' years", lines: func(seed uint64) []ledger.Line {
		return movedTo(promptedRun(seed), nanosEnd.Add(-3*time.Hour))
	}},
	{name: "before the nanoseconds' years", lines: func(seed uint64) []ledger.Line { return yearsBack(promptedRun(seed), 500) }},
}

// againRun is a run as pagedRun makes it, with a request of s's in flight
// across the midnight after its first, going upstream again under its id
// after midnight while in flight, a line coming of each time it went: the
// traces count it once, as wentUp has it.
func againRun(seed uint64) []ledger.Line {
	lines, _ := pagedRun(seed)
	first := lines[slices.IndexFunc(lines, func(l ledger.Line) bool { return l.Session == "s" })]
	midnight := dayfile.DayStart(first.At.Local(), 1)
	l := ledger.Line{
		Request: "again", At: midnight.Add(-10 * time.Minute).UTC(), Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: "work",
		Reason: "sticky", Status: 200, Attempts: 1, TotalMS: (30 * time.Minute).Milliseconds(), Class: ledger.ClassMain, Prompt: "pagain",
		Usage: json.RawMessage(foldUsages[2]),
	}
	again := l
	again.At, again.TotalMS = midnight.Add(5*time.Minute).UTC(), 2000
	lines = append(lines, l, again)
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	return lines
}

// movedTo is lines moved in time, each by as much, the first to arrive
// arriving at at.
func movedTo(lines []ledger.Line, at time.Time) []ledger.Line {
	by := at.Sub(lines[arrivalOrder(lines)[0]].At)
	for i := range lines {
		lines[i].At = lines[i].At.Add(by)
	}
	return lines
}

// yearsBack is lines moved the given number of years back, each on its own
// date of that year.
func yearsBack(lines []ledger.Line, years int) []ledger.Line {
	for i := range lines {
		lines[i].At = lines[i].At.AddDate(-years, 0, 0)
	}
	return lines
}

// The hard moments of a page folded day by day, of a fold of a day D
// resting on the state of an earlier day B.
const (
	pageTurnAcross   = "a turn begun before D taking a request in on D, in two days' parts"
	pageAgentAcross  = "a subagent first seen before D, its requests going on on D"
	pageFlightAcross = "a request in flight across D's end, stored"
	pageAgain        = "a request going upstream again on D, in flight since before it, counted once"
	pageFar          = "times beyond the nanoseconds' years"
	pageFarAfter     = "times beyond the nanoseconds' years in D's part, after times within them in an earlier one"
	pageRefused      = "answers of D the points can't take in, the page folded whole again"
	pageGap          = "D after a day of none of the session's requests"
	pageIdle         = "an idle gap folded"
)

// A session's page folded at the end of each day it's named on, as a day's
// digests keep it, each fold resting on the state the last such day before
// it kept, restored from what it carries and the chain of parts behind it, as
// restoredPaged has it, its points taking every account's answers of each
// day since, oldest first, each whole, then its requests since told on, as
// the store tells them, sealed at the day's end: is, restored from the
// state each day keeps and its chain of parts, the page folded whole of
// every line by the day's end. Its turns, agents, sums and all else alike,
// its times as lists, its points' points and shares; and each day's part
// exactly what its fold added: the turns it took a request into, or began,
// the agents first seen, and its times. A fold whose points can't take a
// day's answers in folds whole again, its part whole. Of generated worlds,
// meeting the hard moments the constants above name.
func TestAPageFoldedDayByDayThroughItsStatesAndPartsIsThePageFoldedWhole(t *testing.T) {
	met := make(map[string]bool)
	for _, w := range pageWorlds {
		for seed := range uint64(pageSeeds) {
			c := newPageChain(t, fmt.Sprintf("%s %d", w.name, seed), w.lines(seed), met)
			for _, day := range c.days() {
				c.fold(day)
			}
		}
	}
	for _, moment := range []string{
		pageTurnAcross, pageAgentAcross, pageFlightAcross, pageAgain, pageFar, pageFarAfter, pageRefused, pageGap, pageIdle,
	} {
		if !met[moment] {
			t.Errorf("no chain meets %s", moment)
		}
	}
}

// pageChain is a generated session's page as a day's digests keep it,
// folded day by day: named by its world and seed; the session's lines; every
// account's answers, by the date of the day whose files hold them, as
// answeredOf has each of the page of s; the state the last day folded kept,
// none before the first; and the hard moments the chains met.
type pageChain struct {
	t       *testing.T
	name    string
	lines   []ledger.Line
	answers map[string][]answered
	kept    *keptPage
	met     map[string]bool
}

// keptPage is a session's state as a day's digest keeps it: its day; its
// state, as its bytes read back; and the parts of its page's chain, oldest
// first, from one folded on none through the day's own.
type keptPage struct {
	day    LedgerDay
	stored storedSession
	parts  []json.RawMessage
}

// newPageChain is the page of s of lines, named as given, folded on no day
// yet, noting the moments it meets in met.
func newPageChain(t *testing.T, name string, lines []ledger.Line, met map[string]bool) *pageChain {
	byDate := make(map[string][]ledger.Line)
	for _, l := range lines {
		byDate[dayfile.DateOf(l.At)] = append(byDate[dayfile.DateOf(l.At)], l)
	}
	own := slices.DeleteFunc(slices.Clone(lines), func(l ledger.Line) bool { return l.Session != "s" })
	return &pageChain{t: t, name: name, lines: own, answers: answersOf(byDate, "s"), met: met}
}

// days are the days a line of the session's names it on, as a day's summary
// names it, in order.
func (c *pageChain) days() []LedgerDay {
	var dates []string
	for _, l := range c.lines {
		if l.CountedSession() == "s" {
			dates = append(dates, dayfile.DateOf(l.At))
		}
	}
	var days []LedgerDay
	for _, date := range slices.Compact(slices.Sorted(slices.Values(dates))) {
		days = append(days, dayOf(date))
	}
	return days
}

// fold folds the session at the end of the day given, as foldedOn has it,
// keeps its state and its page's chain of parts as a day's digest would,
// and holds the part to what the fold added, and the state restored to the
// page folded whole.
func (c *pageChain) fold(day LedgerDay) {
	c.t.Helper()
	held, base := c.foldedOn(day)
	f, err := held.folded(base)
	if err != nil {
		c.t.Fatalf("%s, folded at the end of %s: %v", c.name, day.Date, err)
	}
	var st storedSession
	if err := json.Unmarshal(f.State, &st); err != nil {
		c.t.Fatalf("%s, folded at the end of %s, reading back %s: %v", c.name, day.Date, f.State, err)
	}
	parts := []json.RawMessage{f.Part}
	if base != nil {
		parts = append(slices.Clone(c.kept.parts), f.Part)
	}
	c.partExactly(day, held.page, base, f.Part)
	c.note(day, held.page, base, parts)
	c.kept = &keptPage{day: day, stored: st, parts: parts}
	c.check(day)
}

// foldedOn is the session folded at the end of the day given, as a day's
// digest folds it, and the tally its page was folded on: resting on the
// state kept last, restored onto the day with its parts, its points taking
// every account's answers of each day after the state's through the day
// given, oldest first, each whole, then its requests since told on, each
// once, sealed at the day's end. Where none is kept yet, or its points
// can't take the answers in, it's folded whole, on no tally.
func (c *pageChain) foldedOn(day LedgerDay) (*heldSession, *pageTally) {
	c.t.Helper()
	if c.kept == nil {
		return c.whole(day), nil
	}
	s, err := c.kept.stored.restoredPaged(ledger.Pricing, day, c.kept.parts)
	if err != nil {
		c.t.Fatalf("%s, kept at the end of %s, restored on %s: %v", c.name, c.kept.day.Date, day.Date, err)
	}
	base, points := s.page, s.page.points
	for _, date := range slices.Sorted(maps.Keys(c.answers)) {
		if date <= c.kept.day.Date || date > day.Date {
			continue
		}
		var ok bool
		if points, ok = points.took(date, c.answers[date], true, day); !ok {
			c.met[pageRefused] = true
			return c.whole(day), nil
		}
	}
	s.page = base.pointed(points)
	if since := c.since(day); len(since) > 0 {
		s = s.with(pricedAt(distinct(heldRequestsOf(since)), ledger.Pricing, day.Date))
	}
	return s.sealedAt(endOfDay(day)), base
}

// since are the session's lines that arrived after the day kept last ended,
// before the day given ended.
func (c *pageChain) since(day LedgerDay) []ledger.Line {
	from, until := endOfDay(c.kept.day).time(), endOfDay(day).time()
	return slices.DeleteFunc(slices.Clone(c.lines), func(l ledger.Line) bool { return l.At.Before(from) || !l.At.Before(until) })
}

// whole is the session folded whole at the end of the day given, its page
// tallied, of every line of it that arrived by then, and every account's
// answers of the days through it.
func (c *pageChain) whole(day LedgerDay) *heldSession {
	end := endOfDay(day)
	return readOf(arrivedBefore(c.lines, end.time()), datesThrough(c.answers, day.Date), ledger.Pricing, day, end, true)
}

// check holds the page the state kept last carries, restored on the day
// after its own with its parts, to the page folded whole at the end of the
// day given, its own.
func (c *pageChain) check(day LedgerDay) {
	c.t.Helper()
	s, err := c.kept.stored.restoredPaged(ledger.Pricing, Today(endOfDay(day).time()), c.kept.parts)
	if err != nil {
		c.t.Fatalf("%s, kept at the end of %s, restored: %v", c.name, day.Date, err)
	}
	if whole := c.whole(day).page; !samePage(s.page, whole) {
		c.t.Errorf("%s, folded day by day to the end of %s, restored with %d parts, is\n%s\nwant, as folded whole\n%s",
			c.name, day.Date, len(c.kept.parts), pageSaid(s.page), pageSaid(whole))
	}
}

// partExactly holds data, the part of the day's fold of after on base, to
// what the fold added: every turn after holds that base doesn't, or holds
// otherwise, by its index, and no other; each agent after knows that base
// doesn't; and the times after holds beyond base's, as many.
func (c *pageChain) partExactly(day LedgerDay, after, base *pageTally, data json.RawMessage) {
	c.t.Helper()
	part := partRead(c.t, data)
	if base == nil {
		base = newPageTally()
	}
	var want, got []int
	for i, t := range after.turns {
		if i >= len(base.turns) || !reflect.DeepEqual(t, base.turns[i]) {
			want = append(want, i)
		}
	}
	for _, t := range part.Turns {
		got = append(got, t.Index)
	}
	agents := maps.Clone(after.agents)
	maps.DeleteFunc(agents, func(agent, _ string) bool { _, known := base.agents[agent]; return known })
	times, err := part.Times.times()
	if !slices.Equal(got, want) || !maps.Equal(part.Agents, agents) || err != nil || len(times) != after.upstream.len()-base.upstream.len() {
		c.t.Errorf("%s, folded at the end of %s, has the part %s, %v; want the turns %v, the agents %v, %d times",
			c.name, day.Date, data, err, want, agents, after.upstream.len()-base.upstream.len())
	}
}

// note notes the hard moments the fold of the day given meets, of after on
// base, its chain of parts as given.
func (c *pageChain) note(day LedgerDay, after, base *pageTally, parts []json.RawMessage) {
	end := endOfDay(day).time()
	c.met[pageFlightAcross] = c.met[pageFlightAcross] || slices.ContainsFunc(c.lines, func(l ledger.Line) bool { return l.At.Before(end) && !ended(&l).Before(end) })
	c.met[pageIdle] = c.met[pageIdle] || len(after.folds) > 0
	c.met[pageFar] = c.met[pageFar] || after.upstream.far != nil
	if base == nil {
		return
	}
	c.met[pageGap] = c.met[pageGap] || c.kept.day.Date < yesterdayOf(day)
	part := partRead(c.t, parts[len(parts)-1])
	c.met[pageTurnAcross] = c.met[pageTurnAcross] || slices.ContainsFunc(part.Turns, func(t storedTurn) bool { return t.Index < len(base.turns) })
	for _, l := range c.since(day) {
		_, known := base.agents[l.AgentID]
		c.met[pageAgentAcross] = c.met[pageAgentAcross] || l.AgentID != "" && known
		c.met[pageAgain] = c.met[pageAgain] || slices.ContainsFunc(base.flying, func(f flight) bool {
			return f.request == l.Request && f.at != instantOf(l.At) && f.inFlightAt(l.At)
		})
	}
	if len(part.Times.Far) > 0 && slices.ContainsFunc(parts[:len(parts)-1], func(p json.RawMessage) bool { return len(partRead(c.t, p).Times.Nanos) > 0 }) {
		c.met[pageFarAfter] = true
	}
}

// partRead is the part data puts, read back.
func partRead(t *testing.T, data json.RawMessage) storedPart {
	t.Helper()
	var part storedPart
	if err := json.Unmarshal(data, &part); err != nil {
		t.Fatalf("reading back the part %s: %v", data, err)
	}
	return part
}

// samePage reports whether two pages' tallies hold the same, as a page is
// built of them: their times as lists, their points' points and shares, and
// all else alike, but what a copy of either owns.
func samePage(a, b *pageTally) bool {
	if !slices.Equal(timesOf(a.upstream), timesOf(b.upstream)) ||
		!reflect.DeepEqual(a.points.worked(), b.points.worked()) || !reflect.DeepEqual(a.points.taken(), b.points.taken()) {
		return false
	}
	x, y := *a, *b
	for _, p := range []*pageTally{&x, &y} {
		p.own, p.ownMaps, p.upstream, p.points = nil, false, upstreamTimes{}, nil
	}
	return reflect.DeepEqual(x, y)
}

// timesOf are the times u holds, oldest first.
func timesOf(u upstreamTimes) []time.Time {
	times := make([]time.Time, u.len())
	for i := range times {
		times[i] = u.at(i)
	}
	return times
}

// pageSaid is what a page's tally holds, as its stored forms put it: what
// it carries, all of it as a part, and its points' points and shares; none
// of none.
func pageSaid(p *pageTally) string {
	if p == nil {
		return "none"
	}
	said, err := json.Marshal(struct {
		Carried storedPage
		Part    storedPart
		Points  map[string]map[string]int
		Shares  map[string]map[string]float64
	}{p.carried(), p.partOver(nil), p.points.worked(), p.points.taken()})
	if err != nil {
		return err.Error()
	}
	return string(said)
}

// pinnedPage is a page's tally holding a value of every key its stored
// forms have: two turns, the first of its own conversation, ended, calling
// a tool, the second its own conversation's request pending, not yet ended,
// a subagent's ended after it, as only a tally told on holds a request
// pending; an agent first seen; a request in none in flight; an idle gap
// folded and a stretch open; where its model went, why it's kept there,
// what its account and its totals spent; a request still in flight; and
// its points, a day's answers let go of. Its times are three, the second a
// second and a half after the first.
func pinnedPage() *pageTally {
	at := func(minutes int) time.Time { return time.Date(2026, 10, 7, 12, minutes, 0, 0, time.UTC) }
	var spent ledger.Spent
	spent.Add(foldOpus, "us", ledger.SpendOf(&ledger.Line{Kind: ledger.KindMessage, Usage: json.RawMessage(`{"input_tokens":10,"output_tokens":300}`)}))
	end := func(minutes int, stop string) turnEnd {
		return turnEnd{end: at(minutes), lined: stop != "", stop: stop, set: true}
	}
	return &pageTally{
		turns: []*turnTally{
			{
				prompt: "p1", started: at(0), last: end(1, "tool_use"), lastMain: end(1, "tool_use"), requests: 2, tools: map[string]int{"Read": 2},
				read: 90000, written: 2000, out: 300, spent: spent, accounts: []string{"work"}, mainAccounts: []string{"work"},
			},
			{
				prompt: "p2", started: at(70), last: end(71, ""), lastMain: turnEnd{set: true}, requests: 2, tools: map[string]int{}, accounts: []string{},
				mainAccounts: []string{}, pending: true, mainPending: true,
			},
		},
		prompts: map[string]int{"p1": 0, "p2": 1}, agents: map[string]string{"a1": "p2"}, inNone: true,
		begun: true, first: at(0), last: at(70), folds: []Fold{{From: at(1), To: at(70)}}, resumed: at(70), byMain: true,
		stretched: [2]stretching{
			{stretches: []Stretch{{Account: "work", From: at(0)}, {From: at(70)}}, open: true},
			{stretches: []Stretch{{Account: "work", From: at(0)}}},
		},
		goes:     []SessionModel{{Model: foldOpus, Account: "work"}},
		accounts: []accountSums{{account: "work", from: at(0), to: at(1), requests: 2, spent: spent}},
		kept:     map[string]keptLines{foldOpus: {account: "work", came: "new", kept: "sticky"}},
		totals:   sessionSums{requests: 2, spent: spent, read: 90000, sent: 92010},
		upstream: upstreamTimes{}.with([]time.Time{at(0), at(0).Add(1500 * time.Millisecond), at(70)}),
		flying:   []flight{{request: "r3", at: instantOf(at(70)), end: instantOf(at(71))}},
		points:   &pointing{days: []string{"2026-10-07"}, folded: map[string]int{"2026-10-07": 3}},
	}
}

// What a page's tally carries is all but its turns, agents and times, each
// by a short key, every zero left out: how many turns, agents and times it
// has, and its times in UTC; what its account and its totals spent in
// ledger.Spent's own stored form, never a worth; and its points in their
// own stored form. Its part, folded on none, is all its turns, by their
// indices, a request's end not yet come left out, its agents, and its times
// as nanoseconds, each after the first as how many after the one before:
// its turns first and its times last. Restored, they're the tally, owning
// nothing a copy of it shares, as told leaves one.
func TestAPagesCarriedStateAndPartAreAllItHoldsByShortKeys(t *testing.T) {
	p := pinnedPage()
	carried, err := json.Marshal(p.carried())
	spent := `[{"m":"claude-opus-5-5","g":"us","i":10,"o":300,"c":true}]`
	want := `{"tn":2,"ag":1,"tm":3,"nn":true,"b":true,"f":{"s":1791374400},"l":{"s":1791378600},"fd":[{"f":{"s":1791374460},"t":{"s":1791378600}}],` +
		`"rs":{"s":1791378600},` +
		`"bm":true,"st":[{"s":[{"a":"work","f":{"s":1791374400}},{"f":{"s":1791378600}}],"o":true},{"s":[{"a":"work","f":{"s":1791374400}}]}],` +
		`"g":[{"m":"claude-opus-5-5","a":"work"}],"a":[{"ac":"work","f":{"s":1791374400},"t":{"s":1791374460},"r":2,"sp":` + spent + `}],` +
		`"k":{"claude-opus-5-5":{"a":"work","c":"new","k":"sticky"}},"tt":{"r":2,"sp":` + spent + `,"rd":90000,"s":92010},` +
		`"fl":[{"r":"r3","a":{"s":1791378600},"e":{"s":1791378660}}],"pt":{"d":["2026-10-07"],"f":{"2026-10-07":3}}}`
	if err != nil || string(carried) != want {
		t.Errorf("carried as\n%s, %v, want\n%s", carried, err, want)
	}
	part, err := json.Marshal(p.partOver(nil))
	want = `{"t":[{"p":"p1","s":{"s":1791374400},"l":{"e":{"s":1791374460},"l":true,"s":"tool_use"},"lm":{"e":{"s":1791374460},"l":true,"s":"tool_use"},` +
		`"r":2,"tl":{"Read":2},"rd":90000,"w":2000,"o":300,"sp":` + spent + `,"a":["work"],"ma":["work"]},` +
		`{"i":1,"p":"p2","s":{"s":1791378600},"l":{"e":{"s":1791378660}},"lm":{},"r":2,"pn":true,"mp":true}],"a":{"a1":"p2"},` +
		`"s":{"n":[1791374400000000000,1500000000,4198500000000]}}`
	if err != nil || string(part) != want {
		t.Errorf("its part, folded on none, put as\n%s, %v, want\n%s", part, err, want)
	}
	restored, err := (&storedSession{Date: "2026-10-07", Page: carried}).pageOf([]json.RawMessage{part})
	if err != nil {
		t.Fatalf("restored with error %v", err)
	}
	if !samePage(restored, p) || restored.own != nil || restored.ownMaps {
		t.Errorf("restored as\n%s, owning %v, its maps %t; want\n%s, owning nothing", pageSaid(restored), restored.own, restored.ownMaps, pageSaid(p))
	}
}

// A page's part over the tally its fold rested on is what the fold added:
// the turns it took a request into, as a copy told on makes each its own,
// and those it began; the agents first seen; and the times after the
// base's. Laid over the base's own part, it restores the tally the fold
// made. Its times beyond the years nanoseconds since the epoch hold are
// each an instant, as the tally holds them then, the base's passed over.
func TestAPagesPartIsWhatItsFoldAddedToTheTallyItRestedOn(t *testing.T) {
	base := pinnedPage()
	at := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	after := *base
	after.turns = slices.Clone(base.turns)
	after.turns[1] = base.turns[1].cloned()
	after.turns[1].requests++
	after.turns = append(after.turns, &turnTally{prompt: "p3", started: at, tools: map[string]int{}, accounts: []string{}, mainAccounts: []string{}})
	after.prompts = map[string]int{"p1": 0, "p2": 1, "p3": 2}
	after.agents = map[string]string{"a1": "p2", "a2": "p3"}
	after.upstream = base.upstream.with([]time.Time{at, at.Add(2 * time.Second)})
	part, err := json.Marshal(after.partOver(base))
	want := `{"t":[{"i":1,"p":"p2","s":{"s":1791378600},"l":{"e":{"s":1791378660}},"lm":{},"r":3,"pn":true,"mp":true},{"i":2,"p":"p3","s":{"s":1791450000}}],` +
		`"a":{"a2":"p3"},"s":{"n":[1791450000000000000,2000000000]}}`
	if err != nil || string(part) != want {
		t.Errorf("its part over its base put as\n%s, %v, want\n%s", part, err, want)
	}
	carried, err := json.Marshal(after.carried())
	if err != nil {
		t.Fatal(err)
	}
	root, err := json.Marshal(base.partOver(nil))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := (&storedSession{Date: "2026-10-08", Page: carried}).pageOf([]json.RawMessage{root, part})
	if err != nil || !samePage(restored, &after) {
		t.Errorf("laid over its base's part, restored with error %v as\n%s\nwant\n%s", err, pageSaid(restored), pageSaid(&after))
	}
	far := time.Date(2400, 3, 1, 12, 0, 0, 0, time.UTC)
	after.upstream = base.upstream.with([]time.Time{far})
	times, err := json.Marshal(after.partOver(base).Times)
	if want := `{"f":[{"s":13574692800}]}`; err != nil || string(times) != want {
		t.Errorf("its times past the nanoseconds' years put as %s, %v, want %s", times, err, want)
	}
}

// A page's times that are the zero time, as a page not begun holds, or one
// never come back after a gap, are left out of what it carries, and read
// back as the zero time; one at the epoch, which an instant of no seconds
// stores, is carried, and read back as the epoch, exactly.
func TestAPagesZeroTimesAreLeftOutAndTheEpochKept(t *testing.T) {
	epoch := time.Unix(0, 0).UTC()
	begun := newPageTally()
	begun.begun, begun.first, begun.last = true, epoch, epoch
	tests := []struct {
		name string
		page *pageTally
		want string
	}{
		{name: "not begun", page: newPageTally(), want: `{"pt":{}}`},
		{name: "begun at the epoch", page: begun, want: `{"b":true,"f":{},"l":{},"pt":{}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.page.points = &pointing{}
			carried, err := json.Marshal(tt.page.carried())
			if err != nil || string(carried) != tt.want {
				t.Errorf("carried as %s, %v, want %s", carried, err, tt.want)
			}
			restored, err := (&storedSession{Date: "2026-10-07", Page: carried}).pageOf([]json.RawMessage{json.RawMessage(`{}`)})
			if err != nil || !samePage(restored, tt.page) {
				t.Errorf("restored with error %v as\n%s\nwant\n%s", err, pageSaid(restored), pageSaid(tt.page))
			}
		})
	}
}

// A page's chain of parts is laid over what its state carries oldest first:
// a turn as the newest part holding it has it, every agent any part first
// saw, and every part's times one after another. It's refused, for the page
// to be read whole, where the state carries no page, or its page or a part
// isn't one; it counts more turns than its parts hold, or fewer than none,
// however many, before room is made for them; a part holds a turn beyond
// those the state counts, or leaves one below in none; two turns serve one
// prompt; its parts give other than as many agents or times as it counts,
// as a chain without its whole part at its root does; a part holds its
// times both as nanoseconds and as instants; or its points won't restore.
func TestAPagesChainOfPartsIsLaidOverWhatItsStateCarries(t *testing.T) {
	tests := []struct {
		name    string
		page    string
		parts   []string
		turns   []int
		agents  []string
		times   []int64
		refused bool
	}{
		{
			name: "a turn as its newest part has it", page: `{"tn":2}`,
			parts: []string{`{"t":[{"p":"p1","r":1},{"i":1,"p":"p2","r":1}]}`, `{"t":[{"p":"p1","r":3}]}`}, turns: []int{3, 1},
		},
		{name: "every agent first seen", page: `{"ag":2}`, parts: []string{`{"a":{"a1":"p1"}}`, `{}`, `{"a":{"a2":"p1"}}`}, agents: []string{"a1", "a2"}},
		{
			name: "every part's times in turn", page: `{"tm":4}`, parts: []string{`{"s":{"n":[1000,5]}}`, `{"s":{"n":[2000]}}`, `{"s":{"f":[{"n":3000}]}}`},
			times: []int64{1000, 1005, 2000, 3000},
		},
		{name: "no page", parts: []string{`{}`}, refused: true},
		{name: "a page that isn't one", page: `{"tn":"two"}`, parts: []string{`{}`}, refused: true},
		{name: "a part that isn't one", page: `{}`, parts: []string{`{"t":{}}`}, refused: true},
		{name: "more turns than any room", page: `{"tn":9000000000000000000}`, parts: []string{`{}`}, refused: true},
		{name: "more turns than its parts hold", page: `{"tn":2}`, parts: []string{`{"t":[{"p":"p1"}]}`}, refused: true},
		{name: "fewer turns than none", page: `{"tn":-1}`, parts: []string{`{}`}, refused: true},
		{name: "a turn below the count in no part", page: `{"tn":2}`, parts: []string{`{"t":[{"p":"p1"}]}`, `{"t":[{"p":"p1"}]}`}, refused: true},
		{name: "a turn beyond the count", page: `{"tn":1}`, parts: []string{`{"t":[{"p":"p1"},{"i":1,"p":"p2"}]}`}, refused: true},
		{name: "more agents than its parts give", page: `{"ag":2}`, parts: []string{`{"a":{"a1":"p1"}}`}, refused: true},
		{name: "fewer agents than its parts give", page: `{"ag":1}`, parts: []string{`{"a":{"a1":"p1"}}`, `{"a":{"a2":"p1"}}`}, refused: true},
		{name: "more times than its parts give", page: `{"tm":3}`, parts: []string{`{"s":{"n":[1000,5]}}`}, refused: true},
		{
			name: "a chain without its whole part, a later one holding every turn", page: `{"tn":1,"ag":1,"tm":3}`,
			parts: []string{`{"t":[{"p":"p1","r":2}],"s":{"n":[2000]}}`}, refused: true,
		},
		{name: "a turn before the first", page: `{"tn":1}`, parts: []string{`{"t":[{"i":-1,"p":"p1"}]}`}, refused: true},
		{name: "two turns serving one prompt", page: `{"tn":2}`, parts: []string{`{"t":[{"p":"p1"},{"i":1,"p":"p1"}]}`}, refused: true},
		{name: "times as nanoseconds and instants at once", page: `{}`, parts: []string{`{"s":{"n":[1000],"f":[{"n":2000}]}}`}, refused: true},
		{name: "points whose days aren't dates", page: `{"pt":{"d":["7 October"]}}`, parts: []string{`{}`}, refused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := storedSession{Date: "2026-10-07"}
			if tt.page != "" {
				st.Page = json.RawMessage(tt.page)
			}
			var parts []json.RawMessage
			for _, part := range tt.parts {
				parts = append(parts, json.RawMessage(part))
			}
			p, err := st.pageOf(parts)
			if (err != nil) != tt.refused {
				t.Fatalf("restored with error %v, want it refused %t", err, tt.refused)
			}
			if tt.refused {
				return
			}
			var turns []int
			for _, turn := range p.turns {
				turns = append(turns, turn.requests)
			}
			var times []int64
			for _, at := range timesOf(p.upstream) {
				times = append(times, at.UnixNano())
			}
			if agents := slices.Sorted(maps.Keys(p.agents)); !slices.Equal(turns, tt.turns) || !slices.Equal(agents, tt.agents) || !slices.Equal(times, tt.times) {
				t.Errorf("restored with turns of %v requests, the agents %v, the times %v; want %v, %v, %v", turns, agents, times, tt.turns, tt.agents, tt.times)
			}
		})
	}
}

// A session whose page JSON can't carry, its points holding a share that
// isn't a finite number, has its state stored without it, its story and
// counts all the same, for its page to be read whole: folded, it gives no
// part, and its state, restored, holds no page, and won't restore paged.
func TestASessionsPageJSONCantCarryIsLeftOutOfItsState(t *testing.T) {
	day, end := digestDay()
	held := readOf(arrivedBefore(generatedSession(2), end), nil, ledger.Pricing, day, instantOf(end), true)
	held.page = held.page.pointed(&pointing{done: map[string]map[string]float64{"work": {"5h": math.NaN()}}})
	f, err := held.folded(nil)
	if err != nil || f.Part != nil {
		t.Fatalf("folded with error %v, the part %s; want it folded, no part", err, f.Part)
	}
	var st storedSession
	if err := json.Unmarshal(f.State, &st); err != nil || st.Page != nil || st.Counts[day.Date] == 0 {
		t.Fatalf("its state, %s, reads with error %v; want it read, its counts and no page", f.State, err)
	}
	next := Today(end)
	if s, err := st.restored(ledger.Pricing, next); err != nil || s.page != nil {
		t.Errorf("its state restored with error %v, its page tallied %t; want it restored untallied", err, err == nil && s.page != nil)
	}
	if _, err := st.restoredPaged(ledger.Pricing, next, nil); err == nil {
		t.Error("its state restored paged, want it refused")
	}
}

// A session's state restored for its story alone, as every reader but the
// page's restores it, never reads what its page carries: a page that won't
// restore leaves it restored all the same.
func TestASessionsStateRestoredForItsStoryNeverReadsItsPage(t *testing.T) {
	day, end := digestDay()
	st, err := readOf(arrivedBefore(generatedSession(2), end), nil, ledger.Pricing, day, instantOf(end), false).stored()
	if err != nil {
		t.Fatal(err)
	}
	st.Page = json.RawMessage(`{"tn":"two"}`)
	if _, err := st.restored(ledger.Pricing, Today(end)); err != nil {
		t.Errorf("restored with error %v, want it restored", err)
	}
	if _, err := st.restoredPaged(ledger.Pricing, Today(end), []json.RawMessage{json.RawMessage(`{}`)}); err == nil {
		t.Error("restored paged, want it refused")
	}
}
