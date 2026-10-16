package views

import (
	"encoding/json"
	"fmt"
	"iter"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// Session reads as a Follower does: the session's lines filed by now, the
// latest arrived first.
func (l filedLedger) Session(id string) iter.Seq[ledger.Held] {
	return func(yield func(ledger.Held) bool) {
		filed := linesInOrder(l.filed())
		for _, line := range slices.Backward(filed) {
			if line.Session == id && !yield(ledger.Held{Line: line}) {
				return
			}
		}
	}
}

// joinsLate is how long the stream is deaf to a run's long request told of
// late, as a dashboard started as it runs hears of it: by then it arrived
// further back than the horizon the session is sealed behind.
const joinsLate = 90 * time.Minute

// pagedRun is a generated session as a page's run of feeds is given it: as
// generatedRun makes it, each of its requests of its own conversation, or
// unclassed, serving the prompt before or a new one, its side requests and
// compactions the prompt then, a subagent's naming any prompt so far, as
// only its first request's counts, and now and then one naming none; each
// answer of its own conversation stopping to call a tool or ending its
// turn; and of one in five, the long request the stream tells of told late,
// as late says.
func pagedRun(seed uint64) (lines []ledger.Line, late string) {
	lines = generatedRun(seed)
	r := rand.New(rand.NewPCG(seed, 8))
	var prompts []string
	for _, i := range arrivalOrder(lines) {
		l := &lines[i]
		switch {
		case l.Class == ledger.ClassMain || l.Class == "":
			if len(prompts) == 0 || r.IntN(4) == 0 {
				prompts = append(prompts, fmt.Sprintf("p%d", len(prompts)+1))
			}
			l.Prompt = prompts[len(prompts)-1]
			l.Answer.Stop = []string{"tool_use", "end_turn", "pause_turn"}[r.IntN(3)]
			l.Answer.Tools = [][]string{nil, {"Bash"}, {"Read", "Edit"}}[r.IntN(3)]
		case l.AgentID != "" && len(prompts) > 0:
			l.Prompt = prompts[r.IntN(len(prompts))]
		case len(prompts) > 0:
			l.Prompt = prompts[len(prompts)-1]
		}
		if r.IntN(15) == 0 {
			l.Prompt = ""
		}
	}
	if seed%5 == 1 {
		long := slices.IndexFunc(lines, func(l ledger.Line) bool { return l.TotalMS == (2 * time.Hour).Milliseconds() })
		late = lines[long].Request
	}
	return lines, late
}

// arrivalOrder are the indices of lines in the order they arrived.
func arrivalOrder(lines []ledger.Line) []int {
	order := make([]int, len(lines))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return lines[a].At.Compare(lines[b].At) })
	return order
}

// paging is a run of feeds of the store over a generated session whose page
// is open, as the watch feeds it a read of the ledger's tail at each look,
// the page taking in what the read gained and its session as the store
// holds it: the session's lines, as filed, and the request the stream
// tells of late; the store and the page; when it was last fed; the turn
// open, 0 till one is; and the hard moments the runs met.
type paging struct {
	t      *testing.T
	seed   uint64
	lines  []ledger.Line
	late   string
	prices ledger.Table
	store  HeldSessions
	page   HeldPage
	fed    time.Time
	open   int
	met    map[string]bool
}

// streamAt is what the stream tells at now of the session's requests: each
// in flight, sent moments after it arrived, and each ended since the last
// look, as the stream holds a request till a read of the ledger's tail
// reads its line; but the one it never tells of, and the one it tells of
// late, untold for joinsLate after it arrived.
func (p *paging) streamAt(now time.Time) Stream {
	s := Stream{Live: true, Requests: make(map[string]Told)}
	for _, l := range p.lines {
		if l.At.After(now) || !ended(&l).After(p.fed) || l.Request == unseen || l.Request == p.late && now.Sub(l.At) < joinsLate || !l.Counted() {
			continue
		}
		t := Told{
			Request: l.Request, Session: l.Session, Model: l.Model, Account: l.Account, Prompt: l.Prompt, Class: l.Class, AgentID: l.AgentID,
			Sent: l.At.Add(100 * time.Millisecond), Heard: now,
		}
		if !ended(&l).After(now) {
			t.Ended, t.Status, t.Heard = ended(&l), l.Status, ended(&l)
		}
		s.Requests[l.Request] = t
	}
	return s
}

