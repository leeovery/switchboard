package watch

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The models the stream's requests in the tests ask for.
const (
	opus  = "claude-opus-5-5"
	haiku = "claude-haiku-5-5"
)

// past is d past when the clock starts, in UTC, as the request stream gives
// its times.
func past(d time.Duration) time.Time {
	return start.Add(d).UTC()
}

// told is the request stream's event of the kind given of the request with
// the given id, d28c's opus, on the account with the given id, d past when
// the clock starts.
func told(kind, request, account string, d time.Duration) router.StreamEvent {
	return router.StreamEvent{At: past(d), Kind: kind, Request: request, Attempt: 1, Session: idD28C, Model: opus, Account: account}
}

// alter is e changed as change says.
func alter(e router.StreamEvent, change func(*router.StreamEvent)) router.StreamEvent {
	change(&e)
	return e
}

// ofHaiku is e, of d28c's haiku.
func ofHaiku(e router.StreamEvent) router.StreamEvent {
	return alter(e, func(e *router.StreamEvent) { e.Model = haiku })
}

// heldNowhere says no limit holds any account back.
func heldNowhere(string) bool {
	return false
}

// pinnedToSide is the request stream telling of d28c's opus moved off work
// to side by a pin, d past when the clock starts, no refusal moving it.
func pinnedToSide(d time.Duration) router.StreamEvent {
	return alter(told(router.StreamMoved, "r1", "side", d), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", "side", "pinned" })
}

func TestWhatEachStreamEventDoes(t *testing.T) {
	sent := told(router.StreamSent, "r1", "work", 0)
	first := told(router.StreamFirst, "r1", "work", time.Second)
	progress := alter(told(router.StreamProgress, "r1", "work", 2*time.Second), func(e *router.StreamEvent) { e.Chars = 4936 })
	done := func(status int) router.StreamEvent {
		return alter(told(router.StreamDone, "r1", "work", 5*time.Second), func(e *router.StreamEvent) { e.Status, e.Chars = status, 4936 })
	}
	refusal := func(kind string, status int) router.StreamEvent {
		return alter(told(kind, "r1", "work", 500*time.Millisecond), func(e *router.StreamEvent) { e.Status = status })
	}
	tests := []struct {
		name   string
		events []router.StreamEvent
		at     time.Duration
		// want is what travels d28c's cord on work at, as the frame draws
		// it, nil where nothing does.
		want *dashboard.Call
	}{
		{
			name: "sent: asking, a pulse running out to the jack", events: []router.StreamEvent{sent}, at: 270 * time.Millisecond,
			want: &dashboard.Call{Doing: dashboard.Asking, Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5}},
		},
		{name: "the pulse at the jack: asking still", events: []router.StreamEvent{sent}, at: pulseFor, want: &dashboard.Call{Doing: dashboard.Asking}},
		{
			name: "its first byte: streaming, its shimmer a step on each shimmerStep", events: []router.StreamEvent{sent, first}, at: time.Second + 3*shimmerStep,
			want: &dashboard.Call{Doing: dashboard.Streaming, Shimmer: 3},
		},
		{
			name: "its progress: streaming still", events: []router.StreamEvent{sent, first, progress}, at: 2 * time.Second,
			want: &dashboard.Call{Doing: dashboard.Streaming, Shimmer: 13},
		},
		{
			name: "done with success: its answer's end, a pulse running back", at: 5*time.Second + 270*time.Millisecond,
			events: []router.StreamEvent{sent, first, progress, done(200)},
			want:   &dashboard.Call{Doing: dashboard.Answered, Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5, Back: true}},
		},
		{
			name: "its answer's end, its pulse all but home", at: 5*time.Second + pulseFor - time.Millisecond,
			events: []router.StreamEvent{sent, first, done(200)},
			want:   &dashboard.Call{Doing: dashboard.Answered, Pulsing: true, Pulse: dashboard.Pulse{Along: float64(pulseFor-time.Millisecond) / float64(pulseFor), Back: true}},
		},
		{name: "its answer's end, its pulse home: nothing", events: []router.StreamEvent{sent, first, done(200)}, at: 5*time.Second + pulseFor},
		{name: "done unanswered, as its client went: nothing", events: []router.StreamEvent{sent, done(0)}, at: 5 * time.Second},
		{
			name: "limited: refused, a red pulse running back", events: []router.StreamEvent{sent, refusal(router.StreamLimited, 429)}, at: 770 * time.Millisecond,
			want: &dashboard.Call{Doing: dashboard.Refused, Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5, Back: true, Red: true}},
		},
		{
			name: "refused alone: the same", events: []router.StreamEvent{sent, refusal(router.StreamRefused, 403)}, at: 770 * time.Millisecond,
			want: &dashboard.Call{Doing: dashboard.Refused, Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5, Back: true, Red: true}},
		},
		{
			name: "limited, its pulse home: refused still, until it's done with", events: []router.StreamEvent{sent, refusal(router.StreamLimited, 429), done(429)},
			at: 5*time.Second - time.Millisecond, want: &dashboard.Call{Doing: dashboard.Refused},
		},
		{name: "limited, then done with: nothing", events: []router.StreamEvent{sent, refusal(router.StreamLimited, 429), done(429)}, at: 5 * time.Second},
		{
			name: "limited, then done with at once: refused till its pulse is home", at: 500*time.Millisecond + pulseFor - time.Millisecond,
			events: []router.StreamEvent{sent, refusal(router.StreamLimited, 429), alter(done(429), func(e *router.StreamEvent) { e.At = past(600 * time.Millisecond) })},
			want:   &dashboard.Call{Doing: dashboard.Refused, Pulsing: true, Pulse: dashboard.Pulse{Along: float64(pulseFor-time.Millisecond) / float64(pulseFor), Back: true, Red: true}},
		},
		{
			name: "limited, then done with at once, its pulse home: nothing", at: 500*time.Millisecond + pulseFor,
			events: []router.StreamEvent{sent, refusal(router.StreamLimited, 429), alter(done(429), func(e *router.StreamEvent) { e.At = past(600 * time.Millisecond) })},
		},
		{
			name: "limited, then its 429 passed on, as the router taps its body: refused still", at: 4 * time.Second,
			events: []router.StreamEvent{sent, refusal(router.StreamLimited, 429), first, done(429)},
			want:   &dashboard.Call{Doing: dashboard.Refused},
		},
		{
			name: "refused, then its 429 passed on: refused still", at: 4 * time.Second,
			events: []router.StreamEvent{sent, refusal(router.StreamRefused, 429), first, done(429)},
			want:   &dashboard.Call{Doing: dashboard.Refused},
		},
		{
			name: "throttled: no pulse", events: []router.StreamEvent{sent, refusal(router.StreamThrottled, 429)}, at: 600 * time.Millisecond,
			want: &dashboard.Call{Doing: dashboard.Throttled},
		},
		{
			name: "throttled, until it's sent again", events: []router.StreamEvent{sent, refusal(router.StreamThrottled, 429)}, at: time.Minute,
			want: &dashboard.Call{Doing: dashboard.Throttled},
		},
		{
			name: "throttled, then sent again: asking afresh", at: 2*time.Second + 270*time.Millisecond,
			events: []router.StreamEvent{sent, refusal(router.StreamThrottled, 429), alter(told(router.StreamSent, "r1", "work", 2*time.Second), func(e *router.StreamEvent) { e.Attempt = 2 })},
			want:   &dashboard.Call{Doing: dashboard.Asking, Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5}},
		},
		{name: "the client's quota check: nothing", events: []router.StreamEvent{alter(sent, func(e *router.StreamEvent) { e.Check = true })}, at: 270 * time.Millisecond},
		{name: "a request of no session: nothing", events: []router.StreamEvent{alter(sent, func(e *router.StreamEvent) { e.Session = "" })}, at: 270 * time.Millisecond},
		{
			name: "in flight as the stream opens, sent with nothing back yet", at: time.Second,
			events: []router.StreamEvent{alter(told(router.StreamInFlight, "r1", "work", time.Second), func(e *router.StreamEvent) { e.SentAt = past(-38 * time.Second) })},
			want:   &dashboard.Call{Doing: dashboard.Asking},
		},
		{
			name: "in flight as the stream opens, its answer streaming", at: time.Second,
			events: []router.StreamEvent{alter(told(router.StreamInFlight, "r1", "work", time.Second), func(e *router.StreamEvent) {
				e.SentAt, e.FirstAt, e.Chars = past(-20*time.Second), past(-18*time.Second), 4800
			})},
			want: &dashboard.Call{Doing: dashboard.Streaming, Shimmer: 237},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drawn := traffic{}.took(tt.events, heldNowhere, past(tt.at)).at(past(tt.at))
			got, ok := drawn.Calls[dashboard.Plug{Session: idD28C, Account: "work"}]
			switch {
			case tt.want == nil && ok:
				t.Errorf("it's drawn %+v, want nothing travelling it", got)
			case tt.want != nil && !reflect.DeepEqual(got, *tt.want):
				t.Errorf("it's drawn\n%+v\nwant\n%+v", got, *tt.want)
			}
		})
	}
}

