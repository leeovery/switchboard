package views

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// The models of the generated sessions: Claude Opus 5.5, whose writes to
// the cache cost more from their second day, as changingPrices has it;
// Claude Haiku 4.5, which the table prices in no geo but the global one; and
// Claude Sonnet 5.5.
const (
	foldOpus   = "claude-opus-5-5"
	foldHaiku  = "claude-haiku-4-5"
	foldSonnet = "claude-sonnet-5-5"
)

// foldUsages are the usages of the generated sessions' answers: none, of an
// answer cut short; null; writes to the cache for an hour, for five minutes,
// and for each; writes it doesn't break down; reads alone; and none of the
// cache's.
var foldUsages = []string{
	"",
	`null`,
	`{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":100000,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":1000},"output_tokens":500}`,
	`{"input_tokens":10,"cache_creation_input_tokens":2000,"cache_read_input_tokens":90000,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":2000,"ephemeral_1h_input_tokens":0},"output_tokens":300}`,
	`{"input_tokens":4,"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_5m_input_tokens":120,"ephemeral_1h_input_tokens":3000},"output_tokens":80}`,
	`{"input_tokens":12,"cache_creation_input_tokens":3120,"cache_read_input_tokens":5000,"output_tokens":40}`,
	`{"input_tokens":3,"cache_read_input_tokens":120000,"output_tokens":700}`,
	`{"input_tokens":100,"output_tokens":10}`,
}

// changingPrices are ledger.Pricing, but Claude Opus 5.5's writes to the
// cache costing twice as much from 8 October, the generated sessions' second
// day, as changingPricesFrom has them.
func changingPrices() ledger.Table {
	return changingPricesFrom("2026-10-08")
}

// changingPricesFrom are ledger.Pricing, but Claude Opus 5.5's writes to the
// cache costing twice as much from the day with the given date.
func changingPricesFrom(date string) ledger.Table {
	t := ledger.Pricing
	t.Models = slices.Clone(t.Models)
	for i, m := range t.Models {
		if slices.Contains(m.IDs, foldOpus) {
			later := m.Prices[len(m.Prices)-1]
			later.From, later.CacheWrite5m, later.CacheWrite1h = date, 2*later.CacheWrite5m, 2*later.CacheWrite1h
			t.Models[i].Prices = append(slices.Clone(m.Prices), later)
		}
	}
	return t
}

// generatedSession is a session's lines as the seed given makes them, in
// the order the ledger files them, as each request ends: starting on the
// evening of 7 October, some running past midnight; its requests coming
// moments apart, at times to the millisecond together, at times after its
// cache has run out; of its own conversation, unclassed, a subagent's, a
// compaction or a side request; some moving their model to another account,
// some of those after one couldn't serve them; some running long, so their
// lines come after those of requests that arrived after them; and a quota
// check or a count of tokens among them.
func generatedSession(seed uint64) []ledger.Line {
	r := rand.New(rand.NewPCG(seed, 0x5e551011))
	at := time.Date(2026, 10, 7, 20, 0, 0, 0, time.Local).Add(time.Duration(r.IntN(240)) * time.Minute)
	goes := make(map[string]string)
	var lines []ledger.Line
	for range 10 + r.IntN(50) {
		at = foldStep(r, at)
		l := foldLine(r, at, goes)
		if n := len(lines); n > 0 && lines[n-1].At.Equal(l.At) && r.IntN(2) == 0 {
			l.TotalMS = lines[n-1].TotalMS
		}
		lines = append(lines, l)
	}
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	return lines
}

// foldStep is when a generated session's next request comes after one at
// at: at the same millisecond; after its cache has run out, of five minutes
// or of an hour; or moments after.
func foldStep(r *rand.Rand, at time.Time) time.Time {
	switch p := r.IntN(100); {
	case p < 15:
		return at
	case p < 25:
		return at.Add(time.Duration(6+r.IntN(70)) * time.Minute)
	}
	return at.Add(time.Duration(r.IntN(90_000)) * time.Millisecond)
}

