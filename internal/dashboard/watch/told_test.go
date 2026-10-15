package watch

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// toldOf is what the traffic tells the views of the request with the given
// id, reporting false where it tells of none, whenever they read it.
func toldOf(tr traffic, request string) (views.Told, bool) {
	r, ok := tr.stream(start).Requests[request]
	return r, ok
}

// d28cAsked is what the stream tells of d28c's request r1 as it goes out on
// work, as the request names its directory, prompt, class and agent.
func d28cAsked() views.Told {
	return views.Told{
		Request: "r1", Session: idD28C, Dir: "~/Code/switchboard", Model: opus, Account: "work",
		Prompt: "p1", Class: "main", AgentID: "a1", Sent: past(0), Heard: past(0),
	}
}

// named is e naming d28cAsked's directory, prompt, class and agent, as
// every event of the stream's does.
func named(e router.StreamEvent) router.StreamEvent {
	e.Dir, e.Prompt, e.Class, e.AgentID = "~/Code/switchboard", "p1", "main", "a1"
	return e
}

func TestEachEventFillsInWhatTheStreamToldOfItsRequest(t *testing.T) {
	sent := named(told(router.StreamSent, "r1", "work", 0))
	answer := func(kind string, status int, d time.Duration) router.StreamEvent {
		return alter(named(told(kind, "r1", "work", d)), func(e *router.StreamEvent) { e.Status = status })
	}
	asked := d28cAsked()
	tests := []struct {
		name   string
		events []router.StreamEvent
		want   func(r *views.Told)
	}{
		{name: "sent: asking, from when it went, named as it names itself", events: []router.StreamEvent{sent}, want: func(*views.Told) {}},
		{
			name: "its first byte", events: []router.StreamEvent{sent, named(told(router.StreamFirst, "r1", "work", time.Second))},
			want: func(r *views.Told) { r.First, r.Heard = past(time.Second), past(time.Second) },
		},
		{
			name: "its progress: its characters so far, never fewer",
			events: []router.StreamEvent{
				sent,
				alter(named(told(router.StreamProgress, "r1", "work", 2*time.Second)), func(e *router.StreamEvent) { e.Chars = 4936 }),
				alter(named(told(router.StreamProgress, "r1", "work", 3*time.Second)), func(e *router.StreamEvent) { e.Chars = 4000 }),
			},
			want: func(r *views.Told) { r.Chars, r.Heard = 4936, past(3*time.Second) },
		},
		{
			name: "done: ended, what its client was answered, and its characters in all",
			events: []router.StreamEvent{
				sent,
				alter(answer(router.StreamDone, 200, 5*time.Second), func(e *router.StreamEvent) { e.Chars = 5000 }),
			},
			want: func(r *views.Told) {
				r.Ended, r.Status, r.Chars, r.Heard = past(5*time.Second), 200, 5000, past(5*time.Second)
			},
		},
		{
			name: "limited: its verdict, and the answer's status", events: []router.StreamEvent{sent, answer(router.StreamLimited, 429, time.Second)},
			want: func(r *views.Told) { r.Verdict, r.Status, r.Heard = views.VerdictLimited, 429, past(time.Second) },
		},
		{
			name: "throttled", events: []router.StreamEvent{sent, answer(router.StreamThrottled, 429, time.Second)},
			want: func(r *views.Told) { r.Verdict, r.Status, r.Heard = views.VerdictThrottled, 429, past(time.Second) },
		},
		{
			name: "refused", events: []router.StreamEvent{sent, answer(router.StreamRefused, 403, time.Second)},
			want: func(r *views.Told) { r.Verdict, r.Status, r.Heard = views.VerdictRefused, 403, past(time.Second) },
		},
		{
			name:   "sent again once throttled: waiting on another answer, sent as it first went",
			events: []router.StreamEvent{sent, answer(router.StreamThrottled, 429, time.Second), named(told(router.StreamSent, "r1", "work", 3*time.Second))},
			want:   func(r *views.Told) { r.Heard = past(3 * time.Second) },
		},
		{
			name: "moved off work at its limit, and sent again on side: out on side, its verdict there none yet",
			events: []router.StreamEvent{
				sent,
				answer(router.StreamLimited, 429, time.Second),
				alter(named(told(router.StreamMoved, "r1", "side", time.Second)), func(e *router.StreamEvent) { e.From, e.To = "work", "side" }),
				named(told(router.StreamSent, "r1", "side", time.Second)),
			},
			want: func(r *views.Told) { r.Account, r.Heard = "side", past(time.Second) },
		},
		{
			name: "a move alone tells nothing of the request",
			events: []router.StreamEvent{
				sent,
				alter(named(told(router.StreamMoved, "r1", "side", time.Second)), func(e *router.StreamEvent) { e.From, e.To = "work", "side" }),
			},
			want: func(*views.Told) {},
		},
		{
			name: "in flight as the stream opens: as it stands then",
			events: []router.StreamEvent{alter(named(told(router.StreamInFlight, "r1", "work", 0)), func(e *router.StreamEvent) {
				e.SentAt, e.FirstAt, e.Chars, e.Verdict, e.Status = past(-10*time.Second), past(-5*time.Second), 800, router.StreamThrottled, 429
			})},
			want: func(r *views.Told) {
				r.Sent, r.First, r.Chars, r.Verdict, r.Status = past(-10*time.Second), past(-5*time.Second), 800, views.VerdictThrottled, 429
			},
		},
		{
			name: "in flight as the stream opens again, sent again since: as it first went",
			events: []router.StreamEvent{
				sent,
				alter(named(told(router.StreamInFlight, "r1", "work", 4*time.Second)), func(e *router.StreamEvent) { e.SentAt = past(3 * time.Second) }),
			},
			want: func(r *views.Told) { r.Heard = past(4 * time.Second) },
		},
		{
			name:   "Claude Code's quota check: kept, and said to be one",
			events: []router.StreamEvent{alter(sent, func(e *router.StreamEvent) { e.Check = true })},
			want:   func(r *views.Told) { r.Check = true },
		},
		{
			name:   "a count of tokens: kept, and said to be one",
			events: []router.StreamEvent{alter(sent, func(e *router.StreamEvent) { e.Count = true })},
			want:   func(r *views.Told) { r.Count = true },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := asked
			tt.want(&want)
			tr := traffic{}.took(tt.events, heldNowhere, past(5*time.Second))
			if got, ok := toldOf(tr, "r1"); !ok || got != want {
				t.Errorf("told of r1 %t:\n  %+v\nwant\n  %+v", ok, got, want)
			}
		})
	}
}

