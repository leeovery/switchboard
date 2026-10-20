package views

import (
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// Session reads as a Follower does: the session's lines filed by now, the
// latest arrived first, of the days ledger.Spanning gives of the spans given.
func (l filedLedger) Session(id string, within ...ledger.Span) iter.Seq[ledger.Held] {
	return func(yield func(ledger.Held) bool) {
		filed, spanned := linesInOrder(l.filed()), ledger.Spanning(within)
		for _, line := range slices.Backward(filed) {
			if line.Session == id && spanned(dayfile.DateOf(line.At)) && !yield(ledger.Held{Line: line}) {
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
// turn; of one in five, the long request the stream tells of told late, as
// late says; of one in seven, a request of its own two days after the day
// of its last, beginning another run of its days, and of another, one at
// two in the morning of the day after, joining its run once its points
// sealed past the day's start. Among them, other sessions' requests,
// on its accounts and another, as othersOf makes them, from the day before
// its first; and every answer reading its account's windows, as limitedOf
// has them. The lines are in the order the ledger files them.
func pagedRun(seed uint64) (lines []ledger.Line, late string) {
	lines = promptedRun(seed)
	if seed%5 == 1 {
		long := slices.IndexFunc(lines, func(l ledger.Line) bool { return l.TotalMS == (2 * time.Hour).Milliseconds() })
		late = lines[long].Request
	}
	r := rand.New(rand.NewPCG(seed, 10))
	if after, ok := map[uint64]time.Duration{3: 48 * time.Hour, 5: 26 * time.Hour}[seed%7]; ok {
		lines = append(lines, ledger.Line{
			Request: fmt.Sprintf("%08x", r.Uint32()), At: dayfile.DayStart(ended(&lines[len(lines)-1]), 0).Add(after), Kind: ledger.KindMessage, Session: "s",
			Model: foldOpus, Account: "work", Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 3000, Class: ledger.ClassMain, Prompt: "pfar",
			Usage: json.RawMessage(foldUsages[2]),
		})
	}
	var opusReads time.Time
	if seed%4 == 2 {
		opusReads = lines[arrivalOrder(lines)[0]].At.Add(2 * time.Hour)
	}
	lines = append(lines, othersOf(seed, lines)...)
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	for i := range lines {
		limitedOf(&lines[i], r, opusReads)
	}
	return lines, late
}

// promptedRun is a generated session as generatedRun makes it, its
// requests serving prompts as pagedRun has them.
func promptedRun(seed uint64) []ledger.Line {
	lines := generatedRun(seed)
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
	return lines
}

// othersOf are other sessions' requests among a generated session's lines,
// as the seed given makes them: on its accounts and another, from the day
// before its first arrived until two hours after its last, some running
// long, now and then one without an id, as from before requests had them;
// and of one in six, one whose answer's headers came two hours before it
// ended.
func othersOf(seed uint64, lines []ledger.Line) []ledger.Line {
	r := rand.New(rand.NewPCG(seed, 11))
	order := arrivalOrder(lines)
	from, until := lines[order[0]].At.Add(-26*time.Hour), lines[order[len(order)-1]].At.Add(2*time.Hour)
	var others []ledger.Line
	for range 15 + r.IntN(30) {
		l := ledger.Line{
			At: from.Add(time.Duration(r.Int64N(int64(until.Sub(from))))).Truncate(time.Millisecond).UTC(), Request: fmt.Sprintf("o%07x", r.Uint32()),
			Kind: ledger.KindMessage, Session: []string{"o1", "o2", "o3"}[r.IntN(3)], Model: []string{foldOpus, foldHaiku, foldSonnet}[r.IntN(3)],
			Account: []string{"work", "side", "personal"}[r.IntN(3)], Reason: "sticky", Status: 200, Attempts: 1, TotalMS: int64(200 + r.IntN(20_000)),
			Class: ledger.ClassMain, Prompt: "q", Usage: json.RawMessage(foldUsages[2+r.IntN(len(foldUsages)-2)]),
		}
		if r.IntN(6) == 0 {
			l.TotalMS = int64(60_000 + r.IntN(540_000))
		}
		if r.IntN(15) == 0 {
			l.Request = ""
		}
		others = append(others, l)
	}
	if seed%6 == 5 {
		first := int64(1000)
		long := others[0]
		long.Request, long.At, long.TotalMS, long.FirstMS = "olong", from.Add(until.Sub(from)/2).Truncate(time.Millisecond), (2 * time.Hour).Milliseconds(), &first
		others = append(others, long)
	}
	return others
}

// limitedOf has l's answer read its account's windows as they stand when
// its headers came, a third of them before it ended: the five-hour window
// and the week, every model's, each climbing through the window, every
// account from its own use, now and then reading lower, as headers that
// came late read; and Claude Sonnet's own week, read by its answers, and by
// Claude Opus's from opusReads on, where that's given, as a model first
// reads a window only some models share.
func limitedOf(l *ledger.Line, r *rand.Rand, opusReads time.Time) {
	if r.IntN(3) == 0 {
		first := l.TotalMS / int64(2+r.IntN(4))
		l.FirstMS = &first
	}
	read := ended(l)
	if l.FirstMS != nil {
		read = l.At.Add(time.Duration(*l.FirstMS) * time.Millisecond)
	}
	base := map[string]float64{"work": 0, "side": 0.1, "personal": 0.2}[l.Account]
	use := func(through float64) string {
		used := base + 0.7*through
		if r.IntN(8) == 0 {
			used -= 0.05
		}
		return strconv.FormatFloat(max(used, 0), 'f', 4, 64)
	}
	fiveHour := read.Truncate(5 * time.Hour)
	week := time.Date(2026, 10, 5, 10, 0, 0, 0, time.Local)
	weekThrough := float64(read.Sub(week)) / float64(7*24*time.Hour)
	l.Limits = map[string]string{
		"5h-utilization": use(float64(read.Sub(fiveHour)) / float64(5*time.Hour)), "5h-reset": strconv.FormatInt(fiveHour.Add(5*time.Hour).Unix(), 10),
		"7d-utilization": use(weekThrough), "7d-reset": strconv.FormatInt(week.AddDate(0, 0, 7).Unix(), 10),
	}
	if l.Model == foldSonnet || l.Model == foldOpus && !opusReads.IsZero() && read.After(opusReads) {
		l.Limits["7d_sonnet-utilization"], l.Limits["7d_sonnet-reset"] = use(weekThrough/2), strconv.FormatInt(week.AddDate(0, 0, 7).Unix(), 10)
	}
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
	// untold are the requests the stream never tells of: in one run in
	// three, each subagent's first.
	untold map[string]bool
}

// untoldOf are the requests of lines the stream never tells of, as paging's
// untold has them, of the run of the given seed.
func untoldOf(lines []ledger.Line, seed uint64) map[string]bool {
	untold := make(map[string]bool)
	if seed%3 != 2 {
		return untold
	}
	agents := make(map[string]bool)
	for _, i := range arrivalOrder(lines) {
		if l := &lines[i]; l.AgentID != "" && !agents[l.AgentID] {
			agents[l.AgentID], untold[l.Request] = true, true
		}
	}
	return untold
}

// streamAt is what the stream tells at now of the session's requests: each
// in flight, sent moments after it arrived, and each ended since the last
// look, as the stream holds a request till a read of the ledger's tail
// reads its line, but an IdleHour after it ended at most, as the watch lets
// it go by the clock; but the one it never tells of, and the one it tells
// of late, untold for joinsLate after it arrived.
func (p *paging) streamAt(now time.Time) Stream {
	s := Stream{Live: true, Requests: make(map[string]Told)}
	for _, l := range p.lines {
		untold := l.Request == "" || l.Request == unseen || p.untold[l.Request] || l.Request == p.late && now.Sub(l.At) < joinsLate
		if l.At.After(now) || !ended(&l).After(p.fed) || !now.Before(ended(&l).Add(IdleHour)) || untold || !l.Counted() {
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
// tells: row for row, in order, each of the same request, by its line or
// as the stream told of it alike, those whose lines the store has yet to
// be fed among them.
func (p *paging) checkTurn(now time.Time, l filedLedger, stream Stream) {
	p.t.Helper()
	if p.page.turn == nil {
		return
	}
	read := l.readWhole(now)
	p.untoldSince(read.sessions["s"].Lines, stream)
	whole := HoldPage("s", read, p.prices, 0, now)
	whole.turn = &turnLines{prompt: p.page.turn.prompt}
	whole = whole.Turned(HeldTurn{prompt: p.page.turn.prompt, lines: recordsOfLines(read.sessions["s"].Lines)})
	live := PageLive{Stream: stream, Now: now}
	rowsOf := func(h HeldPage) []string {
		w := h.worked(live)
		lines, _ := w.turnRequests(1)
		rows := make([]string, len(lines))
		for i, r := range lines {
			rows[i] = r.Row.Request + " told"
			if r.record != nil {
				rows[i] = r.Row.Request + " lined"
				if !w.rec.linedAt(r.record.request, r.record.arrived()) {
					p.met["a turn opened between feeds, a line of it filed since"] = true
				}
			}
		}
		return rows
	}
	if got, want := rowsOf(p.page), rowsOf(whole); !slices.Equal(got, want) {
		p.t.Fatalf("session %d's turn, opened at %v between feeds, has the rows %v; want %v, as the page read whole has them", p.seed, now, got, want)
	}
}

// untoldSince notes where a line filed, among those given, since the last
// feed, is of a request the stream never told of: a subagent's first.
func (p *paging) untoldSince(filed []ledger.Line, stream Stream) {
	fed := filedLedger{lines: p.lines, now: p.fed}.filed()
	for _, l := range filed {
		_, told := stream.Requests[l.Request]
		if !told && p.untold[l.Request] && !slices.ContainsFunc(fed, func(f ledger.Line) bool { return f.Request == l.Request && f.At.Equal(l.At) }) {
			p.met["a subagent's first request, untold, filed since the feed before a turn opened"] = true
		}
	}
}

// look feeds the store a read of the ledger's tail at now, as the watch
// does with the page open: what the ledger filed since the last look,
// today's, or where again says, every one of today's again, as a read
// afresh gives them, and each earlier day that gained a line read again
// whole; the page rolled on to the day, as On has it, its turn open taking
// the lines in, as TurnOf has it, then its session as the store holds it,
// as Held has it. The page is held to the page of every line filed, held
// whole, its turn open of every line of the session, as check has it; and,
// where the look comes on a later day than the last, so first, as it's
// built before the feed, as checkHeldOn has it.
func (p *paging) look(now time.Time, again bool) {
	p.t.Helper()
	today := Today(now)
	if today.Date > Today(p.fed).Date {
		p.checkHeldOn(now)
	}
	l := filedLedger{lines: p.lines, now: now}
	was := filedLedger{lines: p.lines, now: p.fed}.filed()
	var gained []ledger.Held
	days := make(map[string][]ledger.Held)
	for _, line := range l.filed() {
		fresh := !slices.ContainsFunc(was, func(w ledger.Line) bool { return w.Request == line.Request && w.At.Equal(line.At) })
		switch date := dayfile.DateOf(line.At); {
		case date < today.Date && fresh:
			days[date] = append(days[date], ledger.Held{Line: line})
		case date == today.Date && (fresh || again):
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
	p.notePoints(before, slices.Concat(slices.Concat(slices.Collect(maps.Values(days))...), gained), gains["s"].whole, store.sessions["s"])
	if again && before != nil && before.page.points.folded[today.Date] > 0 {
		p.met["today's lines given again afresh, its points having sealed some"] = true
	}
	p.store, p.fed = store, now
	turn, opened := p.page.turn, p.page
	read := [][]ledger.Held{gained}
	for _, lines := range days {
		read = append(read, lines)
	}
	p.page, _ = p.page.On(today.Date)
	p.page, _ = p.page.TurnOf(opened, read...).Held(p.store)
	if turn != nil && len(p.page.turn.lines) > len(turn.lines) {
		p.met["its turn open taking in lines as they come"] = true
	}
	p.check(now, l, stream)
}

// notePoints notes the hard moments of its points a feed meets, the session
// held before it as before, giving the lines given, whole saying whether it
// read the session whole, the session held after it as after: its points
// sealed across midnight; answers held of a day it made no request on, for
// it to join its days; a run of its days done as a later one began; and the
// session read whole as a line read before what its points sealed and ended
// after came, or as a model first read a window only some models share,
// sealed.
func (p *paging) notePoints(before *heldSession, given []ledger.Held, whole bool, after *heldSession) {
	if before == nil {
		return
	}
	points := before.page.points
	if r := points.run; r != nil && r.first != r.last && r.through.compare(startOf(r.last)) > 0 {
		p.met["its points sealed across midnight"] = true
	}
	for date := range points.held {
		if !points.isDay(date) {
			p.met["answers held of a day it made no request on"] = true
		}
	}
	switch r := after.page.points.run; {
	case whole || points.run == nil || r == nil:
	case r.first != points.run.first:
		p.met["a run of its days done as a later one began"] = true
	case r.last != points.run.last && before.horizon.compare(startOf(r.last)) > 0:
		p.met["a day joining its run once its horizon passed the day's start"] = true
	}
	r := points.run
	if !whole || r == nil {
		return
	}
	for _, h := range given {
		a := answeredOf(&h.Line, "s")
		if !isAnswer(&h.Line) || !r.holds(dayfile.DateOf(h.At)) {
			continue
		}
		if both, either := a.sealedBy(r.through); either && !both && points.held[dayfile.DateOf(h.At)].count(a.key()) == 0 {
			p.met["an answer read before its points sealed and ended after, read whole"] = true
		}
		if !r.takes([]answered{a}) {
			p.met["a model first reading a window only some models share, sealed, read whole"] = true
		}
	}
}

// readAtOnce reports whether two of lines, answers of the same account,
// were read at the same moment, their windows reading otherwise.
func readAtOnce(lines []ledger.Line) bool {
	read := make(map[string]map[string]bool)
	for i := range lines {
		a := answeredOf(&lines[i], "s")
		at := a.account.Value() + " " + a.read.time().String()
		if read[at] == nil {
			read[at] = make(map[string]bool)
		}
		read[at][lines[i].Limits["5h-utilization"]] = true
		if len(read[at]) > 1 {
			return true
		}
	}
	return false
}

// readWhole is a read of every line l files for the page of the session s:
// its lines, and every account's answers, by the date of the day each is
// filed on.
func (l filedLedger) readWhole(now time.Time) SessionsRead {
	filed := l.filed()
	read := linesRead(now, "s", slices.DeleteFunc(slices.Clone(filed), func(line ledger.Line) bool { return line.Session != "s" }))
	for _, line := range filed {
		read.answers.take(dayfile.DateOf(line.At), line)
	}
	return read
}

// linesRead is a read at now of the session with the given id, of its lines
// alone, resting on none, as a ledger holding no digest gives them, and of
// no account's answers.
func linesRead(now time.Time, id string, lines []ledger.Line) SessionsRead {
	read := readOn(nil, Today(now))
	read.sessions = ledger.SessionsWhole(map[string][]ledger.Line{id: lines})
	return read
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
// after others; midnight; and a session read whole as the stream told late
// of a request that arrived behind its horizon.
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
	read := l.readWhole(now)
	whole := HoldPage("s", read, p.prices, 0, now)
	if p.page.turn != nil {
		whole.turn = &turnLines{prompt: p.page.turn.prompt}
		whole = whole.Turned(HeldTurn{prompt: p.page.turn.prompt, lines: recordsOfLines(read.sessions["s"].Lines)})
	}
	live := PageLive{Stream: stream, Now: now}
	got, gotOK := p.page.Build(live)
	want, wantOK := whole.Build(live)
	if gotOK != wantOK || !reflect.DeepEqual(sentAlike(got), sentAlike(want)) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		p.t.Fatalf("session %d's page, fed at %v, is\n%s\nwant it built whole\n%s", p.seed, now, gotJSON, wantJSON)
	}
	if got.RequestsLoading {
		p.t.Fatalf("session %d's page, fed at %v, reads its turn open's requests still, want them read", p.seed, now)
	}
}

// checkHeldOn holds the page, held across midnight and built at now, before
// its store's next feed, to the page built whole at now of every line filed
// by the last feed: priced at once at the new day's prices, as one read
// whole is, where a model its lines asked for costs otherwise on it, its
// tally kept in tokens. A page the stream has to be read whole, as
// joinedLate has it, builds none to hold. Its turn open's requests aren't
// held to the whole's, as they may hold lines filed since the last feed.
func (p *paging) checkHeldOn(now time.Time) {
	p.t.Helper()
	live := PageLive{Stream: p.streamAt(now), Now: now}
	if p.page.readsWhole(live) {
		return
	}
	was, day := Today(p.fed).Date, Today(now).Date
	for model := range p.page.record.page.kept {
		if !p.prices.SamePrices(model, was, day) {
			p.met["midnight, its page held across a change of its prices"] = true
		}
	}
	whole := HoldPage("s", filedLedger{lines: p.lines, now: p.fed}.readWhole(now), p.prices, 0, now)
	got, gotOK := p.page.Page(live)
	want, wantOK := whole.Page(live)
	if gotOK != wantOK || !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		p.t.Fatalf("session %d's page, held from %v to %v, is\n%s\nwant it built whole\n%s", p.seed, p.fed, now, gotJSON, wantJSON)
	}
}

// sentAlike is b, its story's times held as SentAt holds them, so pages
// sealed apart are held alike by the times they tell.
func sentAlike(b BuiltPage) BuiltPage {
	b.Story.Sent = SentAt(slices.Collect(b.Story.Sent.All())...)
	return b
}

// The page of a session, its session held in the store fed a read of the
// ledger's tail at each look, its requests sealed behind its horizon and
// tallied once, is after each the page built whole of every line filed, as
// the stream tells: of generated sessions with requests in flight among its
// lines, subagents counting in their first request's turn across others,
// idle gaps folded, its stretches told by its own conversation once one
// comes after others, across midnight, a model's prices changing on the
// new day, held across it, and a stream row joining late, behind its
// horizon. Past midnight, it's so before the store's feed too, priced at the
// new day's prices at once, as one read whole is. The turn it
// opens, read from the ledger as it opens, its lines taken in as they come,
// is the turn's every request. Its points, sealed as its requests are, are
// those of the page built whole: of its accounts' every request, other
// sessions' among them, from the day before its first; a run of its days
// across midnight, and another begun after a day without; readings read at
// once; today's lines given again afresh; and the page read whole as a model
// first reads a window only some models share, sealed, or as an answer read
// before what its points sealed comes after.
func TestThePageFedLookByLookIsThePageBuiltWhole(t *testing.T) {
	prices := changingPrices()
	met := make(map[string]bool)
	for seed := range uint64(120) {
		lines, late := pagedRun(seed)
		r := rand.New(rand.NewPCG(seed, 9))
		first := slices.IndexFunc(lines, func(l ledger.Line) bool { return l.Session == "s" })
		at := lines[first].At.Add(time.Duration(r.IntN(30)) * time.Minute)
		read := filedLedger{lines: lines, now: at}
		p := &paging{t: t, seed: seed, lines: lines, late: late, prices: prices, met: met, fed: at, untold: untoldOf(lines, seed)}
		p.page = ReadPage(read, "s", p.streamAt(at), prices, 0, at)
		p.store = HoldSessions(prices).With(p.page.Found())
		p.page = p.page.Joined(p.store)
		if readAtOnce(lines) {
			met["readings of an account read at once, reading otherwise"] = true
		}
		opens := first + r.IntN(len(lines)-first)
		for i := first; i < len(lines); i += 1 + r.IntN(4) {
			now := later(instantOf(at), instantOf(ended(&lines[i]))).time().Add(time.Duration(r.IntN(5000)) * time.Millisecond)
			if between := at.Add(now.Sub(at) / 2); i >= opens && p.open == 0 {
				p.opened(filedLedger{lines: lines, now: between}, PageLive{Stream: p.streamAt(between), Now: between})
				p.checkTurn(between, filedLedger{lines: lines, now: between}, p.streamAt(between))
			}
			p.look(now, r.IntN(10) == 0)
			at = now
		}
		p.look(ended(&lines[len(lines)-1]).Add(3*time.Hour), false)
	}
	for _, moment := range []string{
		"a request in flight", "a subagent's request naming another turn's prompt", "an idle gap folded",
		"stretches told by its own conversation after others", "midnight", "midnight, its page held across a change of its prices",
		"a stream row joining late, read whole", "its page fed on of requests sealed", "its turn open taking in lines as they come",
		"a turn opened between feeds, a line of it filed since", "a subagent's first request, untold, filed since the feed before a turn opened",
		"its points sealed across midnight", "answers held of a day it made no request on", "a run of its days done as a later one began",
		"today's lines given again afresh, its points having sealed some", "an answer read before its points sealed and ended after, read whole",
		"a model first reading a window only some models share, sealed, read whole", "readings of an account read at once, reading otherwise",
		"a day joining its run once its horizon passed the day's start",
	} {
		if !met[moment] {
			t.Errorf("no generated run meets %s", moment)
		}
	}
}

// A page's points take each line as often as the ledger's files hold it
// when today's lines are given again, as a read of today afresh gives them
// once its file is replaced: another session's lines without ids, sharing
// each rise with the session's, one sealed and one held open filed twice,
// and lines new since among them, two alike, each taken in. Its points are
// the page's read whole, and nothing is read whole for them.
func TestAPagesPointsTakeLinesWithoutIDsGivenAgainAsOftenAsFiled(t *testing.T) {
	start := time.Date(2026, 10, 7, 10, 0, 0, 0, time.Local)
	reset := strconv.FormatInt(start.Add(5*time.Hour).Unix(), 10)
	first := int64(1000)
	line := func(request, session string, minutes int, took time.Duration, used float64) ledger.Line {
		return ledger.Line{
			Request: request, At: start.Add(time.Duration(minutes) * time.Minute).UTC(), Kind: ledger.KindMessage, Session: session, Model: foldOpus,
			Account: "work", Reason: "sticky", Status: 200, Attempts: 1, FirstMS: &first, TotalMS: took.Milliseconds(), Class: ledger.ClassMain, Prompt: "p1",
			Usage: json.RawMessage(foldUsages[2]), Limits: map[string]string{"5h-utilization": strconv.FormatFloat(used, 'f', 4, 64), "5h-reset": reset},
		}
	}
	var lines []ledger.Line
	for i := range 19 {
		lines = append(lines, line(fmt.Sprintf("s%02d", i), "s", 10*i, 6*time.Minute, 0.10+0.04*float64(i)))
		lines = append(lines, line("", "o", 10*i+5, 2*time.Minute, 0.12+0.04*float64(i)))
	}
	lines = append(lines, lines[3], lines[33])
	alike := line("", "o", 186, 30*time.Second, 0)
	alike.Limits = nil
	lines = append(lines, alike, alike, line("", "o", 187, 2*time.Minute, 0.96))
	opened := start.Add(3*time.Hour + 5*time.Minute)
	page := ReadPage(filedLedger{lines: lines, now: opened}, "s", Stream{}, ledger.Pricing, 0, opened)
	store := HoldSessions(ledger.Pricing).With(page.Found())
	page = page.Joined(store)
	today, twice := Today(opened).Date, answeredOf(&lines[33], "s")
	if points := page.record.page.points; points.folded[today] == 0 || points.held[today].count(twice.key()) != 2 {
		t.Fatalf("the page opened sealed %d answers, holding the one filed twice %d times; want some sealed, it held twice",
			points.folded[today], points.held[today].count(twice.key()))
	}
	now := opened.Add(10 * time.Minute)
	l := filedLedger{lines: lines, now: now}
	var again []ledger.Held
	for _, filed := range l.filed() {
		again = append(again, ledger.Held{Line: filed})
	}
	store, _ = store.Fed(l, Took(again, nil), SessionsWanted{}.Paging(page, Stream{}), now)
	page, _ = page.Held(store)
	got, _ := page.Build(PageLive{Now: now})
	want, _ := HoldPage("s", l.readWhole(now), ledger.Pricing, 0, now).Build(PageLive{Now: now})
	if !reflect.DeepEqual(got.Page.Accounts, want.Page.Accounts) || store.Holding().Read > 0 {
		t.Errorf("given today's lines again, the page's accounts are %+v, reading %d requests whole; want them as read whole, %+v, none read",
			got.Page.Accounts, store.Holding().Read, want.Page.Accounts)
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
		page, _ := HoldPage("s", linesRead(now, "s", lines), ledger.Pricing, 0, now).Page(PageLive{Now: now})
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
// before 1678 or after 2262: of requests sealed and open alike.
func TestAPagesStoryTellsWhenARequestWentUpstreamWhateverItsYear(t *testing.T) {
	long, now, far := time.Date(1500, 3, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC), time.Date(2400, 3, 1, 12, 0, 0, 0, time.UTC)
	line := func(id string, at time.Time) ledger.Line {
		return ledger.Line{
			Request: id, At: at, Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: "work", Reason: "sticky",
			Status: 200, Attempts: 1, TotalMS: 1000, Class: ledger.ClassMain, Prompt: "p1", Usage: json.RawMessage(foldUsages[2]),
		}
	}
	for _, at := range []time.Time{now, far.Add(2 * time.Hour)} {
		lines := []ledger.Line{line("a", long), line("b", now.Add(-2*time.Hour)), line("c", far)}
		page := ReadPage(filedLedger{lines: lines, now: far.Add(2 * time.Hour)}, "s", Stream{}, ledger.Pricing, 0, at)
		built, ok := page.Build(PageLive{Now: at})
		if want, got := []time.Time{long, now.Add(-2 * time.Hour), far}, slices.Collect(built.Story.Sent.All()); !ok || !slices.EqualFunc(got, want, time.Time.Equal) {
			t.Errorf("read at %v, the story tells its requests went upstream at %v, want %v", at, got, want)
		}
	}
}

// A story's times are given oldest first, those sealed and the rest
// between each other however they fall.
func TestAStorysTimesAreGivenOldestFirstSealedOrNot(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	minutes := func(ms ...int) []time.Time {
		var times []time.Time
		for _, m := range ms {
			times = append(times, at.Add(time.Duration(m)*time.Minute))
		}
		return times
	}
	sent := SentTimes{sealed: upstreamTimes{}.with(minutes(1, 3, 3)), open: minutes(0, 2, 3, 4, 5)}
	if got, want := slices.Collect(sent.All()), minutes(0, 1, 2, 3, 3, 3, 4, 5); !slices.EqualFunc(got, want, time.Time.Equal) || sent.Len() != 8 {
		t.Errorf("the story's times are %v, %d of them; want %v", got, sent.Len(), want)
	}
}

// Times held as nanoseconds since the epoch are held so while every one
// falls in the years those hold, and each as an instant once one doesn't,
// exactly, those held before among them.
func TestUpstreamTimesHoldEachExactlyWhateverItsYear(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 123, time.UTC)
	far := time.Date(2400, 3, 1, 12, 0, 0, 0, time.UTC)
	near := upstreamTimes{}.with([]time.Time{at, at.Add(time.Second)})
	if near.far != nil || len(near.nanos) != 2 {
		t.Errorf("times in the nanoseconds' years are held as %d nanoseconds, %d instants; want 2 nanoseconds", len(near.nanos), len(near.far))
	}
	held := near.with([]time.Time{far}).with([]time.Time{far.Add(time.Second)})
	var got []time.Time
	for i := range held.len() {
		got = append(got, held.at(i))
	}
	if want := []time.Time{at, at.Add(time.Second), far, far.Add(time.Second)}; held.nanos != nil || !slices.EqualFunc(got, want, time.Time.Equal) || near.len() != 2 {
		t.Errorf("one beyond the nanoseconds' years held, the times are %v, as nanoseconds %t, those before %d; want %v, as instants, those before 2", got,
			held.nanos != nil, near.len(), want)
	}
}

// A turn open holds, of the lines the ledger gains as it's open, only those
// that may be its own: its session's requests naming its prompt, and every
// subagent's, as a subagent's first request may land after those its page
// knows of; never another turn's, nor another session's.
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
		held = append(held, l.request)
	}
	if want := []string{"d", "g", "h"}; !slices.Equal(held, want) {
		t.Errorf("the turn open holds the lines %v, want %v", held, want)
	}
}

// A turn open holds a request's line with an id once, however many times
// the reads it takes in give it, at once or one after another; one of the
// same id that arrived at another moment is another request's, as an id is
// unique only while the router that gave it runs, and is held too. One
// without an id is held as often as a read gives it beyond those the turn
// holds alike: a line read again, once; one a read gives twice, as the
// ledger's files hold it twice, twice.
func TestATurnOpenHoldsEachLineAsItsKeyKnowsIt(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	line := func(id string, minutes int) ledger.Line {
		return ledger.Line{
			Request: id, At: at.Add(time.Duration(minutes) * time.Minute), Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: "work",
			Status: 200, Attempts: 1, Prompt: "p1",
		}
	}
	a, later, idless := line("a", 0), line("a", 60), line("", 1)
	tests := []struct {
		name  string
		reads [][]ledger.Line
		want  []string
	}{
		{name: "a line given three times at once", reads: [][]ledger.Line{{a, a, a}}, want: []string{`"a" at 0s`}},
		{name: "a line held, given twice again", reads: [][]ledger.Line{{a}, {a, a}}, want: []string{`"a" at 0s`}},
		{name: "a line held, then one of its id arriving later", reads: [][]ledger.Line{{a}, {later}}, want: []string{`"a" at 0s`, `"a" at 1h0m0s`}},
		{name: "two lines of one id arriving apart, at once", reads: [][]ledger.Line{{a, later}}, want: []string{`"a" at 0s`, `"a" at 1h0m0s`}},
		{name: "a line without an id held, given again", reads: [][]ledger.Line{{idless}, {idless}}, want: []string{`"" at 1m0s`}},
		{name: "a line without an id given twice at once", reads: [][]ledger.Line{{idless, idless}}, want: []string{`"" at 1m0s`, `"" at 1m0s`}},
	}
	now := at.Add(2 * time.Hour)
	live := PageLive{Now: now}
	page := ReadPage(filedLedger{lines: []ledger.Line{a}, now: now}, "s", Stream{}, ledger.Pricing, 0, now).Opening(1, live)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := page
			for _, read := range tt.reads {
				given := make([]ledger.Held, len(read))
				for i := range read {
					given[i] = ledger.Held{Line: read[i]}
				}
				p = p.TurnOf(p, given)
			}
			var held []string
			for _, l := range p.turn.lines {
				held = append(held, fmt.Sprintf("%q at %v", l.request, l.arrived().Sub(at)))
			}
			if !slices.Equal(held, tt.want) {
				t.Errorf("the turn open holds %v, want %v", held, tt.want)
			}
		})
	}
}

// A count of a turn's session's tokens is none of its requests, as
// ledger.Line.Counted counts none, though it names the turn's prompt, or a
// subagent sent it: the turn read holds none, nor grown by the lines the
// ledger gains, its rows show none, and the Log wants no session for a count
// alone.
func TestACountOfATurnsSessionIsNoneOfItsRequests(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	line := func(id, kind, agent, session string, minutes int) ledger.Line {
		return ledger.Line{
			Request: id, At: at.Add(time.Duration(minutes) * time.Minute), Kind: kind, Session: session, Model: foldOpus, Account: "work",
			Status: 200, Attempts: 1, TotalMS: 1000, Class: ledger.ClassMain, Prompt: "p1", AgentID: agent,
		}
	}
	read := []ledger.Line{line("m1", ledger.KindMessage, "", "s", 0), line("c1", ledger.KindCount, "", "s", 1), line("c2", ledger.KindCount, "x", "s", 2)}
	gained := []ledger.Held{{Line: line("c3", ledger.KindCount, "", "s", 3)}, {Line: line("c4", ledger.KindCount, "y", "s", 4)}}
	now := at.Add(time.Hour)
	live := PageLive{Now: now}
	page := ReadPage(filedLedger{lines: read, now: now}, "s", Stream{}, ledger.Pricing, 0, now).Opening(1, live)
	page = page.Turned(page.ReadTurn(filedLedger{lines: read, now: now}, live))
	page = page.TurnOf(page, gained)
	var held, rows []string
	for _, l := range page.turn.lines {
		held = append(held, l.request)
	}
	for _, r := range page.TurnRequests(1, live) {
		rows = append(rows, r.Row.Request)
	}
	if want := []string{"m1"}; !slices.Equal(held, want) || !slices.Equal(rows, want) {
		t.Errorf("the turn holds %v and shows the rows %v, want %v of each", held, rows, want)
	}
	if wanted := ReadLog(ledger.Pricing).Wanted(Stream{}, RecordsOf(gained), RecordsOf([]ledger.Held{{Line: line("c5", ledger.KindCount, "", "k", 5)}})); len(wanted) > 0 {
		t.Errorf("the Log wants the sessions %v for counts alone, want none", wanted)
	}
}

// sessionGiving is a session's lines as a ledger gives them, newest first,
// given as they are, whatever the session asked for, of a ledger holding no
// day's rows.
type sessionGiving struct {
	noRows
	lines []ledger.Held
}

// Session gives g's lines, as they are.
func (g sessionGiving) Session(string, ...ledger.Span) iter.Seq[ledger.Held] {
	return slices.Values(g.lines)
}

// A turn decodes none of the lines it's given that mine passes over,
// another session's, a check or a count of its own, or its session's
// request of another turn, reading its lines or taking in those the ledger
// gains: decoding a line allocates, as its usage and its windows are read,
// so a turn given a thousand such lines allocates no more than one given
// none of them, but for less than one allocation a line.
func TestATurnDecodesOnlyTheLinesThatMayBeItsOwn(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	own := ledger.Line{
		Request: "m1", At: at, Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: "work", Status: 200, Attempts: 1, TotalMS: 1000,
		Class: ledger.ClassMain, Prompt: "p1", Usage: json.RawMessage(foldUsages[2]), Limits: map[string]string{"5h-utilization": "0.42", "5h-reset": "1791370800"},
	}
	passed := make([]ledger.Held, 1000)
	for i := range passed {
		l := own
		l.Request, l.At = fmt.Sprintf("o%04d", i), at.Add(time.Duration(len(passed)-i)*time.Millisecond)
		switch i % 4 {
		case 0:
			l.Session = "elsewhere"
		case 1:
			l.Kind = ledger.KindCount
		case 2:
			l.Kind = ledger.KindCheck
		default:
			l.Prompt = "p2"
		}
		passed[i] = ledger.Held{Line: l}
	}
	now := at.Add(time.Hour)
	live := PageLive{Now: now}
	page := ReadPage(filedLedger{lines: []ledger.Line{own}, now: now}, "s", Stream{}, ledger.Pricing, 0, now).Opening(1, live)
	alone, given := []ledger.Held{{Line: own}}, append(slices.Clone(passed), ledger.Held{Line: own})
	reads := map[string]func(given []ledger.Held) []lineRecord{
		"reading its lines":           func(given []ledger.Held) []lineRecord { return page.ReadTurn(sessionGiving{lines: given}, live).lines },
		"taking in the ledger's gain": func(given []ledger.Held) []lineRecord { return page.TurnOf(page, given).turn.lines },
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			if got := read(given); len(got) != 1 || got[0].request != "m1" {
				t.Fatalf("the turn holds %d lines, want m1 alone", len(got))
			}
			base := testing.AllocsPerRun(20, func() { read(alone) })
			if more := testing.AllocsPerRun(20, func() { read(given) }) - base; more >= float64(len(passed)) {
				t.Errorf("given %d lines it passes over, the turn allocates %.0f more, want fewer than one a line", len(passed), more)
			}
		})
	}
}

// mine answers of a line's facts, read of it undecoded, as of its record's,
// over generated runs, each line of every kind, of its session and of none:
// its facts are its record's, and mine takes it alike of both, for a turn
// open of its session and of another, on its prompt, another of the run's
// and none, whether it arrived before the turn began or since. So a line is
// passed over undecoded by the one rule its record is judged by.
func TestMineJudgesALinesFactsAsItsRecords(t *testing.T) {
	for seed := range uint64(20) {
		lines, _ := pagedRun(seed)
		for i := range lines {
			for _, kind := range []string{ledger.KindMessage, ledger.KindCheck, ledger.KindCount} {
				for _, session := range []string{lines[i].Session, ""} {
					l := lines[i]
					l.Kind, l.Session = kind, session
					rec := lineRecordOf(&l)
					of, recorded := turnFactsOf(&l), rec.turnFacts()
					if of != recorded {
						t.Fatalf("run %d, line %d of kind %q, session %q: its facts are %+v, want its record's, %+v", seed, i, kind, session, of, recorded)
					}
					for _, id := range []string{lines[i].Session, "elsewhere"} {
						for _, prompt := range []string{l.Prompt, lines[(i+1)%len(lines)].Prompt, ""} {
							h := HeldPage{id: id, turn: &turnLines{prompt: prompt}}
							for _, early := range []bool{false, true} {
								if got, want := h.mine(of, early), h.mine(recorded, early); got != want {
									t.Errorf("run %d, line %d, a turn of %q on %q, early %t: mine takes its facts %t, want as its record's, %t",
										seed, i, id, prompt, early, got, want)
								}
							}
						}
					}
				}
			}
		}
	}
}

// A turn open counts a subagent's requests in it as the page read whole
// does, though the subagent's first request, naming the turn's prompt, is
// untold by the stream and filed only after its next, naming another, was
// fed: the turn opened before that first request's line lands, the store
// fed it after; and opened after it lands, before the store is fed it.
func TestATurnCountsASubagentWhoseFirstLineLandsLateInIt(t *testing.T) {
	t0 := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	line := func(id, prompt, agent string, minutes float64, took time.Duration) ledger.Line {
		return ledger.Line{
			Request: id, At: t0.Add(time.Duration(minutes * float64(time.Minute))), Kind: ledger.KindMessage, Session: "s", Model: foldOpus,
			Account: "work", Reason: "sticky", Status: 200, Attempts: 1, TotalMS: took.Milliseconds(), Class: ledger.ClassMain, Prompt: prompt, AgentID: agent,
			Usage: json.RawMessage(foldUsages[2]),
		}
	}
	lines := []ledger.Line{
		line("m1", "p1", "", 0, time.Second), line("a1", "p1", "x", 1, 10*time.Minute),
		line("m2", "p2", "", 1.5, time.Second), line("a2", "p2", "x", 2, time.Second),
	}
	rowsOf := func(h HeldPage, live PageLive) []string {
		built, _ := h.Build(live)
		var rows []string
		for _, r := range built.Requests {
			rows = append(rows, fmt.Sprintf("%s lined %t", r.Row.Request, r.record != nil))
		}
		return rows
	}
	fedAt, landed := t0.Add(3*time.Minute), t0.Add(12*time.Minute)
	for name, opened := range map[string]time.Time{"opened before it lands": t0.Add(5 * time.Minute), "opened after it lands": landed} {
		t.Run(name, func(t *testing.T) {
			page := ReadPage(filedLedger{lines: lines, now: fedAt}, "s", Stream{}, ledger.Pricing, 0, fedAt)
			store := HoldSessions(ledger.Pricing).With(page.Found())
			page = page.Joined(store)
			live := PageLive{Now: opened}
			page = page.Opening(1, live)
			page = page.Turned(page.ReadTurn(filedLedger{lines: lines, now: opened}, live))
			if opened.Before(landed) {
				gained := []ledger.Held{{Line: lines[1]}}
				store, _ = store.Fed(filedLedger{lines: lines, now: landed}, Took(gained, nil), SessionsWanted{}.Paging(page, Stream{}), landed)
				page, _ = page.TurnOf(page, gained).Held(store)
			}
			live = PageLive{Now: landed}
			whole := HoldPage("s", linesRead(landed, "s", lines), ledger.Pricing, 0, landed)
			whole.turn = &turnLines{prompt: "p1"}
			whole = whole.Turned(HeldTurn{prompt: "p1", lines: recordsOfLines(lines)})
			if got, want := rowsOf(page, live), rowsOf(whole, live); !slices.Equal(got, want) {
				t.Errorf("turn 1's rows are %v, want %v, as the page read whole has them", got, want)
			}
		})
	}
}

// A turn open between feeds counts a subagent's requests in the turn its
// first request names, as the page read whole does, though that first
// request, untold by the stream, arrived before the turn began and was filed
// only since the store was last fed, after the subagent's next, naming the
// turn's prompt: the turn shows none of the subagent's requests. Its read
// holds that first request, and of the lines before the turn began none the
// store was fed, and reads none from before the session's horizon but the
// first.
func TestATurnOpenBetweenFeedsCountsASubagentByAFirstRequestFiledSince(t *testing.T) {
	t0 := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	line := func(id, prompt, agent string, minutes float64, took time.Duration) ledger.Line {
		return ledger.Line{
			Request: id, At: t0.Add(time.Duration(minutes * float64(time.Minute))), Kind: ledger.KindMessage, Session: "s", Model: foldOpus,
			Account: "work", Reason: "sticky", Status: 200, Attempts: 1, TotalMS: took.Milliseconds(), Class: ledger.ClassMain, Prompt: prompt, AgentID: agent,
			Usage: json.RawMessage(foldUsages[2]),
		}
	}
	lines := []ledger.Line{
		line("o1", "p0", "", -180, time.Second), line("o2", "p0", "", -120, time.Second),
		line("m1", "p1", "", 0, time.Second), line("a0", "p1", "y", 0.5, time.Second), line("a1", "p1", "x", 1, 5*time.Minute),
		line("m2", "p2", "", 2.5, time.Second), line("a2", "p2", "x", 3, time.Second),
	}
	fedAt, opened := t0.Add(4*time.Minute), t0.Add(7*time.Minute)
	page := ReadPage(filedLedger{lines: lines, now: fedAt}, "s", Stream{}, ledger.Pricing, 0, fedAt)
	page = page.Joined(HoldSessions(ledger.Pricing).With(page.Found()))
	live := PageLive{Now: opened}
	page = page.Opening(3, live)
	counted := readCounting{LedgerSession: filedLedger{lines: lines, now: opened}, read: new(int)}
	read := page.ReadTurn(counted, live)
	page = page.Turned(read)
	var held []string
	for _, l := range read.lines {
		held = append(held, l.request)
	}
	if want := []string{"a2", "m2", "a1"}; !slices.Equal(held, want) || *counted.read != 6 {
		t.Errorf("the turn's read holds %v, reading %d lines; want %v, reading 6, the last the first before its horizon", held, *counted.read, want)
	}
	whole := HoldPage("s", linesRead(opened, "s", lines), ledger.Pricing, 0, opened)
	whole.turn = &turnLines{prompt: "p2"}
	whole = whole.Turned(HeldTurn{prompt: "p2", lines: recordsOfLines(lines)})
	rowsOf := func(h HeldPage) []string {
		requests, _ := h.worked(live).turnRequests(3)
		var rows []string
		for _, r := range requests {
			rows = append(rows, r.Row.Request)
		}
		return rows
	}
	if got, want := rowsOf(page), rowsOf(whole); !slices.Equal(got, want) || !slices.Equal(want, []string{"m2"}) {
		t.Errorf("turn 3's rows are %v, want %v, as the page read whole has them: m2 alone", got, want)
	}
}

// A turn's read takes in, of the lines that arrived before it began, back to
// the horizon, a subagent's alone: never one naming its prompt that no
// subagent sent, though it was filed only since the store was fed, so the
// turn the store tallies began after it arrived.
func TestATurnReadsNoLineNamingItsPromptFromBeforeItBegan(t *testing.T) {
	t0 := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	line := func(id, prompt string, minutes float64, took time.Duration) ledger.Line {
		return ledger.Line{
			Request: id, At: t0.Add(time.Duration(minutes * float64(time.Minute))), Kind: ledger.KindMessage, Session: "s", Model: foldOpus,
			Account: "work", Reason: "sticky", Status: 200, Attempts: 1, TotalMS: took.Milliseconds(), Class: ledger.ClassMain, Prompt: prompt,
		}
	}
	lines := []ledger.Line{line("m1", "p1", 0, time.Second), line("e1", "p2", 0.5, 5*time.Minute), line("m2", "p2", 2.5, time.Second)}
	fedAt, opened := t0.Add(4*time.Minute), t0.Add(7*time.Minute)
	page := ReadPage(filedLedger{lines: lines, now: fedAt}, "s", Stream{}, ledger.Pricing, 0, fedAt)
	page = page.Joined(HoldSessions(ledger.Pricing).With(page.Found()))
	live := PageLive{Now: opened}
	page = page.Opening(2, live)
	read := page.ReadTurn(filedLedger{lines: lines, now: opened}, live)
	var held []string
	for _, l := range read.lines {
		held = append(held, l.request)
	}
	if want := []string{"m2"}; read.prompt != "p2" || !slices.Equal(held, want) {
		t.Errorf("the read of the turn of %q holds %v, want of p2's %v alone", read.prompt, held, want)
	}
}

// A turn open given again the lines it holds, as a read afresh gives them,
// takes none of them in again, and decodes none: a turn holding three
// thousand of a subagent's lines, each of its id, is given them again for
// fewer allocations than lines, as decoding one allocates, its usage and its
// windows read.
func TestATurnOpenDecodesNoLineItHoldsGivenAgain(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	line := func(id, agent string, ms int) ledger.Line {
		return ledger.Line{
			Request: id, At: at.Add(time.Duration(ms) * time.Millisecond), Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: "work",
			Status: 200, Attempts: 1, TotalMS: 1000, Class: ledger.ClassMain, Prompt: "p1", AgentID: agent, Usage: json.RawMessage(foldUsages[2]),
			Limits: map[string]string{"5h-utilization": "0.42", "5h-reset": "1791370800"},
		}
	}
	lines := []ledger.Line{line("m1", "", 0)}
	for i := range 3000 {
		lines = append(lines, line(fmt.Sprintf("a%04d", i), "x", 1+i))
	}
	now := at.Add(time.Hour)
	live := PageLive{Now: now}
	page := ReadPage(filedLedger{lines: lines[:1], now: now}, "s", Stream{}, ledger.Pricing, 0, now).Opening(1, live)
	page = page.Turned(HeldTurn{prompt: "p1", lines: recordsOfLines(lines)})
	again := make([]ledger.Held, len(lines))
	for i := range lines {
		again[i] = ledger.Held{Line: lines[i]}
	}
	if held := len(page.TurnOf(page, again).turn.lines); held != len(lines) {
		t.Fatalf("given its lines again, the turn holds %d lines, want the %d it held", held, len(lines))
	}
	if allocs := testing.AllocsPerRun(10, func() { page.TurnOf(page, again) }); allocs >= float64(len(again)) {
		t.Errorf("given the %d lines it holds again, the turn allocates %.0f times, want fewer than once a line", len(again), allocs)
	}
}

// readCounting reads a session's lines through the ledger it holds,
// counting in read those it gives.
type readCounting struct {
	LedgerSession
	read *int
}

func (c readCounting) Session(id string, within ...ledger.Span) iter.Seq[ledger.Held] {
	return func(yield func(ledger.Held) bool) {
		for held := range c.LedgerSession.Session(id, within...) {
			*c.read++
			if !yield(held) {
				return
			}
		}
	}
}