// opened is the page with its turn open, as the watch opens it at now: its
// newest, read through l as live builds the page.
func (p *paging) opened(l LedgerSession, live PageLive) {
	turns := len(p.page.worked(live).tally.turns)
	if turns == 0 {
		return
	}
	p.open = turns
	p.page = p.page.Opening(p.open, live)
	p.page = p.page.Turned(p.page.ReadTurn(l, live))
}

// checkTurn holds the turn open, opened at now, between feeds, to the turn
// of the page read whole at now, of every line l files, as the stream
// tells: each of its requests a row once, those whose lines the store has
// yet to be fed among them; and those of no subagent, in the same order,
// but those filed since the feed that the stream never told of, which the
// page learns of only at the next, as it does a subagent's turn, the one
// its first request names, where that's such a request.
func (p *paging) checkTurn(now time.Time, l filedLedger, stream Stream) {
	p.t.Helper()
	if p.page.turn == nil {
		return
	}
	filed := l.filed()
	whole := HoldPage("s", SessionsRead{day: Today(now), lines: map[string][]ledger.Line{"s": filed}}, p.prices, 0, now)
	whole.turn = &turnLines{prompt: p.page.turn.prompt}
	whole = whole.Turned(HeldTurn{prompt: p.page.turn.prompt, lines: filed})
	live := PageLive{Stream: stream, Now: now}
	fed := filedLedger{lines: p.lines, now: p.fed}.filed()
	unknown := make(map[string]bool)
	for _, l := range filed {
		_, told := stream.Requests[l.Request]
		unknown[l.Request] = l.AgentID != "" || !told && !slices.ContainsFunc(fed, func(f ledger.Line) bool { return sameLine(&f, &l) })
	}
	rowsOf := func(h HeldPage) (all, main []string) {
		w := h.worked(live)
		lines, _ := w.turnRequests(1)
		for _, r := range lines {
			all = append(all, r.Row.Request)
			if !unknown[r.Row.Request] {
				main = append(main, r.Row.Request)
			}
			if r.line != nil && !w.rec.linedAt(r.line.Request, r.line.At) {
				p.met["a turn opened between feeds, a line of it filed since"] = true
			}
		}
		return all, main
	}
	all, got := rowsOf(p.page)
	_, want := rowsOf(whole)
	if len(slices.Compact(slices.Sorted(slices.Values(all)))) != len(all) || !slices.Equal(got, want) {
		p.t.Fatalf("session %d's turn, opened at %v between feeds, has the rows %v, of those it knows of %v; want each once, and %v, as the page read whole has them",
			p.seed, now, all, got, want)
	}
}

// look feeds the store a read of the ledger's tail at now, as the watch
// does with the page open: what the ledger filed since the last look,
// today's, and each earlier day that gained a line read again whole; the
// page taking each in, as Take has it, its turn open too, as TurnOf has it,
// then its session as the store holds it, as Held has it. The page is held
// to the page of every line filed,
// held whole, its turn open of every line of the session, as check has it.
func (p *paging) look(now time.Time) {
	p.t.Helper()
	today := Today(now)
	l := filedLedger{lines: p.lines, now: now}
	was := filedLedger{lines: p.lines, now: p.fed}.filed()
	var gained []ledger.Held
	days := make(map[string][]ledger.Held)
	for _, line := range l.filed() {
		if slices.ContainsFunc(was, func(w ledger.Line) bool { return sameLine(&w, &line) }) {
			continue
		}
		if date := dayfile.DateOf(line.At); date < today.Date {
			days[date] = append(days[date], ledger.Held{Line: line})
		} else {
			gained = append(gained, ledger.Held{Line: line})
		}
	}
	for date := range days {
		days[date] = l.dayOf(date)
	}
	stream := p.streamAt(now)
	before := p.store.sessions["s"]
	store, gains := p.store.Fed(l, Took(gained, days), SessionsWanted{}.Paging(p.page, stream), now)
	p.note(now, before, stream, gains["s"].whole)
	p.store, p.fed = store, now
	turn, opened := p.page.turn, p.page
	read := [][]ledger.Held{gained}
	for date, lines := range days {
		p.page, read = p.page.Take(date, lines), append(read, lines)
	}
	p.page, _ = p.page.Take(today.Date, gained).TurnOf(opened, read...).Held(p.store)
	if turn != nil && len(p.page.turn.lines) > len(turn.lines) {
		p.met["its turn open taking in lines as they come"] = true
	}
	p.check(now, l, stream)
}

// dayOf is every line l files on the day with the given date, as a day
// read again whole gives them.
func (l filedLedger) dayOf(date string) []ledger.Held {
	var day []ledger.Held
	for _, line := range l.filed() {
		if dayfile.DateOf(line.At) == date {
			day = append(day, ledger.Held{Line: line})
		}
	}
	return day
}