func TestAMoveARefusalBringsShowsAsItsPulseComesHome(t *testing.T) {
	const refused = 300 * time.Millisecond
	tr := traffic{}.took(movedToSide(refused), heldNowhere, past(refused))
	shows := refused + pulseFor
	tests := []struct {
		name string
		at   time.Duration
		// work and side are what travels d28c's cords there, and moved
		// whether the views read its move.
		work, side dashboard.Call
		moved      bool
	}{
		{
			name: "its refusal bouncing back: ✕ on work, nothing on side yet", at: refused + 270*time.Millisecond,
			work: dashboard.Call{Doing: dashboard.Refused, Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5, Back: true, Red: true}},
		},
		{
			name: "all but home", at: shows - time.Millisecond,
			work: dashboard.Call{Doing: dashboard.Refused, Pulsing: true, Pulse: dashboard.Pulse{Along: float64(pulseFor-time.Millisecond) / float64(pulseFor), Back: true, Red: true}},
		},
		{
			name: "home: the ✕ cleared, its pulse setting out on side", at: shows,
			side: dashboard.Call{Doing: dashboard.Asking, Pulsing: true}, moved: true,
		},
		{
			name: "after: its pulse out along side's cord", at: shows + 270*time.Millisecond,
			side: dashboard.Call{Doing: dashboard.Asking, Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5}}, moved: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drawn := tr.at(past(tt.at))
			if got := drawn.On(idD28C, "work"); got != tt.work {
				t.Errorf("on work, it's drawn %+v, want %+v", got, tt.work)
			}
			if got := drawn.On(idD28C, "side"); got != tt.side {
				t.Errorf("on side, it's drawn %+v, want %+v", got, tt.side)
			}
			if len(drawn.Loose) > 0 {
				t.Errorf("the loose cords are %+v, want none: work's limit moved it, its stub standing for its cord", drawn.Loose)
			}
			if moves := tr.stream(past(tt.at)).Moves; (len(moves) == 1) != tt.moved || len(moves) > 1 {
				t.Errorf("the views read the moves %+v, want its move read: %v, as the cords show it", moves, tt.moved)
			}
		})
	}
}