// A router gives each of its requests an id of its own while it runs, so the
// stream tells nothing of a request once it has ended; and of one lost, only
// that it's in flight still, as it opens again. Any other event under the id
// of one held is another request, as another run's is: it never takes on
// what the stream told of the one before.
func TestARequestIsItsIdAndWhenSoAnotherNeverTakesOnWhatTheOneBeforeWasTold(t *testing.T) {
	first := alter(told(router.StreamSent, "r1", "work", 0), func(e *router.StreamEvent) { e.Dir, e.Prompt = "~/Code/switchboard", "p1" })
	flying := traffic{live: true}.took([]router.StreamEvent{first}, heldNowhere, past(0))
	ended := flying.took([]router.StreamEvent{alter(told(router.StreamDone, "r1", "work", 5*time.Second), func(e *router.StreamEvent) { e.Status = 200 })},
		heldNowhere, past(5*time.Second))
	lost := flying.afresh()
	later := 10 * time.Second
	sent, first2 := told(router.StreamSent, "r1", "work", later), told(router.StreamFirst, "r1", "work", later)
	inFlight := alter(told(router.StreamInFlight, "r1", "work", later), func(e *router.StreamEvent) { e.SentAt = past(later) })
	tests := []struct {
		name string
		held traffic
		e    router.StreamEvent
		same bool
	}{
		{name: "in flight, sent again", held: flying, e: sent, same: true},
		{name: "in flight, its first byte", held: flying, e: first2, same: true},
		{name: "in flight, told of in flight as the stream opens again", held: flying, e: inFlight, same: true},
		{name: "ended, then sent", held: ended, e: sent},
		{name: "ended, then a first byte", held: ended, e: first2},
		{name: "ended, then told of in flight as the stream opens again", held: ended, e: inFlight},
		{name: "lost, then told of in flight as the stream opens again", held: lost, e: inFlight, same: true},
		{name: "lost, then sent", held: lost, e: sent},
		{name: "lost, then a first byte", held: lost, e: first2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := toldOf(tt.held.took([]router.StreamEvent{tt.e}, heldNowhere, past(later)), "r1")
			if same := r.Dir == first.Dir && r.Prompt == first.Prompt && r.Sent.Equal(past(0)); same != tt.same {
				t.Errorf("r1 is told %+v: the request first sent: %t, want %t", r, same, tt.same)
			}
			if !tt.same && (r.Lost || !r.Ended.IsZero()) {
				t.Errorf("r1 is told %+v, want it neither lost nor ended, another request", r)
			}
		})
	}
}

