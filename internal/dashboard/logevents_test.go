package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// An event's card names its session as the Log's tables do, its directory
// cut to as many cells, of the one rule of how many.
func TestAnEventsCardNamesItsSessionAsTheTablesDo(t *testing.T) {
	lines, _, at := rowsOfEachKind()
	dir := "~/Code/a-directory-named-at-length"
	for i := range lines {
		lines[i].Dir = dir
	}
	moved := status.Event{ID: 3, At: lines[1].At.Add(500 * time.Millisecond), Kind: status.EventMoved, Session: lines[1].Session, Model: lines[1].Model, From: "work", To: "side"}
	read := viewstest.ReadLog(sessionsHeld(lines), heldOf(lines), ledger.Pricing, at)
	events := views.LogEvents(views.LogEventSources{From: theDayBefore, Events: views.ReadEvents(filedEvents{moved}, theDayBefore, at), Held: read.Held, Now: at})
	if len(events.Rows) != 1 {
		t.Fatalf("the Events hold %d rows, want the move's alone", len(events.Rows))
	}
	d, _ := events.Details(events.Rows[0].Key, read.Read)
	j := slices.IndexFunc(d.Happened, func(it views.DetailItem) bool { return it.Name == "session" })
	if want := sessionNamed(lines[1].Session, dir); j < 0 || d.Happened[j].Value.String() != want {
		t.Errorf("the move's card names its session %q, want %q, as the tables name it", d.Happened[max(j, 0)].Value.String(), want)
	}
}

// eventsSaid says the Events' columns as a test holds them: each's head and
// the cells it takes, from its first to its last.
func eventsSaid(columns []eventColumn) string {
	said := make([]string, len(columns))
	for i, c := range columns {
		said[i] = fmt.Sprintf("%s %d–%d", eventHeads[c.part], c.x, c.x+c.width-1)
	}
	return strings.Join(said, " · ")
}

// The Events' columns at every width, two cells apart: the session, whole,
// leaving first, where what happened would fall short of its 40 cells; then
// what happened cut toward 20; then the account toward its least, 8. The
// account at its natural till its turn, and what happened taking the rest.
func TestTheEventsColumnsAreLaidOutAtEveryWidth(t *testing.T) {
	names := eventsNaturals{dir: 12, account: 15}
	tests := []struct {
		width int
		n     eventsNaturals
		want  string
	}{
		{width: 52, n: names, want: "at 2–9 · event 12–19 · account 22–29 · what happened 32–51"},
		{width: 59, n: names, want: "at 2–9 · event 12–19 · account 22–36 · what happened 39–58"},
		{width: 78, n: names, want: "at 2–9 · event 12–19 · account 22–36 · what happened 39–77"},
		{width: 97, n: names, want: "at 2–9 · event 12–19 · account 22–36 · what happened 39–96"},
		{width: 98, n: names, want: "at 2–9 · session 12–28 · event 31–38 · account 41–55 · what happened 58–97"},
		{width: 160, n: names, want: "at 2–9 · session 12–28 · event 31–38 · account 41–55 · what happened 58–159"},
		{width: 160, n: eventsNaturals{account: 4}, want: "at 2–9 · session 12–18 · event 21–28 · account 31–38 · what happened 41–159"},
		{width: 73, n: eventsNaturals{dir: 12, account: 42}, want: "at 2–9 · event 12–19 · account 22–50 · what happened 53–72"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d with %+v", tt.width, tt.n), func(t *testing.T) {
			if got := eventsSaid(eventsLaid(tt.width, tt.n)); got != tt.want {
				t.Errorf("eventsLaid(%d, %+v) =\n%s\nwant\n%s", tt.width, tt.n, got, tt.want)
			}
		})
	}
}

// The Events' naturals are the widest the rows shown need: the directory's
// as views.SessionDirName names it; and the account's as its words say it,
// each account's name as a column of names counts it, labelMost at the most.
func TestTheEventsNaturalsAreTheWidestTheRowsShowNeed(t *testing.T) {
	long := "an-account-named-at-great-length"
	moved := views.LogEvent{Dir: "~/Code/a-directory-named-at-length"}
	moved.Line.Account = status.Words{status.Named(long), status.Plain(" → "), status.Named("side")}
	capped := views.LogEvent{Dir: "~/Code/api"}
	capped.Line.Account = status.Words{status.Named("personal")}
	var got eventsNaturals
	for _, e := range []views.LogEvent{capped, moved} {
		got = got.with(e, status.Moment{})
	}
	if want := (eventsNaturals{dir: views.SessionDirCells, account: labelMost + 3 + 4}); got != want {
		t.Errorf("the naturals are %+v, want %+v", got, want)
	}
}