// foldLine is a generated session's request that arrived at at, its model
// going where goes has it, which a move changes.
func foldLine(r *rand.Rand, at time.Time, goes map[string]string) ledger.Line {
	model := []string{foldOpus, foldOpus, foldOpus, foldHaiku, foldSonnet}[r.IntN(5)]
	l := ledger.Line{
		At: at.UTC(), Request: fmt.Sprintf("%08x", r.Uint32()), Kind: ledger.KindMessage, Session: "s", Model: model,
		Account: cmp.Or(goes[model], "work"), Reason: "sticky", Status: 200, Attempts: 1, TotalMS: int64(200 + r.IntN(20_000)),
	}
	switch p := r.IntN(100); {
	case p < 45:
		l.Class = ledger.ClassMain
	case p < 60:
	case p < 75:
		l.Class, l.AgentID = "subagent", []string{"a1", "a2", "a3"}[r.IntN(3)]
	case p < 80:
		l.Class = classCompaction
	default:
		l.Class = "title"
	}
	if goes[model] == "" {
		l.Reason = "new"
	} else if r.IntN(100) < 12 {
		l.From, l.Account, l.Reason = goes[model], []string{"work", "side", "personal"}[r.IntN(3)], "moved: work hit its limit"
		if r.IntN(3) == 0 {
			l.Tried = []ledger.Tried{{Account: l.From, Why: "hit its limit"}}
		}
	}
	goes[model] = l.Account
	if r.IntN(5) == 0 {
		l.TotalMS = int64(60_000 + r.IntN(540_000))
	}
	l.Dir = []string{"", "", "", "/code/app", "/code/other"}[r.IntN(5)]
	if r.IntN(10) == 0 {
		l.Shape.InferenceGeo = "us"
	}
	if usage := foldUsages[r.IntN(len(foldUsages))]; usage != "" {
		l.Usage = json.RawMessage(usage)
	} else {
		l.CutOff, l.Status = true, 0
	}
	if r.IntN(30) == 0 {
		l.Kind = []string{ledger.KindCheck, ledger.KindCount}[r.IntN(2)]
	}
	return l
}

// linesInOrder are lines in the order every story takes them in: by when
// each arrived, then when it ended, then by its id, afresh.
func linesInOrder(lines []ledger.Line) []ledger.Line {
	ordered := slices.Clone(lines)
	slices.SortStableFunc(ordered, func(a, b ledger.Line) int {
		return cmp.Or(a.At.Compare(b.At), ended(&a).Compare(ended(&b)), strings.Compare(a.Request, b.Request))
	})
	return ordered
}

// sameStory reports whether two stories hold the same, told by the same
// prices: so whatever is asked of one, the other tells alike.
func sameStory(a, b *story) bool {
	x, y := *a, *b
	x.prices, y.prices = ledger.Table{}, ledger.Table{}
	return reflect.DeepEqual(x, y)
}

// storyTold is what a story tells, as the views read it, for one told over
// held requests to be held to one told over lines: its first and last
// request, what names it, its directory, its last requests that gave usage,
// whether it's classed, its requests on the day and their worth, its moves,
// when each thread last ended and how long its writes last; and what it says
// of each line, of moving it at moments after its last request, and of the
// moves that brought each model onto each account.
type storyTold struct {
	First, Last, Main, Latest, Dir, Used, UsedMain string
	Classed                                        bool
	Requests                                       int
	Worth                                          Worth
	Moves                                          []moveTold
	Ends                                           map[string]int64
	Lives                                          map[string]time.Duration
	Told, Costs, Onto                              []string
}

// moveTold is what a story tells of a move, as storyTold has it.
type moveTold struct {
	By, Model, From, To, Reason, Rewrote string
	At                                   int64
	Settled                              bool
	Written                              int
	Cost                                 *ledger.Picodollars
	Routing                              RoutingSaid
}