func TestForgettingWhatTheStreamToldLosesTheRequestsInFlightTillItTellsOfThemAgain(t *testing.T) {
	tr := traffic{live: true}.took([]router.StreamEvent{
		told(router.StreamSent, "r1", "work", 0),
		told(router.StreamSent, "r2", "work", time.Second),
		alter(told(router.StreamDone, "r2", "work", 2*time.Second), func(e *router.StreamEvent) { e.Status = 200 }),
	}, heldNowhere, past(2*time.Second)).afresh()

	if s := tr.stream(past(2 * time.Second)); s.Live {
		t.Error("forgotten, the stream is live, want it no longer")
	}
	if r, _ := toldOf(tr, "r1"); !r.Lost || r.InFlight() {
		t.Errorf("r1, in flight as the stream was forgotten, is %+v, want it lost", r)
	}
	if r, _ := toldOf(tr, "r2"); r.Lost || r.Ended != past(2*time.Second) {
		t.Errorf("r2, ended before, is %+v, want it ended, not lost", r)
	}

	tr = tr.took([]router.StreamEvent{alter(told(router.StreamInFlight, "r1", "work", 3*time.Second), func(e *router.StreamEvent) { e.SentAt = past(0) })}, heldNowhere, past(3*time.Second))
	if r, _ := toldOf(tr, "r1"); r.Lost || !r.InFlight() {
		t.Errorf("r1, told of in flight as the stream opened again, is %+v, want it in flight", r)
	}
}

