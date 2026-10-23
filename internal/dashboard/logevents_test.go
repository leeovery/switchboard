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

// logAccounts is the router's document of work, personal and side, the
// accounts the Log's tests name, so a column of names takes their widest.
func logAccounts() status.Document {
	return routerDoc("side", 0, status.Account{ID: "work", TokenSet: true}, status.Account{ID: "personal", TokenSet: true}, status.Account{ID: "side", TokenSet: true})
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
// account at the cells given till its turn, 8 where they're fewer, and what
// happened taking the rest.
func TestTheEventsColumnsAreLaidOutAtEveryWidth(t *testing.T) {
	tests := []struct {
		width, account int
		want           string
	}{
		{width: 52, account: 15, want: "at 2–9 · event 12–19 · account 22–29 · what happened 32–51"},
		{width: 59, account: 15, want: "at 2–9 · event 12–19 · account 22–36 · what happened 39–58"},
		{width: 78, account: 15, want: "at 2–9 · event 12–19 · account 22–36 · what happened 39–77"},
		{width: 97, account: 15, want: "at 2–9 · event 12–19 · account 22–36 · what happened 39–96"},
		{width: 98, account: 15, want: "at 2–9 · session 12–28 · event 31–38 · account 41–55 · what happened 58–97"},
		{width: 160, account: 15, want: "at 2–9 · session 12–28 · event 31–38 · account 41–55 · what happened 58–159"},
		{width: 101, account: 19, want: "at 2–9 · event 12–19 · account 22–40 · what happened 43–100"},
		{width: 102, account: 19, want: "at 2–9 · session 12–28 · event 31–38 · account 41–59 · what happened 62–101"},
		{width: 160, account: 2, want: "at 2–9 · session 12–28 · event 31–38 · account 41–48 · what happened 51–159"},
		{width: 173, account: 49, want: "at 2–9 · session 12–28 · event 31–38 · account 41–89 · what happened 92–172"},
		{width: 73, account: 49, want: "at 2–9 · event 12–19 · account 22–50 · what happened 53–72"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d of %d", tt.width, tt.account), func(t *testing.T) {
			if got := eventsSaid(eventsLaid(tt.width, tt.account)); got != tt.want {
				t.Errorf("eventsLaid(%d, %d) =\n%s\nwant\n%s", tt.width, tt.account, got, tt.want)
			}
		})
	}
}

// The Events' account column takes the longest line a move can make: the
// two widest names of the accounts configured, each counted labelMost at
// the most and labelLeast at the least, as an event may name an account no
// longer configured, and the arrow between; the one name where there's one.
func TestTheEventsAccountIsTheLongestMove(t *testing.T) {
	tests := []struct {
		name     string
		accounts []status.Account
		want     int
	}{
		{name: "none", want: 0},
		{name: "one", accounts: []status.Account{{ID: "personal"}}, want: 8},
		{name: "one short, at labelLeast", accounts: []status.Account{{ID: "ab"}}, want: 8},
		{name: "the two widest, side at labelLeast", accounts: []status.Account{{ID: "work"}, {ID: "personal"}, {ID: "side"}}, want: 8 + 3 + 8},
		{
			name:     "long names, each at labelMost",
			accounts: []status.Account{{ID: "a", Label: "an-account-named-at-great-length"}, {ID: "b", Label: "side-hustle-consulting"}, {ID: "c"}},
			want:     labelMost + 3 + 22,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := frameOf(160, 24).Moment(routerDoc("side", 0, tt.accounts...), now)
			if got := eventsAccountCells(m); got != tt.want {
				t.Errorf("eventsAccountCells = %d, want %d", got, tt.want)
			}
		})
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
				"● 23:50:00  pin       personal             new sessions go there",
				"  Wed 30 Sep  ───────────────────────────────────────────────────────  Thu 1 Oct",
				"╎ 09:30:00  cap       work                 95% of its 5-hour window: its 3 sess…",
				"▸ 14:35:00  moved     work → side          work reac…   1.0M cached again, $8.00",
			},
			costs: true, last: "…",
		},
		{
			width: 100,
			want: []string{
				"● 23:50:00  pin       personal             new sessions go there",
				"  Wed 30 Sep  ───────────────────────────────────────────────────────────────────────────  Thu 1 Oct",
				"╎ 09:30:00  cap       work                 95% of its 5-hour window: its 3 sessions move to side as…",
				"▸ 14:35:00  moved     work → side          work reached its limit   1.0M cached again, $8.00",
			},
			costs: true, last: "…",
		},
		{
			width: 120,
			want: []string{
				"● 23:50:00                     pin       personal             new sessions go there",
				"  Wed 30 Sep  " + rule(95) + "  Thu 1 Oct",
				"╎ 09:30:00                     cap       work                 95% of its 5-hour window: its 3 sessions move to side as…",
				"▸ 14:35:00  d28c               moved     work → side          work reached its limit   1.0M cached again, $8.00",
			},
			costs: true,
		},
		{
			width: 200,
			want: []string{
				"● 23:50:00                     pin       personal             new sessions go there",
				"  Wed 30 Sep  " + rule(175) + "  Thu 1 Oct",
				"╎ 09:30:00                     cap       work                 95% of its 5-hour window: its 3 sessions move to side as their next requests come",
				"▸ 14:35:00  d28c               moved     work → side          work reached its limit   1.0M cached again, $8.00",
			},
			costs: true,
		},
	}
	for _, tt := range tests {
		f := logFrameOf(p, tt.width, 24)
		c := f.page(logAccounts(), at)
		rows := textOf(f.Draw(logAccounts(), at))[f.top()+1 : f.top()+1+len(tt.want)]
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
