package dashboard

import (
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
				"● 23:50:00 pin      personal  new sessions go there",
				"  Wed 30 Sep  ───────────────────────────  Thu 1 Oct",
				"╎ 09:30:00 cap      work      95% of its 5-hour win…",
				"▸ 14:35:00 moved    w… → si…  work reached its limi…",
			},
			last: "…",
		},
		{
			width: 80,
			want: []string{
				"● 23:50:00 pin      personal  new sessions go there",
				"  Wed 30 Sep  ───────────────────────────────────────────────────────  Thu 1 Oct",
				"╎ 09:30:00 cap      work      95% of its 5-hour window: its 3 sessions move to …",
				"▸ 14:35:00 moved    w… → si…  work reached its limit   1.0M cached again, $8.00",
			},
			costs: true, last: "…",
		},
		{
			width: 100,
			want: []string{
				"● 23:50:00                     pin       personal          new sessions go there",
				"  Wed 30 Sep  ───────────────────────────────────────────────────────────────────────────  Thu 1 Oct",
				"╎ 09:30:00                     cap       work              95% of its 5-hour window: its 3 sessions…",
				"▸ 14:35:00  d28c               moved     work → side       work reached…   1.0M cached again, $8.00",
			},
			costs: true, last: "…",
		},
		{
			width: 120,
			want: []string{
				"● 23:50:00                     pin       personal          new sessions go there",
				"  Wed 30 Sep  " + rule(95) + "  Thu 1 Oct",
				"╎ 09:30:00                     cap       work              95% of its 5-hour window: its 3 sessions move to side as the…",
				"▸ 14:35:00  d28c               moved     work → side       work reached its limit   1.0M cached again, $8.00",
			},
			costs: true, last: "…",
		},
		{
			width: 200,
			want: []string{
				"● 23:50:00                     pin       personal          new sessions go there",
				"  Wed 30 Sep  " + rule(175) + "  Thu 1 Oct",
				"╎ 09:30:00                     cap       work              95% of its 5-hour window: its 3 sessions move to side as their next requests come",
				"▸ 14:35:00  d28c               moved     work → side       work reached its limit   1.0M cached again, $8.00",
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
