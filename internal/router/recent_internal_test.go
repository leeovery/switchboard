package router

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

func TestEachEventAsTheDocumentGivesIt(t *testing.T) {
	until := start.Add(time.Hour)
	tests := []struct {
		name  string
		event Event
		want  status.Event
	}{
		{
			name:  "a session started",
			event: SessionStarted{Session: "0b5c6f2e", Model: opus, Account: "work", Reason: "new"},
			want:  status.Event{Kind: status.EventStarted, Account: "work", Session: "0b5c6f2e", Model: opus, Reason: "new"},
		},
		{
			name:  "a limit reached",
			event: LimitReached{Account: "work", Windows: []string{"5h"}, Until: until, Limit: 3},
			want:  status.Event{Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}, Until: until, Limit: 3},
		},
		{
			name:  "a limit reached in no window named",
			event: LimitReached{Account: "work", Until: until, Limit: 3},
			want:  status.Event{Kind: status.EventLimit, Account: "work", Until: until, Limit: 3},
		},
		{
			name:  "a move",
			event: Moved{Session: "0b5c6f2e", Model: opus, From: "work", To: "side", Reason: "moved by pin"},
			want:  status.Event{Kind: status.EventMoved, Session: "0b5c6f2e", Model: opus, From: "work", To: "side", Reason: "moved by pin"},
		},
		{
			name:  "a token refused",
			event: Refused{Account: "work", Status: http.StatusUnauthorized, Until: until},
			want:  status.Event{Kind: status.EventRefused, Account: "work", Until: until, Status: http.StatusUnauthorized},
		},
		{
			name:  "a request refused alone",
			event: Refused{Account: "work", Status: http.StatusForbidden, Family: "opus", Until: until},
			want:  status.Event{Kind: status.EventRefused, Account: "work", Until: until, Status: http.StatusForbidden, Family: "opus"},
		},
		{
			name:  "the router turning unhealthy",
			event: HealthChanged{Reason: "5 of the 5 requests in the last 5 minutes failed"},
			want:  status.Event{Kind: status.EventHealth, Reason: "5 of the 5 requests in the last 5 minutes failed"},
		},
		{
			name:  "the router healthy again",
			event: HealthChanged{Healthy: true},
			want:  status.Event{Kind: status.EventHealth},
		},
		{
			name:  "a prime",
			event: Primed{Account: "side", Window: "5h", ResetsAt: until},
			want:  status.Event{Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}, Until: until},
		},
		{
			name:  "a restart falling due",
			event: RestartDue{Reason: "upgraded"},
			want:  status.Event{Kind: status.EventRestart, Reason: "upgraded"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRecent(&testClock{now: start})
			r.hear(tt.event)
			want := tt.want
			want.ID, want.At = 1, start
			if got := r.events(); !reflect.DeepEqual(got, []status.Event{want}) {
				t.Errorf("events() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestTheNewestFiftyEventsAreKeptNewestFirst(t *testing.T) {
	clock := &testClock{now: start}
	r := newTestRecent(clock)
	if got := r.events(); got != nil {
		t.Fatalf("before any event, events() = %+v, want none", got)
	}
	for i := 1; i <= 52; i++ {
		clock.now = start.Add(time.Duration(i) * time.Second)
		r.hear(RestartDue{Reason: strconv.Itoa(i)})
	}

	got := r.events()
	if len(got) != 50 {
		t.Fatalf("events() gives %d events, want the newest 50", len(got))
	}
	for i, e := range got {
		id := 52 - i
		if e.ID != id || e.Reason != strconv.Itoa(id) || !e.At.Equal(start.Add(time.Duration(id)*time.Second)) {
			t.Errorf("events()[%d] = %+v, want the one heard with id %d", i, e, id)
		}
	}
}

func TestTheEventsGivenAreACopy(t *testing.T) {
	r := newTestRecent(&testClock{now: start})
	r.hear(Primed{Account: "side", Window: "5h", ResetsAt: start.Add(5 * time.Hour)})
	r.hear(LimitReached{Account: "work", Windows: []string{"5h"}, Until: start.Add(time.Hour), Limit: 1})

	given := r.events()
	for i := range given {
		given[i].Windows[0] = "7d"
	}
	r.hear(LimitReached{Account: "work", Windows: []string{"7d_oi", "5h"}, Until: start.Add(time.Hour), Limit: 1, Again: true})
	want := []status.Event{
		{ID: 2, At: start, Kind: status.EventLimit, Account: "work", Windows: []string{"5h", "7d_oi"}, Until: start.Add(time.Hour), Limit: 1},
		{ID: 1, At: start, Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}, Until: start.Add(5 * time.Hour)},
	}
	if got := r.events(); !reflect.DeepEqual(got, want) {
		t.Errorf("events() = %+v, want %+v: untouched by what was done with those given before", got, want)
	}
}

func TestALimitsEventCountsTheSessionsItMoved(t *testing.T) {
	tests := []struct {
		name      string
		moves     []Moved
		wantCount int
		wantTo    string
		// counted is set where the limit counts each move, which then names
		// the limit's event.
		counted bool
	}{
		{
			name:      "each session once, whatever its models",
			moves:     []Moved{forced("one", "work", "side"), forcedModel("one", haiku, "work", "side"), forced("two", "work", "side")},
			wantCount: 2, wantTo: "side", counted: true,
		},
		{
			name:      "to several accounts, naming none",
			moves:     []Moved{forced("one", "work", "side"), forced("two", "work", "personal")},
			wantCount: 2, counted: true,
		},
		{
			name:  "but a move by choice",
			moves: []Moved{{Session: "one", Model: opus, From: "work", To: "side", Reason: "moved by pin"}},
		},
		{
			name:  "but a move off it the limit didn't hold back, as at its reserve",
			moves: []Moved{{Session: "one", Model: opus, From: "work", To: "side", Reason: "moved: work is at its reserve"}},
		},
		{
			name:  "but a move another limit forced",
			moves: []Moved{forcedBy(2, "one", "side", "work")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRecent(&testClock{now: start})
			r.hear(LimitReached{Account: "work", Windows: []string{"5h"}, Until: start.Add(time.Hour), Limit: 1})
			for _, m := range tt.moves {
				r.hear(m)
			}

			got := r.events()
			if len(got) != 1+len(tt.moves) {
				t.Fatalf("events() = %+v, want the limit and each move", got)
			}
			want := status.Event{ID: 1, At: start, Kind: status.EventLimit, Account: "work", To: tt.wantTo, Windows: []string{"5h"}, Until: start.Add(time.Hour), Count: tt.wantCount, Limit: 1}
			if limit := got[len(got)-1]; !reflect.DeepEqual(limit, want) {
				t.Errorf("the limit's event = %+v, want %+v", limit, want)
			}
			for i, m := range tt.moves {
				want := status.Event{ID: 2 + i, At: start, Kind: status.EventMoved, Session: m.Session, Model: m.Model, From: m.From, To: m.To, Reason: m.Reason}
				if tt.counted {
					want.Limit = 1
				}
				if moved := got[len(got)-2-i]; !reflect.DeepEqual(moved, want) {
					t.Errorf("move %d's event = %+v, want %+v", i+1, moved, want)
				}
			}
		})
	}
}

func TestALimitReachedAgainJoinsItsEventInPlace(t *testing.T) {
	clock := &testClock{now: start}
	r := newTestRecent(clock)
	r.hear(LimitReached{Account: "work", Windows: []string{"5h"}, Until: start.Add(time.Hour), Limit: 1})
	r.hear(LimitReached{Account: "side", Windows: []string{"5h"}, Until: start.Add(2 * time.Hour), Limit: 2})
	clock.now = start.Add(time.Minute)
	r.hear(LimitReached{Account: "work", Windows: []string{"7d", "5h"}, Until: start.Add(2 * 24 * time.Hour), Limit: 1, Again: true})
	// Moved after the limit's first reset, as it now holds till its later one.
	clock.now = start.Add(2 * time.Hour)
	r.hear(forced("one", "work", "side"))

	want := []status.Event{
		{ID: 3, At: clock.now, Kind: status.EventMoved, Session: "one", Model: opus, From: "work", To: "side", Reason: "moved: work hit its limit", Limit: 1},
		{ID: 2, At: start, Kind: status.EventLimit, Account: "side", Windows: []string{"5h"}, Until: start.Add(2 * time.Hour), Limit: 2},
		{ID: 1, At: start, Kind: status.EventLimit, Account: "work", To: "side", Windows: []string{"5h", "7d"}, Until: start.Add(2 * 24 * time.Hour), Count: 1, Limit: 1},
	}
	if got := r.events(); !reflect.DeepEqual(got, want) {
		t.Errorf("events() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestANewLimitIsAnEventOfItsOwn(t *testing.T) {
	tests := []struct {
		name string
		// after is how long after the first limit the second is reached, and
		// windows the windows it's reached in.
		after   time.Duration
		windows []string
	}{
		{name: "once the first has lifted", after: time.Hour, windows: []string{"5h"}},
		{name: "while the first holds, naming windows it names none of", after: time.Minute, windows: []string{"7d_oi"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			r := newTestRecent(clock)
			r.hear(LimitReached{Account: "work", Windows: []string{"7d"}, Until: start.Add(5 * time.Minute), Limit: 1})
			clock.now = start.Add(tt.after)
			r.hear(LimitReached{Account: "work", Windows: tt.windows, Until: start.Add(2 * time.Hour), Limit: 2})
			r.hear(forcedBy(2, "one", "work", "side"))

			got := r.events()
			if len(got) != 3 {
				t.Fatalf("events() = %+v, want both limits and the move", got)
			}
			want := []status.Event{
				{ID: 2, At: clock.now, Kind: status.EventLimit, Account: "work", To: "side", Windows: tt.windows, Until: start.Add(2 * time.Hour), Count: 1, Limit: 2},
				{ID: 1, At: start, Kind: status.EventLimit, Account: "work", Windows: []string{"7d"}, Until: start.Add(5 * time.Minute), Limit: 1},
			}
			if !reflect.DeepEqual(got[1:], want) {
				t.Errorf("the limits' events = %+v, want %+v: the move counted by the limit that forced it", got[1:], want)
			}
			if got[0].Limit != 2 {
				t.Errorf("the move names the limit event %d, want 2, the latest's", got[0].Limit)
			}
		})
	}
}

func TestALimitsNewsJoinsItsEventWhateverOrderItComesIn(t *testing.T) {
	clock := &testClock{now: start}
	r := newTestRecent(clock)
	// A session's request, reaching work's limit after another's did, tells
	// of it reached again, and a session it moved, before the first news of
	// it comes, and of side's limit.
	r.hear(LimitReached{Account: "work", Windows: []string{"5h", "7d"}, Until: start.Add(2 * time.Hour), Limit: 2, Again: true})
	r.hear(forcedBy(2, "two", "work", "side"))
	r.hear(forcedBy(3, "three", "side", "work"))
	clock.now = start.Add(time.Second)
	r.hear(LimitReached{Account: "work", Windows: []string{"5h"}, Until: start.Add(time.Hour), Limit: 2})
	r.hear(forcedBy(2, "one", "work", "side"))

	want := []status.Event{
		{ID: 4, At: clock.now, Kind: status.EventMoved, Session: "one", Model: opus, From: "work", To: "side", Reason: "moved: work hit its limit", Limit: 1},
		{ID: 3, At: start, Kind: status.EventMoved, Session: "three", Model: opus, From: "side", To: "work", Reason: "moved: side hit its limit"},
		{ID: 2, At: start, Kind: status.EventMoved, Session: "two", Model: opus, From: "work", To: "side", Reason: "moved: work hit its limit", Limit: 1},
		{ID: 1, At: start, Kind: status.EventLimit, Account: "work", To: "side", Windows: []string{"5h", "7d"}, Until: start.Add(2 * time.Hour), Count: 2, Limit: 2},
	}
	if got := r.events(); !reflect.DeepEqual(got, want) {
		t.Errorf("events() =\n%+v\nwant\n%+v: one limit, counting both its moves", got, want)
	}

	// Side's limit's news comes last, and counts the move it forced.
	r.hear(LimitReached{Account: "side", Windows: []string{"5h"}, Until: start.Add(time.Hour), Limit: 3})
	if got := r.events(); got[0].Kind != status.EventLimit || got[0].Count != 1 || got[0].To != "work" || got[2].Limit != got[0].ID {
		t.Errorf("events() =\n%+v\nwant side's limit counting the move it forced, which names its event", got)
	}
}

func TestARefusalsEventEndsAsItLifts(t *testing.T) {
	const first, second = "a1b2c3d4", "e5f6a7b8"
	clock := &testClock{now: start}
	r := newTestRecent(clock)
	until := start.Add(refusedFor)
	r.hear(Refused{Account: "work", Status: http.StatusUnauthorized, Until: until, Request: first})
	r.hear(Refused{Account: "work", Status: http.StatusForbidden, Family: "opus", Until: until, Request: first})
	r.hear(Refused{Account: "work", Status: http.StatusForbidden, Family: "opus", Until: until, Request: second})
	r.hear(Refused{Account: "side", Status: http.StatusForbidden, Family: "opus", Until: until, Request: second})

	// Work goes out on another token two minutes on, and the second request
	// is refused on every account it went out on a minute later.
	clock.now = start.Add(2 * time.Minute)
	r.hear(RefusalLifted{Account: "work"})
	clock.now = start.Add(3 * time.Minute)
	r.hear(RefusalLifted{Account: "work", Family: "opus", Request: second})
	r.hear(RefusalLifted{Account: "side", Family: "opus", Request: second})
	// Long after, a token's refusal that's no longer in force lifts again.
	clock.now = start.Add(time.Hour)
	r.hear(RefusalLifted{Account: "work"})

	want := []status.Event{
		{ID: 4, At: start, Kind: status.EventRefused, Account: "side", Until: start.Add(3 * time.Minute), Status: http.StatusForbidden, Family: "opus"},
		{ID: 3, At: start, Kind: status.EventRefused, Account: "work", Until: start.Add(3 * time.Minute), Status: http.StatusForbidden, Family: "opus"},
		{ID: 2, At: start, Kind: status.EventRefused, Account: "work", Until: until, Status: http.StatusForbidden, Family: "opus"},
		{ID: 1, At: start, Kind: status.EventRefused, Account: "work", Until: start.Add(2 * time.Minute), Status: http.StatusUnauthorized},
	}
	if got := r.events(); !reflect.DeepEqual(got, want) {
		t.Errorf("events() =\n%+v\nwant\n%+v: each refusal that lifted ending as it did, and the first request's refusal of Opus standing", got, want)
	}
}

func TestALimitReachedAgainWhoseEventIsNoLongerKeptIsDropped(t *testing.T) {
	limit := LimitReached{Account: "work", Windows: []string{"5h"}, Until: start.Add(time.Hour), Limit: 1}
	r := newTestRecent(&testClock{now: start})
	r.hear(limit)
	for range maxEvents {
		r.hear(RestartDue{Reason: "upgraded"})
	}
	before := r.events()

	again := limit
	again.Windows, again.Again = []string{"5h", "7d"}, true
	r.hear(again)
	r.hear(forced("one", "work", "side"))
	got := r.events()
	if !reflect.DeepEqual(got[1:], before[:len(before)-1]) || got[0].Kind != status.EventMoved || got[0].Limit != 0 {
		t.Errorf("events() = %+v, want them as they were, but for the move, counted in no limit: %+v", got, before)
	}
}

func TestComingUnderPressureIsNewsOnceAReset(t *testing.T) {
	reset := start.Add(3 * time.Hour)
	next := reset.Add(5 * time.Hour)
	tests := []struct {
		name string
		// shown are how work stands at each look: under pressure or not, and
		// when its session resets.
		shown []pressing
		// want are the looks, by their place, that tell of pressure.
		want []int
	}{
		{
			name:  "once it comes under it",
			shown: []pressing{{false, reset}, {true, reset}, {true, reset}},
			want:  []int{1},
		},
		{
			name:  "once, though it goes in and out of it",
			shown: []pressing{{false, reset}, {true, reset}, {false, reset}, {true, reset}},
			want:  []int{1},
		},
		{
			name:  "again, once its window has reset",
			shown: []pressing{{false, reset}, {true, reset}, {false, next}, {true, next}},
			want:  []int{1, 3},
		},
		{
			name:  "not as it's first looked at, nor until its window resets",
			shown: []pressing{{true, reset}, {false, reset}, {true, reset}, {false, next}, {true, next}},
			want:  []int{4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRecent(&testClock{now: start})
			var told []int
			for i, p := range tt.shown {
				news := r.news([]status.Account{p.work()}, nil)
				if len(news) == 0 {
					continue
				}
				told = append(told, i)
				want := status.Event{Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: start.Add(time.Hour), Since: start.Add(-30 * time.Minute)}
				if !reflect.DeepEqual(news, []status.Event{want}) {
					t.Errorf("look %d: news() = %+v, want %+v", i, news, want)
				}
			}
			if !slices.Equal(told, tt.want) {
				t.Errorf("the looks that told of pressure = %v, want %v", told, tt.want)
			}
		})
	}
}

func TestAnAccountsFirstReadingUnderPressureIsNoNews(t *testing.T) {
	r := newTestRecent(&testClock{now: start})
	reset := start.Add(3 * time.Hour)
	next := reset.Add(5 * time.Hour)
	// Work is yet to be read, then read under pressure, then not as its
	// window resets, then under it again.
	looks := []status.Account{{ID: "work"}, pressing{true, reset}.work(), pressing{false, next}.work(), pressing{true, next}.work()}

	var told []int
	for i, a := range looks {
		if len(r.news([]status.Account{a}, nil)) > 0 {
			told = append(told, i)
		}
	}
	if want := []int{3}; !slices.Equal(told, want) {
		t.Errorf("the looks that told of pressure = %v, want %v: work's first reading is no news", told, want)
	}
}

func TestAHealthTurnIsToldOfWithinALook(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newTestRouter(t, func() time.Time { return time.Now().UTC() }, &stubProber{})
		began := time.Now().UTC()
		for range minFailures {
			r.health.record(began, true)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			r.recent.run(ctx)
		}()
		defer func() {
			cancel()
			<-done
		}()

		// The failures leave the router's window of health, with no request
		// since, and no one asking after it.
		time.Sleep(healthWindow + lookEvery)
		synctest.Wait()
		got := r.recent.events()
		if len(got) != 2 || got[0].Kind != status.EventHealth || got[0].Reason != "" || got[0].At.Before(began.Add(healthWindow)) || got[0].At.After(began.Add(healthWindow+lookEvery)) {
			t.Errorf("events() = %+v, want the router healthy again, told of within a look of the failures leaving its window, at %v", got, began.Add(healthWindow))
		}
	})
}

// pressing is how work stands under pressure at a look: whether it's under
// it, and when the session it's of resets.
type pressing struct {
	under  bool
	resets time.Time
}

// work is work as the document gives it, as pressing has it stand: running
// out an hour after start, at its rate over the half hour before it.
func (p pressing) work() status.Account {
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.6, ResetsAt: p.resets}
	return status.Account{
		ID:       "work",
		Windows:  []quota.Window{session},
		Pressure: status.Pressure{Window: "5h", Rate: 0.4, Recent: true, Since: start.Add(-30 * time.Minute), RunsOut: start.Add(time.Hour), Under: p.under},
	}
}

func TestPressureIsJudgedAsTheDocumentJudgesItWithTheGlobalPin(t *testing.T) {
	clock := &testClock{now: start}
	reserving := slices.Clone(testConfigured)
	reserving[0].Reserve = 0.2
	s := newState(resolve(reserving, testTokens.Read), testPolicy, claude.Provider{}.Family, clock.read, unkept, unkept)
	sessions := newSessions(clock.read, unkept, unkept)
	r := newRecent(s, sessions, clock.read)
	// At the pace its use since it started sets, work's session reaches its
	// reserve before it resets, but not its limit, which a pin spends to.
	used := quota.Window{Key: "5h", Label: "Session", Utilization: 0.6, ResetsAt: start.Add(90 * time.Minute), Status: quota.StatusAllowed}
	roomy := quota.Window{Key: "7d", Label: "Week", Utilization: 0.5, ResetsAt: start.Add(3 * day), Status: quota.StatusAllowed}
	s.record("work", []quota.Window{used, roomy}, s.mark())

	sessions.setPin(status.Pin{Accounts: []string{"work"}, Since: start}, false)
	r.look()
	r.look()
	if got := r.events(); got != nil {
		t.Fatalf("with work pinned, events() = %+v, want none: it isn't under pressure", got)
	}
	sessions.unpin(false)
	r.look()

	doc := s.document()
	work, _ := doc.Account("work")
	if !work.Pressure.Under {
		t.Fatalf("unpinned, work's pressure reads %+v, want it under pressure", work.Pressure)
	}
	want := []status.Event{{ID: 1, At: start, Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: work.Pressure.RunsOut}}
	if got := r.events(); !reflect.DeepEqual(got, want) {
		t.Errorf("once unpinned, events() = %+v, want %+v, as the document reads it", got, want)
	}
}

func TestRoomAgainOnceALimitLifts(t *testing.T) {
	clock := &testClock{now: start}
	r := newTestRecent(clock)
	r.state.record("work", []quota.Window{session, week}, r.state.mark())
	r.look()
	r.state.limit("work", nil, start.Add(time.Minute), r.state.mark())
	r.look()
	if got := r.events(); got != nil {
		t.Fatalf("while work's limit holds, events() = %+v, want none", got)
	}

	clock.now = start.Add(time.Minute)
	r.look()
	r.look()
	want := []status.Event{{ID: 1, At: clock.now, Kind: status.EventRoom, Account: "work"}}
	if got := r.events(); !reflect.DeepEqual(got, want) {
		t.Errorf("once work's limit lifts, events() = %+v, want %+v", got, want)
	}
}

func TestTheFirstLookKeepsNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newRecording(t)
		// Work's session reaches its limit before it resets, at the pace its
		// use since it started sets; side's limit holds a minute more.
		h.state.record("work", []quota.Window{h.session(0.9, 90*time.Minute), h.week(0.5)}, h.state.mark())
		h.state.record("side", []quota.Window{h.session(0.2, 5*time.Hour), h.week(0.5)}, h.state.mark())
		h.state.limit("side", nil, h.began.Add(time.Minute), h.state.mark())
		if work, _ := h.state.document().Account("work"); !work.Pressure.Under {
			t.Fatalf("work's pressure reads %+v, want it under pressure", work.Pressure)
		}
		h.start()
		h.expect()

		h.after(time.Minute)
		h.expect(status.Event{ID: 1, At: h.began.Add(time.Minute), Kind: status.EventRoom, Account: "side"})
	})
}