func TestTheRequestsNoLongerInFlightGoSeenForAfterTheStreamLastToldOfThem(t *testing.T) {
	tr := traffic{live: true}.took([]router.StreamEvent{
		told(router.StreamSent, "lost", "work", 0),
		told(router.StreamSent, "flying", "work", 0),
	}, heldNowhere, past(0)).afresh()
	tr = tr.took([]router.StreamEvent{
		alter(told(router.StreamInFlight, "flying", "work", time.Second), func(e *router.StreamEvent) { e.SentAt = past(0) }),
		told(router.StreamSent, "done", "work", time.Second),
		alter(told(router.StreamDone, "done", "work", 2*time.Second), func(e *router.StreamEvent) { e.Status = 200 }),
	}, heldNowhere, past(2*time.Second))

	kept := func(at time.Time) []string {
		var ids []string
		for id := range tr.tidied(at).stream(at).Requests {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		return ids
	}
	if got, want := kept(past(seenFor)), []string{"done", "flying"}; !slices.Equal(got, want) {
		t.Errorf("seenFor after the lost one was last told of, the requests kept are %q, want %q", got, want)
	}
	if got, want := kept(past(seenFor+2*time.Second)), []string{"flying"}; !slices.Equal(got, want) {
		t.Errorf("seenFor after the last ended, the requests kept are %q, want %q: the one in flight, however long", got, want)
	}
}

// A request the stream told of goes as soon as a read of the ledger's tail
// reads its line, which tells of it from then on, however soon after it
// ended, its seat seen as late as the stream last told of it, so its
// session's idle counts from its end still: but not one in flight, nor one
// whose id a line of another request has, as a router gives an id again
// once its run has ended. Where it reads none of them, the traffic is as
// it was; where it does, its requests are another map.
func TestARequestGoesOnceAReadOfTheTailReadsItsLine(t *testing.T) {
	tr := traffic{live: true}.took([]router.StreamEvent{
		told(router.StreamSent, "done", "work", 0),
		alter(told(router.StreamDone, "done", "work", 4*time.Minute), func(e *router.StreamEvent) { e.Status = 200 }),
		told(router.StreamSent, "flying", "work", time.Minute),
		told(router.StreamSent, "again", "work", 2*time.Minute),
		alter(told(router.StreamDone, "again", "work", 3*time.Minute), func(e *router.StreamEvent) { e.Status = 200 }),
	}, heldNowhere, past(4*time.Minute))
	read := []ledger.Held{
		took(arrived("done", "work", 0), 4*time.Minute),
		arrived("flying", "work", time.Minute),
		took(arrived("again", "work", -time.Hour), time.Second),
	}

	if unread := tr.lined([]ledger.Held{arrived("other", "work", 0)}); !sameMap(unread.requests, tr.requests) || unread.changes != tr.changes {
		t.Error("a read of no line of a request told of changed the traffic, want it as it was")
	}
	lined := tr.lined(read)
	if got, want := ids(lined), []string{"again", "flying"}; !slices.Equal(got, want) {
		t.Errorf("its line read, the requests kept are %q, want %q", got, want)
	}
	if got, want := lined.stream(past(5*time.Minute)).Seen, map[views.Seat]time.Time{{Session: idD28C, Model: opus, Account: "work"}: past(4 * time.Minute)}; !maps.Equal(got, want) {
		t.Errorf("its line read, the seats seen are %v, want %v, as the stream last told of done", got, want)
	}
	if _, ok := toldOf(tr, "done"); !ok {
		t.Error("lined, the traffic it was lined from lost done, want its requests its own")
	}
	if lined.rowChanges == tr.rowChanges {
		t.Error("its line read, the traffic counts no change to its rows, want one")
	}
	if seen := lined.tidied(past(4*time.Minute + seenFor)).stream(past(4*time.Minute + seenFor)).Seen; len(seen) > 0 {
		t.Errorf("seenFor after done ended, the seats seen are %v, want none", seen)
	}
}

// The views read each move from as soon as it's told of, and once a listing
// shows it done, read it still, listed, for what it says of where its model
// came from.
func TestTheViewsReadEachMoveListedOrNot(t *testing.T) {
	moved := alter(told(router.StreamMoved, "r1", "side", time.Second), func(e *router.StreamEvent) {
		e.From, e.To, e.Reason = "work", "side", status.ReasonMovedOff+"work "+status.WhyLimit
	})
	tr := traffic{live: true}.took([]router.StreamEvent{moved}, heldNowhere, past(time.Second))
	want := []views.Moved{{Request: "r1", Session: idD28C, Model: opus, From: "work", To: "side", Reason: moved.Reason, At: past(time.Second)}}
	if got := tr.stream(past(time.Second)).Moves; !reflect.DeepEqual(got, want) {
		t.Errorf("the moves the views read are %+v, want %+v, from as soon as it's told of", got, want)
	}
	want[0].Listed = true
	if got := tr.listedAs(d28cListedOn("side")).stream(past(time.Second)).Moves; !reflect.DeepEqual(got, want) {
		t.Errorf("listed where it went, the moves the views read are %+v, want %+v, listed", got, want)
	}
}

// A frame never contradicts itself: the views see a move from when the cords
// show it, once the refusal that moved it has bounced back along its cord.
func TestTheViewsSeeAMoveFromWhenTheCordsShowIt(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	h.hear(movedToSide(0)...)

	for _, tt := range []struct {
		name  string
		at    time.Time
		shown bool
	}{
		{name: "as its refusal bounces back", at: past(pulseFor - time.Millisecond)},
		{name: "once it has", at: past(pulseFor), shown: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := h.model.frame(tt.at)
			if drawn, read := f.Traffic.On(idD28C, "side").Doing != 0, len(f.Stream.Moves) > 0; drawn != tt.shown || read != tt.shown {
				t.Errorf("the cords draw d28c on side: %t, and the views read its move: %t, want both %t", drawn, read, tt.shown)
			}
			if got, want := views.RunningCountsOf(h.model.moment(tt.at), f.Sessions, f.Stream)["side"], map[bool]int{true: 1}[tt.shown]; got != want {
				t.Errorf("RunningCounts on side = %d, want %d, as the cords show d28c there", got, want)
			}
		})
	}
}

