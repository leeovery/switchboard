package capture

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestScenarioNamesNameEveryScenarioOnceSorted(t *testing.T) {
	names := ScenarioNames()
	if want := []string{"dashboard", "repatch", "routing"}; !slices.Equal(names, want) {
		t.Errorf("ScenarioNames() = %v, want %v", names, want)
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
		{name: "usage", want: `unknown scenario "usage" ` + available},
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
	for _, s := range scenarios() {
		t.Run(s.Name, func(t *testing.T) {
			b := s.based()
			first, again := b.played(nil, time.Time{}), b.played(nil, time.Time{})
			if len(first.told) == len(b.stream) {
				t.Fatal("its router tells of nothing")
			}
			if !slices.IsSortedFunc(first.told, func(a, b router.StreamEvent) int { return a.At.Compare(b.At) }) {
				t.Error("its router tells of what befalls its requests out of order")
			}
			end := b.start.Add(time.Minute)
			if !reflect.DeepEqual(first.told, again.told) || !reflect.DeepEqual(first.world.document(end), again.world.document(end)) {
				t.Error("played twice, it played otherwise the second time")
			}
		})
	}
}

// A scenario's world reads its base in place: the windows as they run as it
// starts are all a document of it needs, so, nothing played yet, its router
// gives the document, the history and the sessions its world gives, read
// fresh then.
func TestAScenariosBaseGivesWhatItsWorldGives(t *testing.T) {
	for _, s := range scenarios() {
		t.Run(s.Name, func(t *testing.T) {
			b := s.based()
			whole := b.world
			whole.fresh = true
			want := routerOf(whole, b.start)
			w := b.startingOut()
			if got := w.document(b.start); !reflect.DeepEqual(got, want.doc) {
				t.Errorf("its router's document is\n%+v\nwant its world's\n%+v", got, want.doc)
			}
			for _, key := range []string{fiveHourKey, weekKey, fableKey} {
				tl, _ := timelineOf(s, 0)
				got, _ := tl.History(t.Context(), key, time.Minute)
				if wanted, _ := want.History(t.Context(), key, time.Minute); !reflect.DeepEqual(got, wanted) {
					t.Errorf("its router's history of %s is %+v, want its world's, %+v", key, got, wanted)
				}
			}
			if got := w.listing(b.start, b.stream); !sameSessions(got, want.sessions) {
				t.Errorf("its router lists %+v, want its world's, %+v", got, want.sessions)
			}
		})
	}
}

// sameSessions reports whether the sessions are the same, in any order.
func sameSessions(a, b []status.Session) bool {
	byID := func(x, y status.Session) int { return strings.Compare(x.ID, y.ID) }
	a, b = slices.Clone(a), slices.Clone(b)
	slices.SortFunc(a, byID)
	slices.SortFunc(b, byID)
	return reflect.DeepEqual(a, b)
}

func TestARequestIsToldOfAsTheRouterTellsOfIt(t *testing.T) {
	s := testScenario(asks(4, idDB8A, opus, brief))
	b := s.based()

	told := ofSession(b.played(nil, time.Time{}).told, idDB8A)

	var kinds []string
	for _, e := range told {
		kinds = append(kinds, e.Kind)
		if e.Request != told[0].Request || e.Model != opus || e.Account != "side" || e.Attempt != 1 {
			t.Errorf("the stream tells %+v, want each event of db8a's opus's request on side, on its first time upstream", e)
		}
	}
	want := []string{router.StreamSent, router.StreamFirst, router.StreamProgress, router.StreamProgress, router.StreamProgress, router.StreamProgress, router.StreamProgress, router.StreamDone}
	if !slices.Equal(kinds, want) {
		t.Fatalf("the stream tells %v, want %v", kinds, want)
	}
	if sent, first := told[0].At, told[1].At; !sent.Equal(b.start.Add(4*time.Second)) || first.Sub(sent) != brief.wait {
		t.Errorf("the request went at %s, its answer's first byte at %s, want 4s in, and the answer's wait after", sent, first)
	}
	if !slices.IsSortedFunc(told[2:7], func(a, b router.StreamEvent) int { return a.Chars - b.Chars }) || told[2].At.Sub(told[1].At) != progressEvery {
		t.Errorf("its progress is told as %+v, want it rising, every %s from its first byte", told[2:7], progressEvery)
	}
	done := told[len(told)-1]
	if done.Status != 200 || done.Chars != brief.chars || done.Tokens == nil || done.Tokens.Output != brief.chars/charsPerToken {
		t.Errorf("its end is told as %+v, want it answered, every character counted, and its tokens", done)
	}
}

// Each answer reads its account's 5-hour window and its week, used more the
// more it streams; a window is read again only where its use, in whole
// hundredths, has changed, as the readings history keeps it.
func TestAnAnswerIsReadOffForItsAccountsUse(t *testing.T) {
	hundredth := answer{wait: brief.wait, streams: brief.streams, chars: int(0.01 / sessionPerChar)}
	tests := []struct {
		name string
		a    answer
		// read are the windows read again, each at its use, side's 5-hour
		// window at 13% and its week at 78% as the scenario starts.
		read map[string]float64
	}{
		{name: "too brief to move a hundredth", a: brief, read: map[string]float64{}},
		{name: "a hundredth of its 5-hour window", a: hundredth, read: map[string]float64{fiveHourKey: .14}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testScenario(asks(4, idDB8A, opus, tt.a))
			b := s.based()
			first := b.start.Add(4*time.Second + tt.a.wait)
			before, after := b.played(nil, first.Add(-time.Millisecond)).world, b.played(nil, first).world

			read := make(map[string]float64)
			for _, r := range after.readings.read {
				if r.Account != "side" || !r.At.Equal(first) || r.Source != readings.FromAnswer {
					t.Errorf("the answer read %+v, want side's windows read as its first byte came, off it", r)
				}
				last, _ := before.lastRead("side", r.Key)
				if !r.ResetsAt.Equal(last.ResetsAt) {
					t.Errorf("the answer read side's %s to reset at %s, want as it ran, at %s", r.Key, r.ResetsAt, last.ResetsAt)
				}
				read[r.Key] = r.Utilization
			}
			if !reflect.DeepEqual(read, tt.read) {
				t.Errorf("the answer read %v, want %v", read, tt.read)
			}
			week := windowOf{account: "side", key: weekKey}
			if got, want := after.used[week]-before.used[week], float64(tt.a.chars)*sessionPerChar*weekPerSession; !near(got, want) {
				t.Errorf("the answer used %v of side's week, want %v of its 5-hour window's share", got, weekPerSession)
			}
		})
	}
}

