package capture

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestScenarioNamesNameEveryScenarioOnceSorted(t *testing.T) {
	names := ScenarioNames()
	if len(names) == 0 {
		t.Fatal("there are no scenarios")
	}
	if !slices.IsSorted(names) || len(slices.Compact(slices.Clone(names))) != len(names) {
		t.Errorf("ScenarioNames() = %v, want each once, sorted", names)
	}
	for _, name := range names {
		if s, err := ScenarioNamed(name); err != nil || s.Name != name {
			t.Errorf("ScenarioNamed(%q) = the scenario named %q, %v", name, s.Name, err)
		}
	}
}

func TestAnEmptyOrUnknownScenarioNameListsTheScenarios(t *testing.T) {
	available := "(available: " + strings.Join(ScenarioNames(), ", ") + ")"
	tests := []struct {
		name, want string
	}{
		{name: "", want: "name a scenario " + available},
		{name: "accounts-3", want: `unknown scenario "accounts-3" ` + available},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ScenarioNamed(tt.name); err == nil || err.Error() != tt.want {
				t.Errorf("ScenarioNamed(%q) error = %v, want %s", tt.name, err, tt.want)
			}
		})
	}
}

func TestEveryScenarioPlaysToItsEndTheSameWayEachTime(t *testing.T) {
	for _, s := range scenarios(moment(time.UTC)) {
		t.Run(s.Name, func(t *testing.T) {
			first, again := s.played(nil, time.Time{}), s.played(nil, time.Time{})
			if len(first.told) == 0 {
				t.Fatal("its router tells of nothing")
			}
			if !slices.IsSortedFunc(first.told, func(a, b router.StreamEvent) int { return a.At.Compare(b.At) }) {
				t.Error("its router tells of what befalls its requests out of order")
			}
			if !reflect.DeepEqual(first.told, again.told) || !reflect.DeepEqual(first.world.source(s.start).doc, again.world.source(s.start).doc) {
				t.Error("played twice, it played otherwise the second time")
			}
		})
	}
}

func TestARequestIsToldOfAsTheRouterTellsOfIt(t *testing.T) {
	s := testScenario(asks(1, idDB8A, sonnet, brief))

	told := s.played(nil, time.Time{}).told

	var kinds []string
	for _, e := range told {
		kinds = append(kinds, e.Kind)
		if e.Request != told[0].Request || e.Session != idDB8A || e.Model != sonnet || e.Account != "work" || e.Attempt != 1 {
			t.Errorf("the stream tells %+v, want each event of db8a's sonnet's request on work, on its first time upstream", e)
		}
	}
	want := []string{router.StreamSent, router.StreamFirst, router.StreamProgress, router.StreamProgress, router.StreamProgress, router.StreamProgress, router.StreamProgress, router.StreamDone}
	if !slices.Equal(kinds, want) {
		t.Fatalf("the stream tells %v, want %v", kinds, want)
	}
	if sent, first := told[0].At, told[1].At; !sent.Equal(s.start.Add(time.Second)) || first.Sub(sent) != brief.wait {
		t.Errorf("the request went at %s, its answer's first byte at %s, want a second in, and the answer's wait after", sent, first)
	}
	if !slices.IsSortedFunc(told[2:7], func(a, b router.StreamEvent) int { return a.Chars - b.Chars }) || told[2].At.Sub(told[1].At) != progressEvery {
		t.Errorf("its progress is told as %+v, want it rising, every %s from its first byte", told[2:7], progressEvery)
	}
	done := told[len(told)-1]
	if done.Status != 200 || done.Chars != brief.chars || done.Tokens == nil || done.Tokens.Output != brief.chars/charsPerToken {
		t.Errorf("its end is told as %+v, want it answered, every character counted, and its tokens", done)
	}
}

func TestAnAnswerIsReadOffForItsAccountsUse(t *testing.T) {
	s := testScenario(asks(1, idDB8A, sonnet, brief))
	before := s.played(nil, s.start.Add(1500*time.Millisecond)).world.sample("work")
	after := s.played(nil, s.start.Add(2*time.Second)).world.sample("work")

	if got, want := after.session.used-before.session.used, float64(brief.chars)*sessionPerChar; !near(got, want) {
		t.Errorf("work's session rose by %v as its answer came, want %v", got, want)
	}
	if !after.read.Equal(s.start.Add(1700 * time.Millisecond)) {
		t.Errorf("work was read at %s, want as its answer's first byte came", after.read)
	}
}

func TestANewSessionStartsWhereNewSessionsGo(t *testing.T) {
	s := testScenario(starts(1, idB3E9, opus, brief))

	sent := s.played(nil, s.start.Add(time.Second)).world
	if on, _, ok := sent.seatOf(idB3E9, opus); !ok || on.id != "side" {
		t.Fatalf("as its first request went, the session is on %v, want side, the best", on)
	}
	if slices.ContainsFunc(sent.events, func(e status.Event) bool { return e.Session == idB3E9 }) {
		t.Error("the router told of the session starting before it was answered")
	}
	answered := s.played(nil, s.start.Add(2*time.Second)).world
	if got := answered.events[0]; got.Kind != status.EventStarted || got.Session != idB3E9 || got.Account != "side" || got.Reason != status.ReasonNew {
		t.Errorf("once answered, the router tells of %+v, want b3e9 started on side", got)
	}
}