func TestAMoveNoRefusalBroughtShowsAtOnce(t *testing.T) {
	tr := traffic{}.took([]router.StreamEvent{pinnedToSide(0)}, heldNowhere, past(0))

	if moves := tr.stream(past(0)).Moves; len(moves) != 1 || moves[0].To != "side" {
		t.Errorf("as it's told of, the views read the moves %+v, want it, d28c joining side's group at once", moves)
	}
	if loose := tr.at(past(0)).Loose; !slices.Equal(loose, []dashboard.Loose{{Session: idD28C, From: "work"}}) {
		t.Errorf("as it's told of, the loose cords are %+v, want d28c's, hanging from work", loose)
	}
}

func TestACordARefusalLetGoHangsLooseOnceItsPulseIsHome(t *testing.T) {
	const refused = 300 * time.Millisecond
	events := []router.StreamEvent{
		told(router.StreamSent, "r1", "work", 0),
		alter(told(router.StreamRefused, "r1", "work", refused), func(e *router.StreamEvent) { e.Status = 403 }),
		alter(told(router.StreamMoved, "r1", "side", refused), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", "side", "moved: work refused" }),
	}
	tr := traffic{}.took(events, heldNowhere, past(refused))

	if loose := tr.at(past(refused + pulseFor - time.Millisecond)).Loose; len(loose) > 0 {
		t.Errorf("as its refusal bounces back, the loose cords are %+v, want none yet", loose)
	}
	if loose := tr.at(past(refused + pulseFor)).Loose; !slices.Equal(loose, []dashboard.Loose{{Session: idD28C, From: "work"}}) {
		t.Errorf("once it has, the loose cords are %+v, want d28c's, hanging from work, no limit having moved it", loose)
	}
}

func TestTwoModelsOfASessionOnAnAccountTravelItsOneCord(t *testing.T) {
	const at = 2*time.Second + 270*time.Millisecond
	opusStreams := []router.StreamEvent{told(router.StreamSent, "r1", "work", 0), told(router.StreamFirst, "r1", "work", time.Second)}
	opusAnswered := append(slices.Clone(opusStreams), alter(told(router.StreamDone, "r1", "work", 2*time.Second), func(e *router.StreamEvent) { e.Status = 200 }))
	haikuAsks := func(d time.Duration) []router.StreamEvent {
		return []router.StreamEvent{ofHaiku(told(router.StreamSent, "r2", "work", d))}
	}
	haikuStreams := []router.StreamEvent{ofHaiku(told(router.StreamSent, "r2", "work", time.Second)), ofHaiku(told(router.StreamFirst, "r2", "work", 1500*time.Millisecond))}
	alone := func(events []router.StreamEvent) dashboard.Call {
		return traffic{}.took(events, heldNowhere, past(at)).at(past(at)).On(idD28C, "work")
	}
	tests := []struct {
		name          string
		opus, haiku   []router.StreamEvent
		doing         dashboard.Doing
		shimmer, beat []router.StreamEvent
	}{
		{
			name: "opus streaming, haiku asking: streaming, opus's shimmer, haiku's pulse out", opus: opusStreams, haiku: haikuAsks(2 * time.Second),
			doing: dashboard.Streaming, shimmer: opusStreams, beat: haikuAsks(2 * time.Second),
		},
		{
			name: "opus's answer ended, haiku streaming: streaming, haiku's shimmer, opus's pulse back", opus: opusAnswered, haiku: haikuStreams,
			doing: dashboard.Streaming, shimmer: haikuStreams, beat: opusAnswered,
		},
		{
			name: "both asking: the pulse of the one sent last", opus: []router.StreamEvent{told(router.StreamSent, "r3", "work", 2*time.Second)}, haiku: haikuAsks(2*time.Second + 100*time.Millisecond),
			doing: dashboard.Asking, beat: haikuAsks(2*time.Second + 100*time.Millisecond),
		},
		{
			name: "both streaming: the shimmer of the one streaming last, no pulse", opus: opusStreams, haiku: haikuStreams,
			doing: dashboard.Streaming, shimmer: haikuStreams,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drawn := traffic{}.took(slices.Concat(tt.opus, tt.haiku), heldNowhere, past(at)).at(past(at))
			beat := alone(tt.beat)
			want := dashboard.Call{Doing: tt.doing, Shimmer: alone(tt.shimmer).Shimmer, Pulse: beat.Pulse, Pulsing: beat.Pulsing}
			if want.Pulsing != (tt.beat != nil) || (tt.shimmer != nil) != (want.Shimmer > 0) {
				t.Fatalf("the pulse and shimmer to want, %+v, aren't travelling as the case has them", want)
			}
			if got := drawn.On(idD28C, "work"); len(drawn.Calls) != 1 || got != want {
				t.Errorf("the cords drawn are %+v, want d28c's one on work, %+v", drawn.Calls, want)
			}
		})
	}
}

