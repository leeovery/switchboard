package dashboard

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
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
func overviewWorld() (status.Document, []status.Session, views.Stream) {
	doc, _ := routedWorld(false)
	doc.Accounts[1].Primary = true
	told := func(request, account, session, model string, heard time.Time, chars int) views.Told {
		t := views.Told{Request: request, Session: session, Model: model, Account: account, Sent: heard.Add(-5 * time.Second), Heard: heard}
		if chars > 0 {
			t.First, t.Chars = t.Sent.Add(time.Second), chars
		}
		return t
	}
	stream := views.Stream{Live: true, Requests: map[string]views.Told{
		"r1": told("r1", "personal", id41E0, sonnet, routedAt, 860*views.CharsPerToken),
		"r2": told("r2", "side", idC61B, opus, routedAt, 1200*views.CharsPerToken),
		"r3": told("r3", "side", idD28C, opus, routedAt.Add(-3*time.Second), 0),
	}}
	return doc, overviewSessions(), stream
}

// sessionsDrawn are the rows the frame draws SESSIONS' body in, of doc at
// routedAt, from row 0, in rows rows, as text alone.
func sessionsDrawn(f Frame, doc status.Document, rows int) []string {
	c := newCanvas(f.Width, rows)
	f.sessionsSection(doc, routedAt).draw(c, bounds{width: f.Width, rows: rows})
	return c.rows(Look{})
}