func TestTheTracesCountEachRequestTheEventsSendOnce(t *testing.T) {
	events := []router.StreamEvent{
		told(router.StreamSent, "r1", "work", 0),
		alter(told(router.StreamThrottled, "r1", "work", time.Second), func(e *router.StreamEvent) { e.Status = 429 }),
		told(router.StreamSent, "r1", "work", 2*time.Second),
		alter(told(router.StreamSent, "check", "work", 2*time.Second), func(e *router.StreamEvent) { e.Check = true }),
		alter(told(router.StreamSent, "count", "work", 2*time.Second), func(e *router.StreamEvent) { e.Count = true }),
		told(router.StreamFirst, "r1", "work", 3*time.Second),
		alter(told(router.StreamInFlight, "r2", "side", 3*time.Second), func(e *router.StreamEvent) { e.SentAt = past(-time.Second) }),
	}
	tr := traffic{}.took(events, heldNowhere, past(3*time.Second))
	var got []string
	for _, r := range tr.sentOf(events) {
		got = append(got, r.Request+" "+r.Sent.Sub(start).String())
	}
	if want := []string{"r1 0s", "r2 -1s"}; !slices.Equal(got, want) {
		t.Errorf("the requests sent are %q, want %q: each once, as first sent, never the quota check or a count", got, want)
	}
}

func TestTheFrameHoldsWhatTheStreamToldOfEachRequest(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamFirst, "r1", "work", time.Second))

	s := h.model.frame(h.clock.now).Stream
	if r := s.Requests["r1"]; !s.Live || r.First != past(time.Second) || !r.InFlight() {
		t.Errorf("the frame's stream is %+v, want it live, r1 answering", s)
	}
}

func TestARequestInFlightTheStreamOpenedAgainDoesntTellOfIsLost(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamSent, "r2", "work", 0))
	h.endStream()
	rejoin, _ := h.pendingRejoin()
	h.fire(rejoin)
	// It told of r2 in flight as it opened, then of r3 going out: all it
	// tells of those in flight as it opens comes before anything else.
	h.hear(alter(told(router.StreamInFlight, "r2", "work", 2*time.Second), func(e *router.StreamEvent) { e.SentAt = past(0) }),
		told(router.StreamSent, "r3", "work", 2*time.Second))

	s := h.model.frame(h.clock.now).Stream
	if r := s.Requests["r1"]; !r.Lost {
		t.Errorf("r1, in flight as the stream ended, untold of as it opened again, is %+v, want it lost", r)
	}
	if r := s.Requests["r2"]; !r.InFlight() {
		t.Errorf("r2, told of in flight as the stream opened again, is %+v, want it in flight", r)
	}
}

