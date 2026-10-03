package watch

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// opus is the model the stream's requests in the tests ask for.
const opus = "claude-opus-5-5"

// d28cOn is where d28c's opus is plugged in on the account with the given id.
func d28cOn(account string) dashboard.Plug {
	return dashboard.Plug{Account: account, Seat: dashboard.Seat{Session: idD28C, Model: opus}}
}

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

// heldNowhere says no limit holds any account back.
func heldNowhere(string) bool {
	return false
}

func TestWhatEachStreamEventDoes(t *testing.T) {
	sent := told(router.StreamSent, "r1", "work", 0)
	first := told(router.StreamFirst, "r1", "work", time.Second)
	progress := alter(told(router.StreamProgress, "r1", "work", 2*time.Second), func(e *router.StreamEvent) { e.Chars = 4936 })
	done := func(status int, tokens *quota.Tokens) router.StreamEvent {
		return alter(told(router.StreamDone, "r1", "work", 5*time.Second), func(e *router.StreamEvent) {
			e.Status, e.Chars, e.Tokens = status, 4936, tokens
		})
	}
	refusal := func(kind string, status int) router.StreamEvent {
		return alter(told(kind, "r1", "work", 500*time.Millisecond), func(e *router.StreamEvent) { e.Status = status })
	}
	tests := []struct {
		name   string
		events []router.StreamEvent
		at     time.Duration
		// want is what d28c's opus on work is doing at, as the frame draws
		// it, nil where it does nothing.
		want *dashboard.Call
	}{
		{
			name: "sent: asking, a pulse running out to the jack", events: []router.StreamEvent{sent}, at: 270 * time.Millisecond,
			want: &dashboard.Call{Doing: dashboard.Asking, Since: past(0), Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5}},
		},
		{
			name: "the pulse at the jack: asking still", events: []router.StreamEvent{sent}, at: time.Second,
			want: &dashboard.Call{Doing: dashboard.Asking, Since: past(0), Pulse: dashboard.Pulse{Along: 1}},
		},
		{
			name: "its first byte: streaming, its shimmer a step on each shimmerStep", events: []router.StreamEvent{sent, first}, at: time.Second + 3*shimmerStep,
			want: &dashboard.Call{Doing: dashboard.Streaming, Since: past(0), Shimmer: 3, Pulse: dashboard.Pulse{Along: 3 * float64(shimmerStep) / float64(pulseFor)}},
		},
		{
			name: "its progress: its tokens so far, estimated at four characters a token", events: []router.StreamEvent{sent, first, progress}, at: 2 * time.Second,
			want: &dashboard.Call{Doing: dashboard.Streaming, Since: past(0), Tokens: 1234, Shimmer: 13, Pulse: dashboard.Pulse{Along: 1}},
		},
		{
			name: "done with success: answered, its tokens as its closing usage counts them, a pulse running back", at: 5*time.Second + 270*time.Millisecond,
			events: []router.StreamEvent{sent, first, progress, done(200, &quota.Tokens{Input: 12, Output: 1180})},
			want:   &dashboard.Call{Doing: dashboard.Answered, Since: past(0), Tokens: 1180, Exact: true, Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5, Back: true}},
		},
		{
			name: "done without its tokens counted: the estimate stands", at: 6 * time.Second,
			events: []router.StreamEvent{sent, first, progress, done(200, nil)},
			want:   &dashboard.Call{Doing: dashboard.Answered, Since: past(0), Tokens: 1234, Pulse: dashboard.Pulse{Along: 1, Back: true}},
		},
		{name: "answered answeredFor ago: over", events: []router.StreamEvent{sent, first, done(200, nil)}, at: 5*time.Second + answeredFor},
		{name: "done unanswered, as its client went: over", events: []router.StreamEvent{sent, done(0, nil)}, at: 5 * time.Second},
		{
			name: "limited: refused, a red pulse running back", events: []router.StreamEvent{sent, refusal(router.StreamLimited, 429)}, at: 770 * time.Millisecond,
			want: &dashboard.Call{Doing: dashboard.Refused, Since: past(0), Status: 429, Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5, Back: true, Red: true}},
		},
		{
			name: "refused alone: the same", events: []router.StreamEvent{sent, refusal(router.StreamRefused, 403)}, at: 770 * time.Millisecond,
			want: &dashboard.Call{Doing: dashboard.Refused, Since: past(0), Status: 403, Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5, Back: true, Red: true}},
		},
		{
			name: "limited, then ended so: refused still, until answeredFor after", events: []router.StreamEvent{sent, refusal(router.StreamLimited, 429), done(429, nil)},
			at:   5*time.Second + answeredFor - time.Millisecond,
			want: &dashboard.Call{Doing: dashboard.Refused, Since: past(0), Status: 429, Pulse: dashboard.Pulse{Along: 1, Back: true, Red: true}},
		},
		{
			name: "limited, then its 429 passed on, as the router taps its body: refused still", at: 6 * time.Second,
			events: []router.StreamEvent{sent, refusal(router.StreamLimited, 429), first, done(429, nil)},
			want:   &dashboard.Call{Doing: dashboard.Refused, Since: past(0), Status: 429, Pulse: dashboard.Pulse{Along: 1, Back: true, Red: true}},
		},
		{
			name: "refused, then its 429 passed on: refused still", at: 6 * time.Second,
			events: []router.StreamEvent{sent, refusal(router.StreamRefused, 429), first, done(429, nil)},
			want:   &dashboard.Call{Doing: dashboard.Refused, Since: past(0), Status: 429, Pulse: dashboard.Pulse{Along: 1, Back: true, Red: true}},
		},
		{
			name: "throttled: no pulse", events: []router.StreamEvent{sent, refusal(router.StreamThrottled, 429)}, at: 600 * time.Millisecond,
			want: &dashboard.Call{Doing: dashboard.Throttled, Since: past(0), Status: 429, Pulse: dashboard.Pulse{Along: 100 * float64(time.Millisecond) / float64(pulseFor)}},
		},
		{
			name: "throttled, then sent again: asking afresh", at: 2*time.Second + 270*time.Millisecond,
			events: []router.StreamEvent{sent, refusal(router.StreamThrottled, 429), alter(told(router.StreamSent, "r1", "work", 2*time.Second), func(e *router.StreamEvent) { e.Attempt = 2 })},
			want:   &dashboard.Call{Doing: dashboard.Asking, Since: past(2 * time.Second), Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5}},
		},
		{name: "the client's quota check: nothing", events: []router.StreamEvent{alter(sent, func(e *router.StreamEvent) { e.Check = true })}, at: 270 * time.Millisecond},
		{name: "a request of no session: nothing", events: []router.StreamEvent{alter(sent, func(e *router.StreamEvent) { e.Session = "" })}, at: 270 * time.Millisecond},
		{
			name: "in flight as the stream opens, sent with nothing back yet", at: time.Second,
			events: []router.StreamEvent{alter(told(router.StreamInFlight, "r1", "work", time.Second), func(e *router.StreamEvent) { e.SentAt = past(-38 * time.Second) })},
			want:   &dashboard.Call{Doing: dashboard.Asking, Since: past(-38 * time.Second), Pulse: dashboard.Pulse{Along: 1}},
		},
		{
			name: "in flight as the stream opens, its answer streaming", at: time.Second,
			events: []router.StreamEvent{alter(told(router.StreamInFlight, "r1", "work", time.Second), func(e *router.StreamEvent) {
				e.SentAt, e.FirstAt, e.Chars = past(-20*time.Second), past(-18*time.Second), 4800
			})},
			want: &dashboard.Call{Doing: dashboard.Streaming, Since: past(-20 * time.Second), Tokens: 1200, Shimmer: 237, Pulse: dashboard.Pulse{Along: 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drawn := traffic{}.took(tt.events, heldNowhere, past(tt.at)).at(past(tt.at))
			got, ok := drawn.Calls[d28cOn("work")]
			switch {
			case tt.want == nil && ok:
				t.Errorf("it's drawn %+v, want it drawn doing nothing", got)
			case tt.want != nil && !reflect.DeepEqual(got, *tt.want):
				t.Errorf("it's drawn\n%+v\nwant\n%+v", got, *tt.want)
			}
		})
	}
}