func TestANewSessionStartsWhereNewSessionsGo(t *testing.T) {
	s := testScenario(starts(4, idB3E9, opus, "~/Code/cli", brief))
	b := s.based()

	sent := b.played(nil, b.start.Add(4*time.Second))
	if a := sent.world.seat(idB3E9, opus); a == nil || a.Account != "side" || a.Dir != "~/Code/cli" || a.Reason != status.ReasonNew {
		t.Fatalf("as its first request went, the session is listed %+v, want it new on side, the best, in its directory", a)
	}
	if slices.ContainsFunc(sent.world.events, func(e status.Event) bool { return e.Session == idB3E9 }) {
		t.Error("the router told of the session starting before it was answered")
	}
	answered := b.played(nil, b.start.Add(5*time.Second)).world
	if got := answered.events[0]; got.Kind != status.EventStarted || got.Session != idB3E9 || got.Account != "side" || got.Reason != status.ReasonNew {
		t.Errorf("once answered, the router tells of %+v, want b3e9 started on side", got)
	}
	if doc := answered.document(b.start.Add(5 * time.Second)); doc.Sessions != b.sessions+1 || doc.Accounts[2].Sessions != b.world.sessions["side"]+1 {
		t.Errorf("the router counts %d sessions, %d on side, want one more of each", doc.Sessions, doc.Accounts[2].Sessions)
	}
}

