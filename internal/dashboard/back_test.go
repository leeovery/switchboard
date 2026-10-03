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

// flipping is the router's document of work, personal and side, as the
// frames' flipped cards have them: work under pressure, personal at the
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

// flipped is a frame of flipping, every card flipped, work's focused, and
// the keys that work on a card's sessions those of a router that answers.
func flipped(t *testing.T) Frame {
	return Frame{
		Look: Screen(builtin(t, "nord")), Policy: claudeLike, Sessions: flippingSessions(),
		Focus: "work", Flipped: map[string]bool{"work": true, "personal": true, "side": true},
		Patch: []Key{{Key: "↑↓", Does: "select"}, {Key: "1-3", Does: "move it"}},
	}
}

func TestTheBackOfACard(t *testing.T) {
	tests := []struct {
		name string
		id   string
		// selected is the session picked out on work's back.
		selected Seat
		want     []string
	}{
		{
			name: "work's sessions, the one seen last first, each noted, and LATELY", id: "work",
			want: []string{
				"┏━ 1 work ━━━━━━━━━━━━━━━━━━━━━━━━━━━━ sessions ━┓",
				"┃  3 sessions  ·  2 busy                         ┃",
				"┃                                                ┃",
				"┃    ● db8a  sonnet  seen       now              ┃",
				"┃      ╰ its opus is on side                     ┃",
				"┃    ● d28c  opus    seen       now              ┃",
				"┃      ╰ here since 11:12                        ┃",
				"┃    ○ 7f3a  haiku   idle       9m               ┃",
				"┃      ╰ here since 10:12                        ┃",
				"┃                                                ┃",
				"┃  LATELY                                        ┃",
				"┃  12:42  ● came under pressure                  ┃",
				"┃  11:12  ▲ d28c started here, the best          ┃",
				"┃                                                ┃",
				"┃                                                ┃",
				"┃  ↑↓ select  1-3 move it  space flip            ┃",
				"┗━ ● ● ○ ━━━━━━━━━━━━━━━━━━━━━━━━━━━ 3 sessions ━┛",
			},
		},
		{
			name: "a session picked out", id: "work", selected: Seat{Session: idD28C, Model: opus},
			want: []string{
				"┏━ 1 work ━━━━━━━━━━━━━━━━━━━━━━━━━━━━ sessions ━┓",
				"┃  3 sessions  ·  2 busy                         ┃",
				"┃                                                ┃",
				"┃    ● db8a  sonnet  seen       now              ┃",
				"┃      ╰ its opus is on side                     ┃",
				"┃  ▸ ● d28c  opus    seen       now              ┃",
				"┃      ╰ here since 11:12                        ┃",
				"┃    ○ 7f3a  haiku   idle       9m               ┃",
				"┃      ╰ here since 10:12                        ┃",
				"┃                                                ┃",
				"┃  LATELY                                        ┃",
				"┃  12:42  ● came under pressure                  ┃",
				"┃  11:12  ▲ d28c started here, the best          ┃",
				"┃                                                ┃",
				"┃                                                ┃",
				"┃  ↑↓ select  1-3 move it  space flip            ┃",
				"┗━ ● ● ○ ━━━━━━━━━━━━━━━━━━━━━━━━━━━ 3 sessions ━┛",
			},
		},
		{
			name: "none, and why: the limit that moved them", id: "personal",
			want: []string{
				"╭─ 2 personal ──────────────────────── sessions ─╮",
				"│  no sessions                                   │",
				"│                                                │",
				"│  3 moved to side at 12:22,                     │",
				"│  when personal reached its limit               │",
				"│                                                │",
				"│  LATELY                                        │",
				"│  12:22  ■ reached its session limit            │",
				"│  12:22  ▸ 3 sessions moved to side             │",
				"│                                                │",
				"│                                                │",
				"│                                                │",
				"│                                                │",
				"│                                                │",
				"│                                                │",
				"│  space flip                                    │",
				"╰────────────────────────────────── no sessions ─╯",
			},
		},
		{
			name: "side's, each noted as it came, and the sessions personal's limit moved here", id: "side",
			want: []string{
				"╭─ 3 side ─────────────────── sessions · ▲ next ─╮",
				"│  3 sessions  ·  2 busy                         │",
				"│                                                │",
				"│    ● db8a  opus    seen       now              │",
				"│      ╰ its sonnet is on work                   │",
				"│    ● c61b  opus    seen       now              │",
				"│      ╰ here since 13:11                        │",
				"│    ○ 41e0  sonnet  idle       4m               │",
				"│      ╰ moved from personal at 12:22            │",
				"│                                                │",
				"│  LATELY                                        │",
				"│  13:11  ▲ c61b started here, the best          │",
				"│  12:22  ▸ 3 sessions arrived from personal     │",
				"│  12:12  ◇ primed: its 5-hour window started    │",
				"│                                                │",
				"│  ↑↓ select  1-3 move it  space flip            │",
				"╰─ ● ● ○ ─────────────────────────── 3 sessions ─╯",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := flipped(t)
			f.Selected = tt.selected
			got := cardOf(t, f, flipping(), tt.id, 50, densities[0])
			if !slices.Equal(got, tt.want) {
				t.Errorf("drew\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestABackWithMoreSessionsThanFit(t *testing.T) {
	crowded := slices.Concat(flippingSessions(), []status.Session{
		{ID: "5c7a93f1-2e6d-4b08-a4c9-7f1e0d3b8a26", Assignments: []status.Assignment{seatOf(sonnet, "work", "new", time.Hour, 20*time.Minute)}},
		{ID: "e4b06a2d-c71f-4e98-b3d5-0a8f2e6c9b14", Assignments: []status.Assignment{seatOf(haiku, "work", "new", time.Hour, 30*time.Minute)}},
	})
	tests := []struct {
		name     string
		sessions []status.Session
		density  density
		selected Seat
		want     []string
	}{
		{
			name: "every row, without the notes", sessions: flippingSessions(), density: density{midCard, 3},
			want: []string{
				"┏━ 1 work ━━━━━━━━━━━━━━━━━━━━━━━━━━━━ sessions ━┓",
				"┃  3 sessions  ·  2 busy                         ┃",
				"┃                                                ┃",
				"┃    ● db8a  sonnet  seen       now              ┃",
				"┃    ● d28c  opus    seen       now              ┃",
				"┃    ○ 7f3a  haiku   idle       9m               ┃",
				"┃                                                ┃",
				"┃                                                ┃",
				"┃  ↑↓ select  1-3 move it  space flip            ┃",
				"┗━ ● ● ○ ━━━━━━━━━━━━━━━━━━━━━━━━━━━ 3 sessions ━┛",
			},
		},
		{
			name: "every row, the blank under the count given up for them", sessions: flippingSessions(), density: density{compactCard, 2},
			want: []string{
				"┏━ 1 work ━━━━━━━━━━━━━━━━━━━━━━━━━━━━ sessions ━┓",
				"┃  3 sessions  ·  2 busy                         ┃",
				"┃    ● db8a  sonnet  seen       now              ┃",
				"┃    ● d28c  opus    seen       now              ┃",
				"┃    ○ 7f3a  haiku   idle       9m               ┃",
				"┃  ↑↓ select  1-3 move it  space flip            ┃",
				"┗━ ● ● ○ ━━━━━━━━━━━━━━━━━━━━━━━━━━━ 3 sessions ━┛",
			},
		},
		{
			name: "as many rows as fit, and how many more", sessions: crowded, density: density{compactCard, 2},
			want: []string{
				"┏━ 1 work ━━━━━━━━━━━━━━━━━━━━━━━━━━━━ sessions ━┓",
				"┃  5 sessions  ·  2 busy                         ┃",
				"┃    ● db8a  sonnet  seen       now              ┃",
				"┃    ● d28c  opus    seen       now              ┃",
				"┃  +3 more                                       ┃",
				"┃  ↑↓ select  1-3 move it  space flip            ┃",
				"┗━ ● ● ○ ○ ○ ━━━━━━━━━━━━━━━━━━━━━━━ 5 sessions ━┛",
			},
		},
		{
			name: "as many rows as fit, the one picked out last among them", sessions: crowded, density: density{compactCard, 2},
			selected: Seat{Session: "e4b06a2d-c71f-4e98-b3d5-0a8f2e6c9b14", Model: haiku},
			want: []string{
				"┏━ 1 work ━━━━━━━━━━━━━━━━━━━━━━━━━━━━ sessions ━┓",
				"┃  5 sessions  ·  2 busy                         ┃",
				"┃    ○ 5c7a  sonnet  idle       20m              ┃",
				"┃  ▸ ○ e4b0  haiku   idle       30m              ┃",
				"┃  +3 more                                       ┃",
				"┃  ↑↓ select  1-3 move it  space flip            ┃",
				"┗━ ● ● ○ ○ ○ ━━━━━━━━━━━━━━━━━━━━━━━ 5 sessions ━┛",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := flipped(t)
			f.Sessions, f.Selected = tt.sessions, tt.selected
			got := cardOf(t, f, flipping(), "work", 50, tt.density)
			if !slices.Equal(got, tt.want) {
				t.Errorf("drew\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestTheBacksShowTheStreamsMoves(t *testing.T) {
	d28c := Seat{Session: idD28C, Model: opus}
	f := flipped(t)
	f.Traffic = Traffic{
		Live:  true,
		Calls: map[Plug]Call{{Account: "side", Seat: d28c}: {Doing: Streaming, Since: now.Add(-time.Second), Tokens: 1234}},
		Moves: []Move{{Seat: d28c, From: "work", To: "side", At: now.Add(-time.Second), Reason: "moved: work hit its limit"}},
	}

	work := strings.Join(cardOf(t, f, flipping(), "work", 50, densities[0]), "\n")
	if strings.Contains(work, "● d28c") || !strings.Contains(work, "2 sessions  ·  1 busy") {
		t.Errorf("work's back is\n%s\nwant d28c gone from it, as the stream moved it", work)
	}
	side := strings.Join(cardOf(t, f, flipping(), "side", 50, densities[0]), "\n")
	if !strings.Contains(side, "● d28c  opus    streaming  ↓ ~1.2k") || !strings.Contains(side, "╰ here since 13:11") || !strings.Contains(side, "4 sessions  ·  3 busy") {
		t.Errorf("side's back is\n%s\nwant d28c on it, streaming, here since it moved", side)
	}
	if got := f.Seats("side"); !slices.Contains(got, d28c) || slices.Contains(f.Seats("work"), d28c) {
		t.Errorf("side's seats are %+v, and work's %+v, want d28c among side's alone", got, f.Seats("work"))
	}
}

func TestTheSessionPickedOutIsOnTheSelectionsSurface(t *testing.T) {
	f := flipped(t)
	f.Selected = Seat{Session: idD28C, Model: opus}
	faces, shown := f.faces(flipping(), now)
	c := newCanvas(50, densities[0].rows(1))
	f.card(c, flipping(), faces[0], now, 0, 0, 50, densities[0], barRowsFor(len(shown), 44), labelColumn)

	const row = 5
	if got := c.at(3, row); got.glyph != "▸" || got.ink.token != theme.AccentKey || !got.ink.bold {
		t.Errorf("the row picked out leads with %q in %+v, want ▸ in accent.key, bold", got.glyph, got.ink)
	}
	for x := range 50 {
		want := hue{}
		if x >= 2 && x <= 47 {
			want = hue{token: theme.BgSelection}
		}
		if got := c.at(x, row).ink.on; got != want {
			t.Errorf("cell %d of the row picked out is on %+v, want %+v: bg.selection a cell in from each side", x, got, want)
		}
	}
	if got := c.at(3, row+2).ink.on; got != (hue{}) {
		t.Errorf("the row under the note is on %+v, want the canvas", got)
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
			if got := f.seatLine(s, modelColumn, false, now); !slices.Equal(got, tt.want) {
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
	yielded := status.Session{ID: idD28C, Pin: "side", Assignments: []status.Assignment{seatOf(opus, "work", "pin yields: side has no room", time.Hour, time.Minute)}}
	pinnedSince := status.Session{ID: idD28C, Pin: "client", Assignments: []status.Assignment{seatOf(opus, "work", "pin yields: side has no room", time.Hour, time.Minute)}}
	movedByPin := status.Session{ID: idC61B, Assignments: []status.Assignment{seatOf(opus, "work", "pinned", 10*time.Minute, time.Minute)}}
	movedByPin.Assignments[0].Pinned = true
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
		{name: "its own pin having yielded here at a limit", account: "work", session: yielded, want: "its pin to side yielded here"},
		{name: "its own pin to another account since it yielded", account: "work", session: pinnedSince, want: "goes to client from its next request"},
		{name: "its other models' accounts", account: "work", session: split, want: "its opus and haiku are on side, its fable on personal"},
		{name: "its own pin keeping it here", account: "work", session: pinnedHere, want: "pinned here"},
		{name: "its own pin moving it here", account: "work", session: movedByPin, events: moved("pinned"), want: "pinned here at 13:02"},
		{name: "its own pin keeping it where it moved for another reason", account: "work", session: movedByPin, events: moved("moved: side has no room"), want: "pinned here"},
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

func TestABackWithNoSessionsSaysWhy(t *testing.T) {
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
	tests := []struct {
		name     string
		doc      status.Document
		sessions []status.Session
		want     []string
	}{
		{name: "the limit that moved them", doc: flipping(), sessions: flippingSessions(), want: []string{"3 moved to side at 12:22,", "when personal reached its limit"}},
		{name: "a limit that moved them to several", doc: lost, sessions: flippingSessions(), want: []string{"2 moved to other accounts at 12:22,", "when personal reached its limit"}},
		{name: "the last session moving off it", doc: away, sessions: flippingSessions(), want: []string{"d28c moved to side at 13:09 (pin)"}},
		{name: "a session starting on it since: nothing to say", doc: since(status.Event{Kind: status.EventStarted, Account: "personal", Session: idC61B, Model: opus}), sessions: flippingSessions()},
		{name: "a session moving to it since", doc: since(status.Event{Kind: status.EventMoved, Session: idC61B, Model: opus, From: "side", To: "personal"}), sessions: flippingSessions()},
		{name: "sessions another's limit moved to it since", doc: since(status.Event{Kind: status.EventLimit, Account: "side", Count: 2, To: "personal"}), sessions: flippingSessions()},
		{name: "probing, without the router to list them", doc: probed(flipping().Accounts...), want: []string{"the router isn't running"}},
		{name: "nothing told of to say why", doc: routerDoc("", 0, flipping().Accounts...), sessions: flippingSessions()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := flipped(t)
			f.Sessions = tt.sessions
			rows := cardOf(t, f, tt.doc, "personal", 50, densities[0])
			if got := strings.TrimSpace(strings.Trim(rows[1], "│")); got != "no sessions" {
				t.Errorf("its count reads %q, want no sessions", got)
			}
			var got []string
			for _, row := range rows[3 : 3+len(tt.want)] {
				got = append(got, strings.TrimSpace(strings.Trim(row, "│")))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("says %q, want %q", got, tt.want)
			}
			if after := strings.TrimSpace(strings.Trim(rows[3], "│")); len(tt.want) == 0 && after != "" && after != latelyLabel {
				t.Errorf("says %q, want nothing, LATELY at most following the count", rows[3])
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
		if got := (Frame{Sessions: flippingSessions()}).Seats(tt.account); !slices.Equal(got, tt.want) {
			t.Errorf("Seats(%s) = %+v, want %+v", tt.account, got, tt.want)
		}
	}
	if got := (Seat{Session: idD28C, Model: opus}).Shown(); got != "d28c" {
		t.Errorf("Shown() = %q, want d28c", got)
	}
}
