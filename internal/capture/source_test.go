package capture

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestTheRouterGivesItsDocumentWhateverIsAsked(t *testing.T) {
	s, now := autoRouter()
	want := s.doc.WorkedOut(claude.Policy())
	if want.ComingUp == nil || want.Pool.Windows == nil {
		t.Fatalf("WorkedOut() = %+v, want what's coming up and the pool", want)
	}

	for _, r := range []status.Read{{}, {Probe: true}, status.Fresh()} {
		doc, health, err := s.Read(t.Context(), r)
		if err != nil || !reflect.DeepEqual(doc, want) {
			t.Errorf("Read(%+v) = %+v, %v, want the router's document, with what's worked out of it, as the router gives it", r, doc, err)
		}
		if !health.OK || health.PID != routerPID || !health.StartedAt.Equal(routerRun(now)) {
			t.Errorf("Read(%+v) says the router is %+v, want the one the fixtures have, healthy, started as its events' run did", r, health)
		}
	}
	if err := s.Pin(t.Context(), []string{"work"}, true); err != nil {
		t.Errorf("Pin() error = %v, want the order taken", err)
	}
	if err := s.Unpin(t.Context()); err != nil {
		t.Errorf("Unpin() error = %v, want the order taken", err)
	}
	if err := s.PinSession(t.Context(), idD28C, "side"); err != nil {
		t.Errorf("PinSession() error = %v, want the order taken", err)
	}
	if err := s.UnpinSession(t.Context(), idD28C); err != nil {
		t.Errorf("UnpinSession() error = %v, want the order taken", err)
	}
	if doc, _, _ := s.Read(t.Context(), status.Read{}); !reflect.DeepEqual(doc, want) {
		t.Errorf("after orders, Read() = %+v, want the document as it was", doc)
	}
	if !s.RouterAnswers(t.Context()) {
		t.Error("RouterAnswers() = false, want the router answering")
	}
}

func TestAWorldsRouterGivesItsDocumentAndListsItsSessions(t *testing.T) {
	worlds := worldsNow()
	tests := []struct {
		name     string
		world    accountWorld
		sessions int
		// listed are how many sessions the router lists on each account.
		listed map[string]int
	}{
		{name: "auto", world: worlds.auto(), sessions: 6, listed: map[string]int{"personal": 2, "side": 4}},
		{name: "routing's limit, personal's moved to side", world: worlds.limit(), sessions: 6, listed: map[string]int{"side": 6}},
		{name: "work alone, as usage's frame counts its sessions", world: worlds.alone(), sessions: 6, listed: map[string]int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := routerOf(tt.world, worlds.now)
			doc, health, err := router.Read(t.Context(), status.Read{Probe: true})
			if want := tt.world.document(worlds.now); err != nil || !reflect.DeepEqual(doc, want) {
				t.Errorf("Read() = %+v, %v, want the world's document, as the router gives it", doc, err)
			}
			if !health.OK || health.PID != routerPID || !health.StartedAt.Equal(routerRun(worlds.now)) {
				t.Errorf("Read() says the router is %+v, want the one the fixtures have, healthy, started as its events' run did", health)
			}
			if doc.Sessions != tt.sessions {
				t.Errorf("the document counts %d sessions, want %d", doc.Sessions, tt.sessions)
			}
			sessions, err := router.Sessions(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			listed := map[string]int{}
			for _, s := range sessions {
				listed[s.Assignments[0].Account]++
			}
			if !maps.Equal(listed, tt.listed) {
				t.Errorf("the router lists sessions on %v, want %v", listed, tt.listed)
			}
		})
	}
}

func TestAGoneRouterAnswersUntilItsTimeThenFailsEveryRead(t *testing.T) {
	router, now := autoRouter()
	clock := &clock{now: ago(now, readAgo)}
	s := router.goneAt(now).by(clock.read)

	if _, _, err := s.Read(t.Context(), status.Read{}); err != nil || !s.RouterAnswers(t.Context()) {
		t.Errorf("before it's gone, Read() error = %v, RouterAnswers() = %v, want it read, and answering", err, s.RouterAnswers(t.Context()))
	}
	clock.now = now
	for _, r := range []status.Read{{}, {Probe: true}, status.Fresh()} {
		if _, _, err := s.Read(t.Context(), r); !errors.Is(err, errGone) {
			t.Errorf("once it's gone, Read(%+v) error = %v, want %v", r, err, errGone)
		}
	}
	if s.RouterAnswers(t.Context()) {
		t.Error("once it's gone, RouterAnswers() = true, want it not answering")
	}
	if _, _, err := router.by(clock.read).Read(t.Context(), status.Read{}); err != nil {
		t.Errorf("a router never gone, Read() error = %v, want it read", err)
	}
}