// note notes the hard moments a feed at now meets, the session held before
// it as before, the stream telling what stream does, whole saying whether
// it read the session whole: a request in flight among its lines; a
// subagent's request in a turn other than its prompt's; an idle gap folded;
// its stretches told by its own conversation's once a request of it came
// after others; midnight; and a session read whole as the new day's prices
// change what its page tallied, or as the stream told late of a request
// that arrived behind its horizon.
func (p *paging) note(now time.Time, before *heldSession, stream Stream, whole bool) {
	if len(stream.Requests) > 0 {
		p.met["a request in flight"] = true
	}
	if before == nil {
		return
	}
	if t := before.page; t.byMain && !t.stretched[1].open && len(t.stretched[0].stretches) > len(t.stretched[1].stretches) {
		p.met["stretches told by its own conversation after others"] = true
	}
	if len(before.page.folds) > 0 {
		p.met["an idle gap folded"] = true
	}
	for _, line := range p.lines {
		if first, ok := before.page.agents[line.AgentID]; ok && line.Prompt != "" && line.Prompt != first {
			p.met["a subagent's request naming another turn's prompt"] = true
		}
	}
	if !whole {
		if before.page.totals.requests > 0 {
			p.met["its page fed on of requests sealed"] = true
		}
		return
	}
	if Today(now).Date > before.day().Date {
		p.met["midnight"] = true
	}
	if Today(now).Date > before.day().Date && !before.page.pricedAlike(p.prices, before.day().Date, Today(now).Date) {
		p.met["midnight, its page priced again whole"] = true
	}
	for _, pins := range pinsOf(stream) {
		if slices.ContainsFunc(pins, func(pin time.Time) bool { return instantOf(pin.Add(-arrivesWithin)).compare(before.horizon) < 0 }) {
			p.met["a stream row joining late, read whole"] = true
		}
	}
}

// check holds the page, fed at now, to the page of every line l files, held
// whole, as the stream tells, its turn open of every line of the session,
// the prompt the page read it for: page, story, turn's requests and until
// when it stands, alike.
func (p *paging) check(now time.Time, l filedLedger, stream Stream) {
	p.t.Helper()
	filed := l.filed()
	whole := HoldPage("s", SessionsRead{day: Today(now), lines: map[string][]ledger.Line{"s": filed}}, p.prices, 0, now)
	if p.page.turn != nil {
		whole.turn = &turnLines{prompt: p.page.turn.prompt}
		whole = whole.Turned(HeldTurn{prompt: p.page.turn.prompt, lines: filed})
	}
	live := PageLive{Stream: stream, Now: now}
	got, gotOK := p.page.Build(live)
	want, wantOK := whole.Build(live)
	if gotOK != wantOK || !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		p.t.Fatalf("session %d's page, fed at %v, is\n%s\nwant it built whole\n%s", p.seed, now, gotJSON, wantJSON)
	}
	if got.RequestsLoading {
		p.t.Fatalf("session %d's page, fed at %v, reads its turn open's requests still, want them read", p.seed, now)
	}
}

// The page of a session, its session held in the store fed a read of the
// ledger's tail at each look, its requests sealed behind its horizon and
// tallied once, is after each the page built whole of every line filed, as
// the stream tells: of generated sessions with requests in flight among its
// lines, subagents counting in their first request's turn across others,
// idle gaps folded, its stretches told by its own conversation once one
// comes after others, across midnight, a model's prices changing on the
// new day, and a stream row joining late, behind its horizon. The turn it
// opens, read from the ledger as it opens, its lines taken in as they come,
// is the turn's every request.
func TestThePageFedLookByLookIsThePageBuiltWhole(t *testing.T) {
	prices := changingPrices()
	met := make(map[string]bool)
	for seed := range uint64(120) {
		lines, late := pagedRun(seed)
		r := rand.New(rand.NewPCG(seed, 9))
		at := lines[0].At.Add(time.Duration(r.IntN(30)) * time.Minute)
		read := filedLedger{lines: lines, now: at}
		p := &paging{t: t, seed: seed, lines: lines, late: late, prices: prices, met: met, fed: at}
		p.page = ReadPage(read, "s", p.streamAt(at), prices, 0, at)
		p.store = HoldSessions(prices).With(p.page.Found())
		p.page = p.page.Joined(p.store)
		opens := r.IntN(len(lines))
		for i := 0; i < len(lines); i += 1 + r.IntN(4) {
			now := later(instantOf(at), instantOf(ended(&lines[i]))).time().Add(time.Duration(r.IntN(5000)) * time.Millisecond)
			if between := at.Add(now.Sub(at) / 2); i >= opens && p.open == 0 {
				p.opened(filedLedger{lines: lines, now: between}, PageLive{Stream: p.streamAt(between), Now: between})
				p.checkTurn(between, filedLedger{lines: lines, now: between}, p.streamAt(between))
			}
			p.look(now)
			at = now
		}
		p.look(ended(&lines[len(lines)-1]).Add(3 * time.Hour))
	}
	for _, moment := range []string{
		"a request in flight", "a subagent's request naming another turn's prompt", "an idle gap folded",
		"stretches told by its own conversation after others", "midnight", "midnight, its page priced again whole",
		"a stream row joining late, read whole", "its page fed on of requests sealed", "its turn open taking in lines as they come",
		"a turn opened between feeds, a line of it filed since",
	} {
		if !met[moment] {
			t.Errorf("no generated run meets %s", moment)
		}
	}
}