func TestALimitReachedHoldsTheAccountBackAndMovesItsSessions(t *testing.T) {
	s := testScenario(
		reaches(4, id41E0, sonnet, brief),
		asks(6, id9C2E, opus, brief),
	)
	b := s.based()
	refused := b.start.Add(4*time.Second + refuseAfter)
	played := b.played(nil, time.Time{}).world

	if l, held := played.limitOf("personal", b.start.Add(7*time.Second)); !held || l.ID != todaysLimit || !l.Until.Equal(personalResets(b)) {
		t.Errorf("personal's limit is %+v, holding %t, want the run's next, holding till its 5-hour window resets", l, held)
	}
	if r, _ := played.lastRead("personal", fiveHourKey); r.Utilization != 1 || !r.At.Equal(refused) || r.Status != "rejected" {
		t.Errorf("personal's 5-hour window was read %+v, want used in full as the request was refused", r)
	}
	for _, session := range []string{id41E0, id9C2E} {
		model := map[string]string{id41E0: sonnet, id9C2E: opus}[session]
		if a := played.seat(session, model); a.Account != "side" {
			t.Errorf("%s is on %s, want it moved to side", status.ShortID(session), a.Account)
		}
	}
	doc := played.document(b.start.Add(7 * time.Second))
	if doc.Sessions != b.sessions || doc.Accounts[1].Sessions != b.world.sessions["personal"] || doc.Accounts[2].Sessions != b.world.sessions["side"]+2 {
		t.Errorf("the router counts %d sessions, %d sent to personal and %d to side, want each once, as before, but 2 more sent to side",
			doc.Sessions, doc.Accounts[1].Sessions, doc.Accounts[2].Sessions)
	}
	limit := slices.IndexFunc(doc.Events, func(e status.Event) bool {
		return e.Kind == status.EventLimit && e.Account == "personal" && e.ID > b.events
	})
	if limit < 0 || doc.Events[limit].Count != 2 || doc.Events[limit].To != "side" {
		t.Fatalf("the router tells of %+v, want personal's limit, which moved 2 sessions to side", doc.Events)
	}
	var reasons []string
	for _, e := range doc.Events {
		if e.Kind == status.EventMoved && e.ID > b.events {
			if e.Limit != doc.Events[limit].ID || e.ForcedBy != doc.Events[limit].ID {
				t.Errorf("the move %+v isn't counted in the limit's event", e)
			}
			reasons = append(reasons, e.Reason)
		}
	}
	if want := []string{"moved: personal hit its limit", "moved: personal hit its limit"}; !slices.Equal(reasons, want) {
		t.Errorf("the moves are told of for %q, want %q", reasons, want)
	}
}

// personalResets is when personal's 5-hour window resets, as it runs as the
// base's scenario starts.
func personalResets(b *base) time.Time {
	return b.latest[windowOf{account: "personal", key: fiveHourKey}].ResetsAt
}

