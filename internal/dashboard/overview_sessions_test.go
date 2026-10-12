package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// id9C2E is a sample session's id, as idD28C and the rest are.
const id9C2E = "9c2e47b1-0d5a-4e83-b6f2-8a1c3e9d0f57"

// reasonCapped is a move off work at its reserve, as the router gives it.
const reasonCapped = status.ReasonMovedOff + "work " + status.WhyReserve

// overviewSessions are the sessions SESSIONS' frames list at routedAt, as
// the router lists them, the one seen last first: on personal, 41e0, its
// sonnet answering, and 9c2e, idle 9m; on side, c61b answering, started at
// 14:41, and, each moved from work as it reached its cap at 14:31, d28c,
// its opus asking a moment ago, its haiku going to side too, 7f3a, idle 1m,
// and db8a, idle 4m.
func overviewSessions() []status.Session {
	at := func(hour, minute, second int) time.Time {
		return time.Date(2026, 10, 1, hour, minute, second, 0, time.UTC)
	}
	seat := func(model, family, account, reason string, since, seen time.Time, inFlight string) status.Assignment {
		return status.Assignment{Model: model, Family: family, Account: account, Reason: reason, AssignedAt: since, LastSeen: seen, InFlight: inFlight}
	}
	return []status.Session{
		{ID: idC61B, Assignments: []status.Assignment{seat(opus, "opus", "side", status.ReasonNew, at(14, 41, 11), routedAt, status.Answering)}},
		{ID: id41E0, Assignments: []status.Assignment{seat(sonnet, "sonnet", "personal", status.ReasonNew, at(12, 47, 12), routedAt, status.Answering)}},
		{ID: idD28C, Assignments: []status.Assignment{
			seat(opus, "opus", "side", reasonCapped, at(14, 31, 13), routedAt.Add(-3*time.Second), status.Asking),
			seat(haiku, "haiku", "side", status.ReasonNew, at(14, 31, 44), at(14, 31, 44), ""),
		}},
		{ID: id7F3A, Assignments: []status.Assignment{seat(sonnet, "sonnet", "side", reasonCapped, at(14, 31, 20), routedAt.Add(-time.Minute), "")}},
		{ID: idDB8A, Assignments: []status.Assignment{seat(opus, "opus", "side", reasonCapped, at(14, 31, 40), routedAt.Add(-4*time.Minute), "")}},
		{ID: id9C2E, Assignments: []status.Assignment{seat(opus, "opus", "personal", status.ReasonNew, at(11, 55, 31), at(14, 33, 7), "")}},
	}
}

// overviewWorld is the world the Overview's frames draw SESSIONS of at
// routedAt: routedWorld's accounts, personal the primary, and
// overviewSessions, the request stream telling of 41e0's answer, ~860
// tokens so far, c61b's, ~1.2k, and d28c's opus asking.
func overviewWorld() (status.Document, []status.Session, Traffic) {
	doc, _ := routedWorld(false)
	doc.Accounts[1].Primary = true
	plug := func(account, session, model string) Plug {
		return Plug{Account: account, Seat: Seat{Session: session, Model: model}}
	}
	traffic := Traffic{Live: true, Calls: map[Plug]Call{
		plug("personal", id41E0, sonnet): {Doing: Streaming, Tokens: 860},
		plug("side", idC61B, opus):       {Doing: Streaming, Tokens: 1200},
		plug("side", idD28C, opus):       {Doing: Asking},
	}}
	return doc, overviewSessions(), traffic
}

// sessionsDrawn are the rows the frame draws SESSIONS' body in, of doc at
// routedAt, from row 0, in rows rows, as text alone.
func sessionsDrawn(f Frame, doc status.Document, rows int) []string {
	c := newCanvas(f.Width, rows)
	f.sessionsSection(doc, routedAt).draw(c, bounds{width: f.Width, rows: rows})
	return c.rows(Look{})
}