func TestAMoveShowsOnceTheRefusalThatMovedItHasBouncedBack(t *testing.T) {
	refused := 300 * time.Millisecond
	events := []router.StreamEvent{
		told(router.StreamSent, "r1", "work", 0),
		alter(told(router.StreamLimited, "r1", "work", refused), func(e *router.StreamEvent) { e.Status = 429 }),
		alter(told(router.StreamMoved, "r1", "side", refused), func(e *router.StreamEvent) {
			e.From, e.To, e.Reason = "work", "side", "moved: work hit its limit"
		}),
		alter(told(router.StreamSent, "r1", "side", refused), func(e *router.StreamEvent) { e.Attempt = 2 }),
	}
	tr := traffic{}.took(events, heldNowhere, past(refused))
	shows := refused + pulseFor

	during := tr.at(past(refused + 100*time.Millisecond))
	if len(during.Moves) > 0 || during.Calls[d28cOn("work")].Doing != dashboard.Refused {
		t.Errorf("while its refusal bounces back, the traffic is %+v, want work's call refused, and no move yet", during)
	}
	later := tr.at(past(shows + 270*time.Millisecond))
	wantMove := dashboard.Move{Seat: d28cOn("work").Seat, From: "work", To: "side", At: past(refused), Reason: "moved: work hit its limit", Held: true}
	if len(later.Moves) != 1 || later.Moves[0] != wantMove {
		t.Errorf("once it has, the moves are %+v, want %+v, held, as work's limit moved it", later.Moves, wantMove)
	}
	wantCall := dashboard.Call{Doing: dashboard.Asking, Since: past(refused), New: true, Pulsing: true, Pulse: dashboard.Pulse{Along: 0.5}}
	if got := later.Calls[d28cOn("side")]; got != wantCall {
		t.Errorf("on side, it's drawn %+v, want %+v, new, its pulse out from when the move shows", got, wantCall)
	}

	listed := tr.listedAs(d28cListedOn("side"))
	if moves := listed.at(past(shows + time.Second)).Moves; len(moves) > 0 {
		t.Errorf("once the sessions are listed with d28c on side, the moves are %+v, want none: work's limit's own stubs stand for it", moves)
	}
}