func TestALimitReachedHoldsTheAccountBackAndMovesItsSessions(t *testing.T) {
	s := testScenario(
		reaches(1, idD28C, opus, brief),
		asks(3, id7F3A, haiku, brief),
	)
	played := s.played(nil, time.Time{})

	work := played.world.sample("work")
	if !work.held(s.start.Add(4*time.Second)) || work.session.used != 1 || !work.session.limited.Equal(s.start.Add(time.Second+refuseAfter)) {
		t.Errorf("work's session is %+v, want it used up, its limit reached as the request was refused, and holding", work.session)
	}
	for _, seat := range []string{idD28C, id7F3A} {
		if on, _, ok := played.world.seatOf(seat, map[string]string{idD28C: opus, id7F3A: haiku}[seat]); !ok || on.id != "side" {
			t.Errorf("%s is on %v, want it moved to side", seat[:4], on)
		}
	}
	doc := played.world.source(s.start.Add(5 * time.Second)).doc
	limit := slices.IndexFunc(doc.Events, func(e status.Event) bool { return e.Kind == status.EventLimit })
	if limit < 0 || doc.Events[limit].Count != 2 || doc.Events[limit].To != "side" {
		t.Fatalf("the router tells of %+v, want work's limit, which moved 2 sessions to side", doc.Events)
	}
	var reasons []string
	for _, e := range doc.Events {
		if e.Kind == status.EventMoved {
			if e.Limit != doc.Events[limit].ID {
				t.Errorf("the move %+v isn't counted in the limit's event", e)
			}
			reasons = append(reasons, e.Reason)
		}
	}
	if want := []string{"moved: work has no room", "moved: work hit its limit"}; !slices.Equal(reasons, want) {
		t.Errorf("the moves are told of for %q, want %q", reasons, want)
	}
}

func TestARequestThatReachesALimitIsToldOfAsTheRouterTellsOfIt(t *testing.T) {
	s := testScenario(reaches(1, idD28C, opus, brief))

	var told []string
	for _, e := range s.played(nil, time.Time{}).told {
		told = append(told, e.Kind+" "+e.Account+" "+string(rune('0'+e.Attempt)))
		switch e.Kind {
		case router.StreamLimited:
			if e.Status != 429 {
				t.Errorf("the refusal is told as %+v, want a 429", e)
			}
		case router.StreamMoved:
			if e.From != "work" || e.To != "side" || e.Reason != "moved: work hit its limit" {
				t.Errorf("the move is told as %+v, want from work to side, as work hit its limit", e)
			}
		}
	}
	want := []string{"sent work 1", "limited work 1", "moved side 1", "sent side 2", "first side 2"}
	if len(told) < len(want) || !slices.Equal(told[:len(want)], want) || told[len(told)-1] != "done side 2" {
		t.Errorf("the stream tells %v, want %v, its answer on side, then done there", told, want)
	}
}

func TestAPinSendsTheSessionThereFromItsNextRequest(t *testing.T) {
	s := testScenario(asks(1, idD28C, opus, brief), asks(5, idD28C, opus, brief))
	pinned := order{at: s.start.Add(2 * time.Second), give: func(w *world) {
		w.pins = map[string]ownPin{idD28C: {account: "side", at: s.start.Add(2 * time.Second)}}
	}}

	before := s.played([]order{pinned}, s.start.Add(3*time.Second))
	sessions, _ := before.world.source(s.start.Add(3 * time.Second)).Sessions(t.Context())
	if d28c := listed(t, sessions, idD28C); d28c.Pin != "side" || d28c.Assignments[0].Account != "work" || d28c.Assignments[0].Pinned {
		t.Errorf("before its next request, the router lists d28c as %+v, want it on work, where it was, its own pin to side", d28c)
	}
	after := s.played([]order{pinned}, time.Time{})
	sessions, _ = after.world.source(s.start.Add(6 * time.Second)).Sessions(t.Context())
	d28c := listed(t, sessions, idD28C)
	if a := d28c.Assignments[0]; a.Account != "side" || a.Reason != status.ReasonPinned || !a.Pinned || !a.PinnedAt.Equal(s.start.Add(2*time.Second)) {
		t.Errorf("after its next request, the router lists d28c as %+v, want it on side, pinned there since its pin was given", d28c)
	}
	moved := slices.IndexFunc(after.told, func(e router.StreamEvent) bool { return e.Kind == router.StreamMoved })
	if moved < 0 || !after.told[moved].At.Equal(s.start.Add(5*time.Second)) || after.told[moved+1].Kind != router.StreamSent || after.told[moved+1].Account != "side" {
		t.Errorf("the stream tells %+v, want d28c's move to side as its next request goes, then the request going out there", after.told)
	}
}