// The router tells of the requests in flight as the stream opens, but the
// watch may hear them over several messages, as they come.
func TestARequestTheStreamOpenedAgainTellsOfLateInItsFirstLookIsNeverLost(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamSent, "r2", "work", 0))
	h.endStream()
	rejoin, _ := h.pendingRejoin()
	h.fire(rejoin)
	inFlight := func(id string) router.StreamEvent {
		return alter(told(router.StreamInFlight, id, "work", 2*time.Second), func(e *router.StreamEvent) { e.SentAt = past(0) })
	}

	h.hear(inFlight("r1"))
	if r := h.model.frame(h.clock.now).Stream.Requests["r2"]; r.Lost {
		t.Errorf("its first look telling of r1, r2 is %+v, want it not lost, as more of that look may come", r)
	}
	h.hear(inFlight("r2"))
	if s := h.model.frame(h.clock.now).Stream; !s.Live || !s.Requests["r2"].InFlight() {
		t.Errorf("its first look telling of r2 too, r2 is %+v, live %t, want it in flight, live", s.Requests["r2"], s.Live)
	}
}

// The views hear the stream as the watch reads it once as what it told
// changes, not at each frame: a look at the sessions running, of a frame
// of an hour's requests, costs less than one of the same stream heard
// afresh.
func TestEachFrameReadsTheStreamAsTheViewsLastHeardIt(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	var events []router.StreamEvent
	for i := range 300 {
		e := told(router.StreamSent, fmt.Sprintf("r%d", i), "work", -time.Duration(i)*10*time.Second)
		events = append(events, alter(e, func(e *router.StreamEvent) { e.Session = fmt.Sprintf("%08d-0000-4000-8000-000000000000", i%10) }))
	}
	for batch := range slices.Chunk(events, 50) {
		h.hear(batch...)
	}
	now := h.clock.now
	f, mo := h.model.frame(now), h.model.moment(now)
	look := func(s views.Stream) float64 {
		return testing.AllocsPerRun(10, func() { views.RunningOf(mo, views.ListLive{Running: f.Sessions, Stream: s}, f.HeldList) })
	}
	if heard, afresh := look(f.Stream), look(views.Stream{Live: f.Stream.Live, Requests: f.Stream.Requests, Moves: f.Stream.Moves}); heard >= afresh {
		t.Errorf("a look at the frame's stream allocates %.0f times, one at it heard afresh %.0f: the frame's is heard again at each look", heard, afresh)
	}
}