// d28cListedOn is the router's listing of its sessions, d28c's opus on the
// account with the given id.
func d28cListedOn(account string) []status.Session {
	return []status.Session{{ID: idD28C, Assignments: []status.Assignment{{Model: opus, Family: "opus", Account: account}}}}
}

func TestTheViewsReadAMoveTillAListingShowsItDone(t *testing.T) {
	tests := []struct {
		name string
		tr   traffic
		// want is whether the move to side is listed done.
		want bool
	}{
		{name: "listed on work, where it was: read", tr: traffic{}.took([]router.StreamEvent{pinnedToSide(0)}, heldNowhere, past(0)).listedAs(d28cListedOn("work"))},
		{name: "listed on side, where it went: done", tr: traffic{}.took([]router.StreamEvent{pinnedToSide(0)}, heldNowhere, past(0)).listedAs(d28cListedOn("side")), want: true},
		{name: "listed on side before the stream told of it: done as it's told", tr: traffic{}.listedAs(d28cListedOn("side")).took([]router.StreamEvent{pinnedToSide(0)}, heldNowhere, past(0)), want: true},
		{
			name: "moved on again, listed where it went last: done, as the later move is",
			tr: traffic{}.took([]router.StreamEvent{
				pinnedToSide(0), alter(pinnedToSide(time.Second), func(e *router.StreamEvent) { e.From, e.To = "side", "client" }),
			}, heldNowhere, past(time.Second)).listedAs(d28cListedOn("client")),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := slices.IndexFunc(tt.tr.moves, func(m move) bool { return m.to == "side" })
			if i < 0 || tt.tr.moves[i].listed != tt.want {
				t.Errorf("the moves are %+v, want the move to side listed done: %v", tt.tr.moves, tt.want)
			}
			read := slices.ContainsFunc(tt.tr.stream(past(time.Second)).Moves, func(m views.Moved) bool { return m.To == "side" })
			if read == tt.want {
				t.Errorf("the views read the move to side: %v, want %v", read, !tt.want)
			}
		})
	}
}