// Every window the charts draw a reading of has a history that ends at it:
// those whose reading falls in the span GRAPHS charts them over, as
// ChartSpan has it.
func TestEveryWindowsHistoryEndsAtItsUse(t *testing.T) {
	for _, f := range []Fixture{fixture(t, "overview-default-160")} {
		for _, a := range f.router.doc.At(f.router.doc.GeneratedAt, claude.Policy()).Accounts {
			for _, w := range a.Windows {
				if start, _, ok := dashboard.ChartSpan(w); !ok || w.Read.At.Before(start) {
					continue
				}
				h, err := f.router.History(t.Context(), w.Key, time.Minute)
				if err != nil {
					t.Fatal(err)
				}
				points := h.Accounts[slices.IndexFunc(h.Accounts, func(held router.AccountHistory) bool { return held.ID == a.ID })].Points
				if len(points) == 0 {
					t.Errorf("%s: %s's %s has no history", f.Name, a.ID, w.Key)
					continue
				}
				if last := points[len(points)-1].Utilization; last != w.Read.Used {
					t.Errorf("%s: %s's %s history ends at %v, want its use, %v", f.Name, a.ID, w.Key, last, w.Read.Used)
				}
			}
		}
	}
}

func TestHistoryRefusesAStepOfNothing(t *testing.T) {
	s, _ := autoRouter()
	if _, err := s.History(t.Context(), "5h", 0); err == nil {
		t.Error("History() with a step of 0 gave no error, want it refused")
	}
}

func TestEventsAreNumberedTheNewestFirst(t *testing.T) {
	for _, f := range []Fixture{fixture(t, "overview-default-160")} {
		events := f.router.doc.Events
		if len(events) == 0 {
			t.Errorf("%s tells of nothing lately", f.Name)
			continue
		}
		for i := 1; i < len(events); i++ {
			if events[i].ID != events[i-1].ID-1 || events[i].At.After(events[i-1].At) {
				t.Errorf("%s: event %+v follows %+v, want each older than the last, numbered one less", f.Name, events[i], events[i-1])
			}
		}
	}
}

// A world's router answers GET /history from its readings: each account's
// windows as they run, from when each started; asked for a point a minute,
// the last is its use as the document reads it.
func TestAWorldsRouterAnswersHistoryFromItsReadings(t *testing.T) {
	worlds := worldsNow()
	for _, w := range []struct {
		name  string
		world accountWorld
	}{
		{"auto", worlds.auto()}, {"pinned", worlds.pinned()}, {"routing's limit", worlds.limit()},
		{"the bad hour", worlds.badHour()}, {"ten accounts", worlds.many(10)},
	} {
		t.Run(w.name, func(t *testing.T) {
			r := routerOf(w.world, worlds.now)
			for _, a := range r.doc.Accounts {
				for _, window := range a.Windows {
					start, _, ok := window.Span()
					if !ok || !window.ResetsAt.After(worlds.now) {
						continue
					}
					h, err := r.History(t.Context(), window.Key, time.Minute)
					if err != nil {
						t.Fatal(err)
					}
					held := h.Accounts[slices.IndexFunc(h.Accounts, func(held router.AccountHistory) bool { return held.ID == a.ID })]
					if !held.Start.Equal(start) || len(held.Points) == 0 {
						t.Errorf("%s's %s history starts at %s with %d points, want points from %s", a.ID, window.Key, held.Start, len(held.Points), start)
						continue
					}
					if last := held.Points[len(held.Points)-1].Utilization; last != window.Utilization {
						t.Errorf("%s's %s history ends at %v, want its use, %v", a.ID, window.Key, last, window.Utilization)
					}
				}
			}
		})
	}
}