// d28cListedOn is the router's listing of its sessions, d28c's opus on the
// account with the given id.
func d28cListedOn(account string) []status.Session {
	return []status.Session{{ID: idD28C, Assignments: []status.Assignment{{Model: opus, Family: "opus", Account: account}}}}
}

func TestAMoveRePatchesTillAListingShowsItDone(t *testing.T) {
	moved := func(to string, d time.Duration) router.StreamEvent {
		return alter(told(router.StreamMoved, "r1", to, d), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", to, "pinned" })
	}
	tests := []struct {
		name string
		tr   traffic
		// want is whether the move to side is listed done.
		want bool
	}{
		{name: "listed on work, where it was: re-patching", tr: traffic{}.took([]router.StreamEvent{moved("side", 0)}, heldNowhere, past(0)).listedAs(d28cListedOn("work"))},
		{name: "listed on side, where it went: done", tr: traffic{}.took([]router.StreamEvent{moved("side", 0)}, heldNowhere, past(0)).listedAs(d28cListedOn("side")), want: true},
		{name: "listed on side before the stream told of it: done as it's told", tr: traffic{}.listedAs(d28cListedOn("side")).took([]router.StreamEvent{moved("side", 0)}, heldNowhere, past(0)), want: true},
		{
			name: "moved on again, listed where it went last: done, as the later move is",
			tr:   traffic{}.took([]router.StreamEvent{moved("side", 0), alter(moved("client", time.Second), func(e *router.StreamEvent) { e.From = "side" })}, heldNowhere, past(time.Second)).listedAs(d28cListedOn("client")),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := slices.IndexFunc(tt.tr.moves, func(m move) bool { return m.To == "side" })
			if i < 0 || tt.tr.moves[i].Listed != tt.want {
				t.Errorf("the moves are %+v, want the move to side listed done: %v", tt.tr.moves, tt.want)
			}
		})
	}
}