// A request that reaches a limit is refused, and the limit told of, as the
// refusal comes; the session moves, and its request goes out again, as the
// refusal's pulse comes home: as the frames at personal's limit draw it, on
// personal still 178ms after its refusal, on side 620ms after.
func TestARequestThatReachesALimitIsToldOfAsTheRouterTellsOfIt(t *testing.T) {
	s := testScenario(reaches(4, id41E0, sonnet, brief))
	b := s.based()
	sent := b.start.Add(4 * time.Second)
	refused := sent.Add(refuseAfter)

	var told []string
	for _, e := range ofSession(b.played(nil, time.Time{}).told, id41E0) {
		if e.At.Before(sent) {
			continue
		}
		told = append(told, fmt.Sprintf("%s %s %d %s", e.Kind, e.Account, e.Attempt, e.At.Sub(sent)))
		switch e.Kind {
		case router.StreamLimited:
			if e.Status != 429 {
				t.Errorf("the refusal is told as %+v, want a 429", e)
			}
		case router.StreamMoved:
			if e.From != "personal" || e.To != "side" || e.Reason != "moved: personal hit its limit" {
				t.Errorf("the move is told as %+v, want from personal to side, as personal hit its limit", e)
			}
		}
	}
	want := []string{"sent personal 1 0s", "limited personal 1 300ms", "moved side 1 840ms", "sent side 2 840ms", "first side 2 1.54s"}
	if len(told) < len(want) || !slices.Equal(told[:len(want)], want) || !strings.HasPrefix(told[len(told)-1], "done side 2") {
		t.Errorf("the stream tells %v, want %v, its answer on side, then done there", told, want)
	}
	for _, tt := range []struct {
		at      time.Time
		on      string
		counted int
	}{
		{at: refused.Add(178 * time.Millisecond), on: "personal", counted: 0},
		{at: refused.Add(620 * time.Millisecond), on: "side", counted: 1},
	} {
		w := b.played(nil, tt.at).world
		limit := slices.IndexFunc(w.events, func(e status.Event) bool { return e.Kind == status.EventLimit })
		if a := w.seat(id41E0, sonnet); a.Account != tt.on || limit < 0 || w.events[limit].Count != tt.counted {
			t.Errorf("%s after its refusal, 41e0 is on %s, the limit told of %+v, want it on %s, the limit counting %d",
				tt.at.Sub(refused), a.Account, w.events, tt.on, tt.counted)
		}
	}
}

func TestAPinSendsTheSessionThereFromItsNextRequest(t *testing.T) {
	s := testScenario(asks(2, idDB8A, opus, brief), asks(5, idDB8A, opus, brief))
	b := s.based()
	given := b.start.Add(3 * time.Second)
	pinned := order{at: given, give: func(w *world) { w.pinSession(idDB8A, "personal", given) }}

	before := b.played([]order{pinned}, b.start.Add(4*time.Second))
	if db8a := listed(t, before.world.listing(b.start.Add(4*time.Second), before.told), idDB8A); db8a.Pin != "personal" ||
		db8a.Assignments[0].Account != "side" || db8a.Assignments[0].Pinned {
		t.Errorf("before its next request, the router lists db8a as %+v, want it on side, where it was, its own pin to personal", db8a)
	}
	after := b.played([]order{pinned}, time.Time{})
	db8a := listed(t, after.world.listing(b.start.Add(6*time.Second), after.told), idDB8A)
	if a := db8a.Assignments[0]; a.Account != "personal" || a.Reason != status.ReasonPinned || !a.Pinned || !a.PinnedAt.Equal(given.UTC()) {
		t.Errorf("after its next request, the router lists db8a as %+v, want it on personal, pinned there since its pin was given", db8a)
	}
	moved := slices.IndexFunc(after.told, func(e router.StreamEvent) bool { return e.Kind == router.StreamMoved })
	if moved < 0 || !after.told[moved].At.Equal(b.start.Add(5*time.Second)) || after.told[moved+1].Kind != router.StreamSent || after.told[moved+1].Account != "personal" {
		t.Errorf("the stream tells %+v, want db8a's move to personal as its next request goes, then the request going out there", after.told)
	}
}