func TestSESSIONSIsDrawnAsItsFramesAre(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	doc, sessions, traffic := overviewWorld()
	for _, tt := range []struct {
		stem string
		top  int
	}{
		{stem: "overview-sessions-216", top: 54},
		{stem: "overview-sessions-graphs-216", top: 56},
		{stem: "overview-everything-216x106", top: 78},
	} {
		t.Run(tt.stem, func(t *testing.T) {
			f := Frame{Width: 216, Height: 106, Look: nord, View: Overview, Policy: claudeLike, Sessions: sessions, Traffic: traffic}
			c := newCanvas(f.Width, f.Height)
			f.sessionsSection(doc, routedAt).draw(c, bounds{y: tt.top, width: f.Width, rows: 6})
			// The traces and rates are the traces' own, held by their tests.
			in := region{top: tt.top, bottom: tt.top + 5, left: 0, right: traceFrom - 1}
			heldTo(t, tt.stem, reference(t, tt.stem), screenOf(t, c.rows(nord)), in, shadeOf(nord.Canvas()), nil, nil)
		})
	}
}

func TestSESSIONSFoldsToHowManyRunAndWhere(t *testing.T) {
	doc, sessions, traffic := overviewWorld()
	two := slices.Clone(sessions)
	two[0].Assignments = append(slices.Clone(two[0].Assignments), status.Assignment{Model: haiku, Family: "haiku", Account: "personal", Reason: status.ReasonNew, AssignedAt: routedAt, LastSeen: routedAt})
	tests := []struct {
		name     string
		sessions []status.Session
		want     line
	}{
		{
			name: "the frame's: each account with any, in the config's order", sessions: sessions,
			want: line{{"6 running: 2 on personal, 4 on side", mutedInk}, spaces(3), {"no room at 39 rows", dimInk}},
		},
		{
			name: "a session on two accounts, counted once, and on each", sessions: two,
			want: line{{"6 running: 3 on personal, 4 on side", mutedInk}, spaces(3), {"no room at 39 rows", dimInk}},
		},
		{
			name: "none", sessions: nil,
			want: line{{"no sessions running", mutedInk}, spaces(3), {"no room at 39 rows", dimInk}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: 160, Height: 39, View: Overview, Sessions: tt.sessions, Traffic: traffic}
			if got := f.sessionsSection(doc, routedAt).folded(); !slices.Equal(got, tt.want) {
				t.Errorf("folded = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSESSIONSWithNoneRunningSaysSo(t *testing.T) {
	doc, _, _ := overviewWorld()
	f := Frame{Width: 160, Height: 39, View: Overview}
	if got := f.sessionsSection(doc, routedAt); got.least != 3 || got.most != 3 {
		t.Errorf("least %d, most %d; want a row and the heading's two", got.least, got.most)
	}
	c := newCanvas(f.Width, 1)
	f.sessionsSection(doc, routedAt).draw(c, bounds{width: f.Width, rows: 1})
	if got, want := c.cells[0][0].ink, faintInk; got != want {
		t.Errorf("drawn in %+v, want text.faint", got)
	}
	if got := sessionsDrawn(f, doc, 1); got[0] != "no sessions running" {
		t.Errorf("drew %q, want no sessions running", got[0])
	}
}

func TestSESSIONSShowsAsManyAsFitTheLastRowSayingHowManyMore(t *testing.T) {
	doc, sessions, traffic := overviewWorld()
	tests := []struct {
		name  string
		width int
		rows  int
		want  []string
	}{
		{name: "a row a session", width: 160, rows: 3, want: []string{"2 personal ◆    41e0", "                9c2e", "                +4 more"}},
		{name: "two rows a session, under 110 columns", width: 109, rows: 6, want: []string{" 2 personal ◆   41e0", "                started", "                9c2e", "                started", "                +4 more", ""}},
		{name: "every one where they fit", width: 160, rows: 6, want: []string{"2 personal ◆    41e0", "                9c2e", "3 side ▲        c61b", "                d28c", "                7f3a", "                db8a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 40, View: Overview, Sessions: sessions, Traffic: traffic}
			got := sessionsDrawn(f, doc, tt.rows)
			for i, row := range got {
				if !strings.HasPrefix(row, tt.want[i]) || tt.want[i] == "" && row != "" {
					t.Errorf("row %d = %q, want it to start %q", i+1, row, tt.want[i])
				}
			}
		})
	}
	c := newCanvas(160, 3)
	f := Frame{Width: 160, Height: 40, View: Overview, Sessions: sessions, Traffic: traffic}
	f.sessionsSection(doc, routedAt).draw(c, bounds{width: 160, rows: 3})
	if got := c.cells[2][16].ink; got != dimInk {
		t.Errorf("+4 more drawn in %+v, want text.subtle", got)
	}
}

func TestUnder110ColumnsASessionTakesTwoRows(t *testing.T) {
	doc, sessions, traffic := overviewWorld()
	for _, width := range []int{109, 52} {
		f := Frame{Width: width, Height: 40, View: Overview, Sessions: sessions[:2], Traffic: traffic}
		if got := f.sessionsSection(doc, routedAt); got.least != 6 || got.most != 6 {
			t.Errorf("at %d columns: least %d, most %d; want two rows a session and the heading's two", width, got.least, got.most)
		}
		pad := func(left, right string) string {
			return left + strings.Repeat(" ", width-len([]rune(left))-len([]rune(right))) + right
		}
		want := []string{
			pad(" 2 personal ◆   41e0", "↓ ~860"),
			"                started here 12:47",
			pad(" 3 side ▲       c61b", "↓ ~1.2k"),
			"                started here 14:41, the best",
		}
		if got := sessionsDrawn(f, doc, 4); !slices.Equal(got, want) {
			t.Errorf("at %d columns, drew\n  %q\nwant\n  %q", width, got, want)
		}
	}
}

func TestASessionsWhyIsCutToEndBeforeItsTrace(t *testing.T) {
	doc, _, _ := overviewWorld()
	long := status.Session{ID: idD28C, Assignments: []status.Assignment{{
		Model: opus, Family: "opus", Account: "side", Reason: "pin yields: personal has no room for a request that's long", AssignedAt: routedAt, LastSeen: routedAt,
	}}}
	f := Frame{Width: 160, Height: 40, View: Overview, Sessions: []status.Session{long}}
	want := "3 side ▲        d28c  opus    now   idle     its pin to personal yields: personal has no…"
	if got := sessionsDrawn(f, doc, 1)[0]; got != want {
		t.Errorf("drew\n  %q\nwant\n  %q, its why cut short two cells or more before the trace at column %d", got, want, traceFrom)
	}
}

func TestASessionRunningSaysWhatItsDoing(t *testing.T) {
	plug := Plug{Account: "side", Seat: Seat{Session: idD28C, Model: opus}}
	other := Plug{Account: "side", Seat: Seat{Session: idD28C, Model: haiku}}
	tests := []struct {
		name     string
		calls    map[Plug]Call
		inFlight string
		want     span
	}{
		{name: "answering, as the stream counts it", calls: map[Plug]Call{plug: {Doing: Streaming, Tokens: 1234}}, want: span{"↓ ~1.2k", doingInk}},
		{name: "answering, exact", calls: map[Plug]Call{plug: {Doing: Streaming, Tokens: 1321, Exact: true}}, want: span{"↓ 1.3k", doingInk}},
		{name: "answering, nothing counted yet", calls: map[Plug]Call{plug: {Doing: Streaming}}, want: span{"↓", doingInk}},
		{name: "answering on one model, asking on another: answering", calls: map[Plug]Call{other: {Doing: Asking}, plug: {Doing: Streaming, Tokens: 860}}, want: span{"↓ ~860", doingInk}},
		{name: "asking, as the stream tells", calls: map[Plug]Call{plug: {Doing: Asking}}, want: span{"↑ ask", doingInk}},
		{name: "throttled, to be sent again: asking still", calls: map[Plug]Call{plug: {Doing: Throttled}}, want: span{"↑ ask", doingInk}},
		{name: "its answer ended, as the stream tells, though listed answering", calls: map[Plug]Call{plug: {Doing: Answered, Tokens: 900}}, inFlight: status.Answering, want: span{"idle", faintInk}},
		{name: "refused, as the stream tells", calls: map[Plug]Call{plug: {Doing: Refused}}, want: span{"idle", faintInk}},
		{name: "answering, as the router listed it, the stream telling nothing", inFlight: status.Answering, want: span{"↓", doingInk}},
		{name: "asking, as the router listed it", inFlight: status.Asking, want: span{"↑ ask", doingInk}},
		{name: "nothing in flight", want: span{"idle", faintInk}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Traffic: Traffic{Live: true, Calls: tt.calls}}
			r := views.Running{Account: "side", Session: idD28C, Models: []string{opus, haiku}, InFlight: tt.inFlight}
			if got := f.runningDoing(r); got != tt.want {
				t.Errorf("runningDoing = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestASessionsIDIsBrightWhileItsBusy(t *testing.T) {
	tests := []struct {
		name     string
		seen     time.Duration
		inFlight string
		call     *Call
		busy     bool
	}{
		{name: "seen within the minute", seen: 59 * time.Second, busy: true},
		{name: "seen a minute ago", seen: time.Minute},
		{name: "a request in flight, as the router listed it", seen: 9 * time.Minute, inFlight: status.Asking, busy: true},
		{name: "a request in flight, as the stream tells", seen: 9 * time.Minute, call: &Call{Doing: Streaming}, busy: true},
		{name: "its answer ended, as the stream tells", seen: 9 * time.Minute, call: &Call{Doing: Answered}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f Frame
			if tt.call != nil {
				f.Traffic.Calls = map[Plug]Call{{Account: "side", Seat: Seat{Session: idD28C, Model: opus}}: *tt.call}
			}
			r := views.Running{Account: "side", Session: idD28C, Models: []string{opus}, LastSeen: routedAt.Add(-tt.seen), InFlight: tt.inFlight}
			want := span{"d28c  ", mutedInk}
			if tt.busy {
				want.ink = titleInk
			}
			if got := f.runningID(r, routedAt); got != want {
				t.Errorf("runningID = %+v, want %+v", got, want)
			}
		})
	}
}

func TestASessionRunningWasLastSeenNowWithinTheMinute(t *testing.T) {
	tests := []struct {
		ago  time.Duration
		want string
	}{
		{ago: 0, want: "now"},
		{ago: 59 * time.Second, want: "now"},
		{ago: time.Minute, want: "1m"},
		{ago: 9*time.Minute + 10*time.Second, want: "9m"},
		{ago: 2 * time.Hour, want: "2h"},
	}
	for _, tt := range tests {
		if got := runningSeen(routedAt.Add(-tt.ago), routedAt); got != tt.want {
			t.Errorf("seen %s ago: %q, want %q", tt.ago, got, tt.want)
		}
	}
}

func TestTheStreamSeesASessionRunningLaterThanTheRouterListedIt(t *testing.T) {
	doc, sessions, _ := overviewWorld()
	f := Frame{Width: 160, Height: 40, View: Overview, Sessions: sessions}
	f.Traffic = Traffic{Live: true, Seen: map[Plug]time.Time{{Account: "side", Seat: Seat{Session: idDB8A, Model: opus}}: routedAt}}
	var order []string
	for _, r := range f.running(doc, routedAt) {
		order = append(order, sessionID(r.Session))
	}
	if want := []string{"41e0", "9c2e", "c61b", "db8a", "d28c", "7f3a"}; !slices.Equal(order, want) {
		t.Errorf("listed %q, want %q: db8a seen now, as the stream told, after c61b as the router lists them", order, want)
	}
	if !sessions[4].Assignments[0].LastSeen.Equal(routedAt.Add(-4 * time.Minute)) {
		t.Error("the router's listing was changed")
	}
}