// The store holding a session untallied, as the List holds it, takes it as
// the page opened on it holds it, read whole as it opened, reading none of
// it again, and the page holds it as the store does from then on; once the
// page is no longer wanted, the store keeps the session as the List wants
// it, letting go of its page's tally. A store not holding it reads it whole.
func TestTheStoreTakesThePagesSessionInPlaceOfOneUntallied(t *testing.T) {
	lines := generatedSession(3)
	now := ended(&lines[len(lines)-1]).Add(time.Minute)
	l := filedLedger{lines: lines, now: now}
	listed, _ := HoldSessions(ledger.Pricing).Fed(l, Taken{}, SessionsWanted{List: true}, now)
	if s := listed.sessions["s"]; s == nil || s.page != nil {
		t.Fatalf("the List's store holds s %t, tallied %t; want it held untallied", s != nil, s != nil && s.page != nil)
	}
	page := ReadPage(l, "s", Stream{}, ledger.Pricing, 0, now)
	later := now.Add(time.Second)
	paged, _ := listed.Fed(l, Taken{}, SessionsWanted{List: true}.Paging(page, Stream{}), later)
	if paged.Holding().Read > 0 || paged.sessions["s"].page == nil {
		t.Errorf("wanting the page, the store read %d requests whole, holding s tallied %t; want none read, s tallied as the page held it",
			paged.Holding().Read, paged.sessions["s"].page != nil)
	}
	if held, changed := page.Held(paged); !changed || held.record != paged.sessions["s"] {
		t.Errorf("the page took its session from the store %t, holding the store's %t; want it taken, the store's", changed, held.record == paged.sessions["s"])
	}
	unpaged, _ := paged.Fed(l, Taken{}, SessionsWanted{List: true}, later.Add(time.Second))
	if s := unpaged.sessions["s"]; s == nil || s.page != nil {
		t.Errorf("the page no longer wanted, the store holds s %t, tallied %t; want it held for the List, untallied", s != nil, s != nil && s.page != nil)
	}
	fresh, _ := HoldSessions(ledger.Pricing).Fed(l, Taken{}, SessionsWanted{}.Paging(page, Stream{}), later)
	if fresh.Holding().Read == 0 || fresh.sessions["s"].page == nil {
		t.Errorf("wanting the page of a session it doesn't hold, the store read %d requests whole, tallied %t; want it read whole, tallied",
			fresh.Holding().Read, fresh.sessions["s"] != nil && fresh.sessions["s"].page != nil)
	}
}

// A page's folds take requests that arrived to the millisecond together in
// the one order, by when each ended, then by its id, however its lines were
// read: its turns, the accounts it ran on, why each model goes where it
// does, and where each went last, alike.
func TestAPageTakesRequestsTiedToTheMillisecondInTheOneOrder(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local).UTC()
	line := func(id, prompt, account, reason string, totalMS int64) ledger.Line {
		return ledger.Line{
			Request: id, At: at, Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: account, Reason: reason,
			Status: 200, Attempts: 1, TotalMS: totalMS, Class: ledger.ClassMain, Prompt: prompt, Usage: json.RawMessage(foldUsages[2]),
		}
	}
	tied := []ledger.Line{
		line("b", "p2", "side", "moved: work hit its limit", 1000),
		line("a", "p1", "work", "sticky", 1000),
		line("c", "p3", "personal", "new", 500),
	}
	now := at.Add(time.Minute)
	pageOf := func(lines []ledger.Line) SessionPage {
		page, _ := HoldPage("s", SessionsRead{day: Today(now), lines: map[string][]ledger.Line{"s": lines}}, ledger.Pricing, 0, now).Page(PageLive{Now: now})
		return page
	}
	want := pageOf(tied)
	if got := []string{want.Accounts[0].Account, want.Accounts[1].Account, want.Accounts[2].Account}; !slices.Equal(got, []string{"personal", "work", "side"}) {
		t.Fatalf("the tied requests' accounts are %v, want them by end, then id: personal, work, side", got)
	}
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}, {1, 0, 2}, {1, 2, 0}} {
		given := make([]ledger.Line, len(order))
		for i, j := range order {
			given[i] = tied[j]
		}
		if got := pageOf(given); !reflect.DeepEqual(got, want) {
			t.Errorf("given in the order %v, the page is\n%+v\nwant it as given in any other\n%+v", order, got, want)
		}
	}
}