func TestAMoveNoListingShowsDoneGoesSeenForAfter(t *testing.T) {
	tr := traffic{}.took([]router.StreamEvent{pinnedToSide(0)}, heldNowhere, past(0)).listedAs(d28cListedOn("work"))

	if moves := tr.tidied(past(seenFor - time.Second)).moves; len(moves) != 1 {
		t.Errorf("short of seenFor after it, the moves kept are %+v, want it, read still", moves)
	}
	if moves := tr.tidied(past(seenFor)).moves; len(moves) > 0 {
		t.Errorf("seenFor after it, the moves kept are %+v, want none", moves)
	}
}

func TestARouterRestartedForgetsWhatWasInFlightButKeepsItsMoves(t *testing.T) {
	events := []router.StreamEvent{
		told(router.StreamSent, "r1", "work", 0),
		told(router.StreamFirst, "r1", "work", 500*time.Millisecond),
		alter(pinnedToSide(time.Second), func(e *router.StreamEvent) { e.Request = "r2" }),
	}
	tr := traffic{live: true}.took(events, heldNowhere, past(time.Second)).listedAs(d28cListedOn("work")).afresh()
	at := past(time.Second + looseFor/2)
	drawn := tr.at(at)

	if len(drawn.Calls) > 0 {
		t.Errorf("as another router answers, the cords drawn are %+v, want none, what was in flight gone with the router it was on", drawn.Calls)
	}
	if !slices.Equal(drawn.Loose, []dashboard.Loose{{Session: idD28C, From: "work", Fade: 0.5}}) {
		t.Errorf("the loose cords are %+v, want d28c's off work fading still", drawn.Loose)
	}
	s := tr.stream(at)
	if s.Live || !s.Requests["r1"].Lost || len(s.Moves) != 1 {
		t.Errorf("the views read %+v, want it not live, r1 lost, and the move read, no listing having shown it done", s)
	}
	relisted := traffic{}.listedAs(d28cListedOn("side")).afresh().took([]router.StreamEvent{pinnedToSide(time.Second)}, heldNowhere, at)
	if moves := relisted.stream(at).Moves; len(moves) > 0 {
		t.Errorf("listed on side before another router answered, the views read the moves %+v, want none: where the last listing put each seat is kept", moves)
	}
}

