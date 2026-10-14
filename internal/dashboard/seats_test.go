package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The sample sessions' ids, each a Claude Code session's.
const (
	idD28C = "d28c5e17-3a9b-4f60-8c21-7e4d9b0a6f13"
	idDB8A = "db8a71e3-9f04-4c6d-b2a8-5e1f7c39d860"
	id7F3A = "7f3a0c94-5d2e-4b18-a6f7-31c8e9d24b05"
	idC61B = "c61b2f8d-04a7-4e93-9b5c-d81e6a7f2c34"
	id41E0 = "41e0b6c2-8d3f-4a71-95e4-2c7b0f9a1d68"
)

// The sample sessions' models.
const (
	opus   = "claude-opus-5-5"
	sonnet = "claude-sonnet-5-5"
	haiku  = "claude-haiku-4-5"
)

// seatOf is a session's model on an account, as the router assigns it: put
// there made before now, for the reason given, and last seen seen before
// now.
func seatOf(model, account, reason string, made, seen time.Duration) status.Assignment {
	family := strings.Split(model, "-")[1]
	return status.Assignment{
		Model: model, Family: family, Account: account, Reason: reason,
		AssignedAt: now.Add(-made).UTC(), LastSeen: now.Add(-seen).UTC(),
	}
}

// flipping is the router's document of work, personal and side, as
// milestone 5's frames have them: work under pressure, personal at the
// limit that moved its sessions to side, and side, where new sessions go;
// and what the router tells of lately. An hour ago, side was primed; ten
// minutes later, personal reached its limit, moving three sessions to side,
// two of which are told of; work came under pressure half an hour ago;
// and a minute ago, idC61B started on side.
func flipping() status.Document {
	doc := threeRouted()
	doc.Events = []status.Event{
		{ID: 7, At: now.Add(-time.Minute).UTC(), Kind: status.EventStarted, Account: "side", Session: idC61B, Model: opus, Reason: "new"},
		{ID: 6, At: now.Add(-30 * time.Minute).UTC(), Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: now.Add(90 * time.Minute).UTC(), Since: now.Add(-time.Hour).UTC()},
		{ID: 5, At: now.Add(-50 * time.Minute).UTC(), Kind: status.EventMoved, Session: id41E0, Model: sonnet, From: "personal", To: "side", Reason: "moved: personal has no room", Limit: 3},
		{ID: 4, At: now.Add(-50 * time.Minute).UTC(), Kind: status.EventMoved, Session: idDB8A, Model: opus, From: "personal", To: "side", Reason: "moved: personal has no room", Limit: 3},
		{ID: 3, At: now.Add(-50 * time.Minute).UTC(), Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Until: now.Add(time.Hour).UTC(), Count: 3, To: "side"},
		{ID: 2, At: now.Add(-time.Hour).UTC(), Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}, Until: now.Add(4 * time.Hour).UTC()},
		{ID: 1, At: now.Add(-2 * time.Hour).UTC(), Kind: status.EventStarted, Account: "work", Session: idD28C, Model: opus, Reason: "new"},
	}
	return doc
}

// flippingSessions are the sessions the router lists of flipping, the one
// seen last first: idDB8A, whose sonnet is on work and whose opus the limit
// moved to side; idD28C, on work since it started; idC61B, new on side; 41e0,
// which the limit moved to side, idle; and 7f3a, on work, idle.
func flippingSessions() []status.Session {
	return []status.Session{
		{ID: idDB8A, Assignments: []status.Assignment{
			seatOf(sonnet, "work", "new", 2*time.Hour, 3*time.Second),
			seatOf(opus, "side", "moved: personal has no room", 50*time.Minute, 4*time.Second),
		}},
		{ID: idC61B, Assignments: []status.Assignment{seatOf(opus, "side", "new", time.Minute, 5*time.Second)}},
		{ID: idD28C, Assignments: []status.Assignment{seatOf(opus, "work", "new", 2*time.Hour, 50*time.Second)}},
		{ID: id41E0, Assignments: []status.Assignment{seatOf(sonnet, "side", "moved: personal has no room", 50*time.Minute, 4*time.Minute)}},
		{ID: id7F3A, Assignments: []status.Assignment{seatOf(haiku, "work", "new", 3*time.Hour, 9*time.Minute)}},
	}
}