func TestTheGlobalPinSendsNewSessionsToTheBestOfItsAccounts(t *testing.T) {
	tests := []struct {
		name, best string
		pin        []string
		// limited is set where personal reaches its limit before the new
		// session starts.
		limited bool
		want    string
	}{
		{name: "the best, where the pin names it, though not first", best: "side", pin: []string{"personal", "side"}, want: "side"},
		{name: "the first it names, where it doesn't name the best", best: "side", pin: []string{"personal", "work"}, want: "personal"},
		{name: "the first it names with room", best: "side", pin: []string{"personal", "work"}, limited: true, want: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cues []cue
			if tt.limited {
				cues = append(cues, reaches(1, id41E0, sonnet, brief))
			}
			b := testScenario(append(cues, starts(3, idB3E9, opus, "~/Code/cli", brief))...).based()
			b.best = tt.best
			pinned := order{at: b.start, give: func(w *world) { w.setPin(tt.pin, false, b.start) }}

			played := b.played([]order{pinned}, time.Time{})
			if a := played.world.seat(idB3E9, opus); a.Account != tt.want || a.Reason != status.ReasonGlobalPin {
				t.Errorf("the new session started on %s, for %q, want on %s, as pinned", a.Account, a.Reason, tt.want)
			}
			if doc := played.world.document(b.start.Add(4 * time.Second)); doc.Best != tt.want || !slices.Equal(doc.Pin.Accounts, tt.pin) {
				t.Errorf("the document's best is %q, its pin %+v, want %s, and the pin of %v", doc.Best, doc.Pin, tt.want, tt.pin)
			}
		})
	}
}

func TestOrdersInTheSameWorldPlayInTurn(t *testing.T) {
	b := testScenario().based()
	at := b.start.Add(time.Second)
	orders := []order{
		{at: at, give: func(w *world) { w.setPin([]string{"personal"}, false, at) }},
		{at: at, give: func(w *world) { w.setPin([]string{"work"}, false, at) }},
	}

	if got := b.played(orders, time.Time{}).world.pin.Accounts; !slices.Equal(got, []string{"work"}) {
		t.Errorf("after two orders at the same moment, the pin names %q, want the last's, work", got)
	}
	if got := b.played(orders, b.start.Add(500*time.Millisecond)).world.pin; !got.IsZero() {
		t.Errorf("before either order, the pin is %+v, want the world's own, none", got)
	}
}

func TestFlyingAreTheRequestsInFlightAsTheRouterKeepsThem(t *testing.T) {
	s := testScenario(
		asks(4, idDB8A, opus, long),
		asks(4.5, id7F3A, sonnet, brief),
		reaches(5, id41E0, sonnet, long),
		asks(5.2, idC61B, opus, brief),
	)
	b := s.based()
	told := b.played(nil, time.Time{}).told
	at := b.start.Add(6 * time.Second)

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
		f := flight{session: e.Session[:4], account: e.Account, sent: e.SentAt.Sub(b.start), verdict: e.Verdict}
		if !e.FirstAt.IsZero() {
			f.first = e.FirstAt.Sub(b.start)
		}
		got = append(got, f)
	}
	// 41e0's request, refused on personal at 5.3s, went out again on side at
	// 5.84s, a verdict on it there yet to come.
	want := []flight{
		{session: "db8a", account: "side", sent: 4 * time.Second, first: 5100 * time.Millisecond},
		{session: "7f3a", account: "side", sent: 4500 * time.Millisecond, first: 5200 * time.Millisecond},
		{session: "c61b", account: "side", sent: 5200 * time.Millisecond, first: 5900 * time.Millisecond},
		{session: "41e0", account: "side", sent: 5840 * time.Millisecond},
	}
	if !slices.Equal(got, want) {
		t.Errorf("in flight at 6s: %+v, want %+v, the first sent first", got, want)
	}
}