// foldQuery is what a storyTold asks of a story beyond what it holds: of
// each line, what the story tells of it; of moving it, at each moment, with
// a request of the model in flight or not; and of each model and account,
// the move that brought the model there.
type foldQuery struct {
	lines           []ledger.Line
	nows            []time.Time
	models, holders []string
}

// queryOf is what to ask of the story of lines, as foldQuery has it.
func queryOf(lines []ledger.Line) foldQuery {
	last := slices.MaxFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	end := ended(&last)
	return foldQuery{
		lines: lines, nows: []time.Time{end, end.Add(time.Minute), end.Add(6 * time.Minute), end.Add(61 * time.Minute)},
		models: []string{foldOpus, foldHaiku, foldSonnet}, holders: []string{"work", "side", "personal"},
	}
}

// asked is a story as a foldQuery asks it, told over lines or over held
// requests: what it tells of a line, as toldOf has it; what moving it costs
// at now; how long its prompt's writes last and when its cache runs out, as
// moving has them; and the requests that made a model's latest move, and the
// move that brought it onto an account, each named as lineSaid names one.
type asked interface {
	toldSaid(l *ledger.Line) string
	moveCost(now time.Time, flying func(model string) bool) (ledger.Picodollars, bool)
	lasting() (life time.Duration, cold time.Time)
	latestSaid(model string) string
	ontoSaid(model, account string) string
}

// ask has told hold what s tells of what q asks.
func (q foldQuery) ask(s asked, told *storyTold) {
	for i := range q.lines {
		told.Told = append(told.Told, s.toldSaid(&q.lines[i]))
	}
	for _, now := range q.nows {
		for _, in := range []bool{false, true} {
			cost, ok := s.moveCost(now, func(string) bool { return in })
			told.Costs = append(told.Costs, fmt.Sprintf("at %d, flying %t: %d %t", now.UnixNano(), in, cost, ok))
		}
	}
	life, cold := s.lasting()
	told.Costs = append(told.Costs, fmt.Sprintf("lasting %v, cold at %d, %t", life, cold.UnixNano(), cold.IsZero()))
	for _, model := range q.models {
		told.Onto = append(told.Onto, model+" latest by "+s.latestSaid(model))
		for _, account := range q.holders {
			told.Onto = append(told.Onto, fmt.Sprintf("%s onto %s by %s", model, account, s.ontoSaid(model, account)))
		}
	}
}

// lineSaid names the request of a line, nil for none, by its id, arrival
// and end, as the stories' tellings are held alike.
func lineSaid(l *ledger.Line) string {
	if l == nil {
		return ""
	}
	return fmt.Sprintf("%s@%d+%d", l.Request, l.At.UnixNano(), ended(l).UnixNano())
}

// flightSaid names the request of a flight, as lineSaid names a line's.
func flightSaid(f flight) string {
	return fmt.Sprintf("%s@%d+%d", f.request, f.arrived().UnixNano(), f.ended().UnixNano())
}

// sightingSaid names a sighting, as the stories' namings are held alike.
func sightingSaid(s sighting) string {
	return fmt.Sprintf("%s on %s at %d", s.model, s.account, s.at.UnixNano())
}

// endsSaid are when each thread last ended, as the stories' are held alike.
func endsSaid(ends map[string]time.Time) map[string]int64 {
	said := make(map[string]int64, len(ends))
	for thread, end := range ends {
		said[thread] = end.UnixNano()
	}
	return said
}

// toldSaid says what the story tells of a line, as toldOf has it, the move
// it made by its place among the story's moves, -1 for none.
func toldSaid[M any](first bool, moves []M, made func(M) bool, rewrote bool) string {
	return fmt.Sprintf("first %t, moved %d, rewrote %t", first, slices.IndexFunc(moves, made), rewrote)
}