func TestAMoveNoListingShowsDoneGoesSeenForAfter(t *testing.T) {
	moved := alter(told(router.StreamMoved, "r1", "side", 0), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", "side", "pinned" })
	tr := traffic{}.took([]router.StreamEvent{moved}, heldNowhere, past(0)).listedAs(d28cListedOn("work"))

	if moves := tr.tidied(past(seenFor - time.Second)).moves; len(moves) != 1 {
		t.Errorf("short of seenFor after it, the moves kept are %+v, want it, re-patching still", moves)
	}
	if moves := tr.tidied(past(seenFor)).moves; len(moves) > 0 {
		t.Errorf("seenFor after it, the moves kept are %+v, want none", moves)
	}
}

func TestTheStreamOpeningAgainKeepsItsMoves(t *testing.T) {
	moved := alter(told(router.StreamMoved, "r1", "side", 0), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", "side", "pinned" })
	tr := traffic{live: true}.took([]router.StreamEvent{moved}, heldNowhere, past(0)).afresh()

	if drawn := tr.at(past(time.Second)); drawn.Live || len(drawn.Moves) != 1 || drawn.Moves[0].Listed {
		t.Errorf("forgotten as the stream opens again, the traffic is drawn %+v, want its move re-patching still, no listing having shown it done", drawn)
	}
}

func TestASeatsRequestsAtOnceShowTheOneInFlight(t *testing.T) {
	events := []router.StreamEvent{
		told(router.StreamSent, "r1", "work", 0),
		told(router.StreamFirst, "r1", "work", 500*time.Millisecond),
		told(router.StreamSent, "r2", "work", time.Second),
		alter(told(router.StreamDone, "r1", "work", 2*time.Second), func(e *router.StreamEvent) { e.Status = 200 }),
	}
	tr := traffic{}.took(events, heldNowhere, past(2*time.Second))

	if got := tr.at(past(2 * time.Second)).Calls[d28cOn("work")].Doing; got != dashboard.Asking {
		t.Errorf("r1 answered, r2 asking, d28c's opus on work is drawn %v, want asking, r2 in flight", got)
	}
	tr = tr.took([]router.StreamEvent{told(router.StreamFirst, "r2", "work", 3*time.Second)}, heldNowhere, past(3*time.Second))
	if got := tr.at(past(3 * time.Second)).Calls[d28cOn("work")].Doing; got != dashboard.Streaming {
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
	gone := pulseFor + answeredFor

	if _, ok := tr.at(past(gone)).Calls[d28cOn("work")]; ok {
		t.Errorf("answeredFor after its move showed, d28c's opus on work is drawn %+v, want nothing: its request has left", tr.at(past(gone)).Calls[d28cOn("work")])
	}
	if tidied := tr.tidied(past(gone)); len(tidied.calls) > 0 {
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
		// gone is when nothing's left on work: answeredFor once the requests
		// are done with there.
		gone time.Duration
	}{
		{
			name: "two of a session's requests refused at once, the second going on with no move of its own, its session moved for the first",
			events: []router.StreamEvent{
				told(router.StreamSent, "r1", "work", 0), told(router.StreamSent, "r2", "work", 0),
				limited("r1"), limited("r2"), moved, again("r1"), again("r2"),
			},
			gone: refused + pulseFor + answeredFor,
		},
		{
			name:   "a request refused as a pin moves its session: no move told of",
			events: []router.StreamEvent{told(router.StreamSent, "r1", "work", 0), limited("r1"), again("r1")},
			gone:   refused + pulseFor + answeredFor,
		},
		{
			name: "a request refused, then done on another account",
			events: []router.StreamEvent{
				told(router.StreamSent, "r1", "work", 0), limited("r1"),
				alter(told(router.StreamDone, "r1", "side", 2*time.Second), func(e *router.StreamEvent) { e.Status = 200 }),
			},
			gone: 2*time.Second + answeredFor,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := traffic{}.took(tt.events, heldNowhere, past(refused))
			if c := tr.at(past(tt.gone - time.Millisecond)).Calls[d28cOn("work")]; c.Doing != dashboard.Refused {
				t.Errorf("just short of answeredFor once it's done with, d28c's opus on work is drawn %+v, want refused still", c)
			}
			if c, ok := tr.at(past(tt.gone)).Calls[d28cOn("work")]; ok {
				t.Errorf("answeredFor once it's done with, d28c's opus on work is drawn %+v, want nothing, every request of its out on side", c)
			}
			for k := range tr.tidied(past(tt.gone)).calls {
				if k.account == "work" {
					t.Errorf("tidied, %s's call on work is kept, want it gone", k.request)
				}
			}
		})
	}
}

func TestWhenTheStreamLastSawASeatOutlastsItsCalls(t *testing.T) {
	answered := 90 * time.Second
	events := []router.StreamEvent{
		told(router.StreamSent, "r1", "work", 0),
		told(router.StreamFirst, "r1", "work", time.Second),
		alter(told(router.StreamDone, "r1", "work", answered), func(e *router.StreamEvent) { e.Status = 200 }),
	}
	tr := traffic{}.took(events, heldNowhere, past(answered))
	tidied := past(answered + answeredFor)

	drawn := tr.tidied(tidied).at(tidied)
	if c, ok := drawn.Calls[d28cOn("work")]; ok {
		t.Fatalf("answeredFor after its answer ended, d28c's opus on work is drawn %+v, want its call done with", c)
	}
	if got := drawn.Seen[d28cOn("work")]; !got.Equal(past(answered)) {
		t.Errorf("once its call is done with, the stream last saw d28c's opus on work at %s, want %s, as its answer ended", got, past(answered))
	}
	if got := tr.afresh().at(tidied).Seen[d28cOn("work")]; !got.Equal(past(answered)) {
		t.Errorf("as the stream opens again, the stream last saw it at %s, want %s kept", got, past(answered))
	}
	if seen, ok := tr.tidied(past(answered + seenFor)).at(past(answered + seenFor)).Seen[d28cOn("work")]; ok {
		t.Errorf("seenFor after, the stream last saw it at %s, want it forgotten, as the router's listing says since", seen)
	}
}

func TestAMovesCordFadesWithinAFewSeconds(t *testing.T) {
	moved := alter(told(router.StreamMoved, "r1", "side", 0), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", "side", "moved by pin" })
	tr := traffic{}.took([]router.StreamEvent{moved}, heldNowhere, past(0))
	tests := []struct {
		name   string
		listed bool
		at     time.Duration
		// want is the move as it's drawn, nil for none.
		want *dashboard.Move
	}{
		{name: "at once, no refusal to bounce back", at: 0, want: &dashboard.Move{Fade: 0}},
		{name: "half faded", at: looseFor / 2, want: &dashboard.Move{Fade: 0.5}},
		{name: "faded, but re-patching until a listing shows it done", at: looseFor + time.Second, want: &dashboard.Move{Fade: 1}},
		{name: "listed done, fading still", listed: true, at: looseFor / 2, want: &dashboard.Move{Fade: 0.5, Listed: true}},
		{name: "listed done, and faded: gone", listed: true, at: looseFor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := tr
			if tt.listed {
				at = tr.listedAs(d28cListedOn("side"))
			}
			moves := at.at(past(tt.at)).Moves
			switch {
			case tt.want == nil && len(moves) > 0:
				t.Errorf("the moves are %+v, want none", moves)
			case tt.want != nil && (len(moves) != 1 || moves[0].Fade != tt.want.Fade || moves[0].Listed != tt.want.Listed || moves[0].Held):
				t.Errorf("the moves are %+v, want one faded %.1f, listed %v, not held", moves, tt.want.Fade, tt.want.Listed)
			}
		})
	}
}

func TestAMoveOffAnAccountALimitHoldsBackIsHeld(t *testing.T) {
	moved := alter(told(router.StreamMoved, "r1", "side", 0), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", "side", "moved: work has no room" })
	limited := func(id string) bool { return id == "work" }
	moves := traffic{}.took([]router.StreamEvent{moved}, limited, past(0)).at(past(looseFor)).Moves

	if len(moves) != 1 || !moves[0].Held || moves[0].Fade != 0 {
		t.Errorf("the moves are %+v, want one held, unfaded, as a limit holds work back", moves)
	}
}

func TestAMoveYetToShowShowsOnceItsRefusalHasBouncedBack(t *testing.T) {
	refused := 300 * time.Millisecond
	tr := traffic{}.took(movedToSide(refused), heldNowhere, past(refused))

	if shows, ok := tr.showing(past(refused + time.Millisecond)); !ok || !shows.Equal(past(refused+pulseFor)) {
		t.Errorf("as its refusal bounces back, the move shows next at %s, %v, want at %s", shows, ok, past(refused+pulseFor))
	}
	if shows, ok := tr.showing(past(refused + pulseFor)); ok {
		t.Errorf("once it shows, the next move shows at %s, want none yet to", shows)
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
		return [2]int{drawn.Calls[d28cOn("work")].Shimmer, drawn.Calls[dashboard.Plug{Account: "work", Seat: dashboard.Seat{Session: id7F3A, Model: opus}}].Shimmer}
	}

	if before, on := shimmers(step.Add(-time.Millisecond)), shimmers(step); before != [2]int{0, 0} || on != [2]int{1, 1} {
		t.Errorf("their first bytes 40ms apart, the shimmers stood at %v just before the clock's step and %v on it, want both a step on together, {1, 1}", before, on)
	}
}