// The views hear each change to the requests the traffic holds through its
// one Hearing, by whichever path it comes: taken in from the stream, let go
// at a tick, lost as the stream is forgotten, or lost as it opens again
// without telling of them; and a tick that lets go of none of them has them
// heard again by none. The Hearing reads a change by the requests being
// another map, so they're another once anything in them changes, the map
// before left as it was, and the very same map where nothing does.
func TestTheViewsHearEachChangeToTheRequestsAndNoTickThatChangesNone(t *testing.T) {
	main := func(e router.StreamEvent) router.StreamEvent {
		return ofHaiku(alter(e, func(e *router.StreamEvent) { e.Class = ledger.ClassMain }))
	}
	done := main(alter(told(router.StreamDone, "r1", "work", time.Second), func(e *router.StreamEvent) { e.Status = 200 }))
	asking := traffic{live: true}.took([]router.StreamEvent{main(told(router.StreamSent, "r1", "work", 0))}, heldNowhere, past(0))
	answered := asking.took([]router.StreamEvent{done}, heldNowhere, past(time.Second))
	tidiedAt := func(d time.Duration) func(traffic) traffic {
		return func(tr traffic) traffic { return tr.tidied(past(d)) }
	}
	steps := []struct {
		name    string
		from    traffic
		to      func(traffic) traffic
		at      time.Duration
		changes bool
	}{
		{name: "its answer taken in", from: asking, to: func(tr traffic) traffic { return tr.took([]router.StreamEvent{done}, heldNowhere, past(time.Second)) }, at: time.Second, changes: true},
		{name: "let go at a tick", from: answered, to: tidiedAt(time.Second + seenFor), at: time.Second + seenFor, changes: true},
		{name: "lost as the stream is forgotten", from: asking, to: traffic.afresh, at: time.Second, changes: true},
		{name: "lost as the stream opens again without it", from: asking.reopened(), to: traffic.settled, at: time.Second, changes: true},
		{name: "lost as the stream opens again telling of another first", from: asking.reopened(), to: func(tr traffic) traffic {
			return tr.took([]router.StreamEvent{told(router.StreamSent, "r2", "work", time.Second)}, heldNowhere, past(time.Second))
		}, at: time.Second, changes: true},
		{name: "a tick with nothing to let go", from: answered, to: tidiedAt(time.Second + pulseFor/2), at: time.Second},
		{name: "a tick letting go of a call's pulse alone", from: answered, to: tidiedAt(time.Second + pulseFor), at: time.Second + pulseFor},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			at := past(s.at)
			from := s.from
			from.heard = new(views.Hearing)
			heard, kept := runningOnWorkHeard(from.stream(at), at), maps.Clone(from.requests)
			to := s.to(from)
			if to.heard != from.heard {
				t.Fatal("the traffic's Hearing is another, want the one it had carried")
			}
			stream := to.stream(at)
			got, want := runningOnWorkHeard(stream, at), runningOnWorkHeard(views.Stream{Live: stream.Live, Requests: stream.Requests, Moves: stream.Moves}, at)
			if s.changes && reflect.DeepEqual(heard, want) {
				t.Fatalf("the views run %+v before and after, want the step to change what they show", want)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("the views run %+v, heard afresh %+v: the Hearing holds what it heard before", got, want)
			}
			if !maps.Equal(from.requests, kept) {
				t.Error("the requests before changed where they lie, want them left as they were")
			}
			if same := sameMap(from.requests, to.requests); same == s.changes {
				t.Errorf("the requests after are the very same map: %t, want %t, as they changed: %t", same, !s.changes, s.changes)
			}
		})
	}
}

// runningOnWorkHeard is what the views run at at, of the stream s as it's
// heard, of d28c as the router lists it on work: its opus seen a minute
// before, and its haiku two.
func runningOnWorkHeard(s views.Stream, at time.Time) []views.Running {
	seen := func(model string, ago time.Duration) status.Assignment {
		return status.Assignment{Model: model, Family: strings.Split(model, "-")[1], Account: "work", Reason: "new", AssignedAt: at.Add(-2 * time.Hour), LastSeen: at.Add(-ago)}
	}
	listed := []status.Session{{ID: idD28C, Assignments: []status.Assignment{seen(opus, time.Minute), seen(haiku, 2*time.Minute)}}}
	return views.RunningOf(routerDocument(three()...).At(at, policy), views.ListLive{Running: listed, Stream: s}, views.HeldList{})
}

// sameMap reports whether a and b are the very same map, not alike ones.
func sameMap[K comparable, V any](a, b map[K]V) bool {
	return reflect.ValueOf(a).UnsafePointer() == reflect.ValueOf(b).UnsafePointer()
}

// d28cRunning is d28c, as the views have it run on work in the frame of the
// model h drives, at now.
func d28cRunning(t *testing.T, h *harness, now time.Time) views.Running {
	t.Helper()
	f := h.model.frame(now)
	running := views.RunningOf(h.model.moment(now), views.ListLive{Running: f.Sessions, Stream: f.Stream}, f.HeldList)
	i := slices.IndexFunc(running, func(r views.Running) bool { return r.Session == idD28C && r.Account == "work" })
	if i < 0 {
		t.Fatalf("the views run %+v, want d28c on work", running)
	}
	return running[i]
}