// lineTelling is a story told over lines, as a foldQuery asks it.
type lineTelling struct{ *lineStory }

func (s lineTelling) toldSaid(l *ledger.Line) string {
	first, made, rewrote := s.toldOf(l)
	return toldSaid(first, s.moves, func(m lineMove) bool { return made != nil && m.by == made.by }, rewrote)
}

func (s lineTelling) lasting() (time.Duration, time.Time) {
	_, life, cold := s.moving()
	return life, cold
}

func (s lineTelling) latestSaid(model string) string { return lineMoveSaid(s.latestMove(model)) }

func (s lineTelling) ontoSaid(model, account string) string {
	return lineMoveSaid(s.moveOnto(model, account))
}

// lineMoveSaid names the request that made m, "" for none.
func lineMoveSaid(m *lineMove) string {
	if m == nil {
		return ""
	}
	return lineSaid(m.by)
}

// heldTelling is a story told over held requests, as a foldQuery asks it.
type heldTelling struct{ *story }

func (s heldTelling) toldSaid(l *ledger.Line) string {
	first, made, rewrote := s.toldOf(l.Request, l.At)
	return toldSaid(first, s.moves, func(m move) bool { return made != nil && m.by == made.by }, rewrote)
}

func (s heldTelling) lasting() (time.Duration, time.Time) {
	_, life, cold := s.moving()
	return life, cold
}

func (s heldTelling) latestSaid(model string) string { return moveSaid(s.latestMove(model)) }

func (s heldTelling) ontoSaid(model, account string) string {
	return moveSaid(s.moveOnto(model, account))
}

// moveSaid names the request that made m, "" for none.
func moveSaid(m *move) string {
	if m == nil {
		return ""
	}
	return flightSaid(m.by)
}

// toldOfLines is what the story s, told over lines, tells, asked q.
func toldOfLines(s *lineStory, q foldQuery) storyTold {
	told := storyTold{
		First: lineSaid(s.first), Last: lineSaid(s.last), Main: sightingSaid(s.naming.main), Latest: sightingSaid(s.naming.latest),
		Dir: s.dir, Used: lineSaid(s.used), UsedMain: lineSaid(s.usedMain), Classed: s.classed, Requests: s.requests, Worth: s.worth,
		Ends: endsSaid(s.ends), Lives: s.lives,
	}
	for _, m := range s.moves {
		by := lineRecordOf(m.by)
		routing := routedOf(&by, nil).said(m.by.Account, false, nil)
		routing.Cost, routing.Rewrote = m.cost, m.rewrote != nil
		told.Moves = append(told.Moves, moveTold{
			By: lineSaid(m.by), At: m.at.UnixNano(), Model: m.model, From: m.from, To: m.to, Reason: m.reason, Settled: m.settled,
			Written: m.written, Cost: m.cost, Rewrote: lineSaid(m.rewrote), Routing: routing,
		})
	}
	q.ask(lineTelling{s}, &told)
	return told
}

// toldOfHeld is what the story s, told over held requests, tells, asked q.
func toldOfHeld(s *story, q foldQuery) storyTold {
	begun := func(f flight) string {
		if !s.begun {
			return ""
		}
		return flightSaid(f)
	}
	metered := func(r heldRequest) string {
		if !r.metered() {
			return ""
		}
		return flightSaid(r.flight)
	}
	told := storyTold{
		First: begun(s.first), Last: begun(s.last), Main: sightingSaid(s.naming.main), Latest: sightingSaid(s.naming.latest),
		Dir: s.dir, Used: metered(s.used), UsedMain: metered(s.usedMain), Classed: s.classed, Requests: s.requests, Worth: s.worth,
		Ends: endsSaid(s.ends), Lives: s.lives,
	}
	for _, m := range s.moves {
		rewrote := ""
		if m.rewrote != nil {
			rewrote = flightSaid(m.rewrote.flight)
		}
		told.Moves = append(told.Moves, moveTold{
			By: flightSaid(m.by), At: m.at.UnixNano(), Model: m.model, From: m.from, To: m.to, Reason: m.reason, Settled: m.settled,
			Written: m.written, Cost: m.cost, Rewrote: rewrote, Routing: m.routing(),
		})
	}
	q.ask(heldTelling{s}, &told)
	return told
}