// A page's story tells when each request went upstream exactly, whatever
// the year it arrived in, as nanoseconds since the epoch alone don't hold
// before 1678 or after 2262.
func TestAPagesStoryTellsWhenARequestWentUpstreamWhateverItsYear(t *testing.T) {
	long, far := time.Date(1500, 3, 1, 12, 0, 0, 0, time.UTC), time.Date(2400, 3, 1, 12, 0, 0, 0, time.UTC)
	line := func(id string, at time.Time) ledger.Line {
		return ledger.Line{
			Request: id, At: at, Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: "work", Reason: "sticky",
			Status: 200, Attempts: 1, TotalMS: 1000, Class: ledger.ClassMain, Prompt: "p1", Usage: json.RawMessage(foldUsages[2]),
		}
	}
	now := far.Add(time.Minute)
	page := HoldPage("s", SessionsRead{day: Today(now), lines: map[string][]ledger.Line{"s": {line("a", long), line("b", far)}}}, ledger.Pricing, 0, now)
	built, ok := page.Build(PageLive{Now: now})
	if want := []time.Time{long, far}; !ok || !slices.EqualFunc(built.Story.Sent, want, time.Time.Equal) {
		t.Errorf("the story tells its requests went upstream at %v, want %v", built.Story.Sent, want)
	}
}

// A turn open holds, of the lines the ledger gains as it's open, only those
// that may be its own: its session's requests naming its prompt, and its
// subagents', or those of one whose turn its page doesn't know yet; never
// another turn's, another session's, nor one of a subagent of another turn.
func TestATurnOpenHoldsOnlyTheLinesThatMayBeItsOwn(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	line := func(id, prompt, agent, session string, minutes int) ledger.Line {
		return ledger.Line{
			Request: id, At: at.Add(time.Duration(minutes) * time.Minute), Kind: ledger.KindMessage, Session: session, Model: foldOpus, Account: "work",
			Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 1000, Class: ledger.ClassMain, Prompt: prompt, AgentID: agent,
		}
	}
	read := []ledger.Line{line("a", "p1", "", "s", 0), line("b", "p2", "", "s", 1), line("c", "p2", "other-agent", "s", 2)}
	now := at.Add(3 * time.Hour)
	live := PageLive{Now: now}
	page := ReadPage(filedLedger{lines: read, now: now}, "s", Stream{}, ledger.Pricing, 0, now).Opening(1, live)
	var gained []ledger.Held
	for _, l := range []ledger.Line{
		line("d", "p1", "", "s", 150), line("e", "p2", "", "s", 151), line("f", "p1", "", "elsewhere", 152),
		line("g", "p1", "other-agent", "s", 153), line("h", "p2", "new-agent", "s", 154),
	} {
		gained = append(gained, ledger.Held{Line: l})
	}
	page = page.TurnOf(page, gained)
	var held []string
	for _, l := range page.turn.lines {
		held = append(held, l.Request)
	}
	if want := []string{"d", "h"}; !slices.Equal(held, want) {
		t.Errorf("the turn open holds the lines %v, want %v", held, want)
	}
}

// A turn open holds a request's line once, however many times the lines it
// takes in at once give it.
func TestATurnOpenHoldsALineGivenTwiceAtOnceOnce(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	l := ledger.Line{Request: "a", At: at, Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: "work", Status: 200, Attempts: 1, Prompt: "p1"}
	now := at.Add(time.Hour)
	live := PageLive{Now: now}
	page := ReadPage(filedLedger{lines: []ledger.Line{l}, now: now}, "s", Stream{}, ledger.Pricing, 0, now).Opening(1, live)
	page = page.TurnOf(page, []ledger.Held{{Line: l}, {Line: l}})
	if n := len(page.turn.lines); n != 1 {
		t.Errorf("the turn open holds the line %d times, want once", n)
	}
}