func TestOnceTheStreamEndsTheViewsGoByTheRoutersListingKeepingWhatItTold(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	h.hear(told(router.StreamSent, "r1", "work", 0))
	if a := d28cRunning(t, h, h.clock.now).Activity; a.Doing != views.Asking {
		t.Fatalf("as the stream tells of r1 going out, d28c is %+v, want it asking", a)
	}

	// The router restarts: its listing has nothing in flight.
	h.endStream()
	f := h.model.frame(h.clock.now)
	if f.Stream.Live {
		t.Error("the stream ended, the views read it live, want it no longer")
	}
	if a := d28cRunning(t, h, h.clock.now).Activity; a.InFlight() {
		t.Errorf("the stream ended, d28c is %+v, want it as the router lists it, nothing in flight", a)
	}
	if r := f.Stream.Requests["r1"]; !r.InFlight() {
		t.Errorf("the stream ended, r1 is %+v, want it kept as the stream told it till the stream opens again", r)
	}
}

func TestARequestStillRunningAsTheStreamOpensAgainIsNeverLost(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	h.hear(told(router.StreamSent, "r1", "work", 0))
	h.endStream()
	rejoin, _ := h.pendingRejoin()
	h.fire(rejoin)

	s := h.model.frame(h.clock.now).Stream
	if r := s.Requests["r1"]; r.Lost {
		t.Errorf("the stream opened again, its first look yet to come, r1 is %+v, want it not lost", r)
	}
	if s.Live {
		t.Error("the stream opened again, its first look yet to come, the views read it live, want them going by the router's listing")
	}
	h.hear(alter(told(router.StreamInFlight, "r1", "work", 2*time.Second), func(e *router.StreamEvent) { e.SentAt = past(0) }))
	if s := h.model.frame(h.clock.now).Stream; !s.Live || !s.Requests["r1"].InFlight() {
		t.Errorf("the stream's first look telling of r1 again, it's %+v, live %t, want it in flight, live", s.Requests["r1"], s.Live)
	}
}

func TestARequestStillRunningAsAViewSwitchOpensTheStreamAgainIsNeverLost(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	h.hear(told(router.StreamSent, "r1", "work", 0))
	h.keys(tabKey, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if h.opened() != 2 {
		t.Fatalf("Runway shown, then Sessions again, the stream opened %d times in all, want twice", h.opened())
	}

	if r := h.model.frame(h.clock.now).Stream.Requests["r1"]; r.Lost {
		t.Errorf("the stream opened again, its first look yet to come, r1 is %+v, want it not lost", r)
	}
}

func TestAStreamOpenedAgainThatTellsOfNothingInFlightLosesWhatWasOnceJoinForHasPassed(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	h.hear(told(router.StreamSent, "r1", "work", 0))
	h.endStream()
	rejoin, _ := h.pendingRejoin()
	h.fire(rejoin)

	joining, ok := h.pending(joinedMsg{gen: h.model.stream.gen})
	if !ok || joining.delay != joinFor {
		t.Fatalf("the stream opened again, the wait for its first look is %+v, want one of %v", joining, joinFor)
	}
	h.fire(joining)
	if s := h.model.frame(h.clock.now).Stream; !s.Live || !s.Requests["r1"].Lost {
		t.Errorf("joinFor passed with nothing told, r1 is %+v, live %t, want it lost, live", s.Requests["r1"], s.Live)
	}
}

func TestAnotherRouterAnsweringLosesTheRequestsTheLastOnesStreamToldInFlight(t *testing.T) {
	h := streamingHarness(t)
	h.source.health = router.Health{OK: true, PID: 100, StartedAt: start.UTC()}
	h.tickUntilAsked()
	h.keys(toSessions...)
	h.hear(told(router.StreamSent, "r1", "work", 0))
	h.endStream()

	h.source.health = router.Health{OK: true, PID: 200, StartedAt: start.Add(time.Minute).UTC()}
	h.tickUntilAsked()
	if r := h.model.frame(h.clock.now).Stream.Requests["r1"]; !r.Lost {
		t.Errorf("another router answering, r1, in flight on the last, is %+v, want it lost", r)
	}
}
