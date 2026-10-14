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
)

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
		Read: views.ReadLogHeld(sessionsHeld(lines), heldOf(lines), ledger.Pricing, at), Now: at,
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