// foldDays are the days the generated sessions' stories are told on: the
// day each starts, and the next, on whose prices Claude Opus 5.5's writes
// cost more, as changingPrices has them.
var foldDays = []time.Time{time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local), time.Date(2026, 10, 8, 12, 0, 0, 0, time.Local)}

// The story told over held requests tells what the story over lines told,
// of the same requests, in the same order: of generated sessions whose
// requests end out of the order they came in, come to the millisecond
// together, move with their caches warm and cold, of five minutes and of an
// hour, as subagents, side requests, compactions and lines without a class,
// across midnight, told on each day, a model's prices changing on the
// second; priced, as the List and a session's page tell it, and unpriced, as
// the Log does.
func TestAStoryOfHeldRequestsTellsWhatTheStoryOfLinesTold(t *testing.T) {
	prices := changingPrices()
	for seed := range uint64(300) {
		filed := generatedSession(seed)
		ordered := linesInOrder(filed)
		q := queryOf(filed)
		for _, at := range foldDays {
			day := Today(at)
			t.Run(fmt.Sprintf("session %d told on %s", seed, day.Date), func(t *testing.T) {
				priced := requestsOf(ordered, prices, day.Date)
				want := toldOfLines(lineStoryOf(priced, prices, day), q)
				if got := toldOfHeld(storyOf(pricedRequestsOf(filed, prices, day.Date), prices, day), q); !reflect.DeepEqual(got, want) {
					t.Errorf("priced, the story tells\n%+v\nwant, as told over lines\n%+v", got, want)
				}
				unpriced := slices.Clone(priced)
				for i := range unpriced {
					unpriced[i].worth = Worth{}
				}
				want = toldOfLines(lineStoryOf(unpriced, prices, day), q)
				if got := toldOfHeld(storyOf(heldRequestsOf(filed), prices, day), q); !reflect.DeepEqual(got, want) {
					t.Errorf("unpriced, the story tells\n%+v\nwant, as told over lines\n%+v", got, want)
				}
			})
		}
	}
}