func TestTheGlobalPinSendsNewSessionsToTheBestOfItsAccounts(t *testing.T) {
	tests := []struct {
		name, best string
		// held is set where work reaches its limit before the new session
		// starts.
		held bool
		want string
	}{
		{name: "the best, where the pin names it, though not first", best: "side", want: "side"},
		{name: "the first it names, where it doesn't name the best", best: "client", want: "work"},
		{name: "the first it names with room", best: "client", held: true, want: "side"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cues []cue
			if tt.held {
				cues = append(cues, reaches(0.5, idD28C, opus, brief))
			}
			s := testScenario(append(cues, starts(2, idB3E9, opus, brief))...)
			s.setting = func(now time.Time) world {
				return world{samples: []sample{work(now), side(now), client(now)}, best: tt.best}
			}
			pinned := order{at: s.start, give: func(w *world) { w.pin = status.Pin{Accounts: []string{"work", "side"}, Since: s.start} }}

			played := s.played([]order{pinned}, time.Time{})
			if on, _, _ := played.world.seatOf(idB3E9, opus); on.id != tt.want {
				t.Errorf("the new session started on %s, want %s", on.id, tt.want)
			}
			if doc := played.world.source(s.start.Add(3 * time.Second)).doc; doc.Best != tt.want || !doc.Pin.Has("work") || !doc.Pin.Has("side") {
				t.Errorf("the document's best is %q, its pin %+v, want %s, and the pin of work and side", doc.Best, doc.Pin, tt.want)
			}
		})
	}
}

func TestOrdersInTheSameWorldPlayInTurn(t *testing.T) {
	s := testScenario()
	orders := []order{
		{at: s.start.Add(time.Second), give: func(w *world) { w.best = "work" }},
		{at: s.start.Add(time.Second), give: func(w *world) { w.best = "side" }},
	}

	if got := s.played(orders, time.Time{}).world.best; got != "side" {
		t.Errorf("after two orders at the same moment, the best is %q, want the last's, side", got)
	}
	if got := s.played(orders, s.start.Add(500*time.Millisecond)).world.best; got != "side" {
		t.Errorf("before either order, the best is %q, want the world's own, side", got)
	}
}

func TestFlyingAreTheRequestsInFlightAsTheRouterKeepsThem(t *testing.T) {
	s := testScenario(
		asks(1, idD28C, opus, long),
		asks(1.5, idDB8A, sonnet, brief),
		reaches(2, id7F3A, haiku, long),
		asks(2.2, idC61B, opus, brief),
	)
	told := s.played(nil, time.Time{}).told
	at := s.start.Add(3 * time.Second)

	type flight struct {
		session, account string
		sent, first      time.Duration
		verdict          string
	}
	var got []flight
	for _, e := range flying(told, at) {
		if e.Kind != router.StreamInFlight || !e.At.Equal(at) {
			t.Errorf("in flight is told as %+v, want an inflight event, as the reader joins", e)
		}
		f := flight{session: e.Session[:4], account: e.Account, sent: e.SentAt.Sub(s.start), verdict: e.Verdict}
		if !e.FirstAt.IsZero() {
			f.first = e.FirstAt.Sub(s.start)
		}
		got = append(got, f)
	}
	// 7f3a's request, refused on work at 2.4s, went out again on side then,
	// a verdict on it there yet to come.
	want := []flight{
		{session: "d28c", account: "work", sent: time.Second, first: 2100 * time.Millisecond},
		{session: "db8a", account: "work", sent: 1500 * time.Millisecond, first: 2200 * time.Millisecond},
		{session: "c61b", account: "side", sent: 2200 * time.Millisecond, first: 2900 * time.Millisecond},
		{session: "7f3a", account: "side", sent: 2400 * time.Millisecond},
	}
	if !slices.Equal(got, want) {
		t.Errorf("in flight at 3s: %+v, want %+v, the first sent first", got, want)
	}
}

// testScenario is a scenario of the samples' work and side, side the best,
// starting at the fixtures' moment, in UTC, with the cues given.
func testScenario(cues ...cue) Scenario {
	nord, _ := theme.Builtin(theme.DefaultDark)
	return Scenario{
		Name: "test", Size: wide(34), start: moment(time.UTC), cues: cues, theme: nord,
		setting: func(now time.Time) world {
			return world{samples: []sample{work(now), side(now)}, best: "side", events: lately(now)[:1]}
		},
	}
}

// listed is the session with the given id among those listed, failing the
// test where it isn't one, or has no assignment.
func listed(t *testing.T, sessions []status.Session, id string) status.Session {
	t.Helper()
	i := slices.IndexFunc(sessions, func(l status.Session) bool { return l.ID == id })
	if i < 0 || len(sessions[i].Assignments) == 0 {
		t.Fatalf("the router lists %+v, want %s among them", sessions, status.ShortID(id))
	}
	return sessions[i]
}

// near reports whether a and b are the same, but for rounding.
func near(a, b float64) bool {
	return a-b < 1e-9 && b-a < 1e-9
}