func TestASessionsRequestsAtOnceShowTheOneInFlight(t *testing.T) {
	events := []router.StreamEvent{
		told(router.StreamSent, "r1", "work", 0),
		told(router.StreamFirst, "r1", "work", 500*time.Millisecond),
		told(router.StreamSent, "r2", "work", time.Second),
		alter(told(router.StreamDone, "r1", "work", 2*time.Second), func(e *router.StreamEvent) { e.Status = 200 }),
	}
	tr := traffic{}.took(events, heldNowhere, past(2*time.Second))

	if got := tr.at(past(2*time.Second)).On(idD28C, "work").Doing; got != dashboard.Asking {
		t.Errorf("r1 answered, r2 asking, d28c on work is drawn %v, want asking, r2 in flight", got)
	}
	tr = tr.took([]router.StreamEvent{told(router.StreamFirst, "r2", "work", 3*time.Second)}, heldNowhere, past(3*time.Second))
	if got := tr.at(past(3*time.Second)).On(idD28C, "work").Doing; got != dashboard.Streaming {
		t.Errorf("r2's answer streaming, it's drawn %v, want streaming", got)
	}
}

func TestARequestMovedOffAnAccountLeavesNothingThereOnceItsMoveHasShown(t *testing.T) {
	events := []router.StreamEvent{
		told(router.StreamSent, "r1", "work", 0),
		alter(told(router.StreamLimited, "r1", "work", 0), func(e *router.StreamEvent) { e.Status = 429 }),
		alter(told(router.StreamMoved, "r1", "side", 0), func(e *router.StreamEvent) { e.From, e.To = "work", "side" }),
	}
	tr := traffic{}.took(events, heldNowhere, past(0))

	if c, ok := tr.at(past(pulseFor)).Calls[dashboard.Plug{Session: idD28C, Account: "work"}]; ok {
		t.Errorf("as its move shows, d28c on work is drawn %+v, want nothing: its request has left", c)
	}
	if tidied := tr.tidied(past(pulseFor)); len(tidied.calls) > 0 {
		t.Errorf("tidied, the calls kept are %+v, want none, its request never going out where it was brought", tidied.calls)
	}
}

func TestARequestIsOutOnOneAccountAtATime(t *testing.T) {
	const refused = 300 * time.Millisecond
	limited := func(request string) router.StreamEvent {
		return alter(told(router.StreamLimited, request, "work", refused), func(e *router.StreamEvent) { e.Status = 429 })
	}
	again := func(request string) router.StreamEvent {
		return alter(told(router.StreamSent, request, "side", refused), func(e *router.StreamEvent) { e.Attempt = 2 })
	}
	moved := alter(told(router.StreamMoved, "r1", "side", refused), func(e *router.StreamEvent) {
		e.From, e.To, e.Reason = "work", "side", "moved: work hit its limit"
	})
	tests := []struct {
		name   string
		events []router.StreamEvent
		// gone is when nothing's left on work: once the requests are done
		// with there.
		gone time.Duration
	}{
		{
			name: "two of a session's requests refused at once, the second going on with no move of its own, its session moved for the first",
			events: []router.StreamEvent{
				told(router.StreamSent, "r1", "work", 0), told(router.StreamSent, "r2", "work", 0),
				limited("r1"), limited("r2"), moved, again("r1"), again("r2"),
			},
			gone: refused + pulseFor,
		},
		{
			name:   "a request refused as a pin moves its session: no move told of",
			events: []router.StreamEvent{told(router.StreamSent, "r1", "work", 0), limited("r1"), again("r1")},
			gone:   refused + pulseFor,
		},
		{
			name: "a request refused, then done on another account",
			events: []router.StreamEvent{
				told(router.StreamSent, "r1", "work", 0), limited("r1"),
				alter(told(router.StreamDone, "r1", "side", 2*time.Second), func(e *router.StreamEvent) { e.Status = 200 }),
			},
			gone: 2 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := traffic{}.took(tt.events, heldNowhere, past(refused))
			if c := tr.at(past(tt.gone-time.Millisecond)).On(idD28C, "work"); c.Doing != dashboard.Refused {
				t.Errorf("just short of when it's done with, d28c on work is drawn %+v, want refused still", c)
			}
			if c, ok := tr.at(past(tt.gone)).Calls[dashboard.Plug{Session: idD28C, Account: "work"}]; ok {
				t.Errorf("once it's done with, d28c on work is drawn %+v, want nothing, every request of its out on side", c)
			}
			for k := range tr.tidied(past(tt.gone)).calls {
				if k.account == "work" {
					t.Errorf("tidied, %s's call on work is kept, want it gone", k.request)
				}
			}
		})
	}
}