// The generated sessions hold what the story's hard moments are made of, so
// a fold that passes over one fails the test above.
func TestTheGeneratedSessionsHoldTheStorysHardMoments(t *testing.T) {
	prices, day := changingPrices(), Today(foldDays[1])
	tied := func(filed []ledger.Line, i int, also func(a, b *ledger.Line) bool) bool {
		return slices.ContainsFunc(filed[:i], func(o ledger.Line) bool { return o.At.Equal(filed[i].At) && also(&o, &filed[i]) })
	}
	ofLines := map[string]func(filed []ledger.Line, i int) bool{
		"a request that ended after one that arrived after it": func(filed []ledger.Line, i int) bool { return i > 0 && filed[i].At.Before(filed[i-1].At) },
		"two arriving to the millisecond together": func(filed []ledger.Line, i int) bool {
			return tied(filed, i, func(*ledger.Line, *ledger.Line) bool { return true })
		},
		"two arriving and ending together": func(filed []ledger.Line, i int) bool {
			return tied(filed, i, func(a, b *ledger.Line) bool { return ended(a).Equal(ended(b)) })
		},
		"a quota check or a count of tokens": func(filed []ledger.Line, i int) bool { return !filed[i].Counted() },
		"a request without a class":          func(filed []ledger.Line, i int) bool { return filed[i].Class == "" },
		"a subagent's request":               func(filed []ledger.Line, i int) bool { return filed[i].AgentID != "" },
		"a compaction":                       func(filed []ledger.Line, i int) bool { return filed[i].Class == classCompaction },
		"a side request":                     func(filed []ledger.Line, i int) bool { return filed[i].Class == "title" },
		"a request after midnight":           func(filed []ledger.Line, i int) bool { return day.Holds(filed[i].At) },
	}
	ofMoves := map[string]func(m move) bool{
		"a move whose cache was warm":               func(m move) bool { return m.rewrote != nil },
		"a move whose cache had run out":            func(m move) bool { return m.settled && m.rewrote == nil && m.written > 0 },
		"a move after an account couldn't serve it": func(m move) bool { return len(m.tried) > 0 },
		"a move priced at the second day's prices":  func(m move) bool { return m.cost != nil && m.model == foldOpus },
	}
	held := make(map[string]bool)
	for seed := range uint64(300) {
		filed := generatedSession(seed)
		s := storyOf(pricedRequestsOf(filed, prices, day.Date), prices, day)
		for moment, holds := range ofLines {
			for i := range filed {
				held[moment] = held[moment] || holds(filed, i)
			}
		}
		for moment, holds := range ofMoves {
			held[moment] = held[moment] || slices.ContainsFunc(s.moves, holds)
		}
		for _, life := range s.lives {
			held["writes lasting "+life.String()] = true
		}
	}
	moments := slices.Concat(slices.Collect(maps.Keys(ofLines)), slices.Collect(maps.Keys(ofMoves)), []string{"writes lasting 5m0s", "writes lasting 1h0m0s"})
	for _, moment := range slices.Sorted(slices.Values(moments)) {
		if !held[moment] {
			t.Errorf("no generated session holds %s", moment)
		}
	}
}

// A session's story is told alike however its lines were read, whichever
// order they came in, as every story takes its requests in the one order.
func TestAStoryIsToldAlikeHoweverItsLinesWereRead(t *testing.T) {
	prices := changingPrices()
	day := Today(foldDays[1])
	for seed := range uint64(100) {
		filed := generatedSession(seed)
		q := queryOf(filed)
		want := toldOfHeld(storyOf(pricedRequestsOf(filed, prices, day.Date), prices, day), q)
		r := rand.New(rand.NewPCG(seed, 1))
		for read := range 3 {
			shuffled := slices.Clone(filed)
			r.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
			if got := toldOfHeld(storyOf(pricedRequestsOf(shuffled, prices, day.Date), prices, day), q); !reflect.DeepEqual(got, want) {
				t.Errorf("session %d read in order %d tells\n%+v\nwant, as read as filed\n%+v", seed, read, got, want)
			}
		}
	}
}

// Requests that arrived at the same millisecond are taken in by when they
// ended, then by their ids, whichever order their lines came in: so the
// later to end names the session, as the latest taken in does.
func TestRequestsTiedToTheMillisecondAreTakenInByTheirEndsThenTheirIDs(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	asked := func(id, account string, ms int64) ledger.Line {
		return ledger.Line{At: at, Request: id, Kind: ledger.KindMessage, Session: "s", Model: foldOpus, Account: account, Class: ledger.ClassMain, Attempts: 1, TotalMS: ms}
	}
	tests := []struct {
		name  string
		lines []ledger.Line
		want  string
	}{
		{name: "the later to end last", lines: []ledger.Line{asked("a", "work", 5000), asked("b", "side", 2000)}, want: "work"},
		{name: "ending together, the later id last", lines: []ledger.Line{asked("b", "work", 2000), asked("a", "side", 2000)}, want: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, lines := range [][]ledger.Line{tt.lines, {tt.lines[1], tt.lines[0]}} {
				s := storyOf(heldRequestsOf(lines), ledger.Pricing, Today(at))
				if account, _ := s.naming.names(nil); account != tt.want {
					t.Errorf("read as %s then %s, the session is on %s, want %s", lines[0].Request, lines[1].Request, account, tt.want)
				}
			}
		})
	}
}