func TestSESSIONSFoldsToHowManyRunAndWhere(t *testing.T) {
	doc, sessions, stream := overviewWorld()
	two := slices.Clone(sessions)
	two[0].Assignments = append(slices.Clone(two[0].Assignments), status.Assignment{Model: haiku, Family: "haiku", Account: "personal", Reason: status.ReasonNew, AssignedAt: routedAt, LastSeen: routedAt})
	tests := []struct {
		name     string
		sessions []status.Session
		want     line
	}{
		{
			name: "the frame's: each account with any, in the config's order", sessions: sessions,
			want: line{{"6 running: 2 on personal, 4 on side", mutedInk}},
		},
		{
			name: "a session on two accounts, counted once, and on each", sessions: two,
			want: line{{"6 running: 3 on personal, 4 on side", mutedInk}},
		},
		{
			name: "none", sessions: nil,
			want: line{{"no sessions running", mutedInk}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: 160, Height: 39, View: Overview, Sessions: tt.sessions, Stream: stream}
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
	doc, sessions, stream := overviewWorld()
	tests := []struct {
		name  string
		width int
		rows  int
		want  []string
	}{
		{name: "a row a session", width: 160, rows: 3, want: []string{"3 side ▲        c61b", "                d28c", "                +4 more"}},
		{name: "two rows a session, where its row's parts don't fit a row", width: 82, rows: 6, want: []string{" 3 side ▲       c61b", "                started", "                d28c", "                moved", "                +4 more", ""}},
		{name: "every one where they fit", width: 160, rows: 6, want: []string{"3 side ▲        c61b", "                d28c", "                7f3a", "                db8a", "2 personal ◆    41e0", "                9c2e"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 40, View: Overview, Sessions: sessions, Stream: stream}
			got := sessionsDrawn(f, doc, tt.rows)
			for i, row := range got {
				if !strings.HasPrefix(row, tt.want[i]) || tt.want[i] == "" && row != "" {
					t.Errorf("row %d = %q, want it to start %q", i+1, row, tt.want[i])
				}
			}
		})
	}
	c := newCanvas(160, 3)
	f := Frame{Width: 160, Height: 40, View: Overview, Sessions: sessions, Stream: stream}
	f.sessionsSection(doc, routedAt).draw(c, bounds{width: 160, rows: 3})
	if got := c.cells[2][16].ink; got != dimInk {
		t.Errorf("+4 more drawn in %+v, want text.subtle", got)
	}
}

// A click on a session's row in SESSIONS opens its page: each row of each
// session shown, across the body, both of a session's on two rows a session,
// and a session's on each account its models go to; none on the row saying
// how many more there are.
func TestEachRowOfASessionInSESSIONSOpensItsPage(t *testing.T) {
	doc, sessions, stream := overviewWorld()
	two := slices.Clone(sessions)
	two[0].Assignments = append(slices.Clone(two[0].Assignments), status.Assignment{Model: haiku, Family: "haiku", Account: "personal", Reason: status.ReasonNew, AssignedAt: routedAt, LastSeen: routedAt})
	tests := []struct {
		name     string
		width    int
		sessions []status.Session
		rows     int
		want     []string
	}{
		{name: "a row a session, the last saying how many more", width: 160, sessions: sessions, rows: 3, want: []string{idC61B, idD28C, ""}},
		{name: "two rows a session, where its row's parts don't fit a row", width: 82, sessions: sessions, rows: 5, want: []string{idC61B, idC61B, idD28C, idD28C, ""}},
		{
			name: "a session on two accounts, on each", width: 160, sessions: two, rows: 7,
			want: []string{idC61B, idD28C, id7F3A, idDB8A, idC61B, id41E0, id9C2E},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 40, View: Overview, Sessions: tt.sessions, Stream: stream}
			body := bounds{x: 0, y: 30, width: tt.width, rows: tt.rows}
			got := make([]string, tt.rows)
			for _, target := range f.sessionsSection(doc, routedAt).targets(body) {
				if target.On != OnSession || target.X != body.x || target.Width != body.width {
					t.Errorf("SESSIONS gives %+v, want a session's row across the body", target)
				}
				got[target.Y-body.y] = target.Session
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("its rows open %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWhereItsRowsPartsDontFitASessionTakesTwoRows(t *testing.T) {
	doc, sessions, stream := overviewWorld()
	for _, width := range []int{82, 52} {
		f := Frame{Width: width, Height: 40, View: Overview, Sessions: sessions[:2], Stream: stream}
		if got := f.sessionsSection(doc, routedAt); got.least != 5 || got.most != 6 {
			t.Errorf("at %d columns: least %d, most %d; want a session's two rows and how many more, to two rows a session, and the heading's two", width, got.least, got.most)
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
	before := func(width int) (string, int) {
		f := Frame{Width: width, Height: 40, View: Overview, Sessions: []status.Session{long}}
		trace := f.routingColumns(f.Moment(doc, routedAt)).trace
		return strings.TrimRight(string([]rune(sessionsDrawn(f, doc, 1)[0])[:trace]), " "), trace
	}
	for width, want := range map[int]string{
		173: "3 side ▲        d28c  opus    now   idle     its pin to personal yields: personal has no…",
		108: "3 side ▲        d28c  opus    now   idle     its pin to person…",
	} {
		if got, trace := before(width); got != want {
			t.Errorf("at %d columns, drew\n  %q\nwant\n  %q, its why cut short two cells or more before the trace at column %d", width, got, want, trace)
		}
	}
	for width := 83; width <= 220; width++ {
		if got, trace := before(width); !strings.HasSuffix(got, "…") || ansi.StringWidth(got) > trace-beforeTraceAt {
			t.Errorf("at %d columns, drew %q, want its why cut to end %d cells or more before the trace at column %d", width, got, beforeTraceAt, trace)
		}
	}
}

func TestASessionRunningSaysWhatItsDoing(t *testing.T) {
	tests := []struct {
		name string
		a    views.Activity
		want span
	}{
		{name: "answering, as the stream counts it", a: views.Activity{Doing: views.Answering, Chars: 4936}, want: span{"↓ ~1.2k", doingInk}},
		{name: "answering, nothing counted yet", a: views.Activity{Doing: views.Answering}, want: span{"↓", doingInk}},
		{name: "asking", a: views.Activity{Doing: views.Asking}, want: span{"↑ ask", doingInk}},
		{name: "throttled, to be sent again: asking still", a: views.Activity{Doing: views.Throttled, Status: 529}, want: span{"↑ ask", doingInk}},
		{name: "refused at a limit, before it goes out again elsewhere: idle, as it has no word of its own", a: views.Activity{Doing: views.Refused, Status: 429}, want: span{"idle", faintInk}},
		{name: "refused by a 403: idle", a: views.Activity{Doing: views.Refused, Status: 403}, want: span{"idle", faintInk}},
		{name: "idle", a: views.Activity{Since: routedAt}, want: span{"idle", faintInk}},
		{name: "answering as last known, the router not answering: as it was, dimmed", a: views.Activity{Doing: views.Answering, Chars: 4936, Stale: true}, want: span{"↓ ~1.2k", dimInk}},
		{name: "asking as last known: as it was, dimmed", a: views.Activity{Doing: views.Asking, Stale: true}, want: span{"↑ ask", dimInk}},
		{name: "throttled as last known: as it was, dimmed", a: views.Activity{Doing: views.Throttled, Status: 529, Stale: true}, want: span{"↑ ask", dimInk}},
		{name: "refused as last known: SESSIONS' word for it, as it was, dimmed", a: views.Activity{Doing: views.Refused, Status: 429, Stale: true}, want: span{"idle", dimInk}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runningDoing(tt.a); got != tt.want {
				t.Errorf("runningDoing = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestASessionsIDIsBrightWhileItsBusy(t *testing.T) {
	tests := []struct {
		name  string
		doing views.Doing
		seen  time.Duration
		busy  bool
	}{
		{name: "seen within the minute", seen: 59 * time.Second, busy: true},
		{name: "seen a minute ago", seen: time.Minute},
		{name: "a request in flight", doing: views.Asking, seen: 9 * time.Minute, busy: true},
		{name: "a request refused, before it goes out again", doing: views.Refused, seen: 9 * time.Minute, busy: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := views.Running{Account: "side", Session: idD28C, Activity: views.Activity{Doing: tt.doing, LastSeen: routedAt.Add(-tt.seen)}}
			want := span{"d28c  ", mutedInk}
			if tt.busy {
				want.ink = titleInk
			}
			if got := runningIDOf(r, routedAt); got != want {
				t.Errorf("runningIDOf = %+v, want %+v", got, want)
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

func TestASessionSeenLongerAgoThanItsColumnHoldsIsDrawnWhole(t *testing.T) {
	doc, _, _ := overviewWorld()
	c := newCanvas(160, 1)
	r := views.Running{Account: "side", Session: idD28C, Family: "opus", Activity: views.Activity{LastSeen: routedAt.Add(-1000 * time.Hour)}}
	f := Frame{Width: 160}
	f.wideRunning(c, f.Moment(doc, routedAt), r, f.runningForm(routingColumnsOf(f.Width, 16)).laid([]views.Running{r}), 0)
	if got := rowOf(c, 0); !strings.Contains(got, "d28c  opus   1000h   idle") {
		t.Errorf("drew %q, want 1000h whole, no blank before it", got)
	}
}

// A frame holding the sessions running lists them once, for SESSIONS to list
// and ROUTING to count, the stream heard once a frame; a look at another
// moment lists them afresh.
func TestTheSessionsRunningAreListedOnceAFrame(t *testing.T) {
	doc, sessions, _ := overviewWorld()
	f := Frame{Width: 160, Height: 40, View: Overview, Sessions: sessions}.holdRunning(doc, routedAt)
	listed := f.running(doc, routedAt)
	if len(listed) == 0 {
		t.Fatal("no session runs, want those the router lists")
	}
	if again := f.running(doc, routedAt); &again[0] != &listed[0] {
		t.Error("asked again, the frame lists the sessions running afresh, want the list it holds")
	}
	if later := f.running(doc, routedAt.Add(time.Minute)); len(later) > 0 && &later[0] == &listed[0] {
		t.Error("asked of another moment, the frame gives the list it holds, want them listed afresh")
	}
}

// While no router lists the sessions, what its last listing had in flight
// is as last known, as SESSIONS, ROUTING's counts and Detail list the
// sessions running: never at work; while one does, at work. It's the
// watch's one value of it, Unlisted, they read, never when the status line
// says the router was lost, which is zero while a read failed for a reason
// of its own.
func TestWhileNoRouterListsTheSessionsWhatsInFlightIsAsLastKnown(t *testing.T) {
	doc, sessions, _ := overviewWorld()
	for _, tt := range []struct {
		unlisted bool
		lost     time.Time
	}{
		{unlisted: false},
		{unlisted: true, lost: routedAt.Add(-time.Minute)},
		{unlisted: true},
	} {
		f := Frame{Width: 160, Height: 40, View: Overview, Sessions: sessions, Unlisted: tt.unlisted, Lost: tt.lost}
		for _, r := range f.running(doc, routedAt) {
			if r.Activity.InFlight() && (r.Activity.Stale != tt.unlisted || r.Activity.Works() == r.Activity.Stale) {
				t.Errorf("unlisted %t, lost %t, %s is in flight, as last known %t, at work %t; want as last known %t, and at work only where it isn't",
					tt.unlisted, !tt.lost.IsZero(), status.ShownID(r.Session), r.Activity.Stale, r.Activity.Works(), tt.unlisted)
			}
		}
	}
}

func TestTheStreamSeesASessionRunningLaterThanTheRouterListedIt(t *testing.T) {
	doc, sessions, _ := overviewWorld()
	f := Frame{Width: 160, Height: 40, View: Overview, Sessions: sessions}
	answered := views.Told{Request: "r1", Session: idDB8A, Model: opus, Account: "side", Sent: routedAt.Add(-9 * time.Second), Ended: routedAt, Status: 200, Heard: routedAt}
	f.Stream = views.Stream{Live: true, Requests: map[string]views.Told{answered.Request: answered}}
	var order []string
	for _, r := range f.running(doc, routedAt) {
		order = append(order, status.ShownID(r.Session))
	}
	if want := []string{"41e0", "9c2e", "c61b", "db8a", "d28c", "7f3a"}; !slices.Equal(order, want) {
		t.Errorf("listed %q, want %q: db8a seen now, as the stream told, after c61b as the router lists them", order, want)
	}
	if !sessions[4].Assignments[0].LastSeen.Equal(routedAt.Add(-4 * time.Minute)) {
		t.Error("the router's listing was changed")
	}
}

// busyAt is a session sending requests on an account: rate of them in the
// minute to routedAt.
type busyAt struct {
	account, session string
	rate             int
}

// busyNow are the traces of the sessions given, each sending at its rate.
func busyNow(busy ...busyAt) views.Traces {
	var told []views.Told
	for _, b := range busy {
		for i := range b.rate {
			told = append(told, views.Told{Request: fmt.Sprintf("%s-%d", b.session, i), Session: b.session, Account: b.account, Sent: routedAt.Add(-time.Duration(i+1) * time.Second)})
		}
	}
	return views.Traces{}.Told(told, routedAt)
}

// sessionsOnPage are the rows of SESSIONS' body as the frame draws the
// Overview of doc at routedAt, trimmed.
func sessionsOnPage(f Frame, doc status.Document) []string {
	laid := f.laidOut(doc, routedAt, 0)
	body := laid[slices.IndexFunc(laid, func(p planned) bool { return p.Section == SectionSessions })].body
	var shown []string
	for _, row := range f.overviewDrawn(doc, routedAt).rows(Look{})[body.y : body.y+body.rows] {
		shown = append(shown, strings.TrimRight(row, " "))
	}
	return shown
}

// rowsStart reports whether there are as many rows as wanted, each starting
// as wanted, and one wanted blank blank.
func rowsStart(rows, wanted []string) bool {
	if len(rows) != len(wanted) {
		return false
	}
	for i, row := range rows {
		if !strings.HasPrefix(row, wanted[i]) || wanted[i] == "" && row != "" {
			return false
		}
	}
	return true
}

// SESSIONS lists the busiest accounts first, each one's sessions together,
// busiest first, as views.BusiestFirst has them; and keeps the sessions it
// shows, and their order, as the layout was made, till the next: a session
// coming goes into how many more, one ending leaves its row blank, and as
// rates change, nothing reorders. A resize lays it out afresh, in the order
// the rates give then.
func TestSESSIONSKeepsWhatItShowsInTheOrderTakenTillTheNextLayout(t *testing.T) {
	doc, sessions, stream := overviewWorld()
	f := Frame{
		Width: 160, Height: 13, View: Overview, Sections: SectionsNamed([]string{"sessions"}), Sessions: sessions, Stream: stream,
		Traces: busyNow(busyAt{account: "personal", session: id9C2E, rate: 5}, busyAt{account: "side", session: idD28C, rate: 1}),
	}
	taken := []string{"2 personal ◆    9c2e", "                41e0", "3 side ▲        d28c", "                c61b", "                +2 more"}
	if got := sessionsOnPage(f, doc); !rowsStart(got, taken) {
		t.Fatalf("laid out afresh, SESSIONS shows\n  %q\nwant\n  %q: personal, 5 a minute, before side's 1, though side runs more, and its idle 41e0 before side's d28c", got, taken)
	}
	f.Layout = f.LayoutOf(doc, routedAt)

	without := func(id string) []status.Session {
		return slices.DeleteFunc(slices.Clone(sessions), func(s status.Session) bool { return s.ID == id })
	}
	busier, came, ended, endedFirst := f, f, f, f
	busier.Traces = busyNow(busyAt{account: "side", session: idC61B, rate: 20}, busyAt{account: "personal", session: id9C2E, rate: 5})
	came.Sessions = append(slices.Clone(sessions), status.Session{ID: id1F9A, Assignments: []status.Assignment{
		{Model: opus, Family: "opus", Account: "work", Reason: status.ReasonNew, AssignedAt: routedAt, LastSeen: routedAt},
	}})
	came.Traces = busyNow(busyAt{account: "work", session: id1F9A, rate: 30})
	ended.Sessions, endedFirst.Sessions = without(id41E0), without(id9C2E)
	tests := []struct {
		name string
		f    Frame
		want []string
	}{
		{name: "rates changing: nothing reorders", f: busier, want: taken},
		{name: "a session coming, the busiest: into how many more", f: came, want: []string{"2 personal ◆    9c2e", "                41e0", "3 side ▲        d28c", "                c61b", "                +3 more"}},
		{name: "a session ending: its row left blank", f: ended, want: []string{"2 personal ◆    9c2e", "", "3 side ▲        d28c", "                c61b", "                +2 more"}},
		{name: "an account's first ending: its row left blank, its name on its next", f: endedFirst, want: []string{"", "2 personal ◆    41e0", "3 side ▲        d28c", "                c61b", "                +2 more"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionsOnPage(tt.f, doc); !rowsStart(got, tt.want) {
				t.Errorf("SESSIONS shows\n  %q\nwant\n  %q", got, tt.want)
			}
			if got := tt.f.LayoutOf(doc, routedAt); !sameLayout(got, f.Layout) {
				t.Errorf("the layout is %+v, want it kept, %+v", got, f.Layout)
			}
		})
	}

	resized := busier
	resized.Height++
	want := []string{"3 side ▲        c61b", "                d28c", "                7f3a", "                db8a", "2 personal ◆    9c2e", "                41e0"}
	if got := sessionsOnPage(resized, doc); !rowsStart(got, want) {
		t.Errorf("resized, SESSIONS shows\n  %q\nwant it laid out afresh, side's c61b, 20 a minute, first\n  %q", got, want)
	}
	if sameLayout(resized.LayoutOf(doc, routedAt), f.Layout) {
		t.Error("resized, the layout is as kept, want it made afresh")
	}
}

// A click on SESSIONS' rows as a layout kept lists them opens the session
// drawn there: none on a row left blank.
func TestARowSESSIONSLeftBlankOpensNoPage(t *testing.T) {
	doc, sessions, stream := overviewWorld()
	f := Frame{Width: 160, Height: 13, View: Overview, Sections: SectionsNamed([]string{"sessions"}), Sessions: sessions, Stream: stream}
	f.Layout = f.LayoutOf(doc, routedAt)
	f.Sessions = slices.DeleteFunc(slices.Clone(sessions), func(s status.Session) bool { return s.ID == idD28C })
	got := map[int]string{}
	for _, target := range f.overviewTargets(doc, routedAt) {
		if target.On == OnSession {
			got[target.Y] = target.Session
		}
	}
	laid := f.laidOut(doc, routedAt, f.top())
	body := laid[slices.IndexFunc(laid, func(p planned) bool { return p.Section == SectionSessions })].body
	want := map[int]string{body.y: idC61B, body.y + 2: id7F3A, body.y + 3: idDB8A}
	if !maps.Equal(got, want) {
		t.Errorf("SESSIONS' rows open %v, want %v: d28c's row, left blank, none", got, want)
	}
}