func TestTheStoreLooksAgainAsItHearsAnEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newRecording(t)
		h.state.record("work", []quota.Window{h.session(0.2, 5*time.Hour), h.week(0.5)}, h.state.mark())
		h.start()
		h.state.limit("work", nil, h.began.Add(20*time.Second), h.state.mark())
		h.after(lookEvery)
		h.expect()

		// The limit lifts, and before the next look's due, the store hears an
		// event.
		h.after(10 * time.Second)
		h.r.hear(RestartDue{Reason: "upgraded"})
		synctest.Wait()
		at := h.began.Add(25 * time.Second)
		h.expect(
			status.Event{ID: 2, At: at, Kind: status.EventRoom, Account: "work"},
			status.Event{ID: 1, At: at, Kind: status.EventRestart, Reason: "upgraded"},
		)
	})
}

func TestEventsAreKeptWhateverTheNotificationsAreSetTo(t *testing.T) {
	tests := []struct {
		name          string
		notifications config.Notifications
	}{
		{name: "posting none"},
		{name: "posting every one", notifications: config.Notifications{Limits: true, Room: true, Warning: 0.9, Moves: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(Config{
				Accounts:      testConfigured,
				Token:         testTokens.Read,
				Upstream:      "http://127.0.0.1:1",
				Provider:      claude.Provider{},
				Prober:        &stubProber{},
				Policy:        testPolicy,
				Now:           at(start),
				Notifier:      &noting{},
				Notifications: tt.notifications,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			for range minFailures {
				r.health.record(start, true)
			}
			want := []status.Event{{ID: 1, At: start, Kind: status.EventHealth, Reason: "5 of the 5 requests in the last 5 minutes failed"}}
			if got := r.Status().Events; !reflect.DeepEqual(got, want) {
				t.Errorf("once the router turns unhealthy, the document's events = %+v, want %+v", got, want)
			}
		})
	}
}

// newTestRecent keeps the events of a router over testAccounts, on clock's
// time.
func newTestRecent(clock *testClock) *recent {
	return newRecent(newTestState(clock), newSessions(clock.read, unkept, unkept), clock.read)
}

// recording is the store at work in a synctest bubble, over the state of
// testAccounts, on the bubble's clock read in UTC.
type recording struct {
	t     *testing.T
	state *state
	r     *recent
	// began is when the clock began.
	began time.Time
}

func newRecording(t *testing.T) *recording {
	t.Helper()
	now := func() time.Time { return time.Now().UTC() }
	s := newState(testAccounts(), testPolicy, claude.Provider{}.Family, now, unkept, unkept)
	return &recording{t: t, state: s, r: newRecent(s, newSessions(now, unkept, unkept), now), began: now()}
}

// start runs the store's looks until the test ends, returning once its first
// is done.
func (h *recording) start() {
	ctx, cancel := context.WithCancel(h.t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.r.run(ctx)
	}()
	h.t.Cleanup(func() {
		cancel()
		<-done
	})
	synctest.Wait()
}

// after lets d pass, and the store look at all it brings.
func (h *recording) after(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
}

// expect checks the events kept, the newest first.
func (h *recording) expect(want ...status.Event) {
	h.t.Helper()
	if got := h.r.events(); !reflect.DeepEqual(got, want) {
		h.t.Errorf("events() = %+v, want %+v", got, want)
	}
}

// session is a five-hour window used as given, which resets left after the
// clock began.
func (h *recording) session(used float64, left time.Duration) quota.Window {
	return quota.Window{Key: "5h", Label: "Session", Utilization: used, ResetsAt: h.began.Add(left), Status: quota.StatusAllowed}
}

// week is a week used as given, which resets three days after the clock
// began.
func (h *recording) week(used float64) quota.Window {
	return quota.Window{Key: "7d", Label: "Week", Utilization: used, ResetsAt: h.began.Add(3 * day), Status: quota.StatusAllowed}
}