func TestASeatsRowSaysWhatItsDoing(t *testing.T) {
	busy := line{spaces(2), {"●", positiveInk}, spaces(1), {"d28c  ", titleInk}, {"opus    ", secondaryInk}}
	idle := line{spaces(2), {"○", dimInk}, spaces(1), {"d28c  ", ink{token: theme.TextMuted}}, {"opus    ", secondaryInk}}
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	tests := []struct {
		name string
		// seen is how long ago the router listed it as last seen; live whether
		// the watch reads the request stream; told how long ago the stream
		// last told of it, where it has; and call what it tells of it now,
		// where it tells of anything.
		seen time.Duration
		live bool
		told time.Duration
		call *Call
		want line
	}{
		{
			name: "without the stream, seen in the last minute: busy, seen now", seen: 59 * time.Second,
			want: slices.Concat(busy, line{{"seen       ", secondaryInk}, {"now", secondaryInk}}),
		},
		{
			name: "without the stream, seen longer ago: idle so long, to the nearest minute", seen: 9*time.Minute + 30*time.Second,
			want: slices.Concat(idle, line{{"idle       ", dimInk}, {"10m", secondaryInk}}),
		},
		{
			name: "its answer streaming: busy, its tokens so far estimated", seen: 9 * time.Minute, live: true, told: time.Second,
			call: &Call{Doing: Streaming, Since: ago(time.Minute), Tokens: 1234},
			want: slices.Concat(busy, line{{"streaming  ", streamingInk}, {"↓ ~1.2k", secondaryInk}}),
		},
		{
			name: "sent with nothing back yet: waiting so long, in seconds", seen: 9 * time.Minute, live: true, told: 38 * time.Second,
			call: &Call{Doing: Asking, Since: ago(38*time.Second + 600*time.Millisecond)},
			want: slices.Concat(busy, line{{"waiting    ", waitingInk}, {"38s", secondaryInk}}),
		},
		{
			name: "waiting a minute or more", seen: 9 * time.Minute, live: true, told: 2 * time.Minute,
			call: &Call{Doing: Asking, Since: ago(2*time.Minute + 5*time.Second)},
			want: slices.Concat(busy, line{{"waiting    ", waitingInk}, {"2m", secondaryInk}}),
		},
		{
			name: "throttled, to be sent again: waiting still", seen: 9 * time.Minute, live: true, told: time.Second,
			call: &Call{Doing: Throttled, Since: ago(5 * time.Second), Status: 429},
			want: slices.Concat(busy, line{{"waiting    ", waitingInk}, {"5s", secondaryInk}}),
		},
		{
			name: "its answer just ended: its tokens, exact, though listed long since", seen: 9 * time.Minute, live: true, told: time.Second,
			call: &Call{Doing: Answered, Since: ago(time.Minute), Tokens: 1321, Exact: true},
			want: slices.Concat(busy, line{{"↓ 1.3k     ", streamingInk}, {"", secondaryInk}}),
		},
		{
			name: "its answer just ended, its tokens uncounted: the estimate stands", seen: 9 * time.Minute, live: true, told: time.Second,
			call: &Call{Doing: Answered, Since: ago(time.Minute), Tokens: 1234},
			want: slices.Concat(busy, line{{"↓ ~1.2k    ", streamingInk}, {"", secondaryInk}}),
		},
		{
			name: "with the stream, told of in the last minute: busy, idle so long, in seconds", seen: 9 * time.Minute, live: true, told: 38 * time.Second,
			want: slices.Concat(busy, line{{"idle       ", dimInk}, {"38s", secondaryInk}}),
		},
		{
			name: "with the stream, seen in the last minute as the router listed it: the same", seen: 59 * time.Second, live: true,
			want: slices.Concat(busy, line{{"idle       ", dimInk}, {"59s", secondaryInk}}),
		},
		{
			name: "with the stream, told of longer ago: idle so long, to the nearest minute", seen: 20 * time.Minute, live: true, told: 9*time.Minute + 29*time.Second,
			want: slices.Concat(idle, line{{"idle       ", dimInk}, {"9m", secondaryInk}}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := seated{session: status.Session{ID: idD28C}, assignment: seatOf(opus, "work", "new", time.Hour, tt.seen)}
			f := Frame{Traffic: Traffic{Live: tt.live}}
			if tt.told > 0 {
				f.Traffic.Seen = map[Plug]time.Time{s.plug(): ago(tt.told)}
			}
			if tt.call != nil {
				f.Traffic.Calls = map[Plug]Call{s.plug(): *tt.call}
			}
			if got := f.seatLine(s, modelColumn, now); !slices.Equal(got, tt.want) {
				t.Errorf("reads %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestASeatsNote(t *testing.T) {
	doc := flipping()
	pinnedAway := status.Session{ID: idD28C, Pin: "side", Assignments: []status.Assignment{seatOf(opus, "work", "new", 2*time.Hour, time.Minute)}}
	pinnedHere := status.Session{ID: idD28C, Pin: "work", Assignments: []status.Assignment{seatOf(opus, "work", "new", 2*time.Hour, time.Minute)}}
	split := status.Session{ID: idDB8A, Assignments: []status.Assignment{
		seatOf(sonnet, "work", "new", time.Hour, time.Minute), seatOf(opus, "side", "new", time.Hour, time.Minute),
		seatOf(haiku, "side", "new", time.Hour, time.Minute), seatOf("claude-fable-1", "personal", "new", time.Hour, time.Minute),
	}}
	yielding := func(reason string, yielded bool) status.Session {
		s := status.Session{ID: idD28C, Pin: "side", Assignments: []status.Assignment{seatOf(opus, "work", reason, time.Hour, time.Minute)}}
		s.Assignments[0].Yielded = yielded
		return s
	}
	pinnedSince := status.Session{ID: idD28C, Pin: "client", Assignments: []status.Assignment{seatOf(opus, "work", "pin yields: side has no room", time.Hour, time.Minute)}}
	pinnedAt := func(at time.Duration) status.Session {
		s := status.Session{ID: idC61B, Assignments: []status.Assignment{seatOf(opus, "work", "pinned", 10*time.Minute, time.Minute)}}
		s.Assignments[0].Pinned = true
		if at > 0 {
			s.Assignments[0].PinnedAt = now.Add(-at).UTC()
		}
		return s
	}
	moved := func(reason string) []status.Event {
		return slices.Concat([]status.Event{
			{ID: 9, At: now.Add(-10 * time.Minute).UTC(), Kind: status.EventMoved, Session: idC61B, Model: opus, From: "side", To: "work", Reason: reason},
		}, doc.Events)
	}
	tests := []struct {
		name    string
		account string
		session status.Session
		events  []status.Event
		want    string
	}{
		{name: "its own pin sending it to another account", account: "work", session: pinnedAway, want: "goes to side from its next request"},
		{name: "its own pin having yielded here at a limit", account: "work", session: yielding("pin yields: side has no room", true), want: "its pin to side yielded here"},
		{name: "its own pin having yielded, the session staying where it was", account: "work", session: yielding("new", true), want: "its pin to side yielded here"},
		{name: "its own pin no longer yielding, whatever the reason it came here", account: "work", session: yielding("pin yields: side has no room", false), want: "goes to side from its next request"},
		{name: "its own pin to another account since it yielded", account: "work", session: pinnedSince, want: "goes to client from its next request"},
		{name: "its other models' accounts", account: "work", session: split, want: "its opus and haiku are on side, its fable on personal"},
		{name: "its own pin keeping it here", account: "work", session: pinnedHere, want: "pinned here"},
		{name: "its own pin moving it here", account: "work", session: pinnedAt(10 * time.Minute), events: moved("pinned"), want: "pinned here at 13:02"},
		{name: "its own pin given as it ran here", account: "work", session: pinnedAt(7 * time.Minute), want: "pinned here at 13:05"},
		{name: "its own pin it was launched with, where a move by pin says otherwise", account: "work", session: pinnedAt(0), events: moved("pinned"), want: "pinned here"},
		{name: "its own pin keeping it where it moved for another reason", account: "work", session: pinnedAt(0), events: moved("moved: side has no room"), want: "pinned here"},
		{name: "moved here", account: "side", session: flippingSessions()[3], want: "moved from personal at 12:22"},
		{name: "here since it came", account: "work", session: flippingSessions()[2], want: "here since 11:12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := doc
			if tt.events != nil {
				doc.Events = tt.events
			}
			s := seatedOn([]status.Session{tt.session}, tt.account)[0]
			if got := note(doc, tt.account, s, now); got != tt.want {
				t.Errorf("noted %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWhyAnAccountHasNoSessions(t *testing.T) {
	away := flipping()
	away.Events = []status.Event{{ID: 1, At: now.Add(-3 * time.Minute).UTC(), Kind: status.EventMoved, Session: idD28C, Model: opus, From: "personal", To: "side", Reason: "pinned"}}
	lost := flipping()
	lost.Events = []status.Event{{ID: 1, At: now.Add(-50 * time.Minute).UTC(), Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Count: 2}}
	since := func(e status.Event) status.Document {
		doc := flipping()
		e.ID, e.At = 8, now.Add(-30*time.Second).UTC()
		doc.Events = slices.Concat([]status.Event{e}, doc.Events)
		return doc
	}
	// held has personal held by the limit whose event moved its sessions, by
	// its identity, though a limit reached since in Fable's week alone has
	// moved one more; and heldSince, a session starting on it since.
	held := flipping()
	held.Accounts[1].Limit.ID = 4
	held.Events[4].Limit = 4
	held.Events = slices.Concat([]status.Event{{ID: 8, At: now.Add(-20 * time.Minute).UTC(), Kind: status.EventLimit, Account: "personal", Windows: []string{"7d_oi"}, Count: 1, To: "side", Limit: 5}}, held.Events)
	heldSince := held
	heldSince.Events = slices.Concat([]status.Event{{ID: 9, At: now.Add(-30 * time.Second).UTC(), Kind: status.EventStarted, Account: "personal", Session: idC61B, Model: opus}}, held.Events)
	tests := []struct {
		name     string
		doc      status.Document
		sessions []status.Session
		want     []string
	}{
		{name: "the limit that moved them", doc: flipping(), sessions: flippingSessions(), want: []string{"3 moved to side at 12:22,", "when personal reached its limit"}},
		{name: "a limit that moved them to several", doc: lost, sessions: flippingSessions(), want: []string{"2 moved to other accounts at 12:22,", "when personal reached its limit"}},
		{name: "the limit holding it, by its identity", doc: held, sessions: flippingSessions(), want: []string{"3 moved to side at 12:22,", "when personal reached its limit"}},
		{name: "the limit holding it, but a session starting on it since: nothing to say", doc: heldSince, sessions: flippingSessions()},
		{name: "the last session moving off it", doc: away, sessions: flippingSessions(), want: []string{"d28c moved to side at 13:09 (pin)"}},
		{name: "a session starting on it since: nothing to say", doc: since(status.Event{Kind: status.EventStarted, Account: "personal", Session: idC61B, Model: opus}), sessions: flippingSessions()},
		{name: "a session moving to it since", doc: since(status.Event{Kind: status.EventMoved, Session: idC61B, Model: opus, From: "side", To: "personal"}), sessions: flippingSessions()},
		{name: "sessions another's limit moved to it since", doc: since(status.Event{Kind: status.EventLimit, Account: "side", Count: 2, To: "personal"}), sessions: flippingSessions()},
		{name: "probing, without the router to list them", doc: probed(flipping().Accounts...), want: []string{"the router isn't running"}},
		{name: "nothing told of to say why", doc: routerDoc("", 0, flipping().Accounts...), sessions: flippingSessions()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Policy: claudeLike, Sessions: tt.sessions}
			var got []string
			for _, c := range f.unseatedBy(tt.doc, "personal", now) {
				got = append(got, c.text.plain())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("says %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSeatsAreTheSessionsOnAnAccountInTheRoutersOrder(t *testing.T) {
	tests := []struct {
		account string
		want    []Seat
	}{
		{account: "work", want: []Seat{{idDB8A, sonnet}, {idD28C, opus}, {id7F3A, haiku}}},
		{account: "side", want: []Seat{{idDB8A, opus}, {idC61B, opus}, {id41E0, sonnet}}},
		{account: "personal"},
	}
	for _, tt := range tests {
		if got := seatsOf(Frame{Sessions: flippingSessions()}, tt.account); !slices.Equal(got, tt.want) {
			t.Errorf("the seats on %s are %+v, want %+v", tt.account, got, tt.want)
		}
	}
}

// seatsOf are the seats under the panel of the account with the given id, as
// f lists them.
func seatsOf(f Frame, id string) []Seat {
	var seats []Seat
	for _, s := range seatedOn(f.listing(), id) {
		seats = append(seats, s.seat())
	}
	return seats
}
