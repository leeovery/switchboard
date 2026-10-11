package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

func TestAPrintoutIsEveryCardInFullWithoutTheFooter(t *testing.T) {
	const width = 160
	rows := Printout{Width: width, Policy: claudeLike}.Draw(Snapshot{Doc: accountsOf(8, false)}, now)

	printed := strings.Join(rows, "\n")
	if got := strings.Count(printed, "╭─ "); got != 8 {
		t.Errorf("printed %d cards, want every one of the 8:\n%s", got, printed)
	}
	if !strings.HasPrefix(rows[len(rows)-3], " KEY      ") || strings.Contains(printed, "for the key") {
		t.Errorf("printed\n%s\nwant the key line, and no footer", printed)
	}
	for i, row := range rows {
		if got := ansi.StringWidth(row); got > width {
			t.Errorf("row %d is %d cells wide, want %d at most", i+1, got, width)
		}
	}
}

func TestAPrintoutDrawsWhatItsSnapshotHoldsAndThePreferences(t *testing.T) {
	doc := pressedWork()
	session, _ := doc.Accounts[0].Window("5h")
	start := session.ResetsAt.Add(-5 * time.Hour)
	history := History{{Account: "work", Window: "5h"}: {Start: start, Readings: []score.Reading{
		{At: start, Utilization: 0},
		{At: start.Add(50 * time.Minute), Utilization: 0.18},
		{At: start.Add(110 * time.Minute), Utilization: 0.58},
	}}}
	sessions := []status.Session{
		{ID: "d28c5e17", Assignments: []status.Assignment{{Account: "work", LastSeen: now.Add(-20 * time.Second).UTC()}}},
	}
	plain := Printout{Width: 160, Policy: claudeLike}
	base := plain.Draw(Snapshot{Doc: doc}, now)
	tests := []struct {
		name     string
		printout Printout
		snap     Snapshot
		want     string
	}{
		{name: "an outdated router", printout: plain, snap: Snapshot{Doc: doc, Outdated: true}, want: "restart the router for recent events"},
		{name: "the sessions the router lists", printout: plain, snap: Snapshot{Doc: doc, Sessions: sessions}},
		{name: "the windows' history", printout: plain, snap: Snapshot{Doc: doc, History: history}},
		{name: "the window featured", printout: Printout{Width: 160, Policy: claudeLike, Featured: "7d"}, snap: Snapshot{Doc: doc}},
		{name: "the chart", printout: Printout{Width: 160, Policy: claudeLike, Chart: BurnRate}, snap: Snapshot{Doc: doc}},
		{name: "the look", printout: Printout{Width: 160, Policy: claudeLike, Look: NoColour()}, snap: Snapshot{Doc: doc}, want: "\x1b[1m"},
		{name: "the width", printout: Printout{Width: 80, Policy: claudeLike}, snap: Snapshot{Doc: doc}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := tt.printout.Draw(tt.snap, now)
			printed := strings.Join(rows, "\n")
			if slices.Equal(rows, base) {
				t.Errorf("printed\n%s\nthe same as without it", printed)
			}
			if !strings.Contains(printed, tt.want) {
				t.Errorf("printed\n%s\nwant %q", printed, tt.want)
			}
		})
	}
}