// What was in flight as a scenario starts lands within its first 3 seconds,
// the first sent first: each answer streams on from what it had, and one
// still asking has its first byte come, read off for its account's use.
func TestTheRequestsInFlightAsItStartsLandWithinItsFirstSeconds(t *testing.T) {
	b := testScenario().based()
	flights := flying(b.stream, b.start)
	if len(flights) != 3 {
		t.Fatalf("in flight as it starts are %+v, want the auto world's three", flights)
	}
	told := b.played(nil, b.start.Add(landWithin)).told
	if still := flying(told, b.start.Add(landWithin)); len(still) > 0 {
		t.Errorf("in flight %s in are %+v, want none", landWithin, still)
	}
	for i, f := range flights {
		var done router.StreamEvent
		for _, e := range told {
			if e.Request == f.Request && e.Kind == router.StreamDone {
				done = e
			}
		}
		if ends := b.start.Add(landWithin * time.Duration(i+1) / 3); !done.At.Equal(ends) || done.Status != 200 || done.Chars != f.Chars+brief.chars {
			t.Errorf("%s's request ended %+v, want answered at %s, a brief answer more than its %d characters", f.Session[:4], done, ends, f.Chars)
		}
		first := slices.IndexFunc(told, func(e router.StreamEvent) bool {
			return e.Request == f.Request && e.Kind == router.StreamFirst && e.At.After(b.start)
		})
		if asking := f.FirstAt.IsZero(); asking != (first >= 0) {
			t.Errorf("%s's request had its first byte told %t, want it told only where it was still asking", f.Session[:4], first >= 0)
		}
	}
}

// A scenario's requests are numbered after every id its world's lines and
// stream use, so none is taken for another, as the router's ids never are.
func TestAScenariosRequestsAreNumberedAfterEveryIDItsWorldUses(t *testing.T) {
	for _, s := range scenarios() {
		t.Run(s.Name, func(t *testing.T) {
			b := s.based()
			taken := make(map[string]bool)
			var highest uint64
			for _, f := range b.records.lines {
				taken[f.Request], highest = true, max(highest, requestNumber(f.Request))
			}
			for _, e := range b.stream {
				taken[e.Request], highest = true, max(highest, requestNumber(e.Request))
			}
			numbered := make(map[string]bool)
			for _, e := range b.played(nil, time.Time{}).told {
				if e.Kind != router.StreamSent || e.Attempt != 1 || slices.ContainsFunc(b.stream, func(was router.StreamEvent) bool { return was.Request == e.Request }) {
					continue
				}
				if taken[e.Request] || numbered[e.Request] || requestNumber(e.Request) <= highest || len(e.Request) < 8 {
					t.Errorf("a request is numbered %s, want after %08x, every id its world uses, each its own", e.Request, highest)
				}
				numbered[e.Request] = true
			}
		})
	}
}

func TestACueOfASessionNoAccountHasPanicsNamingIt(t *testing.T) {
	const unknown = "0ddba11c-0000-4000-8000-000000000000"
	tests := []struct {
		name string
		cue  cue
		want string
	}{
		{name: "asks", cue: asks(1, unknown, opus, brief), want: "a scenario asks of 0ddba11c's claude-opus-5-5, which no account has"},
		{name: "reaches", cue: reaches(1, unknown, opus, brief), want: "a scenario has 0ddba11c's claude-opus-5-5 reach a limit, which no account has"},
		{name: "asks of a model it hasn't", cue: asks(1, idDB8A, haikuShort, brief), want: "a scenario asks of db8a71e3's " + haikuShort + ", which no account has"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if got := recover(); got != tt.want {
					t.Errorf("played, it panics with %v, want %q", got, tt.want)
				}
			}()
			testScenario(tt.cue).based().played(nil, time.Time{})
		})
	}
}

// ofSession are the events told of the session's requests.
func ofSession(told []router.StreamEvent, session string) []router.StreamEvent {
	return slices.DeleteFunc(slices.Clone(told), func(e router.StreamEvent) bool { return e.Session != session })
}

// testScenario is a scenario over the auto world, from the worlds' moment,
// with the cues given, at the frames' size, in nord.
func testScenario(cues ...cue) Scenario {
	return Scenario{Name: "test", Size: wide(37), worlds: worldsNow, world: accountWorlds.auto, cues: cues, theme: framesTheme()}
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
	return a-b < 1e-12 && b-a < 1e-12
}