func TestAWorldsRouterAnswersHistoryAPointEachStepAsLastRead(t *testing.T) {
	worlds := worldsNow()
	h, err := routerOf(worlds.auto(), worlds.now).History(t.Context(), sessionKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	personal := h.Accounts[slices.IndexFunc(h.Accounts, func(held router.AccountHistory) bool { return held.ID == "personal" })]
	var got []float64
	for _, p := range personal.Points {
		got = append(got, p.Utilization)
	}
	// Primed at 11:30 and rising evenly to 39% at 14:40, a hundredth each
	// reading, so 12 hundredths read by 12:30.
	if want := []float64{0, 0.12, 0.24, 0.36}; !slices.Equal(got, want) {
		t.Errorf("personal's 5-hour window reads %v each hour from 11:30, want %v", got, want)
	}
}

// d28cOnWork and d28cOnSide are d28c's request r1, on work, and on side,
// where the router sends it again.
var (
	d28cOnWork = request{id: "00000301", session: idD28C, model: opus, account: "work"}
	d28cOnSide = request{id: "00000301", session: idD28C, model: opus, account: "side"}
)

// movedAndSentAgain is the auto world's router at now, its stream telling of
// d28c's request r1, sent on work 1.3s ago and refused there by work's limit
// 1.07s ago, as the router moves d28c to side and sends it again there.
func movedAndSentAgain(now time.Time) source {
	refusedAt := ago(now, 1070*time.Millisecond)
	limited := d28cOnWork.told(router.StreamLimited, refusedAt)
	limited.Status = 429
	moved := d28cOnSide.told(router.StreamMoved, refusedAt)
	moved.From, moved.To, moved.Reason = "work", "side", "moved: work hit its limit"
	again := d28cOnSide.told(router.StreamSent, refusedAt)
	again.Attempt = 2
	return routerOf(worldsNow().auto(), now).telling(d28cOnWork.told(router.StreamSent, ago(now, 1300*time.Millisecond)), limited, moved, again)
}

// A source that turns stands as it did until its time, then as the next
// does, read by the same clock: its document and its sessions the next's,
// and its stream, first opened after the turn, telling all the next's,
// though the watch opened it before, which a later look's fixture needs to
// time what travels its cords.
func TestASourceThatTurnsStandsAsTheNextFromItsTime(t *testing.T) {
	before, now := autoRouter()
	next := movedAndSentAgain(now)
	next.doc.Best = "personal"
	clock := ago(now, time.Minute)
	s := before.then(now, next).by(func() time.Time { return clock })
	look := func() (string, int, []router.StreamEvent) {
		doc, _, err := s.Read(t.Context(), status.Read{})
		if err != nil {
			t.Fatal(err)
		}
		listed, err := s.Sessions(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		events, err := s.Stream(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		told := make([]router.StreamEvent, 0, len(events))
		for range len(events) {
			told = append(told, <-events)
		}
		return doc.Best, len(listed), told
	}
	if best, listed, told := look(); best != before.doc.Best || listed != len(before.sessions) || len(told) != len(before.stream) {
		t.Errorf("before its time, the source gives best %q, %d sessions and %d events, want its own", best, listed, len(told))
	}
	clock = now
	if best, listed, told := look(); best != "personal" || listed != len(next.sessions) || !reflect.DeepEqual(told, next.stream) {
		t.Errorf("from its time, the source gives best %q, %d sessions and the stream %+v, want the next's whole, %+v", best, listed, told, next.stream)
	}
}

// A watch joining the stream again, as it does when it shows a view that
// reads it after one that didn't, is told only what's still in flight, each
// request as it stands, never what befell it before again, as a router tells
// it; a source no watch reads tells it all, each time. Each time, the stream
// stays open once it has told it, telling nothing more.
func TestAWatchJoiningTheStreamAgainIsToldOnlyWhatsStillInFlight(t *testing.T) {
	now := worldsNow().now
	opened := func(s source) []router.StreamEvent {
		events, err := s.Stream(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		told := make([]router.StreamEvent, 0, len(events))
		for range len(events) {
			told = append(told, <-events)
		}
		select {
		case e, open := <-events:
			t.Fatalf("having told %d events, the stream gives %+v, open %t, want it open, telling nothing more", len(told), e, open)
		default:
		}
		return told
	}
	unread := movedAndSentAgain(now)
	read := unread.by(func() time.Time { return now })
	if got := opened(read); !reflect.DeepEqual(got, unread.stream) {
		t.Errorf("first opened, the stream tells %+v, want all of it, %+v", got, unread.stream)
	}
	again := opened(read)
	if len(again) != 1 {
		t.Fatalf("opened again, the stream tells %+v, want d28c's request alone", again)
	}
	if e := again[0]; e.Kind != router.StreamInFlight || e.Request != d28cOnSide.id || e.Account != "side" || e.Attempt != 2 ||
		!e.SentAt.Equal(ago(now, 1070*time.Millisecond).UTC()) || !e.At.Equal(now.UTC()) {
		t.Errorf("opened again, the stream tells %+v, want d28c's request in flight on side, sent again 1.07s ago, as it stands now", e)
	}
	for range 2 {
		if got := opened(unread); !reflect.DeepEqual(got, unread.stream) {
			t.Errorf("read by no watch, the stream tells %+v, want all of it, %+v", got, unread.stream)
		}
	}
}

// A watch reading the source waits on the stream it opened last while what
// that told is yet to be heard, and is held after, as the stream, open,
// tells nothing more; a stream that told nothing holds it at once.
func TestAWatchWaitsOnTheStreamTillItHasHeardAllItTold(t *testing.T) {
	now := worldsNow().now
	read := movedAndSentAgain(now).by(func() time.Time { return now })
	wait := func() tea.Msg { return nil }
	waits := func() []bool {
		if _, err := read.Stream(t.Context()); err != nil {
			t.Fatal(err)
		}
		return []bool{read.await(wait) != nil, read.await(wait) != nil}
	}
	if got, want := waits(), []bool{true, false}; !slices.Equal(got, want) {
		t.Errorf("opened, then listened to again, the watch waits %v, want %v: once, to hear it all", got, want)
	}
	if got, want := waits(), []bool{true, false}; !slices.Equal(got, want) {
		t.Errorf("opened again, telling what's in flight, the watch waits %v, want %v", got, want)
	}
	quiet := routerOf(worldsNow().auto(), now).telling().by(func() time.Time { return now })
	if _, err := quiet.Stream(t.Context()); err != nil {
		t.Fatal(err)
	}
	if quiet.await(wait) != nil {
		t.Error("opened, telling nothing, the watch waits on the stream, want it held")
	}
}

// The auto world's router has the Overview's requests in flight on its
// stream, as it lists their sessions: 41e0's answering on personal, ~860
// tokens so far, c61b's on side, 1.2k, and d28c's asking on side.
func TestTheAutoWorldsStreamHasInFlightWhatItsSessionsAreDoing(t *testing.T) {
	worlds := worldsNow()
	r := routerOf(worlds.auto(), worlds.now)
	type doing struct {
		session, account, family, what string
		tokens                         int
	}
	var flown []doing
	for _, f := range flying(r.stream, worlds.now) {
		what := status.Asking
		if !f.FirstAt.IsZero() {
			what = status.Answering
		}
		flown = append(flown, doing{session: f.Session[:4], account: f.Account, family: claude.Provider{}.Family(f.Model), what: what, tokens: f.Chars / 4})
	}
	want := []doing{
		{session: "41e0", account: "personal", family: "sonnet", what: status.Answering, tokens: 860},
		{session: "c61b", account: "side", family: "opus", what: status.Answering, tokens: 1200},
		{session: "d28c", account: "side", family: "opus", what: status.Asking},
	}
	if !slices.Equal(flown, want) {
		t.Errorf("in flight are %+v, want %+v", flown, want)
	}
	var listed []doing
	for _, s := range r.sessions {
		for _, a := range s.Assignments {
			if a.InFlight != "" {
				listed = append(listed, doing{session: s.ID[:4], account: a.Account, family: a.Family, what: a.InFlight})
			}
		}
	}
	for _, l := range listed {
		if !slices.ContainsFunc(flown, func(f doing) bool { f.tokens = 0; return f == l }) {
			t.Errorf("the router lists %+v, which its stream has nothing in flight of", l)
		}
	}
	if len(listed) != len(flown) {
		t.Errorf("the router lists %d sessions with a request in flight, want the stream's %d", len(listed), len(flown))
	}
}

// The auto world's router last saw each session with a request in flight as
// it was sent, and its stream tells of c61b's answer after d28c's ask, so
// the stream saw c61b last on side, as the frames order side's sessions.
func TestTheAutoWorldsStreamSawC61BLastOnSide(t *testing.T) {
	worlds := worldsNow()
	r := routerOf(worlds.auto(), worlds.now)
	for _, f := range flying(r.stream, worlds.now) {
		i := slices.IndexFunc(r.sessions, func(s status.Session) bool { return s.ID == f.Session })
		if seen := r.sessions[i].Assignments[0].LastSeen; !seen.Equal(f.SentAt) {
			t.Errorf("the router last saw %s at %s, want as its request in flight was sent, %s", f.Session[:4], seen, f.SentAt)
		}
	}
	lastTold := make(map[string]time.Time)
	for _, e := range r.stream {
		lastTold[e.Session] = e.At
	}
	if !lastTold[idC61B].After(lastTold[idD28C]) {
		t.Errorf("the stream last told of c61b at %s, d28c at %s, want c61b after", lastTold[idC61B], lastTold[idD28C])
	}
}

// autoRouter is the auto world's router at the worlds' moment, and the
// moment.
func autoRouter() (source, time.Time) {
	worlds := worldsNow()
	return routerOf(worlds.auto(), worlds.now), worlds.now
}