// The Events draw each kind of row in their columns at each width the
// design checks, a phone's at 52 and 80, the session leaving: a pin, ● in
// accent.key; the day's rule; a cap, ╎ in accent.attention, its use bold in
// its level's colour; and a move, ▸ in accent.key, its two accounts cut as
// moveNames cuts them, and what writing its session's context again cost,
// three cells on, in accent.attention, kept whole as what happened is cut,
// where there's room for it. What's cut ends in an ellipsis, so a row
// reaching the last column ends there, whole.
func TestTheEventsDrawEachKindOfRowAtEachWidth(t *testing.T) {
	lines, _, at := rowsOfEachKind()
	pinned := status.Event{ID: 1, At: theDayBefore, Kind: status.EventPin, Accounts: []string{"personal"}}
	moved := status.Event{
		ID: 3, At: lines[1].At.Add(500 * time.Millisecond), Kind: status.EventMoved, Session: lines[1].Session, Model: lines[1].Model,
		From: "work", To: "side", Reason: lines[1].Reason,
	}
	p := requestsOf(views.LogSources{From: theDayBefore, Now: at}, lines)
	p.Events = views.LogEvents(views.LogEventSources{
		From: theDayBefore, Events: views.ReadEvents(filedEvents{pinned, workAtItsCap, moved}, theDayBefore, at),
		Held: viewstest.ReadLog(sessionsHeld(lines), heldOf(lines), ledger.Pricing, at).Held, Now: at,
	})
	p = p.Turned(1)
	use, cost := inked{"95%", ink{token: theme.VizRamp4, bold: true}}, inked{"1.0M cached again, $8.00", attention}
	inks := [][]inked{{{"●", keyMark}}, nil, {{"╎", ink{token: theme.AccentAttention, bold: true}}, use}, {{"▸", keyMark}, cost}}
	tests := []struct {
		width int
		want  []string
		// costs reports whether the move's cost shows; and last is the last
		// column's cell of the row that reaches it.
		costs bool
		last  string
	}{
		{
			width: 52,
			want: []string{
				"● 23:50:00  pin       personal  new sessions go the…",
				"  Wed 30 Sep  ───────────────────────────  Thu 1 Oct",
				"╎ 09:30:00  cap       work      95% of its 5-hour w…",
				"▸ 14:35:00  moved     w… → si…  work reached its li…",
			},
			last: "…",
		},
		{
			width: 80,
			want: []string{
				"● 23:50:00  pin       personal     new sessions go there",
				"  Wed 30 Sep  ───────────────────────────────────────────────────────  Thu 1 Oct",
				"╎ 09:30:00  cap       work         95% of its 5-hour window: its 3 sessions mov…",
				"▸ 14:35:00  moved     work → side  work reached its…   1.0M cached again, $8.00",
			},
			costs: true, last: "…",
		},
		{
			width: 100,
			want: []string{
				"● 23:50:00           pin       personal     new sessions go there",
				"  Wed 30 Sep  ───────────────────────────────────────────────────────────────────────────  Thu 1 Oct",
				"╎ 09:30:00           cap       work         95% of its 5-hour window: its 3 sessions move to side a…",
				"▸ 14:35:00  d28c     moved     work → side  work reached its limit   1.0M cached again, $8.00",
			},
			costs: true, last: "…",
		},
		{
			width: 120,
			want: []string{
				"● 23:50:00           pin       personal     new sessions go there",
				"  Wed 30 Sep  " + rule(95) + "  Thu 1 Oct",
				"╎ 09:30:00           cap       work         95% of its 5-hour window: its 3 sessions move to side as their next request…",
				"▸ 14:35:00  d28c     moved     work → side  work reached its limit   1.0M cached again, $8.00",
			},
			costs: true, last: "…",
		},
		{
			width: 200,
			want: []string{
				"● 23:50:00           pin       personal     new sessions go there",
				"  Wed 30 Sep  " + rule(175) + "  Thu 1 Oct",
				"╎ 09:30:00           cap       work         95% of its 5-hour window: its 3 sessions move to side as their next requests come",
				"▸ 14:35:00  d28c     moved     work → side  work reached its limit   1.0M cached again, $8.00",
			},
			costs: true,
		},
	}
	for _, tt := range tests {
		f := logFrameOf(p, tt.width, 24)
		c := f.page(routerDoc("side", 0), at)
		rows := textOf(f.Draw(routerDoc("side", 0), at))[f.top()+1 : f.top()+1+len(tt.want)]
		if !slices.Equal(rows, tt.want) {
			t.Errorf("at %d columns, the rows read\n%s\nwant\n%s", tt.width, strings.Join(rows, "\n"), strings.Join(tt.want, "\n"))
			continue
		}
		for i, row := range rows {
			for _, want := range inks[i] {
				if want == cost && !tt.costs {
					continue
				}
				if got := inkOfText(c, f.top()+1+i, row, want.text); got != want.ink {
					t.Errorf("at %d columns, row %d's %q is inked %+v, want %+v", tt.width, i+1, want.text, got, want.ink)
				}
			}
		}
		if tt.last != "" && !endsAtTheLast(c, rows, f.top()+1, tt.width, tt.last) {
			t.Errorf("at %d columns, no row ends at the last column with %q", tt.width, tt.last)
		}
	}
}

// The Events mark the router's health ⊕, bold, by LATELY's one rule: in
// state.destructive while it's unhealthy, and state.positive once it's well
// again.
func TestTheEventsMarkTheRoutersHealthAsLatelyDoes(t *testing.T) {
	unwell := status.Event{ID: 1, At: logDay, Kind: status.EventHealth, Reason: "the config won't load"}
	well := status.Event{ID: 2, At: logDay.Add(time.Minute), Kind: status.EventHealth}
	f := logFrameOf(eventsShown(unwell, well), 160, 24)
	c := f.page(routerDoc("side", 0), logDayNoon)
	var got []ink
	for y := range f.Height {
		if cell := c.at(0, y); cell.glyph == "⊕" {
			got = append(got, cell.ink)
		}
	}
	want := []ink{{token: theme.StateDestructive, bold: true}, {token: theme.StatePositive, bold: true}}
	if !slices.Equal(got, want) {
		t.Errorf("the router unhealthy, then well again, is marked ⊕ in %+v, want %+v", got, want)
	}
}