func TestALooseCordFadesOverLooseFor(t *testing.T) {
	tr := traffic{}.took([]router.StreamEvent{pinnedToSide(0)}, heldNowhere, past(0))
	tests := []struct {
		name   string
		listed bool
		at     time.Duration
		// fade is how far its loose cord has faded, -1 for none hanging;
		// and read whether the views read its move.
		fade float64
		read bool
	}{
		{name: "at once", at: 0, fade: 0, read: true},
		{name: "half faded", at: looseFor / 2, fade: 0.5, read: true},
		{name: "faded, but read until a listing shows it done", at: looseFor, fade: -1, read: true},
		{name: "listed done, fading still", listed: true, at: looseFor / 2, fade: 0.5},
		{name: "listed done, and faded: gone", listed: true, at: looseFor, fade: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := tr
			if tt.listed {
				at = tr.listedAs(d28cListedOn("side"))
			}
			var want []dashboard.Loose
			if tt.fade >= 0 {
				want = []dashboard.Loose{{Session: idD28C, From: "work", Fade: tt.fade}}
			}
			if loose := at.at(past(tt.at)).Loose; !slices.Equal(loose, want) {
				t.Errorf("the loose cords are %+v, want %+v", loose, want)
			}
			if read := len(at.stream(past(tt.at)).Moves) == 1; read != tt.read {
				t.Errorf("the views read its move: %v, want %v", read, tt.read)
			}
		})
	}
}

func TestAMoveOffAnAccountALimitHoldsBackHangsNoLooseCord(t *testing.T) {
	limited := func(id string) bool { return id == "work" }
	tr := traffic{}.took([]router.StreamEvent{pinnedToSide(0)}, limited, past(0))

	if loose := tr.at(past(0)).Loose; len(loose) > 0 {
		t.Errorf("the loose cords are %+v, want none: a limit holds work back, its stub standing for the cord", loose)
	}
	if moves := tr.stream(past(0)).Moves; len(moves) != 1 {
		t.Errorf("the views read the moves %+v, want the one", moves)
	}
	if listed := tr.listedAs(d28cListedOn("side")); len(listed.tidied(past(0)).moves) > 0 {
		t.Errorf("listed where it went, the moves kept are %+v, want none, the stub standing for it", listed.moves)
	}
}

func TestEveryCordsShimmerStepsOnTheClocksSteps(t *testing.T) {
	step := past(0).Add(untilStep(past(0), shimmerStep))
	first := func(request, session string, before time.Duration) []router.StreamEvent {
		return []router.StreamEvent{
			alter(told(router.StreamSent, request, "work", -time.Second), func(e *router.StreamEvent) { e.Session = session }),
			alter(told(router.StreamFirst, request, "work", 0), func(e *router.StreamEvent) { e.Session, e.At = session, step.Add(-before) }),
		}
	}
	tr := traffic{}.took(slices.Concat(first("r1", idD28C, 70*time.Millisecond), first("r2", id7F3A, 30*time.Millisecond)), heldNowhere, step.Add(-time.Millisecond))
	shimmers := func(at time.Time) [2]int {
		drawn := tr.at(at)
		return [2]int{drawn.On(idD28C, "work").Shimmer, drawn.On(id7F3A, "work").Shimmer}
	}

	if before, on := shimmers(step.Add(-time.Millisecond)), shimmers(step); before != [2]int{0, 0} || on != [2]int{1, 1} {
		t.Errorf("their first bytes 40ms apart, the shimmers stood at %v just before the clock's step and %v on it, want both a step on together, {1, 1}", before, on)
	}
}

func TestAFrameIsNextAsTheFirstMoveYetToShowShows(t *testing.T) {
	tr := traffic{moves: []move{{shows: past(2 * time.Second)}, {shows: past(time.Second)}, {shows: past(-time.Second)}}}
	if at, ok := tr.showing(past(0)); !ok || !at.Equal(past(time.Second)) {
		t.Errorf("the next move shows at %s (%t), want %s, the first yet to show", at, ok, past(time.Second))
	}
	if at, ok := tr.showing(past(2 * time.Second)); ok {
		t.Errorf("every move shown, the next shows at %s, want none", at)
	}
}
